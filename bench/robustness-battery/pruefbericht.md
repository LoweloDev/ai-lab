# Prüfbericht — Robustheits-Batterien für alle 8 Suite-Tasks

Host-Prüfung des DeepSeek/OpenCode-Umbaus (Aufträge 01–08, `spec/done-0*.txt`).
Stand: 25.08.2026, 03:30. Geprüft auf dem Host, während zwei echte Polyglot-Läufe
(`oc-qwen36moe-polyglot`, `agy-37flash-low`) und der `llama-server` auf :8080 weiterliefen —
alle drei durchgehend unberührt.

**Urteil: FREIGABE JA.**

---

## 1. Kurzfassung

| Prüfpunkt | Ergebnis |
|---|---|
| Runner deterministisch (2× `--force`) | ✅ inhaltlich byte-identisch, je 24 s |
| 3. Lauf ohne `--force` | ✅ 131–133 ms, `results.json` inkl. mtime unverändert |
| Alle 8 Batterien × 13 Labels | ✅ 104 Paare, Zahlen überall; 2× `buildable:false`, beide mit Grund |
| Batterien reißen auf der Baseline | ✅ alle 8 (A5/A6 korrekt als Build-Fehler) |
| PASS-Abgabe reißt >40 % der Real-Tests | ✅ kein einziger Fall (Maximum 8 %) |
| Workspace-Unversehrtheit (104× `git status`) | ✅ identisch vor/nach; keine `zz_battery`-Reste, keine tmp-Leaks |
| Laufende Benchmarks / Container / GPU | ✅ unberührt |
| `py_compile` (3 geänderte .py) · `node --check` | ✅ |
| `/api/suite` `robust`-Feld · `/api/robustness` | ✅ 121/121 Entries · Scores handgeprüft |
| `renderSuite` gegen echte API ausgeführt | ✅ 21 Render-Prüfungen grün |
| Job `robustheit` ohne GPU-Lock | ✅ lief bei **gesperrter** GPU durch, rc=0, Log da |
| Wiki `robustheit.md` + Index | ✅ |

Gefixt: 2 Befunde (1× mittel, 1× hoch-Darstellung). Kein Blocker gefunden.

---

## 2. Befunde nach Schwere

### Blocker
Keine.

### Hoch — **gefixt**

**H1 · Robustheits-Graph stellte das schlechteste Modell an die Spitze.**
`real_score` zählt nach Konvention nur baubare Tasks. `qwen38-vulkan` hat für A5 und A6
gar keine Abgabe (`undefined: BuildBatches` / `undefined: RankCandidates`) — diese zwei
Tasks fallen aus dem Nenner, und das Label stand mit **100 % (R 60/60) ganz oben**, vor
allen Labels, die über alle 8 Tasks 98,7 % halten. Das `n/8` stand nur klein daneben.
Die Formel ist spec-konform; die **Darstellung** war irreführend.

*Fix (`dashboard/static/app.js`, `static/style.css`, `wiki/robustheit.md`):*
Labels mit `n_missing > 0` bekommen im Balkengraphen einen ⚠-Chip
(„2 Tasks n. baubar", mit Erklärung im Tooltip), einen **gelben statt blauen Balken**
(`.bar-fill.rob.part`), in der Spalte *Robustheit* ein `⚠ 6/8` in Warnfarbe, einen Satz
in der Legende und einen eigenen Wiki-Absatz („Falle: ein hoher Score über wenige Tasks").
Sortierung und Formel blieben unverändert (Spec: „sortiert nach real_score").

### Mittel — **gefixt**

**M1 · `agora-A1-gate`: Path-Test `TestZZBatPathNilContext` maß nichts.**
Der Test wertete ein Panic beim Aufruf mit `nil`-Kontext als Riss. Ihn rissen **alle 13
Abgaben *und* die Baseline** — ein Test, den niemand besteht, trennt nichts. Die Invariante
ist vom Brief auch nicht gedeckt: `tasks/agora-A1-gate/prompt.txt` ist ein reiner
FIFO-Bugfix („Find the bug and fix it"), und die Go-Doku verbietet `nil`-Kontexte
ausdrücklich („Do not pass a nil Context"). Damit verstieß der Test gegen die
Konventionsregel „NUR Invarianten, die der Brief impliziert".

*Fix (`agora-A1-gate/battery_test.go`):* neuer Helfer `zzagCallAllowPanic` — ein Panic ist
hier erlaubt und wird nur zurückgemeldet. Geprüft wird jetzt, was der Brief **wirklich**
impliziert: der Aufruf terminiert (Timeout bleibt ein Riss), das Gate leakt keinen Slot und
ist danach weiter nutzbar. WHY-Kommentar und Änderungsdatum stehen im Test.
Ergebnis: Baseline weiterhin Real 8/10 (die zwei FIFO-Tests reißen unverändert am Bug),
Path jetzt 6/6; alle 13 Abgaben 6/6 statt 5/6. `done-03.txt` ist damit an dieser Stelle veraltet.

**M2 · A5-Zahlen weichen von `a5-results.md` ab — kein Batterie-Fehler.**
`qwen36moe-vulkan` steht dort als „nicht baubar", liefert jetzt Real 6/7 · **Path 3/5**.
Ursache: der Workspace wurde am 25.08. um 02:41 vom Nachzügler-Retry neu befüllt (zweiter
`results.jsonl`-Eintrag). Zusätzlich sind `agy-37flash-low` und `codernext-vulkan` neue
Labels nach dem Snapshot. **Die zwei Path-Risse sind echte Funde**, auf einer Wegwerf-Kopie
reproduziert:
`BuildBatches` kehrt bei `PageSize: 0` nicht zurück (Endlosschleife) und panickt bei
negativer `PageSize` mit `runtime error: makeslice: len out of range`.
*Maßnahme:* Nachtrag am Ende von `a5-results.md` — der Snapshot bleibt als Analyse stehen,
maßgeblich sind `results.json` / Dashboard. A6 stimmt **1:1** mit `a6-results.md` überein
(alle 12 dort gelisteten Labels).

### Niedrig — nur gemeldet

- **N1 · Trennschärfe:** A1, A2, A3 und U1 liefern bei allen 13 Labels 100 % Real. Sie reißen
  zwar auf der Baseline (8/10 · 0/10 · 3/9 · 5/11), messen also etwas — trennen die Abgaben
  aber nicht. Die Unterschiede im Gesamt-Score kommen praktisch nur aus A4, A5, A6 und U2.
  Kein Fehler, aber die ehrliche Lesart der Rangliste.
- **N2 · Totes Binary:** `cmd/battery/battery` (5,3 MB) liegt herum; `run-all.sh` benutzt
  `go run .` und baut selbst. Kann weg.
- **N3 · `run-all.log` wächst unbegrenzt** (append, eine Zeile je Paar; nach den Prüfläufen
  128 KB). Keine Rotation vorgesehen.
- **N4 · `/tmp/gocache-battery` liegt auf tmpfs** und ist auf 2,5 GB gewachsen (16 GB tmpfs,
  13 GB frei). Bei vielen Vollläufen im Blick behalten; ein `rm -rf` kostet nur den nächsten
  Kaltstart.
- **N5 · Kosmetik im Läufe-Assistenten:** für `robustheit` zeigt „Erweitert" noch das Feld
  „Label (optional, für results.jsonl)" mit Platzhalter `-vulkan`. `build_job` ignoriert den
  Wert, es passiert also nichts Falsches.
- **N6 · `.mini-rob`/`.rob-sub` benutzen 10,5 px** statt eines Type-Scale-Tokens — konsistent
  mit dem vorhandenen Code (`.crow .clabel` 13,5 px), daher nur eine Anmerkung. Farben sind
  durchgehend Tokens (`--good`/`--warn`/`--crit`/`--accent`), **keine Literalfarbe** im neuen
  Code, beide Themes definieren alle benutzten Tokens.
- **N7 · Nebenbefund ohne Bezug zum Umbau:** es liefen zwei `python3 server.py`. Auf :8100
  hörte nur PID 779133 (Dashboard, neu gestartet); PID 431403 ist die Testseite unter
  `bench/webapp` und wurde nicht angefasst.

---

## 3. Was im Einzelnen geprüft wurde

### 3.1 Runner (`run-all.sh` / `cmd/battery`)

- `./run-all.sh --force` **dreimal** gefahren (plus der Lauf, den DeepSeek um 03:08 hinterließ):
  alle Läufe nach Abzug von `generated`/`computed`/`seconds` **inhaltlich identisch**.
  Wanduhr je Lauf **24 s** bei `--jobs 2` (Summe der Einzelmessungen 46,8 s), also weit unter
  der 60-s-Marke der Konvention.
- Vierter Lauf **ohne** `--force`: `104 Paare geprueft, 0 neu berechnet` in **131–133 ms**;
  `results.json` blieb byte-identisch, sogar die mtime — der Runner schreibt korrekt gar nicht,
  wenn nichts neu ist.
- **Abdeckung:** 13 Labels × 8 Tasks = 104 Paare, alle mit Zahlen. Genau zwei
  `buildable:false`, beide mit Grund:
  `agora-A5-batcher-scratch/qwen38-vulkan` → `undefined: BuildBatches`,
  `agora-A6-scorer-scratch/qwen38-vulkan` → `undefined: RankCandidates`.
  Kein Paar hat abweichende `real_total`/`path_total` gegenüber `batteries[]`.
- `--task` und `--force` einzeln geprüft (A1-Rerun nach dem Fix: 13 Paare in 1 s).

### 3.2 Plausibilität je Batterie

**Baseline-Gegenprobe** (jede Batterie auf einer `cp -a`-Kopie von `bench/workspaces/<task>`
unter `bench/audit-scratch/robust/_pruef/baseline/`, Skript `baseline-run.sh`):

| Task | Baseline Real | Baseline Path | reißt am Kern? |
|---|---:|---:|---|
| agora-A1-gate | 8/10 | 6/6 | ✅ genau die 2 FIFO-Tests |
| agora-A2-jsonld | 0/10 | 5/5 | ✅ Helfer existiert nicht |
| agora-A3-hls | 3/9 | 5/5 | ✅ 6 webm-abhängige Tests |
| agora-A4-feed | 5/10 | 6/6 | ✅ genau die 5 Mix-Tests |
| agora-A5-batcher-scratch | Build-Fehler | — | ✅ gegradete API fehlt (Scratch-Task) |
| agora-A6-scorer-scratch | Build-Fehler | — | ✅ gegradete API fehlt (Scratch-Task) |
| aiux-U1-paging | 5/11 | 4/4 | ✅ 6 Off-by-one-Tests |
| aiux-U2-denytools | 1/10 | 4/4 | ✅ `denyTools` existiert nicht |

Keine Batterie ist blind: jede reißt dort, wo das Problem des Tasks liegt.

**Gegenprobe „zu implementierungsnah"** (letzter `results.jsonl`-Eintrag je `model`/`task`
gegen `results.json`): **keine offiziell PASS-bewertete Abgabe reißt >40 % der Real-Tests.**
Höchster Wert einer PASS-Abgabe: `codernext-vulkan` auf A6 mit 1/12 = 8 %.
Alle großen Risse liegen ausschließlich auf offiziell FAIL-bewerteten Abgaben und decken
sich mit dem jeweiligen Grader-Befund:

| Abgabe | offizielles Urteil | Real | Riss-Quote |
|---|---|---:|---:|
| `codernext-vulkan` / aiux-U2-denytools | FAIL tests-failing (fail=3) | 2/10 | 80 % |
| `codernext-vulkan` / agora-A4-feed | FAIL tests-red | 5/10 | 50 % |
| `qwen36moe-vulkan` / agora-A6 | FAIL tests-red | 8/12 | 33 % |
| `muse-vulkan` / agora-A6 | FAIL tests-red | 9/12 | 25 % |

**A5/A6 gegen die Berichte:** A6 stimmt für alle 12 dort gelisteten Labels **exakt**
(inkl. `codernext` 11/12, `muse` 9/12, `qwen36moe` 8/12, `qwen38` nicht baubar).
A5: drei Abweichungen, alle außerhalb der Batterie begründet → siehe **M2** und den
Nachtrag in `a5-results.md`. Die Testdateien in `a5/` und `a6/` sind mit denen in
`agora-A5-batcher-scratch/` bzw. `agora-A6-scorer-scratch/` **byte-identisch**.

### 3.3 Unversehrtheit

- `git -C bench/runs/<label>/<task>/ws status --short` für **alle 104 Workspaces**
  vor dem ersten und nach dem letzten Lauf (6 Vollläufe + 2 Dashboard-Jobs dazwischen)
  aufgezeichnet: **Diff leer**. Momentaufnahmen unter
  `bench/audit-scratch/robust/_pruef/git-{before,after,after2,final}.txt`.
- Keine `zz_battery*`-Datei unter `bench/runs/` oder `bench/workspaces/`.
- Keine `/tmp/battery-ws-*`-Reste, keine `results.json.tmp`.
- Unter `bench/tasks` und `bench/workspaces` wurde seit Prüfbeginn **keine Datei** verändert.
  Unter `bench/runs` nur `.git/index`-Dateien — das ist der Stat-Cache, den meine eigenen
  `git status`-Aufrufe auffrischen; die Inhalte und die Statusausgabe sind unverändert.
- `podman ps`, `pgrep run-polyglot/oc-lokal`, `llama-server` (PID 2720932, Vulkan0) durchgehend
  am Leben; die Container arbeiteten während der Prüfung normal weiter.

### 3.4 Dashboard

- `python3 -m py_compile labdata.py labjobs.py server.py labcore.py labregistry.py` → OK.
  `node --check static/app.js` → OK (auch nach meinem Fix).
- `/api/jobs` vor dem Neustart: `jobs: []`, `active: false` — kein Job unterbrochen.
  Alter Prozess (PID 779133) beendet, Neustart via
  `cd dashboard && nohup ./start.sh > logs/dashboard.log 2>&1 &`, :8100 nach **2 s** oben.
- `/api/suite`: **121 von 121** Entries tragen ein `robust`-Feld (alle 8 Tasks vollständig).
- `/api/robustness`: `batteries` (8), `results`, `scores` (13 Labels), `stale: []`.
  **Drei Labels von Hand nachgerechnet**, jeweils exakt getroffen:
  `codernext-vulkan` 63/79 = 79,75 % (n 8/8) · `qwen36moe-vulkan` 74/79 = 93,67 % (n 8/8) ·
  `qwen38-vulkan` 60/60 = 100 % (n 6/8) — inkl. `path_pass/path_total`, `n_tasks`, `n_missing`.
- **Staleness end-to-end geprüft:** ein gespeicherter Fingerprint künstlich verfälscht →
  `/api/robustness` meldet `[["agora-A4-feed","muse-vulkan"]]`, `/api/suite` setzt
  `robust.stale`, die Zelle rendert `⟳ R 10/10 · P 6/6`, die Spalte ein `⟳`, das Detail
  „veraltet". Danach zurückgesichert, `stale` wieder leer. Die Python-Fingerprint-Regel
  reproduziert alle 104 vom Go-Runner gespeicherten Hashes.
- Kosten: `read_suite` kalt 0,21 s (inkl. Fingerprints über alle Workspaces), warm 0,000 s
  (30-s-Cache) — die Suite-Ansicht wird davon nicht langsam.
- **`renderSuite` wirklich ausgeführt**, nicht nur gelesen: `bench/audit-scratch/robust/_pruef/render-test.mjs`
  lädt `app.js` in einem `node:vm`-Kontext mit DOM-Stubs und rendert gegen die **echten**
  Antworten von `/api/suite`, `/api/robustness`, `/api/meta`, `/api/jobs`. **21 Prüfungen grün**:
  Mini-Zeile `R x/y · P x/y` in 104 Zellen · „n. baubar" · Spalte *Robustheit* als **letzte**
  Zelle jeder Zeile (Prozent + Mini-Balken + `n/8`) · Balkengraph mit 13 Zeilen, absteigend nach
  `real_score` · Pill = Path-Quote · Klick klappt alle 8 Tasks auf · Zellen-Detail listet die
  gerissenen Tests namentlich · Wiki-Link `wiki:robustheit.md` · Legende Real/Path ·
  Gesamtzeit-Graph unverändert vorhanden · kein `undefined`/`NaN`/`[object Object]` im HTML ·
  kein Absturz, wenn `/api/robustness` ausfällt (`rob = null`) · Läufe-Tab: Eintrag
  „Robustheit neu berechnen", **☰ CPU**-Chip, Schritt 2 ohne Modell, `--force`-Schalter,
  Start-Knopf aktiv, `CPU 0/1` in der Slots-Leiste.
- **Job `robustheit` über die API gestartet** (`POST /api/jobs {"action":"robustheit"}`) —
  und zwar **während die GPU gesperrt war** („2 Benchmark-Container aktiv"):
  Klasse `cpu`, rc=0, Log `dashboard/runs/<id>.log` mit der Runner-Ausgabe. Zweiter Lauf mit
  `{"force":true}`: rc=0 nach 24,6 s, `104 Paare, 104 neu berechnet`. Ein parallel
  abgeschickter dritter Job wurde korrekt abgelehnt („CPU-Slot belegt"). **Kein GPU-Lock**
  auf dem Weg — `_guard_gpu` wird für die CPU-Klasse nicht angefasst.
- Theme: keine Literalfarbe im neuen JS/CSS; alle benutzten Tokens sind in `:root`,
  im `prefers-color-scheme: dark`-Block und in `[data-theme="dark"]` definiert.
  Statische Dateien werden mit `Cache-Control: no-store` ausgeliefert — kein Cache-Buster nötig.

### 3.5 Wiki

`dashboard/wiki/robustheit.md` ist vorhanden, verständlich (was/warum Zusatz-Metrik, die zwei
Stufen, `real_score`-Formel, wo die Zahlen stehen, wie neu rechnen, Grenzen, Verweise auf
`a5-results.md`/`a6-results.md`/`audit-report.md`) und in `wiki/wiki.json` unter **Konzepte**
registriert — genau wie die anderen Artikel. Die Doc-ID `wiki:robustheit.md` aus dem
Graph-Link entspricht dem Format, das `labdata.read_wiki()` erzeugt. Ich habe den Abschnitt
„Falle: ein hoher Score über wenige Tasks" ergänzt (siehe **H1**).

---

## 4. Robustheits-Tabelle je Label

Aus `/api/robustness` nach den Fixes. `real_score = Σ real_pass / Σ real_total` über alle
Tasks mit `buildable:true`. **Zusatz-Metrik — ändert keine PASS/FAIL-Urteile.**

| # | Label | real_score | R (Σpass/Σtotal) | P | n/8 |
|---:|---|---:|---:|---:|---:|
| 1 | `qwen38-vulkan` | 100,0 % | 60/60 | 30/30 | **6/8 ⚠** |
| 2 | `agy-37flash` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `agy-37flash-low` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `cc-opus48` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `cc-opus5` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `dsh-v4-flash` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `dsh-v4-pro` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `oc-gemini37f` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `oc-v4-flash` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 2 | `oc-v4-pro` | 98,7 % | 78/79 | 44/44 | 8/8 |
| 11 | `muse-vulkan` | 94,9 % | 75/79 | 44/44 | 8/8 |
| 12 | `qwen36moe-vulkan` | 93,7 % | 74/79 | 42/44 | 8/8 |
| 13 | `codernext-vulkan` | 79,7 % | 63/79 | 44/44 | 8/8 |

⚠ `qwen38-vulkan` hat für A5 und A6 **gar keine Abgabe** — die beiden Tasks fallen aus dem
Nenner. Der Wert ist mit den `8/8`-Labels **nicht vergleichbar**; die absoluten Zahlen
`R 60/60` gegen `R 78/79` sind hier die ehrlichere Auskunft. Das Dashboard markiert das jetzt
mit ⚠ und gelbem Balken (siehe **H1**).

Die neun 98,7-%-Labels reißen alle **denselben einen** Test:
`agora-A5-batcher-scratch/TestZZBatRealScoreTiesMobileLeadsWithTopScore` (keine Lösung sortiert
selbst nach Score) — dieselbe Lochklasse, aus der A5 offiziell für alle FAIL ist.

---

## 5. Geänderte Dateien (nur innerhalb der Schreibrechte)

| Datei | Änderung |
|---|---|
| `bench/robustness-battery/agora-A1-gate/battery_test.go` | **M1**: `zzagCallAllowPanic` + entschärfter `TestZZBatPathNilContext` |
| `bench/robustness-battery/a5-results.md` | **M2**: Nachtrag 25.08. (drei abweichende Zeilen erklärt, zwei echte Path-Funde) |
| `bench/robustness-battery/results.json` | vom Runner neu geschrieben (A1-Path jetzt 6/6) |
| `bench/robustness-battery/pruefbericht.md` | dieses Dokument |
| `dashboard/static/app.js` | **H1**: ⚠-Chip + Warnfarbe + Tooltips für Teil-Scores, Legendensatz |
| `dashboard/static/style.css` | **H1**: `.bar-fill.rob.part`, `td.cell.robcol.rob-part .rob-sub` |
| `dashboard/wiki/robustheit.md` | **H1**: Abschnitt „Falle: ein hoher Score über wenige Tasks" |
| `bench/audit-scratch/robust/_pruef/` | Prüf-Artefakte (Baseline-Läufe, git-Snapshots, `render-test.mjs`, `peek.mjs`, `stale-check.mjs`) |

**Nicht angefasst:** `bench/tasks`, `bench/runs`, `bench/workspaces`, `results.jsonl`,
`run-task*.sh`, `dashboard/labdata.py`, `labjobs.py`, `server.py`, `wiki/wiki.json`.

---

## 6. Urteil

**FREIGABE JA.**

Der Runner ist deterministisch, schnell (24 s voll, 0,13 s inkrementell) und lässt die
Abgaben nachweislich unangetastet. Jede der acht Batterien reißt auf der Baseline dort, wo
das Problem des Tasks liegt, und keine ist so implementierungsnah, dass sie eine bestandene
Abgabe zerlegt. Das Dashboard zeigt die Zahlen an allen drei vorgesehenen Stellen, der
Neuberechnungs-Job läuft ohne GPU-Lock neben den echten Benchmarks, und das Wiki erklärt,
was die Zahl bedeutet und was nicht.

Die zwei Dinge, die Tobias morgen hätten stolpern lassen — ein Path-Test, den niemand
bestehen kann, und ein Balkengraph, der das schlechteste Modell an die Spitze setzt —
sind gefixt. Was offen bleibt (N1–N7), ist Aufräumarbeit ohne Auswirkung auf die Zahlen.

**Eine Sache im Kopf behalten:** Die Rangliste trennt derzeit fast nur über A4, A5, A6 und U2.
Wer die Metrik schärfer haben will, erweitert dort — nicht bei A1/A2/A3/U1, wo alle 100 % haben.
