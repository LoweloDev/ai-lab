#!/usr/bin/env python3
"""AI-Lab Control-Dashboard v2 — Python-Stdlib, http.server, 127.0.0.1:8100.

Liest alle Benchmark-Ergebnisse strikt read-only; steuert Läufe ausschließlich
über die Allowlist in labjobs.py. Schreibzugriffe nur unter dashboard/
(registry/, runs/, polyglot-labels.json) plus Modell-Downloads nach models/.

Start: dashboard/start.sh
"""
import json
import os
import re
import time
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from labcore import (DASH, STATIC, RUNSD, WIKID, RUNS_DOM, RUNS_UX,
                     gpu_lock, llama_procs, port_listening)
import labdata
import labjobs
import labregistry


def read_state():
    """Systemzustand fürs Banner — alles passiv erhoben, :8080 wird nie berührt."""
    lock = gpu_lock()
    procs = llama_procs()
    names = labdata.model_names()
    reg = labregistry.load_models()
    file2id = {}
    for e in labregistry.all_model_entries(reg):
        if e.get('kind') != 'vision' and e.get('file'):
            file2id.setdefault(e['file'], e['id'])
    servers = []
    for pr in procs:
        mid = file2id.get(pr['model_file'])
        servers.append({**pr, 'model_id': mid,
                        'model_name': names.get(mid, pr['model_file'] or '?')})
    jl = labjobs.jobs_list()
    return {'lock': lock,
            'llama': servers,
            'port8090': port_listening(8090),
            'api_models': labjobs.api_models(),
            'slots': jl['slots'],
            'jobs_active': jl['active'],
            'now': time.strftime('%H:%M:%S')}


def read_meta():
    """Wizard-Metadaten: verfügbare Benchmarks, Tasks, Modelle, API-Modelle."""
    tasks = labjobs._tasks()
    lock = gpu_lock()
    poly_ok = os.path.isfile(labjobs.POLY_SCRIPT)
    harn = {h['id']: h for h in labregistry.harnesses_view()['harnesses']}
    aider_img = bool(harn.get('aider-bench', {}).get('installed'))
    benches = [
        {'id': 'suite', 'name': 'Suite (lokales Modell)', 'cls': 'gpu',
         'desc': 'OpenCode gegen llama-server auf :8080 — %d Repo-Tasks.' % len(tasks),
         'available': True},
        {'id': 'suite-api', 'name': 'API-Suite (Cloud)', 'cls': 'cloud',
         'desc': 'Dieselben Tasks gegen Cloud-APIs (DeepSeek/Gemini) — GPU-frei.',
         'available': True},
        {'id': 'dom', 'name': 'DOM-Test', 'cls': 'gpu',
         'desc': 'Agent navigiert die Testseite auf :8090 (info/form).',
         'available': True, 'needs_8090': True},
        {'id': 'uxdom', 'name': 'UX-DOM-Review', 'cls': 'gpu',
         'desc': 'UX-Review der Testseite über den HTML-Quelltext.',
         'available': True},
    ]
    if poly_ok and aider_img:
        benches.append({'id': 'polyglot', 'name': 'Polyglot (py+go, 73 Übungen)',
                        'cls': 'gpu', 'available': True,
                        'desc': 'Aider-Benchmark gegen :8080 — dauert Stunden.'})
    benches.append({'id': 'robustheit', 'name': 'Robustheit neu berechnen',
                    'cls': 'cpu', 'available': True,
                    'desc': 'Robustheits-Batterie (Zusatz-Metrik) über alle '
                            'Abgaben — CPU, kein GPU-Lock.'})
    return {'benchmarks': benches, 'tasks': tasks,
            'dom_tasks': ['info', 'form'],
            'backends': ['vulkan', 'rocm'],
            'api_models': labjobs.api_models(),
            'lock': lock,
            'llama': llama_procs(),
            'port8090': port_listening(8090),
            'timeout_default': 1200}


class Handler(BaseHTTPRequestHandler):
    server_version = 'ailab-dash/2'

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

    # ------------------------------------------------------------------ GET
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

            if p == '/api/state':
                return self._json(read_state())
            if p == '/api/overview':
                return self._json(labdata.read_overview())
            if p == '/api/suite':
                return self._json(labdata.read_suite())
            if p == '/api/robustness':
                return self._json(labdata.read_robustness())
            if p == '/api/polyglot':
                return self._json(labdata.read_polyglot(
                    show_hidden=q.get('hidden') == '1'))
            if p == '/api/perf':
                return self._json(labdata.read_perf())
            if p == '/api/dom':
                return self._json(labdata.read_dom())
            if p == '/api/dom/transcript':
                fn = q.get('file', '')
                try:
                    ok = fn in os.listdir(RUNS_DOM)
                except OSError:
                    ok = False
                if not ok:
                    return self._json({'error': 'unbekannte Datei'}, 404)
                return self._json({'file': fn,
                                   'lines': labdata._parse_dom_transcript(
                                       os.path.join(RUNS_DOM, fn))})
            if p == '/api/ux':
                return self._json(labdata.read_ux())
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
                return self._json(labdata.read_wiki())
            if p == '/api/wiki/doc':
                try:
                    return self._json(labdata.read_wiki_doc(q.get('id', '')))
                except ValueError as e:
                    return self._json({'error': str(e)}, 404)
            if p == '/api/meta':
                return self._json(read_meta())
            if p == '/api/registry/models':
                return self._json(labregistry.models_view(
                    labjobs.active_model_ids()))
            if p == '/api/registry/harnesses':
                return self._json(labregistry.harnesses_view())
            if p == '/api/registry/backends':
                return self._json(labregistry.backends_view())
            if p == '/api/jobs':
                return self._json(labjobs.jobs_list())
            m = re.match(r'^/api/jobs/([A-Za-z0-9_-]+)$', p)
            if m:
                st = labjobs.job_status(m.group(1))
                if st is None:
                    return self._json({'error': 'unbekannter Job'}, 404)
                return self._json(st)
            return self.send_error(404)
        except BrokenPipeError:
            pass
        except Exception as e:
            try:
                self._json({'error': '%s: %s' % (type(e).__name__, e)}, 500)
            except Exception:
                pass

    # ----------------------------------------------------------------- POST
    def do_POST(self):
        u = urllib.parse.urlparse(self.path)
        p = u.path
        try:
            n = int(self.headers.get('Content-Length') or 0)
            if n > 1024 * 1024:
                return self._json({'ok': False, 'error': 'Body zu groß'}, 413)
            body = json.loads(self.rfile.read(n).decode() or '{}')
        except (ValueError, UnicodeDecodeError):
            return self._json({'ok': False, 'error': 'Ungültiges JSON'}, 400)
        try:
            if p == '/api/jobs':
                if body.pop('validate_only', False):
                    labjobs.build_job(body)
                    return self._json({'ok': True, 'valid': True})
                meta = labjobs.submit(body)
                return self._json({'ok': True, 'id': meta['id']})
            m = re.match(r'^/api/jobs/([A-Za-z0-9_-]+)/cancel$', p)
            if m:
                return self._json(labjobs.cancel(m.group(1)))
            if p == '/api/registry/models':
                return self._json(labregistry.upsert_model(body))
            if p == '/api/registry/models/remove':
                return self._json(labregistry.remove_model(
                    body, labjobs.active_model_ids()))
            if p == '/api/registry/models/download':
                meta = labjobs.submit({'action': 'download',
                                       'model': body.get('id'),
                                       'force': body.get('force')})
                return self._json({'ok': True, 'id': meta['id']})
            if p == '/api/registry/harnesses':
                return self._json(labregistry.save_harnesses(body))
            if p == '/api/registry/backends':
                return self._json(labregistry.save_backends(body))
            if p == '/api/polyglot/annotate':
                return self._json(labdata.annotate_run(body))
            return self.send_error(404)
        except ValueError as e:
            return self._json({'ok': False, 'error': str(e)}, 400)
        except Exception as e:
            return self._json({'ok': False,
                               'error': '%s: %s' % (type(e).__name__, e)}, 500)


if __name__ == '__main__':
    os.makedirs(RUNSD, exist_ok=True)
    os.makedirs(WIKID, exist_ok=True)
    labregistry.ensure_seed()
    labjobs.load_persisted()
    srv = ThreadingHTTPServer(('127.0.0.1', 8100), Handler)
    print('AI-Lab Control-Dashboard v2: http://127.0.0.1:8100')
    srv.serve_forever()
