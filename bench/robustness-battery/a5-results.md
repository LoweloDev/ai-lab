# Robustheits-Batterie A5 — agora-A5-batcher-scratch

Stand: 2026-08-24. **Zusatz-Metrik** — ändert keine PASS/FAIL-Urteile.
(Alle 13 A5-Einträge in `results.jsonl` stehen ohnehin bereits auf `FAIL tests-red`.)

## Aufbau

- Batterie: `robustness-battery/a5/battery_test.go` (package feed, 12 Tests) — nur Invarianten,
  die der Brief in `tasks/agora-A5-batcher-scratch/prompt.txt` impliziert, nach dem Vorbild der
  Property-Struktur in `grade_test.go` (WHY-Kommentare an jedem Test).
- Runner: `robustness-battery/a5/run-battery.sh` — kopiert die Datei je Workspace als
  `agora-backend/internal/feed/zz_battery_test.go`, läuft `go test ./internal/feed/ -run ZZBat
  -count=1 -json` (Shell-Timeout 120 s, `-timeout 110s`), entfernt die Datei wieder.
- Rohdaten je Label: `robustness-battery/a5/raw/` (`.json`, `.stderr`, `.rc`,
  `.pre-status`/`.post-status`/`.status-diff`, `.basebuild`).
- Selbst-Validierung vor dem Lauf (audit-scratch): grün auf einer Referenz-Implementierung;
  Mutationstests bestätigt, dass der Panic-Fang (PageSize-0-Absturz) und der 5-s-Timeout
  (echte Endlosschleife) greifen und der Testlauf sauber weiterläuft.

**Workspace-Unversehrtheit:** Für alle 11 Workspaces ist `git status --short` vor und nach dem
Lauf byte-identisch (leere `raw/<label>.status-diff`, Runner-Log: „workspace-clean: JA").

## Die zwei Stufen

**ZZBatReal\*** (7) — realistische Kanten, voller Brief-Vertrag (nichts verloren, nichts doppelt,
keine leere Seite, Seitengröße respektiert, Handy = 1 Beitrag/Seite):
Score-Gleichstände Desktop · Gleichstände Mobile (Spitzen-Score zuerst, über 4 deterministische
Eingabe-Anordnungen) · leerer Feed (nil/leer × mobile/desktop) · Ein-Themen-Pool ·
reiner Live-Pool (desktop + mobile) · 200er-Pool (10 Themen, Live-Anteil, Gleichstände) ·
PageSize-Grenzen (1, =Poolgröße, 50).

**ZZBatPath\*** (5) — pathologische Kanten; bestanden heißt nur: kein Panic, kein Verlust,
keine Endlosschleife (jeder Aufruf mit eigenem 5-s-Timeout), keine fachliche Deutung erzwungen:
NaN-Scores · negative/±Inf-Scores · Duplikat-IDs (Dedupe ausdrücklich erlaubt) ·
PageSize 0 (desktop + mobile) · PageSize negativ.

## Ergebnis

| Label | Real x/y | Path x/y | Bemerkung |
|---|---|---|---|
| agy-37flash | 6/7 | 5/5 | übernimmt Eingabe-Reihenfolge (kein Sortieren); reißt Ties-Mobile bei umgekehrter Anordnung (Seite 1: Score 60 statt 90) |
| cc-opus48 | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 bei Anordnung [3 2 1 0] |
| cc-opus5 | 6/7 | 5/5 | dito; Gegencheck: reißt aus demselben Grund auch die aktuelle Grade-Property (c) schon bei Identitäts-Anordnung |
| dsh-v4-flash | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| dsh-v4-pro | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| muse-vulkan | 6/7 | 5/5 | dito, mit eigener Färbung: Live-Kadenz-Heuristik zieht bei umgekehrter Anordnung das Live-Item vor (Seite 1: Score 70 statt 90) |
| oc-gemini37f | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| oc-v4-flash | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| oc-v4-pro | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| qwen36moe-vulkan | nicht baubar | nicht baubar | gegradete API fehlt: `undefined: BuildBatches` (kein Beitrag im feed-Paket) |
| qwen38-vulkan | nicht baubar | nicht baubar | gegradete API fehlt: `undefined: BuildBatches` (nur go.mod angefasst) |

### Gerissene Tests je Lösung

Alle 9 baubaren Lösungen reißen exakt einen Test, denselben:

- `TestZZBatRealScoreTiesMobileLeadsWithTopScore` — Pool mit Score-Gleichstand an der Spitze
  (90, 90, 70-live, 60), geprüft über 4 Eingabe-Anordnungen. Bei aufsteigender (= umgekehrter)
  Anordnung `[3 2 1 0]` liefert jede Lösung als erste Handy-Seite das *erste Eingabe-Item*
  (Score 60; muse-vulkan: das Live-Item, Score 70) statt eines Spitzen-Score-Items.

Alle übrigen 11 Tests bestehen alle 9 Lösungen — insbesondere die komplette pathologische
Stufe: NaN, ±Inf, negative Scores, Duplikat-IDs, PageSize 0 und negativ lösen nirgends Panic,
Verlust oder Endlosschleife aus.

## Einordnung

1. **Eine einzige, systematische Schwäche, überall dieselbe:** Keine der 9 Lösungen ordnet die
   Rangliste selbst nach Score (kein einziger `sort.`-Aufruf in irgendeinem `batch*.go`); alle
   verlassen sich darauf, dass die Eingabe bereits absteigend sortiert ankommt. Der Brief nennt
   die Liste „fertig bewertet, sortierbar" — die Benchmark-Lesart (Grade-Property (c) mit
   gradePermute, gefixt nach dem muse-Präzedenzfall) verlangt Ordnungs-Unabhängigkeit. Die
   Batterie bestätigt diese Lochklasse unabhängig und findet sie flächendeckend; sie ist
   derselbe Grund, aus dem die Lösungen unter dem gefixten Grader bereits durchfallen —
   **kein neues, zusätzliches Loch**.
2. **Robustheit gegen kaputte Eingaben ist durch die Bank gut:** 9/9 baubare Lösungen überstehen
   sämtliche pathologischen Kanten unauffällig — auch die klassische PageSize-0-Falle.
3. Die realistischen Kanten jenseits der Ordnung (leerer Feed, Ein-Themen-Pool, reiner
   Live-Pool, 200er-Pool, PageSize-Grenzen) hält ebenfalls jede baubare Lösung ein.
4. Kein Modell hat einen `TestMain`-/`os.Exit`-Hijack in seinen Testdateien (vorab geprüft);
   die Batterie lief also überall wirklich.
