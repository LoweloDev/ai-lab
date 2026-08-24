# Ergebnisse lesen

**Frage dieser Seite: Was bedeutet eine Zeile in results.jsonl — und wo liegen die Rohdaten zu jedem Lauf?**

## Das Format

Jeder Suite-Runner hängt pro Lauf **eine** JSON-Zeile an `~/ai-lab/bench/results.jsonl`:

```json
{"model":"qwen38-vulkan","task":"agora-A1-gate","grade":"PASS","seconds":78.5,"exit":0,"changed":" 1 file changed, 1 insertion(+), 1 deletion(-)"}
```

| Feld | Bedeutung |
|---|---|
| `model` | das **Label** des Laufs — lokal `<modell>-<backend>` (z. B. `muse-vulkan`), API `dsh-…`/`oc-…`. Nur Beschriftung: lokal zählt, was auf `:8080` lief |
| `task` | Task-Name aus `bench/tasks/` |
| `grade` | letzte stdout-Zeile von `grade.sh`. Beginnt mit `PASS` = grün (auch `PASS pass=52 fail=0`); sonst rot mit Ursache: `FAIL test-file-modified`, `FAIL tests-failing fail=3 pass=49`, `FAIL tests-red`, … |
| `seconds` | Wanduhrzeit des Container-Laufs |
| `exit` | Exit-Code des Containers: `0` normal, `124` Timeout (SIGTERM), `137` SIGKILL nach kill-after, `1` CLI-/Providerfehler |
| `changed` | `git diff --stat` gegen den Baseline-Commit — leer heißt: der Agent hat nichts editiert |

## Die Dedupe-Regel

Die Datei ist append-only — Wiederholungen erzeugen neue Zeilen. **Beim Auswerten zählt die letzte Zeile je (model, task).** Genau so rechnet das Dashboard (`read_suite` in `server.py`). Beispiel aus den echten Daten: `oc-gemini37f × agora-A1-gate` hat drei Zeilen — zweimal `FAIL` (Providerproblem, 1–7 s, `changed` leer), dann `PASS` (124,1 s). Gewertet wird das PASS; die FAIL-Zeilen bleiben als Historie stehen.

Kompakte Sicht:

```bash
jq -r '[.model,.task,.grade,(.seconds|tostring)] | @tsv' ~/ai-lab/bench/results.jsonl | column -t
```

## Run-Status im Läufe-Tab

Über den Läufe-Tab gestartete Benchmarks (Allowlist: `run-task.sh`, `run-task-api.sh`, `dom-agent.py`, `ux-dom-test.py`) bekommen einen Status:

| Status | Bedeutung |
|---|---|
| `läuft` | Prozess aktiv; es kann nur **ein** Lauf zur Zeit laufen (GPU-Lock) |
| `fertig` / `fehler` | beendet mit rc 0 / rc ≠ 0 |
| `abgebrochen` | Dashboard wurde neu gestartet und der Prozess lebt nicht mehr |
| `beendet (Exit unbekannt)` | Prozess überlebte einen Dashboard-Neustart „detached" und ist inzwischen weg |

Log je Lauf: `dashboard/runs/<id>.log` (+ `<id>.json` Metadaten). Ein `fehler` hier heißt nur: das Skript brach ab — das fachliche Ergebnis steht trotzdem in `results.jsonl`, falls es bis zum Grading kam.

## Wo die Rohdaten liegen

| Quelle | Pfad |
|---|---|
| Suite-Ergebnisse | `bench/results.jsonl` (einzige Datei im Repo) |
| Transkripte + Workspace nach dem Lauf | `bench/runs/<label>/<task>/` — OpenCode: `transcript.jsonl`, dsh: `transcript.log` + `dsh-home/sessions/*.jsonl`; immer: `stderr.log`, `ws/` |
| Transkript-Metriken | `bench/analyze-transcript.py <transcript.jsonl>` |
| Polyglot | `bench/aider/aider/tmp.benchmarks/<zeitstempel>--<run-name>/` — pro Übung `.aider.results.json`; `tests_outcomes: [true]` = pass@1, `[false,true]` = pass@2. Zuordnung Lauf→Modell: `dashboard/polyglot-labels.json` |
| Perf | `logs/perf-*.json` (llama-bench, Backend × Kontexttiefe) |
| DOM/UX | `bench/webapp/runs-dom/*.jsonl`, `bench/webapp/runs-ux/*.md` |
| Warum ein Lauf scheiterte | `bench/failure-analysis.md` — die Forensik zum 24.08. |

Faustregel bei allem, was komisch aussieht: erst `stderr.log`, dann das Transkript, dann urteilen.
