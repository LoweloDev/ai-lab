"""AI-Lab Dashboard v2 — Kern: Pfade, Helfer, Systemzustand (strikt read-only).

Sicherheitsregeln (hart):
  - Es wird NIE ein Podman-Container gestoppt/gestartet/signalisiert.
  - Es wird NIE ein llama-server-Prozess beendet.
  - :8080 wird NIE aktiv angesprochen — Erkennung nur über /proc.
  - Schreibzugriffe nur unter dashboard/ (plus Downloads nach models/).
"""
import json
import os
import re
import subprocess
import threading
import time

HOME = os.path.expanduser('~')
LAB = os.path.join(HOME, 'ai-lab')
BENCH = os.path.join(LAB, 'bench')
DASH = os.path.join(LAB, 'dashboard')
STATIC = os.path.join(DASH, 'static')
RUNSD = os.path.join(DASH, 'runs')              # Job-Logs/-Metadaten
REGISTRY = os.path.join(DASH, 'registry')       # Registry-JSONs (Quelle der Wahrheit)
WIKID = os.path.join(DASH, 'wiki')              # Wiki-v2-Inhalte (*.md)
MODELS_DIR = os.path.join(LAB, 'models')
RESULTS = os.path.join(BENCH, 'results.jsonl')
TASKS_DIR = os.path.join(BENCH, 'tasks')
TMPB = os.path.join(BENCH, 'aider', 'aider', 'tmp.benchmarks')
AIDER_ROOT = os.path.join(BENCH, 'aider')
LOGS = os.path.join(LAB, 'logs')
WEBAPP = os.path.join(BENCH, 'webapp')
RUNS_DOM = os.path.join(WEBAPP, 'runs-dom')
RUNS_UX = os.path.join(WEBAPP, 'runs-ux')
POLY_LABELS = os.path.join(DASH, 'polyglot-labels.json')
ANNOT = os.path.join(REGISTRY, 'run-annotations.json')
ENV_FILE = os.path.join(LAB, '.env')
SERVE_SH = os.path.join(LAB, 'serve.sh')

# Container-Images, die die GPU-Sperre auslösen (Benchmark-Harnesses).
LOCK_IMAGES = ('aider-bench', 'agent-bench')

LABEL_RE = re.compile(r'^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$')
MODELID_RE = re.compile(r'^[A-Za-z0-9][A-Za-z0-9._/-]{0,80}$')
FILE_RE = re.compile(r'^[A-Za-z0-9][A-Za-z0-9._-]{0,120}\.gguf$')


# ---------------------------------------------------------------- Helfer

def tail_lines(path, n=40):
    try:
        with open(path, 'rb') as f:
            f.seek(0, 2)
            size = f.tell()
            f.seek(max(0, size - 96 * 1024))
            data = f.read().decode('utf-8', 'replace')
        return data.splitlines()[-n:]
    except OSError:
        return []


def load_json(path, default):
    try:
        with open(path, encoding='utf-8') as f:
            return json.load(f)
    except (OSError, json.JSONDecodeError):
        return default


def save_json(path, obj):
    tmp = path + '.tmp'
    with open(tmp, 'w', encoding='utf-8') as f:
        json.dump(obj, f, ensure_ascii=False, indent=1)
    os.replace(tmp, path)


def fmt_dur(sec):
    sec = int(sec)
    if sec >= 3600:
        return '%d h %02d min' % (sec // 3600, sec % 3600 // 60)
    if sec >= 60:
        return '%d min' % (sec // 60)
    return '%d s' % sec


def port_listening(port):
    """Rein passiv über /proc/net/tcp prüfen — es wird KEINE Verbindung aufgebaut."""
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


class _Cache:
    def __init__(self):
        self._d = {}
        self._lock = threading.Lock()

    def get(self, key, ttl, fn):
        now = time.time()
        with self._lock:
            hit = self._d.get(key)
            if hit and now - hit[0] < ttl:
                return hit[1]
        val = fn()
        with self._lock:
            self._d[key] = (now, val)
        return val

    def clear(self, key=None):
        with self._lock:
            if key is None:
                self._d.clear()
            else:
                self._d.pop(key, None)


CACHE = _Cache()


def run_ro(argv, timeout=10):
    """Read-only-Kommando ausführen (podman ps/images, pacman -Q, …)."""
    try:
        p = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
        return p.returncode, p.stdout, p.stderr
    except (OSError, subprocess.TimeoutExpired) as e:
        return -1, '', str(e)


# ------------------------------------------------- Systemzustand (read-only)

def bench_containers():
    """Laufende Benchmark-Container (aider-bench / agent-bench*). Nur lesen."""
    def fetch():
        rc, out, _err = run_ro(['podman', 'ps', '--format', 'json'])
        if rc != 0:
            return {'error': 'podman ps fehlgeschlagen', 'containers': []}
        try:
            rows = json.loads(out or '[]')
        except json.JSONDecodeError:
            rows = []
        now = time.time()
        conts = []
        for c in rows:
            img = c.get('Image') or ''
            base = img.split('/')[-1].split(':')[0]
            if not any(base.startswith(x) for x in LOCK_IMAGES):
                continue
            names = c.get('Names') or []
            started = c.get('StartedAt') or 0
            conts.append({
                'name': names[0] if names else (c.get('Id') or '')[:12],
                'image': base,
                'started': started,
                'uptime': fmt_dur(max(0, now - started)) if started else '?',
            })
        conts.sort(key=lambda x: x['started'])
        return {'error': None, 'containers': conts}
    return CACHE.get('containers', 4, fetch)


def gpu_lock():
    """GPU-/Modellserver-Sperre: aktiv, solange irgendein Benchmark-Container läuft."""
    info = bench_containers()
    conts = info['containers']
    locked = bool(conts)
    reason = ''
    if locked:
        reason = ('Modellserver-Verwaltung gesperrt: %d Benchmark-Container aktiv (%s). '
                  'Die GPU und :8080 gehören den laufenden Läufen.'
                  % (len(conts), ', '.join('%s · %s' % (c['name'], c['uptime'])
                                           for c in conts)))
    elif info['error']:
        locked = True
        reason = ('Container-Status unbekannt (%s) — Verwaltung vorsichtshalber gesperrt.'
                  % info['error'])
    return {'locked': locked, 'reason': reason, 'containers': conts}


def llama_procs():
    """llama-server-Prozesse über /proc erkennen (Modell aus der Kommandozeile)."""
    def fetch():
        procs = []
        try:
            hz = os.sysconf('SC_CLK_TCK')
            up = float(open('/proc/uptime').read().split()[0])
        except (OSError, ValueError):
            hz, up = 100, 0.0
        for pid in os.listdir('/proc'):
            if not pid.isdigit():
                continue
            try:
                comm = open('/proc/%s/comm' % pid).read().strip()
                if comm != 'llama-server':
                    continue
                argv = open('/proc/%s/cmdline' % pid, 'rb').read().decode(
                    'utf-8', 'replace').split('\0')
                model, port, ctx, device, mmproj = '', 8080, None, '', ''
                for i, a in enumerate(argv):
                    nxt = argv[i + 1] if i + 1 < len(argv) else ''
                    if a == '-m':
                        model = os.path.basename(nxt)
                    elif a == '--mmproj':
                        mmproj = os.path.basename(nxt)
                    elif a == '--port' and nxt.isdigit():
                        port = int(nxt)
                    elif a == '-c' and nxt.isdigit():
                        ctx = int(nxt)
                    elif a == '--device':
                        device = nxt
                uptime = None
                try:
                    stat = open('/proc/%s/stat' % pid).read()
                    start_ticks = float(stat.rsplit(')', 1)[1].split()[19])
                    uptime = max(0, up - start_ticks / hz)
                except (OSError, ValueError, IndexError):
                    pass
                procs.append({'pid': int(pid), 'model_file': model, 'mmproj': mmproj,
                              'port': port, 'ctx': ctx, 'device': device,
                              'uptime': fmt_dur(uptime) if uptime is not None else '?',
                              'uptime_s': uptime})
            except OSError:
                continue
        return procs
    return CACHE.get('llama', 4, fetch)


def env_keys():
    """API-Key-NAMEN aus ~/.ai-lab/.env — nur ob gesetzt, nie den Wert."""
    def fetch():
        vals = {}
        try:
            for line in open(ENV_FILE, encoding='utf-8'):
                line = line.strip()
                if not line or line.startswith('#') or '=' not in line:
                    continue
                k, _, v = line.partition('=')
                vals[k.strip()] = v.strip().strip('"\'')
        except OSError:
            pass
        return vals
    return CACHE.get('envkeys', 30, fetch)


def env_key_present(name):
    v = env_keys().get(name) or os.environ.get(name) or ''
    return bool(v)


def job_env_with_keys(extra=None):
    """Umgebung für Jobs: os.environ + .env-Keys + extra."""
    env = dict(os.environ)
    for k, v in env_keys().items():
        if v and k not in env:
            env[k] = v
    if extra:
        env.update(extra)
    return env
