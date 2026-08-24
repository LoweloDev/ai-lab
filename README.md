# ai-lab — Lokales Agentic-Coding-Labor

Ein komplettes lokales Setup, um Coding-Agenten mit lokalen LLMs zu betreiben **und**
objektiv zu vermessen: Modell-Server, Agent-Harness, sandboxte Benchmark-Suite,
Polyglot-/UX-/App-Steuerungs-Tests und ein Dashboard. Alles läuft offline auf einer
einzelnen Consumer-GPU; Cloud-APIs (DeepSeek, Gemini) docken über dieselbe
Infrastruktur nur als Vergleichswerte an.

**Hardware/Stack:** Ryzen 7 7800X3D · 32 GB DDR5 · **RX 7900 XT 20 GB (gfx1100)** ·
NVMe · CachyOS. Inference: **llama.cpp** aus pacman (`llama-cpp` + `ggml-vulkan` +
`ggml-hip` + `ggml-cpu`) — Backend pro Lauf wählbar (Vulkan oder ROCm), spricht
OpenAI- **und** Anthropic-API. Agent-Harness: **OpenCode** (pacman). Benchmarks laufen
in einer **podman**-Sandbox (rootless, pasta-Netz).

---

## Schnellstart

### 1. Modell-Server starten

```bash
~/ai-lab/serve.sh <modell> [vulkan|rocm] [extra llama-server-Args...]
# Beispiel:
~/ai-lab/serve.sh qwen38 vulkan
```

Der Server lauscht auf `http://127.0.0.1:8080`, API-Key `sk-local`.
Extra-Argumente nach dem Backend gehen direkt an `llama-server` durch
(z. B. `~/ai-lab/serve.sh qwen38 vulkan -c 65536`).

| `serve.sh`-Name | Datei (`models/`) | GGUF-Größe | Kontext | VRAM | Zweck |
|---|---|---|---|---|---|
| `qwen38` | `Qwen3.8-27B-UD-IQ4_XS.gguf` | 13,3 GiB | 32k | komplett in VRAM (~15–16 GB inkl. KV) | Qualitäts-Pick fürs Agentic Coding (dichtes Modell) |
| `qwen38-vision` | dito + `mmproj-F16.gguf` | +0,9 GiB | 16k | komplett in VRAM | GUI-/Vision-Aufgaben (Screenshots, Grounding) |
| `qwen36moe` | `Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf` | 15,7 GiB | 32k | komplett in VRAM | Speed-Pick (MoE, 3B aktiv, ~60–70 tok/s) |
| `muse` | `Muse-Glimmer-30B-UD-Q4_K_XL.gguf` | 15 GiB | 32k | komplett in VRAM | Allrounder — 5/5 in der Repo-Suite |
| `muse-vision` | dito + `mmproj-Muse-Glimmer-30B-Q8_0.gguf` | +2 GiB | 16k | komplett in VRAM | Vision-Variante von Muse |
| `codernext` | `Qwen3-Coder-Next-UD-Q3_K_XL.gguf` | 33,8 GiB | 32k | VRAM + RAM (`--n-cpu-moe 26`, bleibt unter ~19 GB VRAM) | Größtes Coding-Modell (80B-A3B MoE); Experten liegen per mmap im RAM |

### 2. Damit arbeiten

```bash
# OpenCode (Provider "llamacpp" → http://127.0.0.1:8080/v1):
opencode
# Benchmark-Config liegt in bench/opencode-config/, die Alltags-Config in ~/.config/opencode/

# Claude Code direkt gegen das lokale Modell (llama-server spricht die Anthropic-API):
ANTHROPIC_BASE_URL=http://127.0.0.1:8080 ANTHROPIC_API_KEY=sk-local claude
```

### 3. Die wichtigen Flags (stecken alle in `serve.sh`)

- `--jinja` — **Pflicht.** Aktiviert das Chat-Template inkl. Tool-Call-Grammatik;
  ohne läuft z. B. Qwen3.8 über seine Stop-Tokens hinaus.
- `--cache-reuse 256` — Prompt-Präfix-Wiederverwendung zwischen Agent-Turns.
  Der wichtigste einzelne Latenz-Hebel für Agenten-Workloads.
- `-ctk q8_0 -ctv q8_0` — KV-Cache auf 8 Bit quantisieren: halber VRAM-Bedarf
  für den Kontext, vernachlässigbarer Qualitätsverlust.
- `-c <n>` — Kontextfenster. 32k Default; Vision-Varianten fahren 16k (mmproj
  kostet VRAM). Mehr Kontext = mehr KV-VRAM — bei Bedarf pro Lauf überschreiben.
- `--n-cpu-moe <n>` — nur `codernext`: die n Routed-Expert-Schichten bleiben im
  RAM (mmap), Attention + Shared Experts auf der GPU. So läuft ein 34-GiB-GGUF
  auf einer 20-GB-Karte.
- `-t 8` — physische Kerne, nicht SMT-Threads. `-fa on`, `--metrics` sind gesetzt.

---

## Dashboard

```bash
~/ai-lab/dashboard/start.sh        # → http://127.0.0.1:8100
```

Reine Python-Stdlib (`dashboard/server.py`), liest alle Ergebnisse **strikt
read-only** und aktualisiert den aktiven Tab alle 10 s:

| Tab | Zeigt |
|---|---|
| **Übersicht** | Hero-Karten je Modell (Suite-Score, Polyglot, Perf-Kurzwerte) |
| **Suite** | Matrix Modell × Task aus `bench/results.jsonl` (letzte Zeile je Modell+Task zählt) |
| **Polyglot** | Aider-Läufe aus `bench/aider/aider/tmp.benchmarks/`, Zuordnung Lauf→Modell über `dashboard/polyglot-labels.json` |
| **Perf** | llama-bench-Matrizen aus `logs/perf-*.json` (Backend × Kontexttiefe) |
| **Apps & UX** | DOM-Agent-Ergebnisse (`bench/webapp/runs-dom/`) und UX-Findings (`runs-ux/`) |
| **Läufe** | Neue Benchmarks direkt aus dem Browser starten — Allowlist: `run-task.sh`, `run-task-api.sh`, `dom-agent.py`, `ux-dom-test.py`; Logs unter `dashboard/runs/` |
| **Wiki** | Alle Lab-Dokumente inkl. [Guide für neue Benchmarks](dashboard/GUIDE-neue-benchmarks.md) |

Das Dashboard schreibt ausschließlich unter `dashboard/runs/` und startet Läufe nur
über die Allowlist bestehender Skripte — nie beliebige Kommandos.

---

## Von-Null-Reproduktion

1. **Pakete** (CachyOS/Arch):
   ```bash
   sudo pacman -S --needed llama-cpp ggml-vulkan ggml-hip ggml-cpu \
                           opencode podman passt aria2 jq git python
   ```
2. **Modelle laden** (~80 GB):
   ```bash
   ~/ai-lab/download-models.sh     # die drei Qwen-GGUFs + mmproj (unsloth, via aria2c)
   ```
   Muse-Glimmer ist dort nicht enthalten — `Muse-Glimmer-30B-UD-Q4_K_XL.gguf` und
   `mmproj-Muse-Glimmer-30B-Q8_0.gguf` analog nach `models/` laden (gleiches
   aria2c-Muster bzw. `huggingface-cli download`).
3. **Sandbox-Images bauen**:
   ```bash
   cd ~/ai-lab/bench
   podman build -t agent-bench -f Containerfile .        # Arch + go/node/git/ripgrep/opencode
   podman build -t agent-bench-dsh -f Containerfile.dsh . # + DeepSeek Harness (dsh, Version gepinnt)
   ```
4. **Benchmark-Workspaces bauen** — braucht die **zwei Quell-Repos** lokal unter
   `~/Projects/agora-debate` und `~/Projects/ai-ux-framework`:
   ```bash
   ~/ai-lab/bench/prepare-workspaces.sh
   ```
   Erzeugt pro Task einen frischen Klon ohne Remotes/Hooks (inkl. geplanteter Bugs);
   die Originale werden nie berührt. **Achtung:** löscht `bench/workspaces/` komplett
   und baut alle fünf neu.
5. Optional: Polyglot-Setup (siehe unten) und `.env` für API-Vergleiche.

---

## Benchmark-Suite (`bench/`)

### Das Task-Format = Plugin-Format

Ein Suite-Task besteht aus genau zwei Dateien plus einem vorbereiteten Workspace —
das ist zugleich die Schnittstelle, über die neue Benchmarks angedockt werden:

```
bench/tasks/<name>/prompt.txt    # die Aufgabe, die der Agent bekommt
bench/tasks/<name>/grade.sh      # objektives Rot/Grün-Grading
bench/workspaces/<name>/         # Git-Klon, auf dem der Agent arbeitet (nicht im Repo)
```

**Kontrakt von `grade.sh`:** Aufruf `grade.sh <workspace-pfad>`; nur die **letzte
stdout-Zeile** zählt — beginnt sie mit `PASS`, ist der Lauf grün (auch
`PASS pass=52 fail=0`). Grader müssen Manipulationsschutz enthalten (prüfen, dass
Tests unverändert sind). Details und Vorlagen: [dashboard/GUIDE-neue-benchmarks.md](dashboard/GUIDE-neue-benchmarks.md).

Die fünf bestehenden Tasks: `agora-A1-gate` (geplanteter Bug), `agora-A2-jsonld`
(Security-Revert), `agora-A3-hls` (Feature), `aiux-U1-paging` (geplanteter Bug),
`aiux-U2-denytools` (Feature).

### Runner

```bash
# Lokal (Modell = was llama-server auf :8080 gerade serviert):
~/ai-lab/bench/run-task.sh <label> <task> [timeout=1200]

# API-Modelle via OpenCode (Keys als Umgebungsvariablen):
OC_CONFIG=opencode-config-api LABEL=oc-v4-flash \
  ~/ai-lab/bench/run-task-api.sh deepseek/deepseek-v4-flash <task>

# DeepSeek-eigener Harness (dsh, Image agent-bench-dsh):
DSH_MODEL=deepseek-v4-pro ~/ai-lab/bench/run-task-dsh.sh dsh-v4-pro <task>
```

Jeder Runner kopiert den Workspace nach `bench/runs/<label>/<task>/ws`, startet den
Agenten im Container (sieht nur `/work` + RO-Go-Modulcache), gradet danach
host-seitig und hängt eine JSON-Zeile an **`bench/results.jsonl`**:

```json
{"model":"qwen38-vulkan","task":"agora-A1-gate","grade":"PASS","seconds":78.5,"exit":0,"changed":" 1 file changed, ..."}
```

Kompakte Sicht (`exit:124` = Timeout; beim Auswerten zählt die letzte Zeile je Modell+Task):

```bash
jq -r '[.model,.task,.grade,(.seconds|tostring)] | @tsv' ~/ai-lab/bench/results.jsonl | column -t
```

Transkripte liegen unter `bench/runs/<label>/<task>/` (OpenCode: `transcript.jsonl`,
dsh: `transcript.log` + `dsh-home/sessions/`); `bench/analyze-transcript.py` zieht
Metriken aus OpenCode-Transkripten. Warum einzelne Läufe scheiterten:
[bench/failure-analysis.md](bench/failure-analysis.md).

### Perf-Messung

`~/ai-lab/perf-bench.sh [modell]` fährt die volle llama-bench-Matrix
(Backend × Kontexttiefe 0/10k/32k), `perf-bench2.sh` die getrimmte Variante.
Ergebnis: `logs/perf-*.json` → Perf-Tab. **Nie parallel zum laufenden Server** —
gleiche GPU.

---

## Polyglot (Aider-Benchmark)

```bash
~/ai-lab/serve.sh <modell> vulkan          # Server zuerst!
~/ai-lab/bench/aider/run-polyglot-subset.sh
```

Fährt den python+go-Subset (73 Übungen) im Container `aider-bench` gegen den lokalen
Server. Headline-Zahl: **`pass_rate_2`**; Ergebnisse unter
`bench/aider/aider/tmp.benchmarks/<zeitstempel>--<run-name>/`. Neue Läufe fürs
Dashboard in `dashboard/polyglot-labels.json` einem Modell zuordnen.

**Hinweis:** Das Verzeichnis `bench/aider/aider/` (Aider-Klon inkl. `benchmark/`)
und `bench/aider/polyglot-benchmark/` sind **nicht im Repo**. Setup: beide Repos
klonen und das Image bauen —
`cd bench/aider/aider && podman build -f ../Dockerfile.podman -t aider-bench .` —
Details und die API-Varianten (DeepSeek/Gemini statt lokal) in
[API-RUNBOOK.md, §4](API-RUNBOOK.md).

---

## App-Steuerung & UX-Tests (`bench/webapp/`)

Eine kleine lokale Shop-Testseite mit **absichtlich eingebauten UX-Fehlern** —
[bench/webapp/UX-FLAWS.md](bench/webapp/UX-FLAWS.md) ist die Ground Truth
(8 geplante Fehler in zwei Runden: Checklisten-Klassiker + Fluss-Probleme).

```bash
python3 ~/ai-lab/bench/webapp/server.py    # Testseite auf 127.0.0.1:8090
```

| Skript | Was es misst |
|---|---|
| `dom-agent.py <label> <info\|form>` | Website-Steuerung über Text-Snapshots (Playwright-artig, ohne Vision): Info finden bzw. Formular ausfüllen; objektives Grading, Transkript → `runs-dom/` |
| `ux-dom-test.py <label>` | UX-Review **ohne** Vision: Modell liest den rohen HTML/CSS-Quelltext, Findings → `runs-ux/<label>-dom.md` |
| `ux-vision-test.py <label>` | UX-Review **mit** Vision: 5 Screenshots aus `shots/` (nicht im Repo — per Browser/grim neu erzeugen) an den lokalen Vision-Server |

`dom-agent.py` und `ux-dom-test.py` verstehen `API_BASE`, `API_KEY`, `API_MODEL`
als Umgebungsvariablen und laufen damit unverändert auch gegen Cloud-APIs;
`ux-vision-test.py` ist fest auf den lokalen Server (`:8080`) verdrahtet.
`bench/grounding-test.py` prüft separat GUI-Grounding (Klick-Koordinaten gegen
handverifizierte Boxen).

---

## API-Benchmarks (DeepSeek, Gemini)

Der komplette Ablauf — Phasen, Kosten, Modell-IDs, Troubleshooting, Privacy-Notizen —
steht in **[API-RUNBOOK.md](API-RUNBOOK.md)**. Kurzfassung: gleiche Suite, gleiche
Grader, gleiche Container; nur Provider-Config (`bench/opencode-config-api/`) und
Keys kommen dazu.

Keys entweder pro Shell exportieren oder in `~/ai-lab/.env` ablegen:

```bash
DEEPSEEK_API_KEY=sk-...
GEMINI_API_KEY=...
```

`.env` steht in `.gitignore` — **niemals committen**, keine Keys in Skripte oder
Docs schreiben.

---

## Updates

- **`sudo pacman -Syu`** pflegt Runtime und Backends (`llama-cpp`, `ggml-*`) und
  OpenCode in einem Rutsch.
- **Modelle** aktualisieren sich nicht selbst: neue GGUFs manuell nach `models/`
  laden und einen `serve.sh`-Zweig ergänzen (Anleitung im
  [Guide](dashboard/GUIDE-neue-benchmarks.md)).
- **Neue Modell-Architekturen brauchen oft ein frisches llama.cpp** — erst die
  Runtime updaten, dann das neue GGUF laden; sonst gibt es Ladefehler oder
  stillen Unsinn.

---

## Sicherheitsmodell

**Sandbox.** Benchmark-Agenten laufen in rootless podman (`--userns=keep-id`,
`--pull=never`) und sehen ausschließlich `/work` — eine Wegwerf-Kopie des
Task-Workspaces — plus einen read-only Go-Modulcache. Kein `$HOME`, keine SSH-Keys,
keine Secrets. Netz gibt es nur über pasta; der Host-Loopback ist als `169.254.1.2`
gemappt, damit der Agent den Model-Server erreicht. In der OpenCode-Bench-Config ist
`webfetch` verboten; ein `timeout` (Default 1200 s) beendet hängende Läufe hart.

**Keine Remotes.** Die Workspaces sind Klone ohne `origin`, ohne Hooks; `deploy/scripts`,
`.claude/` und `.github/` sind entfernt. Der Agent kann nichts pushen, nichts
deployen und keine CI triggern — die Originale unter `~/Projects` werden von der
gesamten Suite nie angefasst.

**Grader host-seitig.** `grade.sh` läuft nach Container-Ende auf dem Host, nicht im
Container — der Agent kann sein eigenes Grading weder sehen noch manipulieren. Die
Grader prüfen zusätzlich, dass Testdateien unverändert sind, und `results.jsonl`
wird ausschließlich vom Host beschrieben.

---

## Dateikarte

| Pfad | Inhalt |
|---|---|
| `serve.sh` | Modell-Server starten (alle Modelle + Flags) |
| `download-models.sh` | GGUF-Download-Queue (aria2c) |
| `perf-bench.sh`, `perf-bench2.sh` | llama-bench-Matrizen → `logs/perf-*.json` |
| `API-RUNBOOK.md` | Runbook für die API-Vergleiche (DeepSeek/Gemini) |
| `TODO-morgen.md` | Fahrplan/Arbeitsjournal der Benchmark-Kampagne |
| `bench/Containerfile`, `bench/Containerfile.dsh` | Sandbox-Images `agent-bench`, `agent-bench-dsh` |
| `bench/prepare-workspaces.sh` | baut die 5 Task-Workspaces aus den Quell-Repos |
| `bench/run-task.sh`, `run-task-api.sh`, `run-task-dsh.sh` | Suite-Runner (lokal / OpenCode+API / dsh) |
| `bench/tasks/` | Task-Definitionen (`prompt.txt` + `grade.sh`) |
| `bench/results.jsonl` | alle Suite-Ergebnisse (im Repo!) |
| `bench/failure-analysis.md` | Forensik: warum welche Läufe scheiterten |
| `bench/opencode-config/`, `opencode-config-api/` | OpenCode-Configs für Bench-Läufe (lokal / API) |
| `bench/analyze-transcript.py`, `bench/grounding-test.py` | Transkript-Metriken, GUI-Grounding-Test |
| `bench/aider/` | Polyglot: eigenes Skript + `Dockerfile.podman` (Klone/Ergebnisse nicht im Repo) |
| `bench/webapp/` | UX-/App-Steuerungs-Tests: Testseite, DOM-Agent, UX-Reviews, `UX-FLAWS.md` |
| `dashboard/` | Dashboard (`server.py`, `start.sh`, Frontend, [Guide](dashboard/GUIDE-neue-benchmarks.md), `polyglot-labels.json`) |
| `models/`, `logs/`, `bench/runs/`, `bench/workspaces/` | lokal, nicht im Repo (`.gitignore`) |

Detail-Dokumentation statt Duplikation: [API-RUNBOOK.md](API-RUNBOOK.md) ·
[bench/failure-analysis.md](bench/failure-analysis.md) ·
[dashboard/GUIDE-neue-benchmarks.md](dashboard/GUIDE-neue-benchmarks.md) ·
[bench/webapp/UX-FLAWS.md](bench/webapp/UX-FLAWS.md)
