#!/usr/bin/env python3
"""AI-Lab Benchmark-Dashboard (v1) — nur Python-Stdlib, http.server.

Liest alle Benchmark-Ergebnisse unter ~/ai-lab strikt read-only und kann neue
Läufe ausschließlich über eine Allowlist bestehender Skripte starten
(run-task.sh, run-task-api.sh, dom-agent.py, ux-dom-test.py).
Eigene Schreibzugriffe NUR unter ~/ai-lab/dashboard/runs/ (Logs + Metadaten).
Läuft auf http://127.0.0.1:8100 — Start: dashboard/start.sh
"""
import glob
import json
import os
import re
import subprocess
import threading
import time
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HOME = os.path.expanduser('~')
LAB = os.path.join(HOME, 'ai-lab')
BENCH = os.path.join(LAB, 'bench')
DASH = os.path.join(LAB, 'dashboard')
STATIC = os.path.join(DASH, 'static')
RUNSD = os.path.join(DASH, 'runs')            # eigene Lauf-Logs/-Metadaten
RESULTS = os.path.join(BENCH, 'results.jsonl')
TASKS_DIR = os.path.join(BENCH, 'tasks')
TMPB = os.path.join(BENCH, 'aider', 'aider', 'tmp.benchmarks')
LOGS = os.path.join(LAB, 'logs')
WEBAPP = os.path.join(BENCH, 'webapp')
RUNS_DOM = os.path.join(WEBAPP, 'runs-dom')
RUNS_UX = os.path.join(WEBAPP, 'runs-ux')
POLY_LABELS = os.path.join(DASH, 'polyglot-labels.json')

MODELS = ['qwen38', 'qwen36moe', 'muse', 'codernext']
MODEL_META = {
    'qwen38':    {'name': 'Qwen3.8-27B',              'note': 'dicht · IQ4_XS · Vision'},
    'qwen36moe': {'name': 'Qwen3.6-35B-A3B',          'note': 'MoE · 3B aktiv'},
    'muse':      {'name': 'Muse-Glimmer-30B',         'note': 'Q4_K_XL · Vision'},
    'codernext': {'name': 'Qwen3-Coder-Next 80B-A3B', 'note': 'MoE · CPU-Offload'},
}
WIKI = {
    'guide':       ('Guide: Neue Benchmarks', os.path.join(DASH, 'GUIDE-neue-benchmarks.md')),
    'readme':      ('README (Lab-Setup)',     os.path.join(LAB, 'README.md')),
    'api-runbook': ('API-Runbook',            os.path.join(LAB, 'API-RUNBOOK.md')),
    'todo':        ('Fahrplan (TODO)',        os.path.join(LAB, 'TODO-morgen.md')),
    'failures':    ('Failure-Analyse',        os.path.join(BENCH, 'failure-analysis.md')),
    'ux-flaws':    ('UX-Flaws (Testseite)',   os.path.join(WEBAPP, 'UX-FLAWS.md')),
}
LABEL_RE = re.compile(r'^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$')
MODELID_RE = re.compile(r'^[A-Za-z0-9][A-Za-z0-9._/-]{0,80}$')


# ---------------------------------------------------------------- Hilfen

def tail_lines(path, n=40):
    try:
        with open(path, 'rb') as f:
            f.seek(0, 2)
            size = f.tell()
            f.seek(max(0, size - 64 * 1024))
            data = f.read().decode('utf-8', 'replace')
        return data.splitlines()[-n:]
    except OSError:
        return []


def port_listening(port):
    """Prüft rein passiv über /proc/net/tcp, ob lokal jemand auf <port> lauscht.
    Es wird KEINE Verbindung aufgebaut (die GPU-Instanz auf :8080 bleibt unberührt)."""
    hexp = format(port, '04X')
    for fn in ('/proc/net/tcp', '/proc/net/tcp6'):
        try:
            lines = open(fn).read().splitlines()[1:]
        except OSError:
            continue
        for line in lines:
            parts = line.split()
            if len(parts) > 3 and parts[3] == '0A' and parts[1].endswith(':' + hexp):
                return True
    return False


# ------------------------------------------------- Datenquellen (read-only)

def read_suite():
    entries = []
    try:
        with open(RESULTS, encoding='utf-8') as f:
            for i, line in enumerate(f):
                line = line.strip()
                if not line:
                    continue
                try:
                    d = json.loads(line)
                except json.JSONDecodeError:
                    continue
                d['pass'] = str(d.get('grade', '')).startswith('PASS')
                d['line'] = i
                entries.append(d)
    except OSError:
        pass
    try:
        tasks = sorted(x for x in os.listdir(TASKS_DIR)
                       if os.path.isdir(os.path.join(TASKS_DIR, x)))
    except OSError:
        tasks = []
    for e in entries:
        if e.get('task') and e['task'] not in tasks:
            tasks.append(e['task'])
    labels = []
    for e in entries:
        if e.get('model') and e['model'] not in labels:
            labels.append(e['model'])

    def label_key(lbl):
        for i, m in enumerate(MODELS):
            if lbl == m or lbl.startswith(m + '-'):
                return (0, i, lbl)
        return (1, 0, lbl)

    labels.sort(key=label_key)
    return {'tasks': tasks, 'labels': labels, 'entries': entries}


def read_polyglot():
    try:
        label_map = json.load(open(POLY_LABELS, encoding='utf-8'))
    except (OSError, json.JSONDecodeError):
        label_map = {}
    runs = []
    if os.path.isdir(TMPB):
        for run in sorted(os.listdir(TMPB)):
            p = os.path.join(TMPB, run)
            if run == 'polyglot-benchmark' or not os.path.isdir(p):
                continue
            files = glob.glob(os.path.join(p, '*', 'exercises', 'practice', '*',
                                           '.aider.results.json'))
            exercises, newest = [], 0.0
            for fp in files:
                try:
                    d = json.load(open(fp, encoding='utf-8'))
                except (OSError, json.JSONDecodeError):
                    continue
                lang = fp[len(p) + 1:].split(os.sep)[0]
                o = d.get('tests_outcomes') or []
                status = ('err' if not o else
                          'p1' if o[0] else
                          'p2' if True in o else 'fail')
                exercises.append({'name': d.get('testcase') or os.path.basename(os.path.dirname(fp)),
                                  'lang': lang, 'status': status, 'tries': len(o),
                                  'duration': round(d.get('duration') or 0, 1)})
                try:
                    newest = max(newest, os.path.getmtime(fp))
                except OSError:
                    pass
            exercises.sort(key=lambda e: (e['lang'], e['name']))
            n = len(exercises)
            p1 = sum(1 for e in exercises if e['status'] == 'p1')
            p2 = p1 + sum(1 for e in exercises if e['status'] == 'p2')
            expected = 73 if 'py-go' in run else None
            langs = {}
            for e in exercises:
                L = langs.setdefault(e['lang'], {'n': 0, 'p1': 0, 'p2': 0})
                L['n'] += 1
                if e['status'] == 'p1':
                    L['p1'] += 1
                    L['p2'] += 1
                elif e['status'] == 'p2':
                    L['p2'] += 1
            running = bool(expected and n < expected and newest
                           and time.time() - newest < 3600)
            runs.append({'dir': run, 'label': label_map.get(run, ''),
                         'started': run[:19], 'n': n, 'expected': expected,
                         'pass1': p1, 'pass2': p2,
                         'duration': round(sum(e['duration'] for e in exercises), 1),
                         'running': running, 'langs': langs, 'exercises': exercises})
    runs.sort(key=lambda r: r['dir'], reverse=True)
    return {'runs': runs}


def _load_perf_file(path):
    """llama-bench-JSON laden; abgebrochene Dateien (fehlende ']') tolerant kürzen."""
    try:
        txt = open(path, encoding='utf-8', errors='replace').read()
    except OSError:
        return [], True
    try:
        return json.loads(txt), False
    except json.JSONDecodeError:
        pass
    i = txt.rfind('}')
    while i > 0:
        head = txt[:i + 1].rstrip().rstrip(',')
        try:
            return json.loads(head + '\n]'), True
        except json.JSONDecodeError:
            i = txt.rfind('}', 0, i)
    return [], True


def read_perf():
    series = {}
    files = sorted(glob.glob(os.path.join(LOGS, 'perf-*.json')))
    files.sort(key=lambda f: ('partial' in f, f))   # vollständige zuerst
    parsed = []
    for fp in files:
        fn = os.path.basename(fp)
        m = re.match(r'perf-(.+?)-(vulkan|rocm)(?:-([A-Za-z0-9]+))?\.json$', fn, re.I)
        if not m:
            continue
        label, backend = m.group(1), m.group(2).lower()
        entries, partial = _load_perf_file(fp)
        parsed.append({'file': fn, 'label': label, 'backend': backend,
                       'variant': m.group(3) or '', 'partial': partial,
                       'n': len(entries)})
        key = (label, backend)
        s = series.setdefault(key, {'label': label, 'backend': backend,
                                    'model_type': '', 'files': [],
                                    'partial': False, 'entries': {}})
        s['files'].append(fn)
        s['partial'] = s['partial'] or partial
        for e in entries:
            if not isinstance(e, dict):
                continue
            k = (e.get('n_prompt'), e.get('n_gen'), e.get('n_depth'))
            if k in s['entries']:
                continue
            s['entries'][k] = {'pp': e.get('n_prompt'), 'tg': e.get('n_gen'),
                               'depth': e.get('n_depth'),
                               'ts': round(e.get('avg_ts') or 0, 1),
                               'stddev': round(e.get('stddev_ts') or 0, 2)}
            if not s['model_type']:
                s['model_type'] = e.get('model_type') or ''

    out = []
    for s in series.values():
        ents = sorted(s['entries'].values(),
                      key=lambda e: (e['depth'] or 0, -(e['pp'] or 0)))
        out.append({'label': s['label'], 'backend': s['backend'],
                    'model_type': s['model_type'], 'files': s['files'],
                    'partial': s['partial'], 'entries': ents})

    def skey(s):
        for i, mdl in enumerate(MODELS):
            if s['label'].lower().startswith(mdl):
                return (i, s['backend'])
        return (99, s['backend'])

    out.sort(key=skey)
    return {'series': out, 'files': parsed}


def _parse_dom_transcript(path):
    lines = []
    try:
        with open(path, encoding='utf-8', errors='replace') as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    lines.append(json.loads(line))
                except json.JSONDecodeError:
                    lines.append({'raw': line})
    except OSError:
        pass
    return lines


def read_dom():
    """DOM-Navigations-Ergebnisse. Die Transkripte in runs-dom/ enthalten keine
    Ergebniszeile — Erfolg wird aus dem Transkript abgeleitet (Spiegel der
    dom-agent-Grading-Regeln) bzw. aus der Ergebnis-JSON-Zeile im Log eines
    über das Dashboard gestarteten Laufs übernommen."""
    live = {}
    try:
        for fn in os.listdir(RUNSD):
            if not fn.endswith('.log'):
                continue
            for line in reversed(tail_lines(os.path.join(RUNSD, fn), 20)):
                if '"success"' in line and '"label"' in line:
                    try:
                        d = json.loads(line.strip())
                        live[(d.get('label'), d.get('task'))] = d
                    except json.JSONDecodeError:
                        pass
                    break
    except OSError:
        pass

    runs = []
    if os.path.isdir(RUNS_DOM):
        for fn in sorted(os.listdir(RUNS_DOM)):
            if not fn.endswith('.jsonl'):
                continue
            label, _, task = fn[:-6].rpartition('-')
            steps = _parse_dom_transcript(os.path.join(RUNS_DOM, fn))
            fills, answer, submit = [], None, False
            last_click = last_fill = -1
            for i, st in enumerate(steps):
                m = re.search(r'\{.*\}', st.get('model', '') or '', re.S)
                if not m:
                    continue
                try:
                    act = json.loads(m.group(0))
                except json.JSONDecodeError:
                    continue
                a = act.get('action')
                if a == 'fill':
                    fills.append(str(act.get('value', '')))
                    last_fill = i
                elif a == 'click':
                    last_click = i
                elif a == 'submit':
                    submit = True
                elif a == 'answer':
                    answer = str(act.get('text', ''))
            if task == 'info':
                success = bool(answer and '49,90' in answer)
            elif task == 'form':
                need = ['Tobias Test', 'tobias@example.com', 'Gruss 42']
                success = (all(any(v in fv for fv in fills) for v in need)
                           and (submit or last_click > last_fill))
            else:
                success = None
            try:
                mtime = time.strftime('%d.%m. %H:%M', time.localtime(
                    os.path.getmtime(os.path.join(RUNS_DOM, fn))))
            except OSError:
                mtime = ''
            entry = {'file': fn, 'label': label, 'task': task,
                     'steps': len(steps), 'answer': answer, 'success': success,
                     'seconds': None, 'source': 'transkript', 'mtime': mtime}
            lv = live.get((label, task))
            if lv:
                entry.update({'success': bool(lv.get('success')),
                              'steps': lv.get('steps', entry['steps']),
                              'seconds': lv.get('seconds'), 'source': 'ergebnis'})
            runs.append(entry)
    return {'runs': runs}


def read_ux():
    docs = []
    if os.path.isdir(RUNS_UX):
        for fn in sorted(os.listdir(RUNS_UX)):
            if not fn.endswith('.md'):
                continue
            p = os.path.join(RUNS_UX, fn)
            title = fn
            try:
                first = open(p, encoding='utf-8', errors='replace').readline().strip()
                if first.startswith('#'):
                    title = first.lstrip('#').strip()
            except OSError:
                pass
            try:
                docs.append({'file': fn, 'title': title,
                             'size': os.path.getsize(p),
                             'mtime': time.strftime('%d.%m. %H:%M',
                                                    time.localtime(os.path.getmtime(p)))})
            except OSError:
                docs.append({'file': fn, 'title': title, 'size': 0, 'mtime': ''})
    return {'docs': docs}


def read_overview():
    suite = read_suite()
    poly = read_polyglot()
    perf = read_perf()
    dom = read_dom()
    ux = read_ux()

    # Suite: letzter Eintrag pro (Label, Task) zählt; Modell = Label-Präfix.
    last = {}
    for e in suite['entries']:
        last[(e.get('model'), e.get('task'))] = e
    per_model = {m: {'pass': 0, 'total': 0} for m in MODELS}
    for (label, _task), e in last.items():
        for m in MODELS:
            if label == m or (label or '').startswith(m + '-'):
                per_model[m]['total'] += 1
                if e['pass']:
                    per_model[m]['pass'] += 1

    # Polyglot: größter zugeordneter Lauf pro Modell.
    polymap = {}
    for r in poly['runs']:
        if r['label'] in MODELS and r['n'] > 0:
            cur = polymap.get(r['label'])
            if not cur or r['n'] > cur['n']:
                polymap[r['label']] = r

    # Perf: bestes tg/pp bei Kontexttiefe 0.
    perfmap = {}
    for s in perf['series']:
        for m in MODELS:
            if s['label'].lower().startswith(m):
                # nur reine Tests werten (kombinierte pp+tg-Messungen ausschließen)
                tg = next((e['ts'] for e in s['entries']
                           if (e['tg'] or 0) > 0 and not (e['pp'] or 0)
                           and not e['depth']), None)
                pp = next((e['ts'] for e in s['entries']
                           if (e['pp'] or 0) > 0 and not (e['tg'] or 0)
                           and not e['depth']), None)
                cur = perfmap.setdefault(m, {'tg': None, 'tg_backend': '',
                                             'pp': None, 'pp_backend': ''})
                if tg and (not cur['tg'] or tg > cur['tg']):
                    cur.update({'tg': tg, 'tg_backend': s['backend']})
                if pp and (not cur['pp'] or pp > cur['pp']):
                    cur.update({'pp': pp, 'pp_backend': s['backend']})

    finished = {m: v for m, v in polymap.items()
                if not v['running'] and (not v['expected'] or v['n'] >= v['expected'])}
    poly_pct = {m: round(100 * v['pass2'] / v['n']) for m, v in finished.items() if v['n']}
    best_poly = max(poly_pct.values(), default=None)
    tgs = {m: v['tg'] for m, v in perfmap.items() if v.get('tg')}
    best_tg = max(tgs.values(), default=None)
    pps = {m: v['pp'] for m, v in perfmap.items() if v.get('pp')}
    best_pp = max(pps.values(), default=None)

    cards = []
    for m in MODELS:
        s = per_model[m]
        bits = []
        if tgs.get(m) and tgs[m] == best_tg:
            bits.append('schnellste Generierung')
        if m in poly_pct and poly_pct[m] == best_poly:
            bits.append('beste Polyglot-Quote')
        if pps.get(m) and pps[m] == best_pp:
            bits.append('schnellstes Prompt-Processing')
        if s['total'] and s['pass'] == s['total']:
            bits.append('Suite ohne Ausrutscher')
        if not bits:
            if s['total'] and s['pass'] < s['total']:
                bits.append('Suite mit Lücken (%d/%d)' % (s['pass'], s['total']))
            elif not s['total']:
                bits.append('noch keine Suite-Daten')
        if m not in perfmap:
            bits.append('Perf-Messung fehlt')
        pm = polymap.get(m)
        cards.append({
            'model': m,
            'name': MODEL_META[m]['name'],
            'note': MODEL_META[m]['note'],
            'suite': s,
            'poly': ({'pct1': round(100 * pm['pass1'] / pm['n']),
                      'pct2': round(100 * pm['pass2'] / pm['n']),
                      'n': pm['n'], 'expected': pm['expected'],
                      'running': pm['running'], 'dir': pm['dir']} if pm else None),
            'perf': perfmap.get(m),
            'verdict': ' · '.join(bits[:2]) or 'zu wenig Daten',
        })

    counts = {'suite': len(suite['entries']),
              'suite_labels': len(suite['labels']),
              'poly_runs': len([r for r in poly['runs'] if r['n'] > 0]),
              'perf_files': len(perf['files']),
              'dom': len(dom['runs']),
              'ux': len(ux['docs'])}
    return {'cards': cards, 'counts': counts,
            'generated': time.strftime('%d.%m.%Y %H:%M:%S')}


def read_meta():
    try:
        tasks = sorted(x for x in os.listdir(TASKS_DIR)
                       if os.path.isdir(os.path.join(TASKS_DIR, x)))
    except OSError:
        tasks = []
    return {'models': MODELS, 'tasks': tasks, 'backends': ['vulkan', 'rocm'],
            'dom_tasks': ['info', 'form'],
            'port8080': port_listening(8080), 'port8090': port_listening(8090)}


# --------------------------------------------------------- Läufe (Allowlist)

RUN_LOCK = threading.Lock()
RUNS = {}


def _save_meta(meta):
    try:
        with open(os.path.join(RUNSD, meta['id'] + '.meta.json'), 'w',
                  encoding='utf-8') as f:
            json.dump(meta, f, ensure_ascii=False, indent=1)
    except OSError:
        pass


def load_runs():
    if not os.path.isdir(RUNSD):
        return
    for fn in sorted(os.listdir(RUNSD)):
        if not fn.endswith('.meta.json'):
            continue
        try:
            meta = json.load(open(os.path.join(RUNSD, fn), encoding='utf-8'))
        except (OSError, json.JSONDecodeError):
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
                meta['detached'] = True     # Prozess lebt, aber ohne Handle
            else:
                meta['status'] = 'abgebrochen'
                _save_meta(meta)
        RUNS[meta['id']] = meta


def build_run(p):
    script = p.get('script')
    tasks = set(read_meta()['tasks'])
    timeout = p.get('timeout')
    if timeout in (None, ''):
        timeout = None
    else:
        try:
            timeout = int(timeout)
        except (TypeError, ValueError):
            raise ValueError('Timeout muss eine Zahl (Sekunden) sein')
        if not 60 <= timeout <= 14400:
            raise ValueError('Timeout: 60–14400 Sekunden')

    if script == 'suite':
        model = p.get('model')
        backend = p.get('backend') or 'vulkan'
        if model not in MODELS:
            raise ValueError('Modell muss eines der serve.sh-Labels sein')
        if backend not in ('vulkan', 'rocm'):
            raise ValueError('Backend: vulkan oder rocm')
        task = p.get('task')
        if task not in tasks:
            raise ValueError('unbekannter Task')
        label = '%s-%s' % (model, backend)
        argv = ['bash', os.path.join(BENCH, 'run-task.sh'), label, task]
        if timeout:
            argv.append(str(timeout))
        return argv, {}, 'Suite: %s × %s' % (label, task)

    if script == 'suite-api':
        mid = (p.get('model_id') or '').strip()
        if not MODELID_RE.match(mid):
            raise ValueError('ungültige Modell-ID (erlaubt: a-z 0-9 . _ / -)')
        task = p.get('task')
        if task not in tasks:
            raise ValueError('unbekannter Task')
        env = {'OC_CONFIG': 'opencode-config-api'}
        label = (p.get('label') or '').strip()
        if label:
            if not LABEL_RE.match(label):
                raise ValueError('ungültiges Label (erlaubt: a-z 0-9 . _ -)')
            env['LABEL'] = label
        argv = ['bash', os.path.join(BENCH, 'run-task-api.sh'), mid, task]
        if timeout:
            argv.append(str(timeout))
        return argv, env, 'Suite-API: %s × %s' % (label or mid, task)

    if script == 'dom':
        label = (p.get('label') or '').strip()
        if not LABEL_RE.match(label):
            raise ValueError('ungültiges Label (erlaubt: a-z 0-9 . _ -)')
        task = p.get('task')
        if task not in ('info', 'form'):
            raise ValueError('DOM-Aufgabe: info oder form')
        argv = ['python3', os.path.join(WEBAPP, 'dom-agent.py'), label, task]
        return argv, {}, 'DOM-Agent: %s × %s' % (label, task)

    if script == 'uxdom':
        label = (p.get('label') or '').strip()
        if not LABEL_RE.match(label):
            raise ValueError('ungültiges Label (erlaubt: a-z 0-9 . _ -)')
        argv = ['python3', os.path.join(WEBAPP, 'ux-dom-test.py'), label]
        return argv, {}, 'UX-DOM-Review: %s' % label

    raise ValueError('unbekanntes Skript')


def start_run(p):
    argv, env, desc = build_run(p)
    with RUN_LOCK:
        for r in RUNS.values():
            if r.get('status') == 'läuft':
                raise ValueError('Es läuft bereits ein Benchmark — bitte warten. '
                                 '(GPU/Modellserver gehören dem aktiven Lauf.)')
        rid = time.strftime('%Y%m%d-%H%M%S') + '-' + p['script']
        base, n = rid, 1
        while rid in RUNS:
            n += 1
            rid = '%s-%d' % (base, n)
        os.makedirs(RUNSD, exist_ok=True)
        logp = os.path.join(RUNSD, rid + '.log')
        logf = open(logp, 'wb')
        try:
            proc = subprocess.Popen(argv, stdout=logf, stderr=subprocess.STDOUT,
                                    stdin=subprocess.DEVNULL, cwd=BENCH,
                                    env={**os.environ, **env},
                                    start_new_session=True)
        except OSError as e:
            logf.close()
            raise ValueError('Start fehlgeschlagen: %s' % e)
        meta = {'id': rid, 'desc': desc, 'script': p['script'],
                'cmd': ' '.join(argv), 'envnote': ' '.join('%s=%s' % kv for kv in env.items()),
                'status': 'läuft', 'pid': proc.pid, 'rc': None,
                'started': time.strftime('%Y-%m-%d %H:%M:%S'),
                'started_ts': time.time(), 'ended': None, 'seconds': None}
        RUNS[rid] = meta
        _save_meta(meta)
    threading.Thread(target=_wait_run, args=(proc, logf, meta), daemon=True).start()
    return meta


def _wait_run(proc, logf, meta):
    rc = proc.wait()
    try:
        logf.close()
    except OSError:
        pass
    meta.update({'status': 'fertig' if rc == 0 else 'fehler', 'rc': rc,
                 'ended': time.strftime('%Y-%m-%d %H:%M:%S'),
                 'seconds': round(time.time() - meta['started_ts'], 1)})
    _save_meta(meta)


def run_status(rid):
    meta = RUNS.get(rid)
    if not meta:
        return None
    if meta.get('status') == 'läuft' and meta.get('detached'):
        try:
            os.kill(meta['pid'], 0)
        except OSError:
            meta['status'] = 'beendet (Exit unbekannt)'
            _save_meta(meta)
    return {**meta, 'tail': tail_lines(os.path.join(RUNSD, rid + '.log'), 40)}


# ------------------------------------------------------------------- HTTP

class Handler(BaseHTTPRequestHandler):
    server_version = 'ailab-dash/1'

    def log_message(self, *a):
        pass

    def _json(self, obj, code=200):
        body = json.dumps(obj, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header('Content-Type', 'application/json; charset=utf-8')
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _file(self, path, ctype):
        try:
            body = open(path, 'rb').read()
        except OSError:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header('Content-Type', ctype)
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        q = {k: v[0] for k, v in urllib.parse.parse_qs(u.query).items()}
        p = u.path
        try:
            if p in ('/', '/index.html'):
                return self._file(os.path.join(DASH, 'index.html'),
                                  'text/html; charset=utf-8')
            if p.startswith('/static/'):
                name = os.path.basename(p[len('/static/'):])
                full = os.path.join(STATIC, name)
                if not os.path.isfile(full):
                    return self.send_error(404)
                ct = {'css': 'text/css', 'js': 'text/javascript',
                      'svg': 'image/svg+xml', 'html': 'text/html'}.get(
                          name.rsplit('.', 1)[-1], 'application/octet-stream')
                return self._file(full, ct + '; charset=utf-8')
            if p == '/api/overview':
                return self._json(read_overview())
            if p == '/api/suite':
                return self._json(read_suite())
            if p == '/api/polyglot':
                return self._json(read_polyglot())
            if p == '/api/perf':
                return self._json(read_perf())
            if p == '/api/dom':
                return self._json(read_dom())
            if p == '/api/dom/transcript':
                fn = q.get('file', '')
                try:
                    ok = fn in os.listdir(RUNS_DOM)
                except OSError:
                    ok = False
                if not ok:
                    return self._json({'error': 'unbekannte Datei'}, 404)
                return self._json({'file': fn,
                                   'lines': _parse_dom_transcript(
                                       os.path.join(RUNS_DOM, fn))})
            if p == '/api/ux':
                return self._json(read_ux())
            if p == '/api/ux/doc':
                fn = q.get('file', '')
                try:
                    ok = fn in os.listdir(RUNS_UX)
                except OSError:
                    ok = False
                if not ok:
                    return self._json({'error': 'unbekannte Datei'}, 404)
                md = open(os.path.join(RUNS_UX, fn), encoding='utf-8',
                          errors='replace').read()
                return self._json({'file': fn, 'md': md})
            if p == '/api/wiki':
                return self._json({'docs': [{'id': k, 'title': t,
                                             'exists': os.path.isfile(fp)}
                                            for k, (t, fp) in WIKI.items()]})
            if p == '/api/wiki/doc':
                d = WIKI.get(q.get('id', ''))
                if not d:
                    return self._json({'error': 'unbekanntes Dokument'}, 404)
                try:
                    md = open(d[1], encoding='utf-8', errors='replace').read()
                except OSError:
                    return self._json({'error': 'Datei fehlt: ' + d[1]}, 404)
                return self._json({'id': q['id'], 'title': d[0], 'md': md})
            if p == '/api/meta':
                return self._json(read_meta())
            if p == '/api/runs':
                rs = sorted(RUNS.values(),
                            key=lambda r: r.get('started_ts') or 0, reverse=True)
                return self._json({'active': any(r.get('status') == 'läuft'
                                                 for r in rs),
                                   'runs': rs[:50]})
            m = re.match(r'^/api/runs/([A-Za-z0-9_-]+)$', p)
            if m:
                st = run_status(m.group(1))
                if st is None:
                    return self._json({'error': 'unbekannter Lauf'}, 404)
                return self._json(st)
            return self.send_error(404)
        except BrokenPipeError:
            pass
        except Exception as e:
            try:
                self._json({'error': '%s: %s' % (type(e).__name__, e)}, 500)
            except Exception:
                pass

    def do_POST(self):
        u = urllib.parse.urlparse(self.path)
        if u.path != '/api/runs':
            return self.send_error(404)
        try:
            n = int(self.headers.get('Content-Length') or 0)
            body = json.loads(self.rfile.read(n).decode() or '{}')
            meta = start_run(body)
            return self._json({'ok': True, 'id': meta['id']})
        except ValueError as e:
            return self._json({'ok': False, 'error': str(e)}, 400)
        except Exception as e:
            return self._json({'ok': False,
                               'error': '%s: %s' % (type(e).__name__, e)}, 500)


if __name__ == '__main__':
    os.makedirs(RUNSD, exist_ok=True)
    load_runs()
    srv = ThreadingHTTPServer(('127.0.0.1', 8100), Handler)
    print('AI-Lab Benchmark-Dashboard: http://127.0.0.1:8100')
    srv.serve_forever()
