# API-Modelle testen

**Frage dieser Seite: Wie lasse ich Cloud-Modelle gegen dieselbe Suite laufen — und was kostet das wirklich?**

Grundsatz: gleiche Tasks, gleiche Grader, gleiche Container wie lokal. Nur Provider-Config und Keys kommen dazu — die Zahlen bleiben vergleichbar.

## Keys

```bash
export DEEPSEEK_API_KEY='sk-...'     # platform.deepseek.com
export GEMINI_API_KEY='...'          # aistudio.google.com — NUR mit Billing/Tier 1!
```

Alternativ in `~/ai-lab/.env` (steht in `.gitignore` — nie committen, nie in Skripte). Neue Terminals haben exportierte Keys **nicht** automatisch. Kostenloser Check: `[ -n "$DEEPSEEK_API_KEY" ] && echo ok`.

**Gemini Free-Tier ist tabu:** stark gedrosselt **und trainiert auf den Prompts** — für Läufe über Kopien eigener Repos inakzeptabel. **DeepSeek verarbeitet Daten in China** — bewusste, dokumentierte Entscheidung für diese Benchmarks (Workspaces sind Sandbox-Kopien ohne Secrets). **Gemini 3.1 Pro ist gestrichen** — nicht laufen lassen, auch nicht „zum Vergleich".

## Läufe starten

Immer erst **ein** Task als Smoke-Test (~0,01–0,02 €), `stderr.log` lesen, dann der Rest:

```bash
cd ~/ai-lab/bench
TASKS="agora-A1-gate agora-A2-jsonld agora-A3-hls aiux-U1-paging aiux-U2-denytools"

# OpenCode + DeepSeek:
OC_CONFIG=opencode-config-api LABEL=oc-v4-flash ./run-task-api.sh deepseek/deepseek-v4-flash agora-A1-gate
for t in $TASKS; do OC_CONFIG=opencode-config-api LABEL=oc-v4-flash ./run-task-api.sh deepseek/deepseek-v4-flash "$t"; done

# DeepSeek-eigener Harness (dsh, Image agent-bench-dsh):
./run-task-dsh.sh dsh-v4-flash agora-A1-gate
DSH_MODEL=deepseek-v4-pro ./run-task-dsh.sh dsh-v4-pro agora-A1-gate
```

Auch DOM/UX-Tests laufen unverändert gegen APIs — `dom-agent.py` und `ux-dom-test.py` verstehen `API_BASE`, `API_KEY`, `API_MODEL` als Umgebungsvariablen.

## Kosten — Größenordnungen aus den echten Läufen

| Lauf | v4-flash | v4-pro | Gemini 3.7 Flash |
|---|---|---|---|
| Suite (5 Tasks) | ~0,08 € | ~0,24 € | 0,39–1,02 € |
| Polyglot (73 Übungen) | ~0,40 € | ~1,20 € | ~2,10 € |

Ein Suite-Lauf kostet also **8 Cent bis etwa einen halben Euro** — die teuren Posten sind die Polyglot-Läufe. Der komplette API-Vergleich (2 Harnesses × Suite + 3 × Polyglot) war mit **~6–9 € Gesamtbudget** geplant. DeepSeek: Off-Peak ist ~50 % billiger (Peak = Mo–Fr 03–06 und 08–12 Uhr CEST — außerhalb fahren, Nachmittag/Abend ideal), und implizites Prompt-Caching läuft automatisch. DeepSeek läuft prepaid — das Guthaben ist zugleich der harte Kostendeckel [unverifiziert: steht so in keinem Lab-Dokument; auf platform.deepseek.com prüfen].

## dsh vs. OpenCode — was das Wochenende zeigte

- **dsh** (DeepSeeks eigenes CLI, `0.1.1-rc.2`, im Image gepinnt, Developer Preview): beide Modelle **5/5 PASS**, auffallend schnell — v4-flash 32–183 s pro Task (Suite gesamt ~6 min), v4-pro ~7 min. Telemetrie ist per Default aus; nicht einschalten.
- **OpenCode + Gemini 3.7 Flash:** nach dem Provider-Fix ([Troubleshooting](troubleshooting.md)) ebenfalls **5/5 PASS** (88–343 s pro Task). Die ersten Versuche über den OpenAI-kompatiblen Endpoint starben nach Sekunden.
- **OpenCode + DeepSeek** (Suite): Stand 24.08. abends noch nicht gelaufen — in `results.jsonl` gibt es keine `oc-v4-flash`/`oc-v4-pro`-Zeilen. Die Polyglot-Läufe für beide DeepSeek-Modelle waren zu dem Zeitpunkt noch unterwegs.
- **Polyglot Gemini 3.7 Flash:** fertig — pass@2 **95,9 %** (70/73) bei 15,7 s/Übung. Zum Vergleich lokal: 27B 72,6 %, 35B 63,0 %.

Ergebnisse landen wie immer in `results.jsonl` (Labels: `dsh-v4-flash`, `dsh-v4-pro`, `oc-…`); Transkripte unter `bench/runs/<label>/<task>/` — dsh legt zusätzlich `dsh-home/sessions/*.jsonl` ab.
