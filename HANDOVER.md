# HANDOVER — ai-lab Benchmark-Kampagne (Stand 24.08.2026 ~23:12, ergänzt 23:35)

Für einen Agenten OHNE Sitzungskontext (Gemini via `agy`, OpenCode, oder Claude neu).
Alles Wichtige steht in Dateien, nichts hängt an einer Chat-Sitzung. Lies zuerst:
`README.md` (Aufbau), `TODO-morgen.md` (Fahrplan), `bench/failure-analysis.md` (Befunde),
`bench/audit-report.md` (Grader-Audit, sobald vorhanden), `API-RUNBOOK.md` (Cloud-Setup).

## Eiserne Regeln (von Tobias)
1. **Kein Lauf wird abgebrochen, der noch Ergebnisse produziert.** Vor jedem Eingriff:
   `podman ps` und `pgrep -af 'run-task|run-polyglot|nachtkette|nachzuegler'`.
2. **Urteile werden korrigiert, nicht neu gewürfelt:** War ein Grader fehlerhaft → Grader
   fixen, betroffene Workspaces (`bench/runs/<label>/<task>/ws`) NEU BEWERTEN
   (`bash tasks/<task>/grade.sh <ws>`), Zeile in `bench/results.jsonl` IN PLACE korrigieren
   (vorher `cp results.jsonl results.jsonl.bak-<grund>`). Keine neuen Läufe dafür.
3. **Kein PASS, der keiner war** (Präzedenz: muse A5, Permutationsloch → aberkannt).
4. **Ehrlich melden**, was nicht lief; keine geschönten Tabellen.
5. **Geld:** DeepSeek-Hardcap 10 $ (bisher ~1,8 $), Gemini-API Spend-Cap gesetzt. Opus/agy
   laufen über Abos. Keine neuen kostenpflichtigen Konten anlegen.
6. `~/Projects/*` (Tobias' echte Repos) NIE anfassen. Bench arbeitet nur auf Kopien.

## Was gerade läuft (automatisch, braucht keine Sitzung)
| Bahn | Prozess/Skript | Ergebnis landet in | Fertig wenn |
|---|---|---|---|
| Nachtkette (80B Polyglot) | `bench/nachtkette-2.sh` | `bench/aider/tmp.benchmarks/*codernext*` | Log-Zeile `NACHTKETTE-2 KOMPLETT` |
| Qwen-Retries (A5/A6, 64k ctx) | `bench/nachzuegler-a5a6-retry.sh` (wartet auf obige Zeile) | `results.jsonl` (überholt Timeout-Zeilen) | `A5A6-RETRY KOMPLETT` |
| Opus 5 Polyglot (Resume) | `polyglot-oc/run-polyglot-claude.sh claude-opus-5 cc-opus5-polyglot` | `polyglot-oc/runs/cc-opus5-polyglot/summary.json` | run-resume.log `== Fertig` |
| Opus 4.8 Polyglot | Teil der Opus-Kette | `polyglot-oc/runs/cc-opus48-polyglot/` | `OPUS_PROGRAMME_KOMPLETT` |
| agy Polyglot (Abo, 3.7 Flash high) | `polyglot-oc/run-polyglot-agy.sh agy-37flash` | `polyglot-oc/runs/agy-37flash/summary.json` | `AGY_PROGRAMM_KOMPLETT` |

Dashboard: `http://127.0.0.1:8100` (`dashboard/start.sh`), liest alles aus Dateien.

## Offene Aufgaben in Reihenfolge
1. **Opus-Polyglots: FERTIG** (00:11 / 00:26). cc-opus5 73/73 pass@1 (21,25 $ Gegenwert, 68 min Σ),
   cc-opus48 nach Runner-Fix (trinary neu): 67/73 pass@1 (konservativ 66 — Original-Versuch 1 war rot), 73/73 pass@2 (18,34 $). Beide bereinigt (0 Auth-Reste, Testdateien
   byteidentisch). Fußnote für alle Polyglot-Zeilen: 3 Go-Übungen (counter, ledger, markdown) bestehen
   unberührt → "x/70 echte + 3 Freilose" (siehe failure-analysis.md 23:58).
2. **Grader-Härtung (Audit `bench/audit-report.md`, 27 Findings, Abschnitt 4 = Spezifikation):**
   Rückschau: KEIN Modell hat eine Lücke benutzt (98 Bewertungen, 400 Testdateien byteidentisch).
   Staging FERTIG (Workflow 23:50–01:05): `tasks/<task>/grade.v2.sh` (+ `grade_test.v2.go`) für alle 8
   Tasks; Validierung: **0 Flips über alle 98 Bewertungen, 36/36 Exploit-Klassen geschlossen**
   (`audit-scratch/haertung/validierung.md`); `bench/apply-haertung.sh` (547 Z.) mit Trockenlauf bestanden,
   adversarialer Review: Freigabe nach 4 Fixes (Sandbox-Vollanwendung liefert exakt die 8 erwarteten
   A5-Flips unter Vertragslesart, unter streng 0). Bericht: `bench/haertung-report.md`.
   **ANGEWENDET 02:29: Marker 'OK 20260825-022854 flips=0 lesart=streng'**, 98 Regrades, results.jsonl
   unverändert, Backups grade.v1.sh + results.jsonl.bak-audit-20260825-022854. Rollback siehe Kopf von
   apply-haertung.sh. A5-Lesart umschalten: `echo vertrag > bench/.a5-lesart && bash bench/apply-haertung.sh`.
   Manuell nachholen: `bash bench/apply-haertung.sh --dry-run` lesen, dann ohne Flag ausführen.
   Rollback: `grade.v1.sh`/`grade_test.v1.go` + `results.jsonl.bak-audit-*`.
   Batterien fertig: A5 (`robustness-battery/a5-results.md`, 9 Lösungen 11/12, gleiche Kante) und
   A6 (`a6-results.md`: alle 8 Cloud 12/12, 80B 11/12, muse 9/12, 35B 8/12).
3. **A5-Lesart (nur Tobias):** Audit 4.3 empfiehlt Option A (Vertragslesart: 8 FAIL→PASS für agy, Opus 5/4.8,
   dsh-flash/pro, Gemini-OC, oc-v4-flash/pro; muse/80B/Qwens bleiben FAIL). Option B = defensiv bleiben,
   Spec präzisieren. Umschalten: `echo vertrag > bench/.a5-lesart` VOR dem apply (bzw. danach erneut
   apply — idempotent); ohne Flag bleibt A5 streng. Dashboard bekommt in beiden Fällen die Doppelwertung
   als Annotation.
4. **Kandidaten-Runde 100–130B-MoE** (siehe TODO-morgen.md, Nachtrag 23:15): Recherche-Ergebnis
   abwarten/lesen, Tobias wählt, dann Download → `serve.sh`-Case → llama-bench → Suite A1–A6.
   Erst wenn GPU frei (nach Retries).
5. **Morgenbericht:** Artifact/README aktualisieren mit: Suite-Matrix (alle Labels), Zeit-Graph,
   Polyglot-Tabelle (oc-dsflash 95 % pass@2 für 0,24 $ = Harness-Effekt!), agy vs. OC-Latenz
   (Schritte 9x schneller), Opus-5-A6 = 13,36 $ Gegenwert für dieselben Properties wie agy in 3 min,
   A5-Einstimmigkeit 10/10, 80B-Profil (A6 als einziges Lokales, A4 als einziges verhauen),
   UX-Wertungstabelle (agy 7/10 + 1 Bonus in 20 s).
6. **Aufräumen** (erst nach Bericht, Tobias' Freigabe): `bench/audit-scratch` (1,3 GiB),
   `bench/workspaces`/`runs` (vorher denyTools-Patch sichern, s. TODO), podman-Images fragen.
7. **GitHub:** `~/ai-lab` als privates Repo sichern (Schritt 6 in TODO-morgen.md).

## Bekannte Fallen
- **Claude-OAuth im Container:** Token läuft ~8 h; Fix in `bench/cc-lib.sh` (Vorab-Check,
  Sync-back, Vergiftungs-Wächter) ist eingebaut. Symptom vorher: `tok=0/0 rcs=[1,1]` in 2 s.
- **zsh splittet `$var` nicht wie bash** — Ketten immer explizit ausschreiben oder `bash -c`.
- **32k Kontext reicht lokal nicht für A5/A6** ("Context size has been exceeded") → 64k via
  `serve.sh <m> vulkan -c 65536`.
- **agy** braucht das In-Container-Login-Template `bench/agy-auth-home/` (existiert, gültig).
- results.jsonl-Monitor spuckt beim In-Place-Schreiben alte Zeilen erneut aus — kein neuer Lauf.

## Nachtrag 23:35 — Kandidaten-Runde ist verdrahtet (läuft automatisch)
- Auswahl (Claude, Tobias' Freigabe "wähl selber"): **Mistral Small 4 119B-A6.5B**, **Qwen3.5-122B-A10B**,
  **Laguna S 2.1 118B-A8B** — alle imatrix IQ3_XXS (42,7 / 43,9 / 46,1 GiB; mradermacher/bartowski,
  Unsloth-UD hat nichts unter ~50 GiB). Fallback IQ2_M im Download-Skript einkommentierbar.
- `bench/kandidaten-download.sh` lädt (cgroup MemoryHigh=1G, damit der 80B-Polyglot seinen
  Page-Cache behält; aria2c 8 Verbindungen, fortsetzbar). Marker: `models/.kandidaten-download-ok`.
  Log: `logs/kandidaten-download.log`. Bei 14 MB/s Einzelstrom ~3 h, mit 8 Streams hoffentlich <1 h.
- `bench/kandidaten-kette.sh <retry-log>` wartet auf Download-Marker UND `A5A6-RETRY KOMPLETT`,
  dann je Modell: llama-bench (Vulkan+ROCm, ncmoe 40) → Suite 8 Tasks (Label `<m>-vulkan`).
  serve.sh-Cases `qwen35|laguna|mistral4` (NCMOE-Env, Default 40; bei VRAM-OOM erhöhen).
  Registry: dashboard/registry/models.json (Backup .bak-kandidaten). Ende: `KANDIDATEN-RUNDE KOMPLETT`.
- Bekannte Risiken: Laguna 46 GiB = Kante (ggf. Streaming → tg <5 t/s → auf IQ2_M wechseln);
  Laguna-Support in llama.cpp frisch (Reasoning/Tool-Call-Bugs gemeldet, Vulkan ungetestet).
- Zeitplan (Stand 00:15): 80B-Polyglot auf Tobias' Wunsch abgebrochen (12/73, als 'abgebrochen'
  annotiert) → Nachtkette KOMPLETT 00:14 → Qwen-Retries laufen ab ~00:20 (3 Läufe, je ≤60 min) →
  ~03:00 Härtungs-Wächter → Kandidaten-Runde ab ~03:00 (Download endet ~02:00) → Vormittag fertig.

## Nachtrag 00:25 — Flash-low-Suite geplant + Tobias' Zwischenfazit
- `bench/agy-low-suite.sh` läuft (Hintergrund, Start ~02:39 nach Quota-Reset): Gemini 3.7 Flash effort=low
  durch alle 8 Suite-Tasks, Label `agy-37flash-low`, danach `bench/agy-low-tokens.md` (Token-Bilanz vs. high).
  FERTIG 02:58: low 7/8 = high 7/8, 481 s vs 907 s, Output 53k vs 170k, Thinking 6,6k vs 109k → Thema Flash/Abo abgeschlossen.
- Tobias' Zwischenfazit (00:25): Flash "insane"; Runner-up DeepSeek via OpenCode; Claude das Beste, aber
  mit Abstand das Teuerste. Offen für sein Urteil: Härtungs-Ergebnis (Regrades) + Robustheits-Batterien.

## Nachtrag 00:50 — DeepSeek-Budget (Tobias' Konsole) + dsh-Polyglot
- DeepSeek-Konsole 25.08. 00:45: **4,98 $ verbraucht** (41,4 M Tokens, 1.726 Requests, alles vom 24./25.08.),
  5,01 $ bis zum 10-$-Hardcap. Frühere Schätzung (~1,8 $) war zu niedrig (Aider-Polyglots + OC-Läufe).
  Blended ~0,12 $/M Tokens.
- Tobias' Auftrag: **dsh-Polyglot flash zuerst (~0,25 $), danach pro (~1-1,5 $)**, nur wenn Budget reicht.
  Runner `bench/polyglot-oc/run-polyglot-dsh.sh` fertig; Smoke (affine-cipher) PASS 63 s, 0,0032 $.
  Kette LÄUFT seit 00:41: dsh-flash-polyglot → dsh-pro-polyglot (nur wenn flash < 1 $). Ende:
  `DSH_POLYGLOT_KETTE_KOMPLETT` (Logs: runs/dsh-*-polyglot.launch.log). Erwartet ~0,4 $ + ~1,5 $
  (dsh fährt Reasoning 'high' → ~3x Output vs. OpenCode; kein Session-Resume → Versuch 2 mit vollem Prompt).
  Vorbehalt: run-polyglot-dsh.sh wurde 32 s nach Kettenstart vom Bau-Agenten editiert (.env-Fallback);
  Schleife lief da schon — falls der Summary-Block am Ende stolpert: denselben Startbefehl erneut
  ausführen (Resume überspringt alle Übungen, schreibt nur summary.json neu).
  Start: `source ~/ai-lab/.env && cd bench/polyglot-oc && ./run-polyglot-dsh.sh dsh-flash-polyglot`
  bzw. `DSH_MODEL=deepseek-v4-pro ./run-polyglot-dsh.sh dsh-pro-polyglot`.
- Tobias' Einordnung (00:50): DeepSeek und Google nehmen sich wenig; DeepSeek für Burst-Workflow
  potenziell besser (kein Wochen-/5h-Deckel) und ähnlich günstig wie das 20-€-Abo.

## ⚠ Nachtrag 01:40 — Sicherheitsvorfall Git-Push (Claude-Fehler) + Aufräumpflicht
- 01:27 wurde Commit `e343b83` mit `bench/polyglot-oc/runs/**/cc-home/.credentials.json` (~150 Kopien des
  Claude-OAuth-Tokens), `bench/agy-auth-home/` (Antigravity-OAuth-Token) und 3,7 GiB audit-scratch nach
  GitHub (privates Repo LoweloDev/ai-lab) gepusht. Ursache: .gitignore-Muster mit Inline-Kommentar
  (von Git nicht unterstuetzt) + Secret-Scan ohne Abbruch. Beides gefixt (Muster korrigiert, Scan = hartes Gate).
- Lokal ersetzt durch sauberen Commit `746a14a` (55 Dateien, 0 Credentials). **Force-Push durch Tobias
  noetig** (Auto-Modus verweigert Claude das): `cd ~/ai-lab && git push --force-with-lease origin main`.
  Pruefung danach: `git ls-remote origin main` == `git rev-parse HEAD`.
- **Morgen (nach Ende aller Ketten):** Tokens rotieren — Claude `/logout` + `/login`; Antigravity neu
  einloggen UND `bench/agy-auth-home/` per In-Container-Login erneuern (Befehl in API-RUNBOOK/README).
  Optional GitHub-Support bitten, verwaiste Objekte zu loeschen.
- Download der Kandidaten ist komplett (01:36, 3/3 OK); Robustheits-Dashboard-Workflow gestoppt
  (Quota), Spezifikation liegt im Workflow-Skript robustheit-suite-dashboard-opus-*.js — kann von einem
  DeepSeek-/Gemini-Agenten oder nach Quota-Reset fortgesetzt werden.

## Nachtrag 02:00 — Robustheits-Umbau via DeepSeek (OpenCode), Harness-Korrektur
- Harness-Vergleich DeepSeek (Rohdaten): Suite-Summe oc-flash 889 s vs dsh-flash 1331 s (OC 1,5x schneller),
  Qualität 7/8 beide; Polyglot auf 35 gemeinsamen Übungen OC 31/34 vs dsh 30/35 (gleichauf), OC 35 % günstiger.
  → Zweigeteilt (Stand 02:45, dsh-Polyglot komplett 68/73 p1, 73/73 p2, 0,32 $): **Genauigkeit dsh, Tempo/Preis OpenCode.**
  (Claudes frühere Behauptung "dsh 2x schneller" war falsch.)
- dsh-pro-Polyglot gestrichen (Tobias); dsh-flash-Polyglot FERTIG 02:44 (Label dsh-flash-polyglot).
- Umbau "Robustheit über alle 8 Suite-Tasks als Dashboard-Metrik" läuft als `bench/robustness-battery/umbau-dsh.sh`
  (HARNESS=oc, DeepSeek flash): 8 Aufträge nach `robustness-battery/spec/0*.md` (Konvention 00; Runner in Go 01;
  Batterien 02–07; Dashboard 08), je Auftrag eine Container-Session mit ro-Lab + rw nur auf robustness-battery/,
  dashboard/, audit-scratch/robust/; Secrets per tmpfs überdeckt. Fortschritt: `spec/done-NN.txt`, Log `umbau.log`,
  Transkripte `sessions/NN/`. Ende: `UMBAU-DSH KOMPLETT`. Danach: EIN Opus-5-Agent (oder Claude) validiert auf dem Host
  (py_compile, Dashboard-Neustart ohne laufende Jobs, /api/robustness, Determinismus) — siehe Workflow-Skript
  robustheit-suite-dashboard-opus-*.js für die Prüfliste.

## Nachtrag 02:10 — GPU-Reihenfolge geändert: OC-Polyglot für die lokalen Qwens VOR den Kandidaten
- Tobias: Harness-Fairness — die Lokalen liefen Polyglot nur über Aider. `bench/polyglot-oc/oc-lokal-kette.sh`
  wartet auf `.grader-haertung-done`, dann je Modell serve.sh + `OC_CONFIG=opencode-config T_ATTEMPT*=600
  run-polyglot-oc.sh llamacpp/local oc-<m>-polyglot`: qwen36moe (~3,5 h) → codernext (~5 h) → qwen38 (~7 h).
  Marker `bench/.oc-lokal-polyglot-done`; Kandidaten-Kette wartet jetzt zusätzlich darauf. Reihenfolge
  ändern: Kette stoppen und mit Modell-Argumenten neu starten (`oc-lokal-kette.sh qwen36moe`), oder qwen38
  weglassen, wenn das Muster nach zwei Modellen klar ist.
- run-polyglot-oc.sh: Key-Pflicht nur noch für Cloud-Modelle, Timeouts per Env überschreibbar.
- Umbau-Treiber: erster Lauf scheiterte an OpenCodes 'external_directory'-Auto-Reject (cwd war das
  Batterie-Verzeichnis) → cwd = Lab-Wurzel; zweiter Fehlversuch davor: 42 tmpfs-Schatten → crun 'No space
  left' → jetzt hardgelinkte, secret-freie Sicht `bench/.runs-view` (gitignored). Läuft seit 02:08.
- 02:20 Zeitbremse in oc-lokal-kette.sh: qwen36moe/codernext 6 h, qwen38 4 h; bei Ablauf `--summary-only`
  (neu im OC-Runner) + Annotation "(Zeitlimit, n/73)" sichtbar im Polyglot-Tab (Teilergebnis = Stichprobe).
- Flash-low-POLYGLOT FERTIG 03:22: 73/73 pass@1 in 33 min (high 74 min), Output 209k vs 888k, Thinking 68k vs 684k.
  Effort-Kapitel abgeschlossen (Suite + Polyglot): low = gleiche Qualität, halbe Zeit.
- 03:15 Umbau FERTIG (8/8 done, 104 Paare in results.json, 8 Batterien: A1 R10/P6, A2 R10/P5, A3 R9/P5,
  A4 R10/P6, A5 R7/P5, A6 R12/P9, U1 R11/P4, U2 R10/P4; Kosten ≈0,3 $). Host: py_compile + node --check ok.
  Opus-5-Prüfer läuft (Determinismus, Plausibilität, Server-Neustart, /api/robustness, Job, Wiki) →
  `bench/robustness-battery/pruefbericht.md` mit FREIGABE JA/NEIN. Bis dahin läuft das Dashboard noch mit altem Code.
