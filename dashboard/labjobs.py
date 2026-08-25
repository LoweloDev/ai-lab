"""Job-Verwaltung: Allowlist-Aktionen, Slots (1× GPU, 3× Cloud, 2× Download),
harte Timeouts, KEINE automatischen Wiederholungen — ein fehlgeschlagener Job
bleibt fehlgeschlagen, bis ein Mensch klickt.

Harte Regeln:
  - Nur Aktionen aus der Allowlist (run-task.sh, run-task-api.sh, dom-agent.py,
    ux-dom-test.py, run-polyglot-subset.sh, serve.sh, aria2c-Download).
  - GPU-Jobs und serve.sh sind gesperrt, solange Benchmark-Container laufen
    oder (für serve) bereits ein llama-server läuft.
  - Beendet werden ausschließlich Prozessgruppen, die dieser Prozess selbst
    gestartet hat (nie Container anderer Läufe, nie fremde llama-server).
"""
import json
import os
import re
import signal
import subprocess
import threading
import time

from labcore import (BENCH, WEBAPP, RUNSD, MODELS_DIR, LAB, SERVE_SH, TMPB,
                     POLY_LABELS, gpu_lock, llama_procs, port_listening,
                     job_env_with_keys, env_key_present, load_json, save_json,
                     LABEL_RE, MODELID_RE, CACHE)
import labregistry

LIMITS = {'gpu': 1, 'cloud': 3, 'download': 2, 'cpu': 1}
CLASS_NAMES = {'gpu': 'GPU', 'cloud': 'Cloud', 'download': 'Download', 'cpu': 'CPU'}
POLY_SCRIPT = os.path.join(BENCH, 'aider', 'run-polyglot-subset.sh')
ROBUST_SCRIPT = os.path.join(BENCH, 'robustness-battery', 'run-all.sh')

_LOCK = threading.Lock()
JOBS = {}

_FLAG_TOKEN = re.compile(r'^[A-Za-z0-9._=,/-]{1,64}$')
_FORBIDDEN_FLAGS = {'-m', '--model', '--host', '--port', '--api-key', '--mmproj',
                    '--path', '--log-file'}


def _tasks():
    try:
        return sorted(x for x in os.listdir(os.path.join(BENCH, 'tasks'))
                      if os.path.isdir(os.path.join(BENCH, 'tasks', x)))
    except OSError:
        return []


def api_models():
    """API-Modelle aus bench/opencode-config-api/opencode.json + Key-Status."""
    cfg = load_json(os.path.join(BENCH, 'opencode-config-api', 'opencode.json'), {})
    out = []
    for pid, prov in (cfg.get('provider') or {}).items():
        key_env = ''
        m = re.match(r'^\{env:([A-Z0-9_]+)\}$', str((prov.get('options') or {})
                                                    .get('apiKey', '')))
        if m:
            key_env = m.group(1)
        for mid, mdef in (prov.get('models') or {}).items():
            out.append({'id': '%s/%s' % (pid, mid),
                        'name': mdef.get('name', mid),
                        'provider': prov.get('name', pid),
                        'key_env': key_env,
                        'key_present': env_key_present(key_env) if key_env else False})
    return out


def _int_or(v, default, lo, hi, what):
    if v in (None, ''):
        return default
    try:
        v = int(v)
    except (TypeError, ValueError):
        raise ValueError('%s muss eine Zahl sein.' % what)
    if not lo <= v <= hi:
        raise ValueError('%s: %d–%d.' % (what, lo, hi))
    return v


def _extra_flags(p):
    raw = (p.get('extra_flags') or '').split()
    if len(raw) > 8:
        raise ValueError('Maximal 8 zusätzliche Flags.')
    for t in raw:
        if not _FLAG_TOKEN.match(t):
            raise ValueError('Unerlaubtes Zeichen in Flag: %s' % t)
        if t.lower() in _FORBIDDEN_FLAGS:
            raise ValueError('Flag %s ist gesperrt (Host/Port/Modellpfad setzt '
                             'ausschließlich serve.sh).' % t)
    return raw


def _serve_step(p, need_vision=False):
    """Optionaler Vorschritt: llama-server über serve.sh starten.
    Nur erlaubt, wenn keine Benchmark-Container laufen UND kein llama-server läuft."""
    model = (p.get('model') or '').strip()
    entry = labregistry.find_model(model)
    if not entry:
        raise ValueError('Unbekanntes Registry-Modell: %s' % model)
    st = labregistry._file_status(entry)
    if st['status'] != 'installiert':
        raise ValueError('Modell %s ist nicht (vollständig) installiert.' % model)
    backend = p.get('backend') or 'vulkan'
    if backend not in ('vulkan', 'rocm'):
        raise ValueError('Backend: vulkan oder rocm.')
    args = [SERVE_SH, model, backend]
    ctx = _int_or(p.get('ctx'), None, 2048, 131072, 'Kontextgröße')
    if ctx:
        args += ['-c', str(ctx)]
    args += _extra_flags(p)
    return {'type': 'serve', 'argv': ['bash'] + args, 'name': 'serve.sh %s %s' % (model, backend)}


def _guard_gpu(start_server):
    lock = gpu_lock()
    if lock['locked']:
        raise ValueError('GPU gesperrt — ' + lock['reason'])
    with _LOCK:
        act = [j for j in JOBS.values()
               if j['class'] == 'gpu' and j['status'] in ('läuft',)]
    if len(act) >= LIMITS['gpu']:
        raise ValueError('GPU-Slot belegt (max. 1 GPU-Job): %s' % act[0]['desc'])
    procs = llama_procs()
    if start_server and procs:
        raise ValueError('Es läuft bereits ein llama-server (%s) — das Dashboard '
                         'stoppt niemals Modellserver. Ohne „Server starten“ erneut '
                         'absenden, um gegen den laufenden Server zu testen.'
                         % (procs[0]['model_file'] or 'PID %d' % procs[0]['pid']))
    return procs


def _server_matches(procs, model):
    entry = labregistry.find_model(model)
    if not entry or not procs:
        return False
    return any(pr['model_file'] == entry.get('file')
               and (entry.get('kind') != 'vision'
                    or pr.get('mmproj', '') == entry.get('mmproj', ''))
               for pr in procs)


# ----------------------------------------------------------- Job-Builder

def build_job(p):
    """Validiert die Parameter und baut die Schrittliste. Wirft ValueError mit
    deutscher Meldung — es wird erst NACH erfolgreicher Validierung gestartet."""
    action = p.get('action')
    tasks = _tasks()
    label = (p.get('label') or '').strip()
    if label and not LABEL_RE.match(label):
        raise ValueError('Ungültiges Label (erlaubt: a-z 0-9 . _ -).')

    if action == 'suite':
        model = (p.get('model') or '').strip()
        if not labregistry.find_model(model):
            raise ValueError('Bitte ein Modell aus der Registry wählen.')
        backend = p.get('backend') or 'vulkan'
        if backend not in ('vulkan', 'rocm'):
            raise ValueError('Backend: vulkan oder rocm.')
        sel = p.get('tasks') if isinstance(p.get('tasks'), list) else \
            ([p['task']] if p.get('task') else [])
        if p.get('all_tasks'):
            sel = tasks
        if not sel:
            raise ValueError('Bitte mindestens einen Task wählen.')
        bad = [t for t in sel if t not in tasks]
        if bad:
            raise ValueError('Unbekannte Tasks: %s' % ', '.join(bad))
        timeout = _int_or(p.get('timeout'), 1200, 60, 14400, 'Timeout je Task')
        start_server = bool(p.get('start_server'))
        procs = _guard_gpu(start_server)
        if not start_server:
            if not procs:
                raise ValueError('Kein llama-server aktiv. „Server starten“ anhaken '
                                 'oder serve.sh von Hand starten.')
            if not _server_matches(procs, model):
                raise ValueError('Der laufende llama-server bedient %s — nicht das '
                                 'gewählte Modell. Das Dashboard stoppt keine Server; '
                                 'bitte passendes Modell wählen oder Server von Hand '
                                 'wechseln.' % (procs[0]['model_file'] or '?'))
        run_label = label or '%s-%s' % (model, backend)
        steps = []
        if start_server:
            steps.append(_serve_step(p))
        for t in sel:
            steps.append({'type': 'exec', 'name': 'run-task.sh %s' % t,
                          'argv': ['bash', os.path.join(BENCH, 'run-task.sh'),
                                   run_label, t, str(timeout)],
                          'cwd': BENCH})
        return {'class': 'gpu', 'steps': steps,
                'desc': 'Suite: %s × %s' % (run_label,
                                            'alle %d Tasks' % len(sel)
                                            if len(sel) == len(tasks) and len(sel) > 1
                                            else ', '.join(sel)),
                'timeout': timeout * len(sel) + (300 if start_server else 60),
                'model': model, 'params': p}

    if action == 'suite-api':
        mid = (p.get('model_id') or '').strip()
        if not MODELID_RE.match(mid) or '/' not in mid:
            raise ValueError('Modell-ID bitte als „anbieter/modell“ angeben '
                             '(z. B. deepseek/deepseek-v4-flash).')
        known = {m['id']: m for m in api_models()}
        if mid in known and known[mid]['key_env'] and not known[mid]['key_present']:
            raise ValueError('API-Key fehlt: %s ist nicht in ~/ai-lab/.env gesetzt.'
                             % known[mid]['key_env'])
        if mid not in known:
            prov = mid.split('/')[0]
            envn = {'deepseek': 'DEEPSEEK_API_KEY', 'gemini': 'GEMINI_API_KEY'}.get(prov)
            if envn and not env_key_present(envn):
                raise ValueError('API-Key fehlt: %s ist nicht gesetzt.' % envn)
        sel = p.get('tasks') if isinstance(p.get('tasks'), list) else \
            ([p['task']] if p.get('task') else [])
        if p.get('all_tasks'):
            sel = tasks
        if not sel:
            raise ValueError('Bitte mindestens einen Task wählen.')
        bad = [t for t in sel if t not in tasks]
        if bad:
            raise ValueError('Unbekannte Tasks: %s' % ', '.join(bad))
        timeout = _int_or(p.get('timeout'), 1200, 60, 14400, 'Timeout je Task')
        with _LOCK:
            act = [j for j in JOBS.values()
                   if j['class'] == 'cloud' and j['status'] == 'läuft']
        if len(act) >= LIMITS['cloud']:
            raise ValueError('Cloud-Slots belegt (max. %d parallel).' % LIMITS['cloud'])
        env = {'OC_CONFIG': 'opencode-config-api'}
        if label:
            env['LABEL'] = label
        steps = [{'type': 'exec', 'name': 'run-task-api.sh %s' % t,
                  'argv': ['bash', os.path.join(BENCH, 'run-task-api.sh'),
                           mid, t, str(timeout)],
                  'cwd': BENCH, 'env': env} for t in sel]
        return {'class': 'cloud', 'steps': steps,
                'desc': 'API-Suite: %s × %s' % (label or mid,
                                                'alle %d Tasks' % len(sel)
                                                if len(sel) == len(tasks) and len(sel) > 1
                                                else ', '.join(sel)),
                'timeout': timeout * len(sel) + 60, 'params': p}

    if action in ('dom', 'uxdom'):
        model = (p.get('model') or '').strip()
        run_label = label or model
        if not LABEL_RE.match(run_label):
            raise ValueError('Bitte Modell wählen oder gültiges Label angeben.')
        if action == 'dom':
            task = p.get('task')
            if task not in ('info', 'form'):
                raise ValueError('DOM-Aufgabe: info oder form.')
            if not port_listening(8090):
                raise ValueError('Die Testseite auf :8090 läuft nicht '
                                 '(bench/webapp/server.py von Hand starten).')
        start_server = bool(p.get('start_server'))
        procs = _guard_gpu(start_server)
        if not start_server and not procs:
            raise ValueError('Kein llama-server aktiv. „Server starten“ anhaken '
                             'oder serve.sh von Hand starten.')
        steps = []
        if start_server:
            steps.append(_serve_step(p))
        timeout = _int_or(p.get('timeout'), 900, 60, 14400, 'Timeout')
        if action == 'dom':
            steps.append({'type': 'exec', 'name': 'dom-agent.py',
                          'argv': ['python3', os.path.join(WEBAPP, 'dom-agent.py'),
                                   run_label, p['task']], 'cwd': WEBAPP})
            desc = 'DOM-Test: %s × %s' % (run_label, p['task'])
        else:
            steps.append({'type': 'exec', 'name': 'ux-dom-test.py',
                          'argv': ['python3', os.path.join(WEBAPP, 'ux-dom-test.py'),
                                   run_label], 'cwd': WEBAPP})
            desc = 'UX-DOM-Review: %s' % run_label
        return {'class': 'gpu', 'steps': steps, 'desc': desc,
                'timeout': timeout + (300 if start_server else 0),
                'model': model or None, 'params': p}

    if action == 'polyglot':
        if not os.path.isfile(POLY_SCRIPT):
            raise ValueError('run-polyglot-subset.sh fehlt — Polyglot nicht verfügbar.')
        model = (p.get('model') or '').strip()
        if not labregistry.find_model(model):
            raise ValueError('Bitte das Modell wählen, das auf :8080 bedient wird — '
                             'die Zuordnung Lauf→Modell wird damit gepflegt.')
        start_server = bool(p.get('start_server'))
        procs = _guard_gpu(start_server)
        if not start_server:
            if not procs:
                raise ValueError('Kein llama-server aktiv. „Server starten“ anhaken '
                                 'oder serve.sh von Hand starten.')
            if not _server_matches(procs, model):
                raise ValueError('Der laufende llama-server bedient %s — bitte das '
                                 'tatsächlich bediente Modell wählen.'
                                 % (procs[0]['model_file'] or '?'))
        timeout = _int_or(p.get('timeout'), 21600, 1800, 28800, 'Timeout')
        steps = []
        if start_server:
            steps.append(_serve_step(p))
        steps.append({'type': 'exec', 'name': 'run-polyglot-subset.sh',
                      'argv': ['bash', POLY_SCRIPT], 'cwd': os.path.dirname(POLY_SCRIPT)})
        return {'class': 'gpu', 'steps': steps,
                'desc': 'Polyglot (py+go): %s' % model,
                'timeout': timeout, 'model': model,
                'poly_map_model': model, 'params': p}

    if action == 'serve':
        model = (p.get('model') or '').strip()
        _guard_gpu(True)
        step = _serve_step(p)
        return {'class': 'gpu', 'steps': [step],
                'desc': 'Server starten: %s (%s)' % (model, p.get('backend') or 'vulkan'),
                'timeout': 300, 'model': model, 'params': p}

    if action == 'download':
        mid = (p.get('model') or '').strip()
        entry = labregistry.find_model(mid)
        if not entry:
            raise ValueError('Unbekanntes Registry-Modell: %s' % mid)
        url = labregistry.download_url(entry)
        if not url:
            raise ValueError('Für %s ist keine Quelle hinterlegt '
                             '(HF-Repo+Datei oder URL im Eintrag ergänzen).' % mid)
        with _LOCK:
            act = [j for j in JOBS.values()
                   if j['class'] == 'download' and j['status'] == 'läuft']
            if len(act) >= LIMITS['download']:
                raise ValueError('Download-Slots belegt (max. %d parallel).'
                                 % LIMITS['download'])
            if any(j.get('model') == mid for j in act):
                raise ValueError('Für %s läuft bereits ein Download.' % mid)
        fname = entry['file']
        target = os.path.join(MODELS_DIR, fname)
        if os.path.isfile(target) and not p.get('force'):
            raise ValueError('%s existiert bereits in models/ — kein erneuter '
                             'Download ohne ausdrückliche Bestätigung.' % fname)
        steps = [{'type': 'exec', 'name': 'aria2c %s' % fname,
                  'argv': ['aria2c', '-x8', '-s8', '-c', '--file-allocation=falloc',
                           '--console-log-level=warn', '--summary-interval=30',
                           '-d', MODELS_DIR, '-o', fname, url],
                  'cwd': MODELS_DIR}]
        return {'class': 'download', 'steps': steps,
                'desc': 'Download: %s' % fname,
                'timeout': _int_or(p.get('timeout'), 14400, 300, 43200, 'Timeout'),
                'model': mid, 'params': {'model': mid, 'url': url}}

    if action == 'robustheit':
        if not os.path.isfile(ROBUST_SCRIPT):
            raise ValueError('run-all.sh fehlt — Robustheits-Batterie nicht '
                             'verfügbar (bench/robustness-battery/).')
        argv = ['bash', ROBUST_SCRIPT]
        if p.get('force'):
            argv.append('--force')
        with _LOCK:
            act = [j for j in JOBS.values()
                   if j['class'] == 'cpu' and j['status'] == 'läuft']
        if len(act) >= LIMITS['cpu']:
            raise ValueError('CPU-Slot belegt (max. 1 Robustheits-Lauf).')
        return {'class': 'cpu',
                'steps': [{'type': 'exec', 'name': 'run-all.sh',
                           'argv': argv, 'cwd': os.path.dirname(ROBUST_SCRIPT)}],
                'desc': 'Robustheit neu berechnen (Batterie über alle Abgaben)',
                'timeout': _int_or(p.get('timeout'), 3600, 120, 14400, 'Timeout'),
                'params': p}

    raise ValueError('Unbekannte Aktion — erlaubt: suite, suite-api, dom, uxdom, '
                     'polyglot, serve, download, robustheit.')


# ------------------------------------------------------------ Ausführung

def _save(meta):
    m = {k: v for k, v in meta.items() if not k.startswith('_')}
    try:
        save_json(os.path.join(RUNSD, meta['id'] + '.meta.json'), m)
    except OSError:
        pass


def submit(p):
    spec = build_job(p)
    with _LOCK:
        rid = time.strftime('%Y%m%d-%H%M%S') + '-' + p.get('action', 'job')
        base, n = rid, 1
        while rid in JOBS:
            n += 1
            rid = '%s-%d' % (base, n)
        meta = {'id': rid, 'desc': spec['desc'], 'action': p.get('action'),
                'class': spec['class'], 'class_name': CLASS_NAMES[spec['class']],
                'status': 'läuft', 'rc': None,
                'steps': [{'name': s['name'], 'status': 'wartet', 'rc': None}
                          for s in spec['steps']],
                'model': spec.get('model'),
                'timeout': spec['timeout'],
                'params': {k: v for k, v in (spec.get('params') or {}).items()
                           if k not in ('action',)},
                'started': time.strftime('%Y-%m-%d %H:%M:%S'),
                'started_ts': time.time(), 'ended': None, 'seconds': None,
                'pid': None, 'cancel': False,
                '_spec': spec}
        JOBS[rid] = meta
    os.makedirs(RUNSD, exist_ok=True)
    _save(meta)
    threading.Thread(target=_run_job, args=(meta,), daemon=True).start()
    return meta


def _run_job(meta):
    spec = meta['_spec']
    logp = os.path.join(RUNSD, meta['id'] + '.log')
    deadline = meta['started_ts'] + spec['timeout']
    status = 'fertig'
    try:
        with open(logp, 'ab', buffering=0) as logf:
            for i, step in enumerate(spec['steps']):
                if meta['cancel']:
                    status = 'abgebrochen'
                    break
                meta['steps'][i]['status'] = 'läuft'
                logf.write(('\n=== [%s] Schritt %d/%d: %s\n'
                            % (time.strftime('%H:%M:%S'), i + 1,
                               len(spec['steps']), step['name'])).encode())
                _save(meta)
                if step['type'] == 'serve':
                    rc = _do_serve(step, meta, logf, deadline)
                else:
                    rc = _do_exec(step, meta, logf, deadline)
                meta['steps'][i]['rc'] = rc
                meta['steps'][i]['status'] = ('fertig' if rc == 0 else
                                              'abgebrochen' if meta['cancel'] else
                                              'timeout' if rc == -1 else 'fehler')
                _save(meta)
                if rc != 0:
                    status = meta['steps'][i]['status']
                    if status == 'fertig':
                        status = 'fehler'
                    break
            else:
                status = 'fertig'
    except OSError as e:
        status = 'fehler'
        meta['error'] = str(e)
    if meta['cancel'] and status not in ('fertig',):
        status = 'abgebrochen'
    meta.update({'status': status,
                 'rc': 0 if status == 'fertig' else 1,
                 'ended': time.strftime('%Y-%m-%d %H:%M:%S'),
                 'seconds': round(time.time() - meta['started_ts'], 1)})
    if status == 'fertig' and spec.get('poly_map_model'):
        _map_new_poly_run(meta['started_ts'], spec['poly_map_model'])
    meta.pop('_spec', None)
    meta['pid'] = None
    _save(meta)


def _do_exec(step, meta, logf, deadline):
    """Schritt in eigener Prozessgruppe; harter Timeout beendet NUR diese Gruppe."""
    try:
        proc = subprocess.Popen(step['argv'], stdout=logf, stderr=subprocess.STDOUT,
                                stdin=subprocess.DEVNULL,
                                cwd=step.get('cwd') or BENCH,
                                env=job_env_with_keys(step.get('env')),
                                start_new_session=True)
    except OSError as e:
        logf.write(('Start fehlgeschlagen: %s\n' % e).encode())
        return 127
    meta['pid'] = proc.pid
    _save(meta)
    killed = False
    while True:
        try:
            rc = proc.wait(timeout=2)
            break
        except subprocess.TimeoutExpired:
            pass
        if meta['cancel'] or time.time() > deadline:
            if not killed:
                why = 'Abbruch durch Nutzer' if meta['cancel'] else \
                    'Harter Job-Timeout (%d s) erreicht' % meta['timeout']
                logf.write(('\n=== %s — SIGTERM an eigene Prozessgruppe %d\n'
                            % (why, proc.pid)).encode())
                _kill_group(proc.pid, signal.SIGTERM)
                killed = True
                kill_at = time.time() + 30
            elif time.time() > kill_at:
                _kill_group(proc.pid, signal.SIGKILL)
                kill_at = time.time() + 3600
    meta['pid'] = None
    if killed and not meta['cancel']:
        return -1
    return rc


def _kill_group(pgid, sig):
    """Nur die vom Job selbst gestartete Prozessgruppe — nie fremde Prozesse."""
    try:
        os.killpg(pgid, sig)
    except (ProcessLookupError, PermissionError):
        pass


def _do_serve(step, meta, logf, deadline):
    """serve.sh starten (eigene Session, Log separat) und passiv auf :8080 warten.
    Der gestartete llama-server wird bewusst NIE von Timeout/Abbruch beendet."""
    lock = gpu_lock()
    if lock['locked']:
        logf.write(('serve verweigert: %s\n' % lock['reason']).encode())
        return 13
    if llama_procs():
        logf.write(b'serve verweigert: es laeuft bereits ein llama-server.\n')
        return 13
    slog = os.path.join(RUNSD, meta['id'] + '-server.log')
    try:
        with open(slog, 'ab') as sf:
            subprocess.Popen(step['argv'], stdout=sf, stderr=subprocess.STDOUT,
                             stdin=subprocess.DEVNULL, cwd=LAB,
                             env=job_env_with_keys(),
                             start_new_session=True)
    except OSError as e:
        logf.write(('serve.sh Start fehlgeschlagen: %s\n' % e).encode())
        return 127
    logf.write(('llama-server wird gestartet (Log: %s) — warte passiv auf :8080 …\n'
                % os.path.basename(slog)).encode())
    CACHE.clear('llama')
    t0 = time.time()
    while time.time() - t0 < 180 and time.time() < deadline and not meta['cancel']:
        if port_listening(8080):
            logf.write(('OK: :8080 lauscht (nach %.0f s). Modell lädt ggf. noch — '
                        'kurze Schonfrist 5 s.\n' % (time.time() - t0)).encode())
            time.sleep(5)
            CACHE.clear('llama')
            return 0
        time.sleep(2)
    logf.write(b'FEHLER: :8080 kam nicht hoch (180 s). Server-Log pruefen.\n')
    return 1


def _map_new_poly_run(started_ts, model):
    """Nach erfolgreichem Polyglot-Job: neuen Run-Ordner dem Modell zuordnen
    (schreibt NUR dashboard/polyglot-labels.json)."""
    try:
        labels = load_json(POLY_LABELS, {})
        for run in os.listdir(TMPB):
            p = os.path.join(TMPB, run)
            if run in labels or not os.path.isdir(p) or run == 'polyglot-benchmark':
                continue
            if os.path.getmtime(p) >= started_ts - 60:
                labels[run] = model
        save_json(POLY_LABELS, labels)
    except OSError:
        pass


def cancel(rid):
    with _LOCK:
        meta = JOBS.get(rid)
    if not meta:
        raise ValueError('Unbekannter Job.')
    if meta['status'] != 'läuft':
        raise ValueError('Job läuft nicht mehr (Status: %s).' % meta['status'])
    meta['cancel'] = True
    return {'ok': True, 'id': rid,
            'message': 'Abbruch angefordert — die eigene Prozessgruppe erhält SIGTERM. '
                       'Ein per serve.sh gestarteter llama-server läuft weiter.'}


def load_persisted():
    os.makedirs(RUNSD, exist_ok=True)
    for fn in sorted(os.listdir(RUNSD)):
        if not fn.endswith('.meta.json'):
            continue
        meta = load_json(os.path.join(RUNSD, fn), None)
        if not meta or not meta.get('id'):
            continue
        if meta.get('status') == 'läuft':
            alive = False
            if meta.get('pid'):
                try:
                    os.kill(meta['pid'], 0)
                    alive = True
                except OSError:
                    pass
            if alive:
                meta['detached'] = True
            else:
                meta['status'] = 'abgebrochen'
                meta['ended'] = meta.get('ended') or time.strftime('%Y-%m-%d %H:%M:%S')
                _save(meta)
        meta.setdefault('cancel', False)
        JOBS[meta['id']] = meta


def jobs_list():
    with _LOCK:
        rs = sorted(JOBS.values(), key=lambda r: r.get('started_ts') or 0,
                    reverse=True)
        out = []
        for r in rs[:60]:
            out.append({k: v for k, v in r.items()
                        if not k.startswith('_') and k != 'cancel'})
    slots = {c: sum(1 for j in JOBS.values()
                    if j['class'] == c and j['status'] == 'läuft')
             for c in LIMITS}
    return {'jobs': out,
            'active': any(r.get('status') == 'läuft' for r in JOBS.values()),
            'slots': {c: {'used': slots[c], 'max': LIMITS[c]} for c in LIMITS}}


def job_status(rid):
    with _LOCK:
        meta = JOBS.get(rid)
    if not meta:
        return None
    if meta.get('status') == 'läuft' and meta.get('detached'):
        try:
            os.kill(meta['pid'], 0)
        except OSError:
            meta['status'] = 'beendet (Exit unbekannt)'
            _save(meta)
    out = {k: v for k, v in meta.items() if not k.startswith('_') and k != 'cancel'}
    from labcore import tail_lines
    out['tail'] = tail_lines(os.path.join(RUNSD, rid + '.log'), 50)
    return out


def active_model_ids():
    with _LOCK:
        return frozenset(j.get('model') for j in JOBS.values()
                         if j['status'] == 'läuft' and j.get('model'))
