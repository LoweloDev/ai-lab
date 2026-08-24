# API-Runbook: API-Modell-Benchmarks (TODO-morgen.md, Punkt 4)

**Zweck:** Die geplanten API-Vergleiche (DeepSeek v4-flash, v4-pro, Gemini 3.7 Flash) über die
bestehende Benchmark-Infrastruktur fahren — 5-Task-Repo-Suite + Polyglot-Subset.
**Ausführender:** ein lokaler Coding-Agent (OpenCode mit Qwen3.8/Muse) oder Tobias selbst,
ohne weitere Rückfragen. Alle Kommandos sind copy-paste-fähig.
**Stand:** 2026-08-24. Verfasst von Claude; dsh-Modell-Override wurde offline (ohne API-Key,
ohne Kosten) gegen das gebaute Image verifiziert. Punkte, die erst mit echtem Key prüfbar
sind, tragen die Markierung **`[VERIFY-LIVE]`**.

**Gemini 3.1 Pro ist GESTRICHEN** (Tobias' Entscheidung) — nicht laufen lassen, auch nicht
„zum Vergleich".

---

## 0. Reihenfolge (Tobias' Plan) und Kostenrahmen

1. **Phase 1:** dsh (DeepSeek-eigener Harness) mit **v4-flash** und **v4-pro** gegen die 5-Task-Suite.
2. **Phase 2:** OpenCode mit **deepseek-v4-flash**, **deepseek-v4-pro**, **gemini-3.7-flash** — je Suite.
3. **Phase 3:** Polyglot-Subset (aider, python+go) für dieselben drei OpenCode-Modelle.

Danach liest Tobias alles und entscheidet — das Runbook endet mit dem Befüllen von
`results.jsonl` und den Polyglot-Stats, **keine** weiteren Aktionen.

| Lauf | flash | pro | Gemini 3.7 Flash |
|---|---|---|---|
| Suite (5 Tasks) | ~0,08 € | ~0,24 € | 0,39–1,02 € |
| Polyglot (73 Übungen) | ~0,40 € | ~1,20 € | ~2,10 € |

**Gesamt alle Läufe: ~6–9 €.** Bei DeepSeek gilt: Off-Peak ist ~50 % billiger, und implizites
Prompt-Caching läuft automatisch (keine Konfiguration nötig — wiederholte Prompt-Präfixe der
Agent-Turns werden günstiger abgerechnet).

**Off-Peak-Regel (DeepSeek): Peak = Mo–Fr 03:00–06:00 Uhr und 08:00–12:00 Uhr CEST.
Außerhalb dieser Fenster laufen lassen** — Nachmittag/Abend ist ideal und genau so geplant.

---

## 1. Voraussetzungen

### 1.1 API-Keys

- `DEEPSEEK_API_KEY` — von <https://platform.deepseek.com>
- `GEMINI_API_KEY` — von <https://aistudio.google.com>. **Wichtig: Billing muss aktiviert sein
  (Tier 1).** Der Free-Tier ist stark gedrosselt **und trainiert auf den Prompts** — für
  Läufe über Kopien eigener Repos inakzeptabel.

Keys nur in der aktuellen Shell exportieren — **nie** in Dateien, Skripte oder das Repo:

```bash
export DEEPSEEK_API_KEY='sk-...'
export GEMINI_API_KEY='...'
```

Kostenloser Check, dass beide gesetzt sind:

```bash
[ -n "$DEEPSEEK_API_KEY" ] && echo "deepseek: ok" || echo "deepseek: FEHLT"
[ -n "$GEMINI_API_KEY" ]   && echo "gemini: ok"   || echo "gemini: FEHLT"
```

### 1.2 Privacy (dokumentierte Entscheidung)

- **DeepSeek verarbeitet die Daten in China.** Das ist Tobias' bewusste, dokumentierte
  Entscheidung für diese Benchmarks (die Workspaces sind Sandbox-Kopien der eigenen Repos
  agora-debate und ai-ux-framework, ohne Secrets, ohne `.claude`/`.github`/`deploy`).
- Gemini: nur mit Tier 1/Billing fahren (siehe 1.1) — der Free-Tier trainiert auf Prompts.
- dsh-Telemetrie ist per Default **DISABLED** (`DSH_TELEMETRY_MODE` ungesetzt) — im
  Profil-Dump des gebauten Images verifiziert. Nicht setzen.

### 1.3 Infrastruktur-Check (kostenlos, ohne Keys)

```bash
podman image exists localhost/agent-bench      && echo "agent-bench: ok"
podman image exists localhost/agent-bench-dsh  && echo "agent-bench-dsh: ok"
podman image exists localhost/aider-bench      && echo "aider-bench: ok"
ls ~/ai-lab/bench/workspaces/                  # muss 5 Task-Verzeichnisse zeigen
podman run --rm --pull=never --network=none localhost/agent-bench-dsh dsh --version
# erwartet: 0.1.1-rc.2
```

- Fehlen die Workspaces (z. B. nach dem Aufräumen aus TODO Punkt 5):
  `~/ai-lab/bench/prepare-workspaces.sh` neu laufen lassen. Achtung: das löscht
  `bench/workspaces` komplett und baut alle 5 frisch.
- Ein laufender `llama-server` ist für die API-Läufe **nicht** nötig (stört aber auch nicht).
- Die 5 Tasks heißen: `agora-A1-gate agora-A2-jsonld agora-A3-hls aiux-U1-paging aiux-U2-denytools`.

### 1.4 Netz-Smoke-Test aus dem Container (kostenlos)

pasta-Networking gibt dem Container Outbound-Internet; `--map-host-loopback` stört nicht.
Wer sichergehen will (kein API-Call, nur TCP/TLS-Erreichbarkeit):

```bash
podman run --rm --pull=never --network=pasta:--map-host-loopback,169.254.1.2 localhost/agent-bench \
  node -e "fetch('https://api.deepseek.com').then(r=>console.log('deepseek erreichbar:',r.status)).catch(e=>{console.error(e.message);process.exit(1)})"
```

---

## 2. Phase 1: dsh (DeepSeek Harness) gegen die 5-Task-Suite

**Was ist dsh:** DeepSeeks offizielles Agent-CLI, npm `@deepseek-ai/dsh@0.1.1-rc.2`, im Image
`agent-bench-dsh` gepinnt (Containerfile.dsh). **Developer Preview — Breaking Changes möglich.**
Headless-Aufruf: `dsh --profile headless "<prompt>"`; Key kommt aus der Umgebung
(`DEEPSEEK_API_KEY`); Session-JSONL-Logs liegen unter `~/.dsh` (im Container — das Runner-Skript
mountet das nach `bench/runs/<label>/<task>/dsh-home/`, dort unter `sessions/`).

**Modellwahl (offline verifiziert):** Das headless-Profil hat als Default
`provider: deepseek-official, model: deepseek-v4-flash` (Plugin `agent-default-model`).
Override läuft über eine `--patch`-Overlay-Datei; `run-task-dsh.sh` erzeugt sie automatisch,
wenn `DSH_MODEL` gesetzt ist. Mit `dsh --profile headless --patch ... --dump-config` wurde
offline bestätigt, dass der Override greift.

**Runner:** `~/ai-lab/bench/run-task-dsh.sh` (neu, aus `run-task.sh` abgeleitet; gleiche
Mounts/Netz/Grading, Ergebnis-Zeile nach `bench/results.jsonl`).

**Erst ein Einzeltask als Smoke-Test** (kleinster Task, ~0,01–0,02 €), Ergebnis prüfen:

```bash
cd ~/ai-lab/bench
./run-task-dsh.sh dsh-v4-flash agora-A1-gate
cat runs/dsh-v4-flash/agora-A1-gate/stderr.log | head -30   # Fehler? Key angekommen?
```

`[VERIFY-LIVE]` Dass `dsh-credentials-local` den Key wirklich aus `DEEPSEEK_API_KEY` liest,
ist ohne echten Key nicht prüfbar — genau das prüft dieser Smoke-Test. Bei einem
Credentials-Fehler in `stderr.log`: `podman run --rm -e DEEPSEEK_API_KEY localhost/agent-bench-dsh dsh --profile headless --help`
laufen lassen und die Fehlermeldung lesen; notfalls Abschnitt 6 (Fallback).

**Dann die vollen Läufe** (Reihenfolge: erst flash komplett, dann pro):

```bash
cd ~/ai-lab/bench
TASKS="agora-A1-gate agora-A2-jsonld agora-A3-hls aiux-U1-paging aiux-U2-denytools"

# dsh + v4-flash (Default-Modell, kein Override nötig):
for t in $TASKS; do ./run-task-dsh.sh dsh-v4-flash "$t"; done

# dsh + v4-pro (Override via DSH_MODEL):
for t in $TASKS; do DSH_MODEL=deepseek-v4-pro ./run-task-dsh.sh dsh-v4-pro "$t"; done
```

`[VERIFY-LIVE]` Die Modell-ID `deepseek-v4-pro` gegen die echte API (Schreibweise laut Plan
und OpenCode-Config; der `--dump-config`-Default `deepseek-v4-flash` stützt das Schema). Wenn
der pro-Lauf sofort mit „unknown model" o. ä. abbricht: `runs/dsh-v4-pro/<task>/stderr.log` lesen.

Nach jedem Task erscheint eine JSON-Zeile mit `grade` (PASS/FAIL) auf stdout und in
`results.jsonl`. 10 neue Zeilen = Phase 1 fertig.

**Nebenbei, für später:** DeepSeek spricht auch die Anthropic-API
(`https://api.deepseek.com/anthropic`) — d. h. Claude Code könnte direkt dagegen laufen.
Für die Benchmarks hier nicht verwendet (kein Vergleichswert im Plan).

---

## 3. Phase 2: OpenCode gegen die 5-Task-Suite (flash, pro, Gemini)

**Runner:** `~/ai-lab/bench/run-task-api.sh` (neu, aus `run-task.sh` abgeleitet).
Unterschiede zum Original: Modell-ID als 1. Argument (geht an `opencode run -m`),
Config-Verzeichnis über `OC_CONFIG`, Ergebnis-Label über `LABEL`, und die beiden API-Keys
werden in den Container durchgereicht. Alles andere (Image `agent-bench`, Mounts, pasta-Netz,
Grading, `results.jsonl`) ist identisch.

Wer stattdessen das Original anpassen will: in `run-task.sh` sind es **genau zwei Zeilen** —
die Config-Mount-Zeile (`-v "$BENCH/opencode-config:...` → `opencode-config-api`) und die
Modell-Zeile (`opencode run -m llamacpp/local` → gewünschte Modell-ID); zusätzlich müssten
`-e DEEPSEEK_API_KEY -e GEMINI_API_KEY` in den `podman run` hinein. `run-task-api.sh`
erledigt genau das parametrisiert — Original nicht anfassen.

**Provider-Config:** `bench/opencode-config-api/opencode.json` ist fertig — Provider
`deepseek` (baseURL `https://api.deepseek.com/v1`) und `gemini` (OpenAI-kompatibler Endpoint
`https://generativelanguage.googleapis.com/v1beta/openai`), Keys via
`{env:DEEPSEEK_API_KEY}` / `{env:GEMINI_API_KEY}`. Modell-IDs für `-m`:
`deepseek/deepseek-v4-flash`, `deepseek/deepseek-v4-pro`, `gemini/gemini-3.7-flash`.

**Läufe** (wieder: erst ein Task als Smoke-Test, `stderr.log` prüfen, dann der Rest):

```bash
cd ~/ai-lab/bench
TASKS="agora-A1-gate agora-A2-jsonld agora-A3-hls aiux-U1-paging aiux-U2-denytools"

# Smoke-Test (1 Task, ~0,01–0,02 €):
OC_CONFIG=opencode-config-api LABEL=oc-v4-flash ./run-task-api.sh deepseek/deepseek-v4-flash agora-A1-gate
head -30 runs/oc-v4-flash/agora-A1-gate/stderr.log

# DeepSeek v4-flash:
for t in $TASKS; do OC_CONFIG=opencode-config-api LABEL=oc-v4-flash ./run-task-api.sh deepseek/deepseek-v4-flash "$t"; done

# DeepSeek v4-pro:
for t in $TASKS; do OC_CONFIG=opencode-config-api LABEL=oc-v4-pro ./run-task-api.sh deepseek/deepseek-v4-pro "$t"; done

# Gemini 3.7 Flash:
for t in $TASKS; do OC_CONFIG=opencode-config-api LABEL=oc-gemini37f ./run-task-api.sh gemini/gemini-3.7-flash "$t"; done
```

15 neue Zeilen in `results.jsonl` = Phase 2 fertig.

`[VERIFY-LIVE]` Tool-Calling von Gemini über den OpenAI-kompatiblen Endpoint ist ungetestet.
Wenn im Transcript keine Tool-Calls auftauchen und `changed` leer bleibt: als Befund notieren
(nicht stillschweigend wiederholen) — das ist selbst ein Vergleichsergebnis.

---

## 4. Phase 3: Polyglot-Subset (aider) für die API-Modelle

Das bestehende `bench/aider/run-polyglot-subset.sh` ist fest auf den lokalen Server verdrahtet
(`OPENAI_API_BASE=http://169.254.1.2:8080/v1`, `MODEL=openai/local`). **Nicht editieren** —
unten stehen fertige podman-Aufrufe, abgeleitet aus dem Skript. Geändert gegenüber dem
lokalen Lauf: `OPENAI_API_BASE`/`OPENAI_API_KEY`/Modell-ID, `--threads 2` statt 1 (API kann
parallel; bei 429ern auf 1 senken), und `--read-model-settings` entfällt (die Datei
`local-model-settings.yml` gilt nur für `openai/local`: Reasoning-Tag-Stripping und
1800-s-Timeout sind lokalspezifisch). `--edit-format whole` bleibt **absichtlich** gleich,
damit die Zahlen mit den lokalen Läufen vergleichbar sind.

Gemeinsame Variablen (einmal pro Shell):

```bash
BENCH_ROOT="$HOME/ai-lab/bench/aider"
AIDER_DIR="$BENCH_ROOT/aider"
POLYGLOT_DIR="$BENCH_ROOT/polyglot-benchmark"
```

Dann pro Modell **einen** der drei folgenden Blöcke (je ~1–3 h; nacheinander, nicht parallel):

**DeepSeek v4-flash (~0,40 €):**

```bash
RUN_NAME="api-dsv4flash-py-go-$(date +%Y%m%d-%H%M%S)"
podman run --rm \
  --network=pasta:--map-host-loopback,169.254.1.2 \
  --memory=12g --memory-swap=12g \
  -v "$AIDER_DIR":/aider \
  -v "$AIDER_DIR/tmp.benchmarks":/benchmarks \
  -v "$POLYGLOT_DIR":/benchmarks/polyglot-benchmark \
  -e OPENAI_API_BASE=https://api.deepseek.com/v1 \
  -e OPENAI_API_KEY="$DEEPSEEK_API_KEY" \
  -e AIDER_DOCKER=1 \
  -e AIDER_BENCHMARK_DIR=/benchmarks \
  -w /aider \
  aider-bench \
  ./benchmark/benchmark.py "$RUN_NAME" \
    --model openai/deepseek-v4-flash \
    --edit-format whole \
    --threads 2 \
    --languages python,go \
    --exercises-dir polyglot-benchmark \
    --new \
  2>&1 | tee "$BENCH_ROOT/run-$RUN_NAME.log"
```

**DeepSeek v4-pro (~1,20 €):** wie oben, nur

```bash
RUN_NAME="api-dsv4pro-py-go-$(date +%Y%m%d-%H%M%S)"
# ... identischer podman-Aufruf, aber:
#   --model openai/deepseek-v4-pro
```

**Gemini 3.7 Flash (~2,10 €):** wie oben, nur

```bash
RUN_NAME="api-gemini37f-py-go-$(date +%Y%m%d-%H%M%S)"
# ... identischer podman-Aufruf, aber:
#   -e OPENAI_API_BASE=https://generativelanguage.googleapis.com/v1beta/openai \
#   -e OPENAI_API_KEY="$GEMINI_API_KEY" \
#   --model openai/gemini-3.7-flash
```

`[VERIFY-LIVE]` Das `openai/<id>`-Routing (litellm → `OPENAI_API_BASE`) ist beim lokalen
Server bewährt; dass beide API-Endpoints die jeweilige Modell-ID unverändert akzeptieren,
zeigt erst der Lauf. Fehlerbild „model not found" in den ersten Minuten → Lauf abbrechen
(Ctrl-C), Modell-ID gegen die Provider-Doku prüfen, Log lesen.

**Stats ablesen** (jederzeit wiederholbar, kostenlos):

```bash
podman run --rm \
  -v "$AIDER_DIR":/aider \
  -v "$AIDER_DIR/tmp.benchmarks":/benchmarks \
  -e AIDER_BENCHMARK_DIR=/benchmarks \
  -w /aider \
  aider-bench \
  ./benchmark/benchmark.py --stats "$RUN_NAME"
```

Headline-Zahl ist **`pass_rate_2`**. Ergebnisverzeichnisse:
`bench/aider/aider/tmp.benchmarks/<YYYY-MM-DD-HH-MM-SS>--<RUN_NAME>`.

---

## 5. Ergebnisse ablesen

**Suite:** alles landet als JSON-Zeilen in `~/ai-lab/bench/results.jsonl`. Labels dieses Plans:
`dsh-v4-flash`, `dsh-v4-pro`, `oc-v4-flash`, `oc-v4-pro`, `oc-gemini37f`
(vergleichbar mit den lokalen Labels wie `muse-vulkan`). Kompakte Sicht:

```bash
jq -r '[.model,.task,.grade,(.seconds|tostring)] | @tsv' ~/ai-lab/bench/results.jsonl | column -t
```

Soll-Zustand nach allen Phasen: 25 neue Zeilen (10 dsh + 15 OpenCode), plus 3 Polyglot-Läufe.

**Transkripte:** `bench/runs/<label>/<task>/` — OpenCode: `transcript.jsonl`; dsh:
`transcript.log` (finale Antwort) + `dsh-home/sessions/*.jsonl` (voller Session-Log).
`stderr.log` immer zuerst lesen, wenn etwas komisch aussieht.

**Polyglot:** `--stats`-Kommando aus Abschnitt 4; `pass_rate_2` je Modell notieren.
Lokale Vergleichswerte stehen schon da (35B ≈ 63 %; weitere in den `run-*.log`s bzw.
`tmp.benchmarks/`).

---

## 6. Troubleshooting

**„Key nicht gesetzt" / 401 / AuthenticationError** (in `stderr.log` bzw. aider-Log):

- Ist der Key in **dieser** Shell exportiert? (`[ -n "$DEEPSEEK_API_KEY" ] && echo ok`) —
  neue Terminals/tmux-Fenster haben ihn nicht automatisch.
- Kommt er im Container an? Kostenlos prüfbar:
  ```bash
  podman run --rm --pull=never -e DEEPSEEK_API_KEY localhost/agent-bench \
    sh -c '[ -n "$DEEPSEEK_API_KEY" ] && echo "im Container: ok" || echo "im Container: LEER"'
  ```
- OpenCode meldet fehlende Keys teils erst beim ersten Request — im Zweifel Transcript-Ende
  und `stderr.log` lesen.

**Rate Limits (HTTP 429):**

- DeepSeek: außerhalb der Peak-Fenster (Abschnitt 0) fahren; Suite-Läufe laufen ohnehin seriell.
- Gemini: 429 trotz Billing → prüfen, ob der Key wirklich aus dem Tier-1-Projekt stammt
  (AI Studio zeigt das Projekt zum Key). Free-Tier-Keys drosseln hart.
- Polyglot: `--threads 2` auf `--threads 1` senken und neu starten (neuer `RUN_NAME`).

**dsh instabil (Developer Preview, rc-Version):**

- Symptome: Crash beim Start, leeres `transcript.log`, geänderte Flags/Profile nach einem
  versehentlichen Update (Version ist im Image gepinnt — `dsh --version` muss `0.1.1-rc.2` sagen).
- Kostenlose Diagnose: `dsh --help`, `dsh --profile headless --help`,
  `dsh --profile headless --dump-config` im Container mit `--network=none`.
- **Fallback (eingeplant):** Wenn dsh nicht binnen ~30 min zum Laufen kommt: **Phase 1
  überspringen** und den Vergleich rein über OpenCode fahren — Phase 2 deckt beide
  DeepSeek-Modelle ab, der Plan bleibt aussagekräftig. Im Abschlussbericht klar vermerken:
  „dsh (0.1.1-rc.2, developer preview) nicht lauffähig, Fehlerbild: …".

**Timeout / kaputter Lauf vs. echtes FAIL:**

- `"exit":124` in der Ergebnis-Zeile = 1200-s-Timeout hat den Container abgeschossen
  (bei API-Modellen selten; eher Hänger durch Netz/Rate-Limit). Das Grading bewertet dann
  den erreichten Stand.
- `grade: FAIL` **plus** leeres `changed` **plus** Fehler in `stderr.log` = Infrastrukturproblem,
  nicht Modellschwäche — Ursache beheben, Task einmal wiederholen (der Runner überschreibt
  `runs/<label>/<task>/` und hängt eine neue Zeile an `results.jsonl` an; beim Auswerten zählt
  die letzte Zeile pro model+task).
- Echte FAILs (Agent hat editiert, Tests rot) stehen lassen — das ist das Messergebnis.

**Geld-Notbremse:** Jeder Suite-Task kostet höchstens wenige Cent, Polyglot-Läufe sind die
teuren Posten. Ein hängender Polyglot-Lauf: Ctrl-C, Log lesen, Ursache fixen, mit neuem
`RUN_NAME` neu starten — halbe Läufe kosten anteilig, mehr nicht. Budget-Anker: Gesamtplan
~6–9 €; wenn absehbar mehr, abbrechen und Tobias entscheiden lassen.
