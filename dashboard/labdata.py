"""Read-only-Datenquellen: Suite, Polyglot (mit Lauf-Lebenszyklus), Perf,
DOM/UX, Übersicht, Wiki. Rohdaten unter bench/, logs/, models/ werden nie
verändert — Statuskorrekturen laufen über das Overlay registry/run-annotations.json.
"""
import calendar
import glob
import hashlib
import json
import os
import re
import time

from labcore import (BENCH, RESULTS, TASKS_DIR, TMPB, AIDER_ROOT, LOGS, WEBAPP,
                     RUNS_DOM, RUNS_UX, POLY_LABELS, ANNOT, DASH, LAB, WIKID,
                     RUNSD, CACHE, load_json, tail_lines, bench_containers)
import labregistry

ROBUST_DIR = os.path.join(BENCH, 'robustness-battery')
ROBUST_RESULTS = os.path.join(ROBUST_DIR, 'results.json')
RUNS_DIR = os.path.join(BENCH, 'runs')


def base_model_ids():
    d = labregistry.load_models()
    return [e['id'] for e in d.get('models', []) if e.get('kind') == 'base']


def model_names():
    d = labregistry.load_models()
    return {e['id']: e.get('name', e['id']) for e in labregistry.all_model_entries(d)}


# ------------------------------------------------------------------ Suite

def read_suite():
    """results.jsonl — Dedupe: letzte Zeile pro (model, task) gewinnt.
    Ältere Zeilen bleiben sichtbar (superseded=True, per Toggle einblendbar)."""
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
    last = {}
    for e in entries:
        last[(e.get('model'), e.get('task'))] = e['line']
    n_sup = 0
    for e in entries:
        e['superseded'] = last[(e.get('model'), e.get('task'))] != e['line']
        n_sup += 1 if e['superseded'] else 0

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
    models = base_model_ids()

    def label_key(lbl):
        for i, m in enumerate(models):
            if lbl == m or lbl.startswith(m + '-'):
                return (0, i, lbl)
        return (1, 0, lbl)

    labels.sort(key=label_key)
    rob = robustness_map()
    for e in entries:
        e['robust'] = rob.get((e.get('model'), e.get('task')))
    return {'tasks': tasks, 'labels': labels, 'entries': entries,
            'superseded_count': n_sup}


# ------------------------------------------------------------ Robustheit

def _battery_configs():
    """robustness-battery/<task>/battery.json -> {task: {'lang':…, 'cwd':…}}."""
    out = {}
    try:
        names = sorted(os.listdir(ROBUST_DIR))
    except OSError:
        return out
    for name in names:
        p = os.path.join(ROBUST_DIR, name)
        if not os.path.isdir(p):
            continue
        cfg = load_json(os.path.join(p, 'battery.json'), None)
        if isinstance(cfg, dict) and cfg.get('task') == name and cfg.get('cwd'):
            out[name] = {'lang': cfg.get('lang') or 'go', 'cwd': cfg['cwd']}
    return out


def _ws_fingerprint(ws, cwd, lang):
    """sha256 wie der Runner (cmd/battery/main.go): sortierte relative Pfade +
    Inhalte der Nicht-Test-Quelldateien unter ws/<cwd>. None, wenn nicht lesbar."""
    root = os.path.join(ws, cwd)
    if not os.path.isdir(root):
        return None
    skip = {'.git'}
    if lang == 'node':
        skip |= {'node_modules', 'test', 'tests'}
    rels = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in skip]
        for fn in filenames:
            full = os.path.join(dirpath, fn)
            if lang == 'go':
                if fn.endswith('.go') and not fn.endswith('_test.go'):
                    rels.append(os.path.relpath(full, root))
            elif fn.endswith(('.js', '.mjs', '.ts')):
                rels.append(os.path.relpath(full, root))
    rels.sort()
    h = hashlib.sha256()
    for rel in rels:
        try:
            with open(os.path.join(root, rel), 'rb') as f:
                b = f.read()
        except OSError:
            return None
        h.update(rel.encode('utf-8'))
        h.update(b'\x00')
        h.update(b)
        h.update(b'\x00')
    return h.hexdigest()


def _compute_stale():
    """Stale (task,label)-Paare: aktueller ws-Fingerprint != gespeicherter."""
    stale = []
    res = load_json(ROBUST_RESULTS, None)
    if not isinstance(res, dict):
        return stale
    cfgs = _battery_configs()
    for task, bylabel in (res.get('results') or {}).items():
        cfg = cfgs.get(task)
        if not cfg:
            continue
        for label, r in bylabel.items():
            fp = r.get('ws_fingerprint')
            if not fp:
                continue
            cur = _ws_fingerprint(os.path.join(RUNS_DIR, label, task, 'ws'),
                                  cfg['cwd'], cfg['lang'])
            if cur and cur != fp:
                stale.append([task, label])
    stale.sort()
    return stale


def staleness():
    """Staleness je (task,label) — Fingerprint-Vergleich, 30-s-Cache."""
    return CACHE.get('robust_stale', 30, _compute_stale)


def _compute_robustness_map():
    out = {}
    res = load_json(ROBUST_RESULTS, None)
    if not isinstance(res, dict):
        return out
    stale = frozenset(map(tuple, staleness()))
    for task, bylabel in (res.get('results') or {}).items():
        for label, r in bylabel.items():
            out[(label, task)] = {
                'real_pass': r.get('real_pass'),
                'real_total': r.get('real_total'),
                'path_pass': r.get('path_pass'),
                'path_total': r.get('path_total'),
                'buildable': bool(r.get('buildable')),
                'failed': r.get('failed') or [],
                'stale': (task, label) in stale,
                'error': r.get('error') or '',
            }
    return out


def robustness_map():
    """{(label, task): robust-Dict} aus results.json (Schema 2), 30-s-Cache."""
    return CACHE.get('robust_entry', 30, _compute_robustness_map)


def read_robustness():
    """/api/robustness: Batterien, Ergebnisse, Scores je Label, Staleness."""
    res = load_json(ROBUST_RESULTS, None)
    if not isinstance(res, dict):
        res = {}
    batteries = res.get('batteries') or {}
    cfgs = _battery_configs()
    total_tasks = sorted(cfgs) or sorted(batteries)
    by_entry = robustness_map()
    labels = sorted({k[0] for k in by_entry})
    scores = {}
    for label in labels:
        per_task = {}
        rp = rt = pp = pt = 0
        n_tasks = 0
        for task in total_tasks:
            r = by_entry.get((label, task))
            per_task[task] = r
            if r and r['buildable']:
                rp += r['real_pass'] or 0
                rt += r['real_total'] or 0
                pp += r['path_pass'] or 0
                pt += r['path_total'] or 0
                n_tasks += 1
        scores[label] = {
            'real_score': (float(rp) / rt) if rt else None,
            'real_pass': rp, 'real_total': rt,
            'path_pass': pp, 'path_total': pt,
            'n_tasks': n_tasks, 'n_missing': len(total_tasks) - n_tasks,
            'per_task': per_task,
        }
    return {'batteries': batteries,
            'results': res.get('results') or {},
            'scores': scores,
            'stale': staleness()}


# --------------------------------------------------------------- Polyglot

def _dir_utc_ts(run):
    m = re.match(r'^(\d{4})-(\d{2})-(\d{2})-(\d{2})-(\d{2})-(\d{2})', run)
    if not m:
        return None
    try:
        return calendar.timegm(time.strptime(m.group(0), '%Y-%m-%d-%H-%M-%S'))
    except ValueError:
        return None


def _run_log_path(run):
    name = run.split('--', 1)[1] if '--' in run else run
    return os.path.join(AIDER_ROOT, 'run-%s.log' % name)


def _stats_present(run):
    tl = tail_lines(_run_log_path(run), 60)
    return any('pass_rate_2' in ln for ln in tl)


def read_polyglot(show_hidden=False):
    label_map = load_json(POLY_LABELS, {})
    annot = load_json(ANNOT, {})
    conts = bench_containers()['containers']
    aider_conts = [c for c in conts if c['image'].startswith('aider-bench')]
    now = time.time()

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
                exercises.append({'name': d.get('testcase')
                                  or os.path.basename(os.path.dirname(fp)),
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

            # ---- Lebenszyklus -------------------------------------------------
            # Reihenfolge ist heilig: ein aktiv schreibender Lauf ist "läuft",
            # egal was im Log steht (ein getöteter Wrapper kann eine Stats-Fußzeile
            # hinterlassen haben). Und eine Stats-Zeile allein macht einen Lauf mit
            # bekannter Sollzahl nie "fertig", solange Übungen fehlen.
            complete = (bool(expected and n >= expected)
                        or (not expected and _stats_present(run)))
            dts = _dir_utc_ts(run)
            matched = any(dts and c['started'] and abs(dts - c['started']) < 600
                          for c in aider_conts)
            fresh = newest and (now - newest) < 1800
            if matched or (not complete and aider_conts and fresh):
                status = 'läuft'
            elif complete:
                status = 'fertig'
            elif any('Traceback' in ln for ln in tail_lines(_run_log_path(run), 30)):
                status = 'fehler'
            else:
                status = 'verworfen'

            a = annot.get(run) or {}
            hidden = bool(a.get('hidden')) or (status in ('verworfen', 'fehler')
                                               and 'hidden' not in a)
            runs.append({'dir': run,
                         'label': a.get('label') or label_map.get(run, ''),
                         'note': a.get('note', ''),
                         'started': run[:19], 'n': n, 'expected': expected,
                         'pass1': p1, 'pass2': p2,
                         'duration': round(sum(e['duration'] for e in exercises), 1),
                         'status': status, 'running': status == 'läuft',
                         'hidden': hidden, 'newest': newest,
                         'langs': langs, 'exercises': exercises})
    # ---- Zweite Quelle: bench/polyglot-oc/runs (OC-/agy-/Claude-Harness-Läufe) ----
    poly_oc = os.path.join(BENCH, 'polyglot-oc', 'runs')
    if os.path.isdir(poly_oc):
        for label in sorted(os.listdir(poly_oc)):
            p = os.path.join(poly_oc, label)
            if not os.path.isdir(p):
                continue
            exercises, newest = [], 0.0
            for fp in glob.glob(os.path.join(p, '*', '*', 'result.json')):
                try:
                    d = json.load(open(fp, encoding='utf-8'))
                except (OSError, json.JSONDecodeError):
                    continue
                o = d.get('tests_outcomes') or []
                st = ('err' if d.get('error') else
                      'p1' if (o and o[0]) else
                      'p2' if True in o else 'fail')
                exercises.append({'name': d.get('name') or '?',
                                  'lang': d.get('lang') or fp.split(os.sep)[-3],
                                  'status': st, 'tries': len(o),
                                  'duration': round(d.get('seconds') or 0, 1)})
                try:
                    newest = max(newest, os.path.getmtime(fp))
                except OSError:
                    pass
            exercises.sort(key=lambda e: (e['lang'], e['name']))
            n = len(exercises)
            p1 = sum(1 for e in exercises if e['status'] == 'p1')
            p2 = p1 + sum(1 for e in exercises if e['status'] == 'p2')
            langs = {}
            for e in exercises:
                L = langs.setdefault(e['lang'], {'n': 0, 'p1': 0, 'p2': 0})
                L['n'] += 1
                if e['status'] == 'p1':
                    L['p1'] += 1; L['p2'] += 1
                elif e['status'] == 'p2':
                    L['p2'] += 1
            is_val = label.startswith('val-')
            expected = None if is_val else 73
            has_summary = os.path.isfile(os.path.join(p, 'summary.json'))
            fresh = newest and (now - newest) < 1800
            if expected and n >= expected and has_summary:
                status = 'fertig'
            elif fresh:
                status = 'läuft'
            elif is_val:
                status = 'verworfen'
            elif has_summary:
                status = 'fertig'
            else:
                status = 'verworfen'
            a = annot.get('oc:' + label) or {}
            hidden = bool(a.get('hidden')) or (is_val and 'hidden' not in a) \
                or (status == 'verworfen' and 'hidden' not in a)
            runs.append({'dir': 'oc:' + label,
                         'label': a.get('label') or label,
                         'note': a.get('note', 'eigener Harness-Lauf (nicht Aider)'),
                         'started': time.strftime('%Y-%m-%d %H:%M',
                                                  time.localtime(os.path.getmtime(p))),
                         'n': n, 'expected': expected, 'pass1': p1, 'pass2': p2,
                         'duration': round(sum(e['duration'] for e in exercises), 1),
                         'status': status, 'running': status == 'läuft',
                         'hidden': hidden, 'newest': newest,
                         'langs': langs, 'exercises': exercises})

    runs.sort(key=lambda r: r['dir'], reverse=True)
    n_hidden = sum(1 for r in runs if r['hidden'])
    if not show_hidden:
        runs = [r for r in runs if not r['hidden']]
    return {'runs': runs, 'hidden_count': n_hidden}


def annotate_run(p):
    run = (p.get('dir') or '').strip()
    if not re.match(r'^[A-Za-z0-9][A-Za-z0-9._-]{0,200}$', run):
        raise ValueError('Ungültiger Run-Ordnername.')
    if not os.path.isdir(os.path.join(TMPB, run)):
        raise ValueError('Unbekannter Polyglot-Lauf: %s' % run)
    annot = load_json(ANNOT, {})
    a = dict(annot.get(run) or {})
    if 'hidden' in p:
        a['hidden'] = bool(p['hidden'])
    if 'label' in p:
        a['label'] = str(p['label'] or '').strip()[:60]
    if 'note' in p:
        a['note'] = str(p['note'] or '').strip()[:300]
    annot[run] = a
    from labcore import save_json
    save_json(ANNOT, annot)
    return {'ok': True, 'dir': run, 'annotation': a}


# ------------------------------------------------------------------- Perf

def _load_perf_file(path):
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
    files.sort(key=lambda f: ('partial' in f, f))
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
    models = base_model_ids()

    def skey(s):
        for i, mdl in enumerate(models):
            if s['label'].lower().startswith(mdl):
                return (i, s['backend'])
        return (99, s['backend'])

    out.sort(key=skey)
    return {'series': out, 'files': parsed}


# ---------------------------------------------------------------- DOM / UX

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


# -------------------------------------------------------------- Übersicht

def read_overview():
    suite = read_suite()
    poly = read_polyglot(show_hidden=False)
    perf = read_perf()
    dom = read_dom()
    ux = read_ux()
    models = base_model_ids()
    names = model_names()
    reg = labregistry.load_models()
    meta = {e['id']: e for e in reg.get('models', [])}

    last = {}
    for e in suite['entries']:
        if not e['superseded']:
            last[(e.get('model'), e.get('task'))] = e
    per_model = {m: {'pass': 0, 'total': 0} for m in models}
    for (label, _task), e in last.items():
        for m in models:
            if label == m or (label or '').startswith(m + '-'):
                per_model[m]['total'] += 1
                if e['pass']:
                    per_model[m]['pass'] += 1

    # Polyglot: pro Modell den besten fertigen Lauf; sonst den laufenden.
    polymap = {}
    for r in poly['runs']:
        if r['label'] in models and r['n'] > 0:
            cur = polymap.get(r['label'])
            better = (not cur
                      or (r['status'] == 'fertig' and cur['status'] != 'fertig')
                      or (r['status'] == cur['status'] and r['n'] > cur['n']))
            if cur and cur['status'] == 'fertig' and r['status'] != 'fertig':
                better = False
            if better:
                polymap[r['label']] = r

    perfmap = {}
    for s in perf['series']:
        for m in models:
            if s['label'].lower().startswith(m):
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

    finished = {m: v for m, v in polymap.items() if v['status'] == 'fertig' and v['n']}
    poly_pct = {m: round(100 * v['pass2'] / v['n']) for m, v in finished.items()}
    best_poly = max(poly_pct.values(), default=None)
    tgs = {m: v['tg'] for m, v in perfmap.items() if v.get('tg')}
    best_tg = max(tgs.values(), default=None)
    pps = {m: v['pp'] for m, v in perfmap.items() if v.get('pp')}
    best_pp = max(pps.values(), default=None)

    cards = []
    for m in models:
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
        e = meta.get(m, {})
        cards.append({
            'model': m,
            'name': names.get(m, m),
            'note': (e.get('notes') or '').split('.')[0][:60],
            'suite': s,
            'poly': ({'pct1': round(100 * pm['pass1'] / pm['n']),
                      'pct2': round(100 * pm['pass2'] / pm['n']),
                      'n': pm['n'], 'expected': pm['expected'],
                      'running': pm['running'], 'dir': pm['dir']} if pm else None),
            'perf': perfmap.get(m),
            'verdict': ' · '.join(bits[:2]) or 'zu wenig Daten',
        })

    counts = {'suite': sum(1 for e in suite['entries'] if not e['superseded']),
              'suite_labels': len(suite['labels']),
              'poly_runs': len([r for r in poly['runs'] if r['n'] > 0]),
              'poly_running': len([r for r in poly['runs'] if r['running']]),
              'perf_files': len(perf['files']),
              'dom': len(dom['runs']),
              'ux': len(ux['docs'])}
    return {'cards': cards, 'counts': counts,
            'generated': time.strftime('%d.%m.%Y %H:%M:%S')}


# ------------------------------------------------------------------- Wiki

LEGACY_DOCS = {
    'readme': ('README (Lab-Setup)', os.path.join(LAB, 'README.md')),
    'api-runbook': ('API-Runbook', os.path.join(LAB, 'API-RUNBOOK.md')),
    'guide': ('Guide: Neue Benchmarks', os.path.join(DASH, 'GUIDE-neue-benchmarks.md')),
    'failures': ('Failure-Analyse', os.path.join(BENCH, 'failure-analysis.md')),
    'ux-flaws': ('UX-Flaws (Testseite)', os.path.join(WEBAPP, 'UX-FLAWS.md')),
    'todo': ('Fahrplan (TODO)', os.path.join(LAB, 'TODO-morgen.md')),
}

_SECTION_PREFIX = [('konzept', 'konzepte'), ('anleitung', 'anleitungen'),
                   ('howto', 'anleitungen'), ('referenz', 'referenz'),
                   ('ref-', 'referenz')]


def read_wiki():
    """Wiki v2: dashboard/wiki/*.md. Sektionszuordnung bevorzugt über das
    Manifest wiki.json des Autor-Agents ([{section, pages:[{file,title,one_line}]}]);
    Dateien ohne Manifest-Eintrag über Dateinamens-Präfix. Fehlt das Verzeichnis,
    bleiben die Sektionen leer (kein Fehler)."""
    sections = {'konzepte': [], 'anleitungen': [], 'referenz': []}
    seen = set()
    manifest = load_json(os.path.join(WIKID, 'wiki.json'), None)
    if isinstance(manifest, list):
        secmap = {'konzepte': 'konzepte', 'anleitungen': 'anleitungen',
                  'referenz': 'referenz'}
        for m in manifest:
            sec = secmap.get(str(m.get('section', '')).lower())
            if not sec:
                continue
            for pg in m.get('pages') or []:
                fn = pg.get('file') or ''
                if '/' in fn or not fn.endswith('.md'):
                    continue
                if not os.path.isfile(os.path.join(WIKID, fn)):
                    continue
                sections[sec].append({'id': 'wiki:' + fn,
                                      'title': pg.get('title') or fn[:-3],
                                      'sub': pg.get('one_line') or '',
                                      'file': fn})
                seen.add(fn)
    if os.path.isdir(WIKID):
        for fn in sorted(os.listdir(WIKID)):
            if not fn.endswith('.md') or fn in seen:
                continue
            low = fn.lower()
            sec = 'konzepte'
            for pref, target in _SECTION_PREFIX:
                if low.startswith(pref):
                    sec = target
                    break
            title = re.sub(r'^(konzepte?|anleitung(en)?|howto|referenz|ref)[-_]?', '',
                           fn[:-3], flags=re.I).replace('-', ' ').replace('_', ' ').strip() or fn
            try:
                first = open(os.path.join(WIKID, fn), encoding='utf-8',
                             errors='replace').readline().strip()
                if first.startswith('#'):
                    title = first.lstrip('#').strip()
            except OSError:
                pass
            sections[sec].append({'id': 'wiki:' + fn, 'title': title, 'file': fn})
    legacy = [{'id': 'legacy:' + k, 'title': t, 'exists': os.path.isfile(fp)}
              for k, (t, fp) in LEGACY_DOCS.items()]
    return {'sections': [
        {'id': 'konzepte', 'title': 'Konzepte', 'docs': sections['konzepte']},
        {'id': 'anleitungen', 'title': 'Anleitungen', 'docs': sections['anleitungen']},
        {'id': 'referenz', 'title': 'Referenz', 'docs': sections['referenz'],
         'legacy': legacy},
    ], 'empty': not any(sections.values())}


def read_wiki_doc(doc_id):
    if doc_id.startswith('wiki:'):
        fn = doc_id[5:]
        if '/' in fn or not fn.endswith('.md'):
            raise ValueError('Ungültige Wiki-Datei.')
        path = os.path.join(WIKID, fn)
        title = fn
    elif doc_id.startswith('legacy:'):
        d = LEGACY_DOCS.get(doc_id[7:])
        if not d:
            raise ValueError('Unbekanntes Dokument.')
        title, path = d
    else:
        raise ValueError('Unbekanntes Dokument.')
    try:
        md = open(path, encoding='utf-8', errors='replace').read()
    except OSError:
        raise ValueError('Datei fehlt: %s' % path)
    return {'id': doc_id, 'title': title, 'md': md}
