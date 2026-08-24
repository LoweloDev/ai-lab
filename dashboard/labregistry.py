"""Registry: models.json / harnesses.json / backends.json unter dashboard/registry/.

Die Registry ist die einzige Quelle der Wahrheit für Komponenten. Status wird
zur Laufzeit berechnet (Datei vorhanden? Image vorhanden? Paket installiert?).
Installationen von Backends sind GUIDED ONLY (Kommando anzeigen, nie ausführen).
"""
import os
import json
import shutil

from labcore import (REGISTRY, MODELS_DIR, CACHE, run_ro, load_json, save_json,
                     llama_procs, gpu_lock, MODELID_RE, FILE_RE)

MODELS_JSON = os.path.join(REGISTRY, 'models.json')
HARNESS_JSON = os.path.join(REGISTRY, 'harnesses.json')
BACKENDS_JSON = os.path.join(REGISTRY, 'backends.json')


# ------------------------------------------------------------------ Laden

def load_models():
    return load_json(MODELS_JSON, {'models': [], 'catalog': []})


def save_models(d):
    save_json(MODELS_JSON, d)


def all_model_entries(d=None):
    d = d or load_models()
    return list(d.get('models', [])) + list(d.get('catalog', []))


def find_model(mid, d=None):
    for e in all_model_entries(d):
        if e.get('id') == mid:
            return e
    return None


def _file_status(entry):
    path = os.path.join(MODELS_DIR, entry.get('file') or '')
    size = None
    exists = bool(entry.get('file')) and os.path.isfile(path)
    if exists:
        try:
            size = os.path.getsize(path)
        except OSError:
            pass
    mm, mm_exists, mm_size = entry.get('mmproj'), None, None
    if mm:
        mp = os.path.join(MODELS_DIR, mm)
        mm_exists = os.path.isfile(mp)
        if mm_exists:
            try:
                mm_size = os.path.getsize(mp)
            except OSError:
                pass
    if exists and (mm is None or mm_exists):
        status = 'installiert'
    elif exists:
        status = 'teilweise'      # Basisdatei da, mmproj fehlt
    else:
        status = 'fehlt'
    return {'status': status, 'size': size, 'mmproj_exists': mm_exists,
            'mmproj_size': mm_size}


def models_view(active_job_model_ids=frozenset()):
    """Registry-Modelle + berechneter Status + Nutzungs-Info."""
    d = load_models()
    procs = llama_procs()
    served = {p['model_file'] for p in procs if p['model_file']}
    served |= {p['mmproj'] for p in procs if p['mmproj']}
    lock = gpu_lock()

    def enrich(e):
        st = _file_status(e)
        # Vision-Varianten teilen die Basis-GGUF: "Server läuft" gilt für sie nur,
        # wenn derselbe Prozess auch ihren mmproj geladen hat.
        in_server = any(
            p['model_file'] == e.get('file')
            and (e.get('kind') != 'vision'
                 or p.get('mmproj', '') == e.get('mmproj', ''))
            for p in procs)
        in_job = e.get('id') in active_job_model_ids
        out = {**e, **st,
               'served': in_server,
               'in_use': in_server or in_job,
               'use_reason': ('llama-server bedient diese Datei gerade' if in_server
                              else 'ein aktiver Job nutzt dieses Modell' if in_job
                              else '')}
        return out

    known_files = set()
    for e in all_model_entries(d):
        if e.get('file'):
            known_files.add(e['file'])
        if e.get('mmproj'):
            known_files.add(e['mmproj'])
    stray = []
    try:
        for fn in sorted(os.listdir(MODELS_DIR)):
            if fn.endswith('.gguf') and fn not in known_files:
                try:
                    sz = os.path.getsize(os.path.join(MODELS_DIR, fn))
                except OSError:
                    sz = None
                stray.append({'file': fn, 'size': sz})
    except OSError:
        pass
    return {'models': [enrich(e) for e in d.get('models', [])],
            'catalog': [enrich(e) for e in d.get('catalog', [])],
            'unregistered': stray,
            'lock': lock}


# --------------------------------------------------------- Modelle ändern

def _valid_entry(p, for_update=False):
    mid = (p.get('id') or '').strip()
    if not MODELID_RE.match(mid) or '/' in mid:
        raise ValueError('Ungültige Modell-ID (erlaubt: a-z 0-9 . _ -).')
    e = {'id': mid,
         'name': (p.get('name') or mid).strip()[:80],
         'kind': p.get('kind') if p.get('kind') in ('base', 'vision') else 'base',
         'file': (p.get('file') or '').strip(),
         'mmproj': (p.get('mmproj') or '').strip() or None,
         'hf_repo': (p.get('hf_repo') or '').strip() or None,
         'hf_file': (p.get('hf_file') or '').strip() or None,
         'url': (p.get('url') or '').strip() or None,
         'args': p.get('args') if isinstance(p.get('args'), dict) else {},
         'notes': (p.get('notes') or '').strip()[:400],
         'vram': (p.get('vram') or '').strip()[:200]}
    if not e['file'] and e['hf_file']:
        e['file'] = e['hf_file']
    if not e['file'] and e['url']:
        e['file'] = e['url'].rsplit('/', 1)[-1].split('?')[0]
    if not FILE_RE.match(e['file'] or ''):
        raise ValueError('Dateiname fehlt oder ungültig — muss auf .gguf enden, '
                         'ohne Pfadanteile.')
    if e['mmproj'] and not FILE_RE.match(e['mmproj']):
        raise ValueError('mmproj-Dateiname ungültig (muss auf .gguf enden).')
    if e['url']:
        if not e['url'].startswith('https://'):
            raise ValueError('Direkte URL muss mit https:// beginnen.')
    if e['hf_repo'] and e['hf_repo'].count('/') != 1:
        raise ValueError('HF-Repo bitte als "owner/repo" angeben.')
    if not e['url'] and not (e['hf_repo'] and e['hf_file']):
        # Für reine Bestandseinträge ok — aber Download braucht Quelle.
        e['no_source'] = True
    args = {}
    for k in ('ctx', 'ngl', 'ncmoe', 'batch', 'ubatch'):
        v = e['args'].get(k)
        if v in (None, ''):
            continue
        try:
            args[k] = int(v)
        except (TypeError, ValueError):
            raise ValueError('Argument %s muss eine Zahl sein.' % k)
    for k in ('kv', 'extra'):
        v = (e['args'].get(k) or '').strip()
        if v:
            args[k] = v[:120]
    e['args'] = args
    return e


def upsert_model(p):
    d = load_models()
    e = _valid_entry(p)
    for arr in ('models', 'catalog'):
        for i, old in enumerate(d.get(arr, [])):
            if old.get('id') == e['id']:
                keep = {k: old[k] for k in ('seed',) if k in old}
                d[arr][i] = {**keep, **e}
                save_models(d)
                return {'ok': True, 'updated': True, 'id': e['id']}
    d.setdefault('catalog', []).append(e)
    save_models(d)
    return {'ok': True, 'updated': False, 'id': e['id']}


def download_url(entry):
    if entry.get('url'):
        return entry['url']
    if entry.get('hf_repo') and entry.get('hf_file'):
        return 'https://huggingface.co/%s/resolve/main/%s' % (
            entry['hf_repo'], entry['hf_file'])
    return None


def remove_model(p, active_job_model_ids=frozenset()):
    """GGUF löschen — nur Registry-Einträge, nur mit getipptem Namen, nie in Benutzung."""
    mid = (p.get('id') or '').strip()
    confirm = (p.get('confirm') or '').strip()
    entry_only = bool(p.get('entry_only'))
    d = load_models()
    entry = find_model(mid, d)
    if not entry:
        raise ValueError('Unbekannte Modell-ID: %s' % mid)
    if confirm != mid:
        raise ValueError('Bestätigung fehlt: bitte exakt „%s“ eintippen.' % mid)

    if not entry_only:
        served = set()
        for pr in llama_procs():
            served.add(pr['model_file'])
            served.add(pr['mmproj'])
        if entry.get('file') in served or (entry.get('mmproj') or '') in served:
            raise ValueError('Gesperrt: llama-server bedient diese Datei gerade. '
                             'Das Dashboard stoppt niemals Modellserver.')
        if mid in active_job_model_ids:
            raise ValueError('Gesperrt: ein aktiver Job nutzt dieses Modell.')
        lock = gpu_lock()
        if lock['locked'] and entry.get('file') in served:
            raise ValueError(lock['reason'])
        # Datei(en) löschen — mmproj nur, wenn kein anderer Eintrag sie referenziert.
        deleted = []
        others = [e for e in all_model_entries(d) if e.get('id') != mid]
        f = entry.get('file')
        if f and not any(o.get('file') == f or o.get('mmproj') == f for o in others):
            fp = os.path.join(MODELS_DIR, f)
            if os.path.isfile(fp):
                os.remove(fp)
                deleted.append(f)
        mm = entry.get('mmproj')
        if (mm and p.get('with_mmproj')
                and not any(o.get('file') == mm or o.get('mmproj') == mm for o in others)):
            mp = os.path.join(MODELS_DIR, mm)
            if os.path.isfile(mp):
                os.remove(mp)
                deleted.append(mm)
        msg = ('Gelöscht: %s' % ', '.join(deleted)) if deleted else \
            'Keine Datei gelöscht (nicht vorhanden oder von anderem Eintrag genutzt).'
    else:
        msg = 'Nur der Registry-Eintrag wurde entfernt (Dateien unangetastet).'

    if entry_only or not entry.get('seed'):
        for arr in ('models', 'catalog'):
            d[arr] = [e for e in d.get(arr, []) if e.get('id') != mid]
        save_models(d)
    return {'ok': True, 'message': msg}


# ------------------------------------------------------------- Harnesses

def load_harnesses():
    return load_json(HARNESS_JSON, {'harnesses': []})


def harnesses_view():
    d = load_harnesses()

    def podman_images():
        rc, out, _ = run_ro(['podman', 'images', '--format',
                             '{{.Repository}} {{.Tag}} {{.Created}}'], timeout=10)
        imgs = {}
        if rc == 0:
            for line in out.splitlines():
                parts = line.split(None, 2)
                if parts:
                    repo = parts[0].split('/')[-1]
                    imgs[repo] = {'tag': parts[1] if len(parts) > 1 else '',
                                  'created': parts[2] if len(parts) > 2 else ''}
        return imgs
    imgs = CACHE.get('podman-images', 20, podman_images)

    out = []
    for h in d.get('harnesses', []):
        e = dict(h)
        if h.get('type') == 'pacman':
            path = shutil.which(h.get('binary') or h.get('id') or '')
            e['installed'] = bool(path)
            e['detail'] = path or 'Binary nicht im PATH'
        elif h.get('type') == 'podman-image':
            img = imgs.get(h.get('image') or '')
            e['installed'] = bool(img)
            e['detail'] = ('Image %s:%s · %s' % (h.get('image'), img['tag'], img['created'])
                           if img else 'Podman-Image fehlt')
        else:
            e['installed'] = None
            e['detail'] = 'unbekannter Typ'
        out.append(e)
    return {'harnesses': out}


def save_harnesses(p):
    hs = p.get('harnesses')
    if not isinstance(hs, list):
        raise ValueError('Erwartet: {"harnesses": [...]}')
    clean = []
    for h in hs:
        if not isinstance(h, dict) or not h.get('id'):
            raise ValueError('Jeder Harness braucht eine id.')
        clean.append({k: h.get(k) for k in
                      ('id', 'name', 'type', 'binary', 'package', 'image',
                       'version_pinned', 'install', 'notes') if h.get(k) is not None})
    save_json(HARNESS_JSON, {'harnesses': clean})
    return {'ok': True}


# -------------------------------------------------------------- Backends

def load_backends():
    return load_json(BACKENDS_JSON, {'backends': []})


def backends_view():
    d = load_backends()

    def pacman_q():
        pkgs = {}
        names = [b.get('package') for b in d.get('backends', []) if b.get('package')]
        if names:
            rc, out, _ = run_ro(['pacman', '-Q'] + names, timeout=10)
            for line in out.splitlines():
                parts = line.split()
                if len(parts) >= 2:
                    pkgs[parts[0]] = parts[1]
        return pkgs
    pkgs = CACHE.get('pacman-q', 60, pacman_q)

    out = []
    for b in d.get('backends', []):
        e = dict(b)
        ver = pkgs.get(b.get('package') or '')
        e['installed'] = bool(ver)
        e['version'] = ver or None
        e['install_cmd'] = 'sudo pacman -S --needed %s' % b.get('package', '')
        out.append(e)
    return {'backends': out,
            'hinweis': 'Installation ist bewusst manuell: Kommando kopieren und '
                       'selbst im Terminal ausführen (sudo). Das Dashboard führt '
                       'keine Paketinstallationen aus.'}


def save_backends(p):
    bs = p.get('backends')
    if not isinstance(bs, list):
        raise ValueError('Erwartet: {"backends": [...]}')
    clean = []
    for b in bs:
        if not isinstance(b, dict) or not b.get('id') or not b.get('package'):
            raise ValueError('Jedes Backend braucht id und package.')
        clean.append({k: b.get(k) for k in
                      ('id', 'name', 'package', 'ggml_backend', 'notes')
                      if b.get(k) is not None})
    save_json(BACKENDS_JSON, {'backends': clean})
    return {'ok': True}


# ------------------------------------------------------------------ Seed

def ensure_seed():
    os.makedirs(REGISTRY, exist_ok=True)
    if not os.path.isfile(MODELS_JSON):
        save_models(SEED_MODELS)
    if not os.path.isfile(HARNESS_JSON):
        save_json(HARNESS_JSON, SEED_HARNESSES)
    if not os.path.isfile(BACKENDS_JSON):
        save_json(BACKENDS_JSON, SEED_BACKENDS)
    if not os.path.isfile(os.path.join(REGISTRY, 'run-annotations.json')):
        save_json(os.path.join(REGISTRY, 'run-annotations.json'), SEED_ANNOT)


SEED_MODELS = {
    '_hinweis': ('Quelle der Wahrheit für Modelle. "models" = die kuratierten '
                 'Lab-Modelle (Stand aus serve.sh, 24.08.2026), "catalog" = '
                 'verfügbare, noch nicht installierte Kandidaten. Status '
                 '(installiert/fehlt) wird zur Laufzeit aus models/ berechnet.'),
    'models': [
        {'id': 'qwen38', 'seed': True, 'kind': 'base',
         'name': 'Qwen3.8-27B',
         'file': 'Qwen3.8-27B-UD-IQ4_XS.gguf',
         'hf_repo': 'unsloth/Qwen3.8-27B-GGUF', 'hf_file': 'Qwen3.8-27B-UD-IQ4_XS.gguf',
         'args': {'ctx': 32768, 'kv': 'q8_0', 'ngl': 99},
         'notes': 'Qualitäts-Pick fürs Agentic Coding (dichtes Modell).',
         'vram': 'komplett in VRAM (~15–16 GB inkl. KV-Cache)'},
        {'id': 'qwen38-vision', 'seed': True, 'kind': 'vision',
         'name': 'Qwen3.8-27B · Vision',
         'file': 'Qwen3.8-27B-UD-IQ4_XS.gguf', 'mmproj': 'mmproj-F16.gguf',
         'hf_repo': 'unsloth/Qwen3.8-27B-GGUF', 'hf_file': 'mmproj-F16.gguf',
         'args': {'ctx': 16384, 'kv': 'q8_0', 'ngl': 99},
         'notes': 'GUI-/Vision-Aufgaben (Screenshots, Grounding). Teilt die '
                  'Basisdatei mit qwen38.',
         'vram': 'komplett in VRAM (+0,9 GiB mmproj)'},
        {'id': 'qwen36moe', 'seed': True, 'kind': 'base',
         'name': 'Qwen3.6-35B-A3B',
         'file': 'Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf',
         'hf_repo': 'unsloth/Qwen3.6-35B-A3B-GGUF',
         'hf_file': 'Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf',
         'args': {'ctx': 32768, 'kv': 'q8_0', 'ngl': 99},
         'notes': 'Speed-Pick: MoE, 3B aktiv, ~60–70 tok/s.',
         'vram': 'komplett in VRAM'},
        {'id': 'muse', 'seed': True, 'kind': 'base',
         'name': 'Muse-Glimmer-30B',
         'file': 'Muse-Glimmer-30B-UD-Q4_K_XL.gguf',
         'hf_repo': 'unsloth/Muse-Glimmer-30B-GGUF',
         'hf_file': 'Muse-Glimmer-30B-UD-Q4_K_XL.gguf',
         'args': {'ctx': 32768, 'kv': 'q8_0', 'ngl': 99},
         'notes': 'Allrounder — 5/5 in der Repo-Suite. Achtung: HF-Repo-Angabe aus '
                  'dem Namensmuster abgeleitet (Download lief außerhalb von '
                  'download-models.sh) — vor erneutem Download prüfen.',
         'vram': 'komplett in VRAM (~15 GiB)'},
        {'id': 'muse-vision', 'seed': True, 'kind': 'vision',
         'name': 'Muse-Glimmer-30B · Vision',
         'file': 'Muse-Glimmer-30B-UD-Q4_K_XL.gguf',
         'mmproj': 'mmproj-Muse-Glimmer-30B-Q8_0.gguf',
         'hf_repo': 'unsloth/Muse-Glimmer-30B-GGUF',
         'hf_file': 'mmproj-Muse-Glimmer-30B-Q8_0.gguf',
         'args': {'ctx': 16384, 'kv': 'q8_0', 'ngl': 99},
         'notes': 'Vision-Variante von Muse. Bei >4 Screenshots ctx auf 24576 '
                  'erhöhen (siehe TODO).',
         'vram': 'komplett in VRAM (+2 GiB mmproj)'},
        {'id': 'codernext', 'seed': True, 'kind': 'base',
         'name': 'Qwen3-Coder-Next 80B-A3B',
         'file': 'Qwen3-Coder-Next-UD-Q3_K_XL.gguf',
         'hf_repo': 'unsloth/Qwen3-Coder-Next-GGUF',
         'hf_file': 'Qwen3-Coder-Next-UD-Q3_K_XL.gguf',
         'args': {'ctx': 32768, 'kv': 'q8_0', 'ngl': 99, 'ncmoe': 26,
                  'batch': 2048, 'ubatch': 2048},
         'notes': 'Größtes Coding-Modell (80B-A3B MoE). Attention + shared experts '
                  'auf der GPU, routed experts per mmap im RAM (--n-cpu-moe 26).',
         'vram': 'VRAM + RAM — bleibt unter ~19 GB VRAM'},
    ],
    'catalog': [
        {'id': 'qwen36moe-q4', 'seed': True, 'kind': 'base',
         'name': 'Qwen3.6-35B-A3B (UD-Q4_K_XL)',
         'file': 'Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf',
         'hf_repo': 'unsloth/Qwen3.6-35B-A3B-GGUF',
         'hf_file': 'Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf',
         'args': {'ctx': 32768, 'kv': 'q8_0', 'ngl': 99},
         'notes': 'Höherwertige Quantisierung des Speed-Picks (~19–20 GiB). '
                  'Knapp für 20-GB-VRAM: ggf. Kontext reduzieren.',
         'vram': 'knapp — KV q8_0 Pflicht, evtl. -c 16384'},
        {'id': 'qwen38-q5', 'seed': True, 'kind': 'base',
         'name': 'Qwen3.8-27B (UD-Q5_K_XL)',
         'file': 'Qwen3.8-27B-UD-Q5_K_XL.gguf',
         'hf_repo': 'unsloth/Qwen3.8-27B-GGUF',
         'hf_file': 'Qwen3.8-27B-UD-Q5_K_XL.gguf',
         'args': {'ctx': 16384, 'kv': 'q8_0', 'ngl': 99},
         'notes': 'Höherwertige Quantisierung des Qualitäts-Picks (~18–19 GiB). '
                  'Passt nur mit reduziertem Kontext ganz in den VRAM.',
         'vram': 'knapp — Kontext auf 16k begrenzen'},
        {'id': 'codernext-q4', 'seed': True, 'kind': 'base',
         'name': 'Qwen3-Coder-Next 80B (UD-Q4_K_XL)',
         'file': 'Qwen3-Coder-Next-UD-Q4_K_XL.gguf',
         'hf_repo': 'unsloth/Qwen3-Coder-Next-GGUF',
         'hf_file': 'Qwen3-Coder-Next-UD-Q4_K_XL.gguf',
         'args': {'ctx': 32768, 'kv': 'q8_0', 'ngl': 99, 'ncmoe': 32,
                  'batch': 2048, 'ubatch': 2048},
         'notes': 'Größere Quantisierung von Coder-Next (~45 GiB): --n-cpu-moe '
                  'erhöhen, braucht deutlich mehr RAM.',
         'vram': 'VRAM + viel RAM (mmap)'},
    ],
}

SEED_HARNESSES = {
    '_hinweis': ('Harnesses, mit denen Benchmarks laufen. Erkennung zur Laufzeit '
                 '(command -v bzw. podman images). (Neu-)Installation ist manuell — '
                 'das angezeigte Kommando kopieren.'),
    'harnesses': [
        {'id': 'opencode', 'name': 'OpenCode CLI', 'type': 'pacman',
         'binary': 'opencode', 'package': 'opencode',
         'install': 'sudo pacman -S --needed opencode',
         'notes': 'Läuft auf dem Host und im agent-bench-Image (Suite-Tasks).'},
        {'id': 'agent-bench', 'name': 'agent-bench (Basis-Image)',
         'type': 'podman-image', 'image': 'agent-bench',
         'install': 'cd ~/ai-lab/bench && podman build -f Containerfile -t agent-bench .',
         'notes': 'Sandbox-Image für Suite-Läufe (Arch + go/node/opencode). '
                  'Basis für agent-bench-dsh.'},
        {'id': 'aider-bench', 'name': 'aider-bench (Polyglot)',
         'type': 'podman-image', 'image': 'aider-bench',
         'install': 'cd ~/ai-lab/bench/aider/aider && '
                    'podman build -f ../Dockerfile.podman -t aider-bench .',
         'notes': 'Aider-Polyglot-Benchmark (python+go-Subset, 73 Übungen).'},
        {'id': 'dsh', 'name': 'DeepSeek Harness (dsh)',
         'type': 'podman-image', 'image': 'agent-bench-dsh',
         'version_pinned': '0.1.1-rc.2',
         'install': 'cd ~/ai-lab/bench && '
                    'podman build -f Containerfile.dsh -t agent-bench-dsh .',
         'notes': 'Developer Preview, Version im Image gepinnt '
                  '(@deepseek-ai/dsh@0.1.1-rc.2). Baut auf agent-bench auf.'},
    ],
}

SEED_BACKENDS = {
    '_hinweis': ('GGML-Rechen-Backends für llama.cpp (Arch-Pakete). Erkennung via '
                 'pacman -Q. Installation NUR manuell (sudo) — Kommando kopieren.'),
    'backends': [
        {'id': 'vulkan', 'name': 'Vulkan', 'package': 'ggml-vulkan',
         'ggml_backend': 'ggml-vulkan',
         'notes': 'Standard-Backend des Labs (serve.sh: --device Vulkan0).'},
        {'id': 'rocm', 'name': 'ROCm / HIP', 'package': 'ggml-hip',
         'ggml_backend': 'ggml-hip',
         'notes': 'AMD-HIP-Backend (serve.sh: --device ROCm0). Bei pp meist '
                  'schneller, bei tg je nach Modell.'},
        {'id': 'cpu', 'name': 'CPU', 'package': 'ggml-cpu',
         'ggml_backend': 'ggml-cpu',
         'notes': 'Fallback ohne GPU; auch Grundlage des MoE-CPU-Offloads.'},
    ],
}

# Manuelle Overlay-Daten für Polyglot-Läufe (Rohdaten bleiben unangetastet).
SEED_ANNOT = {
    '_hinweis': ('Overlay für Polyglot-Läufe: hidden/label/note pro Run-Ordner. '
                 'Die Rohdaten unter tmp.benchmarks werden nie verändert.'),
    '2026-08-23-18-38-50--dryrun-local': {
        'label': 'Dry-Run', 'hidden': True,
        'note': 'Technischer Probelauf (1 Übung) — kein Benchmark.'},
    '2026-08-24-13-09-40--local-qwen-py-go-20260824-150938': {
        'note': 'Abgebrochener Muse-Start (2/73) — ersetzt durch den Lauf von 13:50.'},
}
