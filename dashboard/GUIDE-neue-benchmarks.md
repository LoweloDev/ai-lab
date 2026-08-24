# Guide: Neue Benchmarks & Modelle anbinden

Kurzreferenz, wie das Lab wächst: neuer Suite-Task, neues Modell, Polyglot für ein
neues Modell, externe Standard-Benchmarks. Alles Genannte erscheint automatisch im
Dashboard, sobald die Daten an den bekannten Orten liegen.

## 1. Neuen Suite-Task anlegen (`bench/tasks/<name>/`)

Ein Task besteht aus genau zwei Dateien plus einem vorbereiteten Workspace:

```
bench/tasks/<name>/prompt.txt    # die Aufgabe, die der Agent bekommt
bench/tasks/<name>/grade.sh      # objektives Rot/Grün-Grading
bench/workspaces/<name>/         # Git-Klon, auf dem der Agent arbeitet
```

**Kontrakt von `grade.sh`** (so wertet `run-task.sh` aus):

- Aufruf: `grade.sh <pfad-zum-workspace>` — `$1` ist die Arbeitskopie nach dem Lauf.
- Nur die **letzte stdout-Zeile** zählt (`tail -1`): beginnt sie mit `PASS`,
  ist der Lauf grün (auch `PASS pass=52 fail=0` ist ok), alles andere ist rot.
  Diagnose-Ausgaben davor sind erlaubt und erwünscht.
- Manipulationsschutz nicht vergessen: prüfen, dass die Tests selbst nicht
  verändert wurden (Vorbild: bestehende `grade.sh` der agora-/aiux-Tasks).

**Workspace vorbereiten** (Vorbild: `bench/prepare-workspaces.sh`):

- Git-Klon ohne Remotes und Hooks; `deploy/`, `.claude/`, `.github/` entfernen.
- Der Baseline-Commit ist wichtig: `run-task.sh` merkt sich `HEAD` und meldet
  hinterher `git diff --stat` als `changed` in `results.jsonl`.

**Ausführen:**

```bash
~/ai-lab/serve.sh qwen38 vulkan          # Modellserver auf :8080
~/ai-lab/bench/run-task.sh qwen38-vulkan <name> [timeout]
```

Die Ergebniszeile landet in `bench/results.jsonl` → Suite-Tab zeigt den neuen
Task sofort als eigene Spalte. Alternativ den Lauf direkt im **Läufe-Tab** starten.

## 2. Neues lokales Modell anbinden

1. **GGUF besorgen** → `~/ai-lab/models/` (Muster: `download-models.sh`,
   z. B. `huggingface-cli download <repo> <datei> --local-dir ~/ai-lab/models`).
   Budget: 20 GB VRAM. Passt das Modell nicht komplett hinein (große MoE),
   Experten in den RAM auslagern: `--n-cpu-moe <n>` (Vorbild `codernext`).
2. **`serve.sh`-Eintrag** ergänzen — neuer `case`-Zweig mit `-m <datei>` und den
   Lab-Standardflags (stecken in `COMMON`): `--jinja` (Pflicht für Tool-Calls),
   `--cache-reuse 256`, `-ctk q8_0 -ctv q8_0`, `-ngl 99`, Kontext nach VRAM.
3. **Messen**, in dieser Reihenfolge:
   - Suite: über den Läufe-Tab oder `run-task.sh <label>-<backend> <task>`.
   - Perf: `~/ai-lab/perf-bench.sh <modell>` → `logs/perf-*.json`
     (**niemals parallel zum laufenden Server** — gleiche GPU).
   - Polyglot: siehe Abschnitt 3.
4. **Dashboard:** neue Labels tauchen in der Suite-Matrix automatisch auf.
   Für eine eigene Hero-Karte in der Übersicht das Label in
   `dashboard/server.py` bei `MODELS` + `MODEL_META` eintragen (zwei Zeilen).

## 3. Polyglot (Aider-Benchmark) für ein neues Modell

```bash
~/ai-lab/serve.sh <modell> vulkan        # Server zuerst!
~/ai-lab/bench/aider/run-polyglot-subset.sh
```

- Das Skript fährt den python+go-Subset (73 Übungen) via podman gegen
  `http://169.254.1.2:8080/v1`; `LANGS`, `THREADS`, `RUN_NAME` oben im Skript.
- Ergebnisse: `bench/aider/aider/tmp.benchmarks/<zeitstempel>--<run-name>/`,
  pro Übung eine `.aider.results.json` (`tests_outcomes`: `[true]` = pass@1,
  `[false, true]` = pass@2). Statistik jederzeit reproduzierbar mit
  `benchmark.py --stats <run>` (Kommando steht im Kopf des Skripts).
- **Wichtig fürs Dashboard:** Der Run-Name enthält das Modell nicht. Deshalb den
  neuen Lauf in `dashboard/polyglot-labels.json` einem Modell-Label zuordnen —
  sonst bleibt die Hero-Karte ohne Polyglot-Wert.

## 4. API-Modelle (DeepSeek, Gemini, …)

- Suite: `run-task-api.sh <modell-id> <task>` mit `OC_CONFIG=opencode-config-api`
  und optional `LABEL=<kurzname>`; API-Keys als Umgebungsvariablen
  (`DEEPSEEK_API_KEY`, `GEMINI_API_KEY`). Der Läufe-Tab hat dafür ein eigenes
  Formular (Skript „Suite — run-task-api.sh“).
- DOM/UX-Tests gegen API-Modelle: `dom-agent.py` und `ux-dom-test.py` verstehen
  `API_BASE`, `API_KEY`, `API_MODEL` als Umgebungsvariablen.

## 5. Externe Standard-Benchmarks (Ausblick / future work)

Noch nicht integriert, aber vorgezeichnet — llama-server spricht die
OpenAI-API, deshalb docken beide ohne Sonderwege an:

- **lm-eval-harness** (EleutherAI): klassische Wissens-/Reasoning-Suiten
  (MMLU, GSM8K, …). In eigener venv/uv-Umgebung installieren, dann z. B.:
  ```bash
  lm_eval --model local-completions \
    --model_args base_url=http://127.0.0.1:8080/v1/completions,model=local,api_key=sk-local \
    --tasks gsm8k --output_path ~/ai-lab/logs/lm-eval/
  ```
  Die JSON-Ausgaben unter `logs/lm-eval/` wären eine neue read-only-Quelle für
  einen eigenen Dashboard-Tab (gleiche Mechanik wie `perf-*.json`).
- **terminal-bench**: Agenten-Benchmark für Terminalaufgaben, läuft
  containerisiert (passt zum podman-Setup) und kann OpenAI-kompatible Endpoints
  ansprechen. Kandidat, sobald ein Harness-Adapter für OpenCode/lokale Server
  stabil ist.
- **Integrations-Muster:** Alles, was eine JSON/JSONL-Ergebnisdatei unter
  `~/ai-lab/` ablegt, kann `dashboard/server.py` als weitere Quelle einlesen —
  eine `read_*()`-Funktion plus ein Tab im Frontend, fertig.

---

*Diese Seite gehört dem Dashboard (`dashboard/GUIDE-neue-benchmarks.md`) und darf
frei erweitert werden — sie ist bewusst die einzige Wiki-Seite, die nicht zu den
bestehenden Lab-Dokumenten gehört.*
