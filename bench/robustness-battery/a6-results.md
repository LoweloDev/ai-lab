# Robustheits-Batterie A6 (agora-A6-scorer-scratch) — Ergebnisse v2

Stand: 2026-08-24, 23:20–23:35 CEST. **Zusatz-Metrik** — ändert keine PASS/FAIL-Urteile des Graders.

- Batterie: `robustness-battery/a6/battery_test.go` (package feed, **21 Tests**: 12 ZZBatReal\*, 9 ZZBatPath\*)
- Runner: `robustness-battery/a6/run-battery.sh` (optional Label-Glob als `$1`)
- Auswertung: `robustness-battery/a6/summarize.py` (`--result <label>` / `--json` / `--markdown`)
- Rohdaten je Label: `robustness-battery/a6/raw/` (`.json` = `go test -json`, `.stderr`, `.rc`, `.basebuild`,
  `.pre-status`/`.post-status`/`.status-diff`, `.wsclean`), lesbare Logs: `robustness-battery/a6/logs/<label>.log`
- Vorgänger (13 Tests, 22:36, ohne cc-opus48-Nachlauf und ohne codernext-vulkan) archiviert unter `robustness-battery/a6/v1/`.

## Methode

Je Workspace: `git status --short` vorher → Baseline-Kompilat **ohne** Batterie (`-run ZZZZNOMATCH`, trennt
„Modellcode nicht baubar“ von „gegradete API fehlt“) → `battery_test.go` als
`agora-backend/internal/feed/zz_battery_test.go` einkopieren → `go test ./internal/feed/ -run ZZBat -count=1 -timeout 110s -json`
(Shell-Timeout 120 s, `GOPROXY=off`, Default `-mod=readonly`, also kein Schreiben an go.mod/go.sum) → Datei entfernen →
`git status --short` nachher, Byte-Vergleich. Zusätzlich ein **Watchdog je Aufruf** (4 s, 15 s für den 3000er-Pool):
Panic und Hänger reißen nur den einen Test, nicht das Binary. Vorab prüft der Runner per `pgrep`, ob ein laufender
Benchmark den Workspace gerade benutzt, und überspringt ihn dann (kam in diesem Lauf nicht vor).

**Nur Brief-Invarianten** (prompt.txt), nie Implementierungsform; Vorbild ist die Property-Struktur der
Grade-Datei (WHY-Kommentar je Test, ceteris-paribus-Pools, erwarteter Verlierer zuerst in der Eingabe). Die Batterie
stützt sich auf zwei Brief-Sätze, die v1 noch nicht ausgeschöpft hatte:

1. *„Die Seiten-Zusammenstellung danach gibt es schon; sie erwartet eine fertig bewertete, sortierte Liste.“* Dieser
   Abnehmer existiert im Paket (`BuildBatches` in `batcher.go`, in keinem Workspace angefasst) und **sortiert die Liste
   vor dem Paginieren nach `RankedItem.Score` neu**. Folgen: die Rangfolge muss im Score stecken (nicht-steigend, nie NaN),
   sonst erreicht sie den Nutzer nie; und PageSize gibt es *nur* dort (`BatchOptions`) — die PageSize-Kanten werden
   deshalb Ende-zu-Ende geprüft (RankCandidates → BuildBatches), statt sie wie in v1 als „nicht ausdrückbar“ zu streichen.
2. *„Gleiche Eingaben müssen dieselbe Reihenfolge ergeben — sonst springt der Feed beim Nachladen.“* Beim Nachladen kommt
   derselbe Pool, aber nichts garantiert dieselbe Zeilenreihenfolge aus dem Store (der Brief kennt kein solches Signal).
   Die **strengste Lesart** — Reihenfolge unabhängig von der Eingabe-Anordnung — steht in einem eigenen Test
   (`TiesInputOrderIndependent`), damit sie getrennt zurechenbar bleibt.

### ZZBatReal\* (12) — realistische Kanten, voller Brief-Vertrag

| Test | prüft |
|---|---|
| EmptyPool | nil/leerer Pool × 5 Modi × Profil leer/reich → leer, kein Panic |
| SingleCandidate | ein Kandidat in jedem Modus: erhalten, Typ intakt, Score nicht NaN |
| ScoreTies | 8 perfekte Zwillinge: Erhalt, Determinismus über zwei Aufrufe, und derselbe gecachte Slice zweimal (Seite 1/Seite 2) |
| TiesInputOrderIndependent | 6 Zwillinge + 4 distinkte Items aus 4 Eingabe-Anordnungen → eine Reihenfolge (strengste Lesart, s. o.) |
| SingleTopicPool | 12 Items, ein Topic (Topic-Signal hebt sich auf): Erhalt, Determinismus, „neu“ chronologisch |
| PureLivePool | 10 Live-Räume, Zwilling idle/aktiv (idle zuerst): Live-Schubs mit und ohne Profil, Erhalt, Determinismus |
| AllSuggestionsPool | 10 KI-Vorschläge ohne menschliche Debatten: Eindämmung darf nicht Löschung werden, Erhalt, Determinismus |
| SuggestionsContainedAtScale | 5 perfekte Vorschläge vs. 5 gesunde Debatten gleicher Relevanz in 30er-Pool: kein Vorschlag über einer Debatte |
| LargePool200 | 200 gemischte Items, 5 Modi: exakte Permutation, Determinismus, „neu“ chronologisch (Nicht-Vorschläge) |
| ModeNewTiedTimestamps | 40 Items in 10 Zeitstempel-Gruppen, Personalisierungszug auf die ältesten: „neu“ monoton, Erhalt, Determinismus |
| ScoreCarriesOrder | 40er-Pool, 5 Modi, Profil leer/reich: Score nicht-steigend und nie NaN (Batcher-Vertrag) |
| PageSizeHandOff | Rank → BuildBatches mit PageSize 1/2/3/=Pool/50: nichts verloren/doppelt über Seiten, keine leere Seite; erste Handy-Seite = Spitzenitem |

Für „hot“/„live“ (im Paket, nicht im Brief) gelten nur die schwachen Invarianten (kein Panic, Erhalt, Determinismus,
Score trägt die Ordnung). In gemischten Pools wird die Chronologie von „neu“ nur über Nicht-Vorschläge geprüft, weil
„schlicht chronologisch“ und „Würze, kein Hauptgericht“ dort kollidieren können; Erhalt bleibt Pflicht.

### ZZBatPath\* (9) — pathologische Kanten; bestanden heißt nur: kein Panic, kein Verlust, Terminierung

NegativeSignals (negative Counts/Scores/Affinitäten/Confidence, Seeds −1/MinInt64/MaxInt64) · NaNAndInfSignals
(NaN/±Inf in Qualität, Vektoren, Balance, Profil) · ExtremeMagnitudes (MaxInt/MinInt-Zähler, ±1e308, Vektor mit
Norm-Überlauf, Jahr 1 und 9999, `Now` = Epoche/9999) · DuplicateIDs (Dedupe ausdrücklich erlaubt — Verlust wird auf
**distinkten** IDs gemessen) · ZeroAndFutureTimes (Nullzeiten, Zukunft, UpdatedAt < CreatedAt, `Now` = 0) ·
VectorShapeMismatch (nil/leer/1-dim/kurz/lang/Nullvektor, beidseitig) · UnknownModeAndDegenerateFields (unbekannter/leerer
Modus und Typ, Ads mit/ohne Promotion, Leerstrings, Nil-UUIDs, `RankOptions{}`) · PageSizeZeroNegative (Rank →
BuildBatches mit 0/−1/−1000, Desktop+Mobile, auch mit NaN-Eingabe) · HugePoolAndCardinalities (3000 Items; 500
Slugs je Item, 2000 Affinitäten, 500 Follows). PageSizeZeroNegative kann Lösungen nur trennen, wenn deren Ausgabe das
Paginieren sprengt — der Batcher ist Baseline-Code; der Test steht für die Vollständigkeit der PageSize-Kante.

### Selbst-Validierung (audit-scratch/a6-bat-selftest, Referenz-Scorer + 6 Mutanten)

Referenz 21/21 grün. Jede Mutante reißt genau die erwartete Klasse, die Suite läuft weiter:
Panic bei leerem Pool → nur EmptyPool · Endlosschleife bei NaN → NaNAndInfSignals + PageSizeZeroNegative
(je 4 s Watchdog, Gesamtlauf 8 s) · Vorschläge verwerfen → alle Erhalt-Tests mit Vorschlägen · „neu“ chronologisch
sortiert, Score aber nicht mitgezogen → ScoreCarriesOrder + PageSizeHandOff (gekoppelt, absichtlich: Ursache und
Produktwirkung) · Stable-Sort ohne ID-Tiebreak → nur TiesInputOrderIndependent · NaN-Score überall → SingleCandidate +
ScoreCarriesOrder. Kein Modell hat `TestMain`/`os.Exit`/`init()` in eigenen Testdateien (vorab geprüft) — die Batterie
lief überall wirklich.

## Tabelle

Offizielles Urteil = letzter Eintrag je Label in `results.jsonl`.

| Label | Real x/y | Path x/y | Bemerkung |
|---|---|---|---|
| agy-37flash | 12/12 | 9/9 | offiziell PASS; keine Risse |
| cc-opus48 | 12/12 | 9/9 | offiziell PASS; keine Risse (v1 hatte den Workspace noch nicht — Lauf war damals aktiv) |
| cc-opus5 | 12/12 | 9/9 | offiziell PASS; keine Risse |
| codernext-vulkan | 11/12 | 9/9 | offiziell PASS; ein Riss, nur strengste Lesart: Zeitstempel-Ties in „neu“ folgen der Eingabereihenfolge (kein Tiebreak in der Chronologie; „für dich“/„top“ haben ID-Tiebreak) |
| dsh-v4-flash | 12/12 | 9/9 | offiziell PASS; keine Risse |
| dsh-v4-pro | 12/12 | 9/9 | offiziell PASS; keine Risse |
| muse-vulkan | 9/12 | 9/9 | offiziell FAIL tests-red; „neu“ ist gewichtete Mischung (Frische 0,45 + Personalisierung), nicht chronologisch — 3 Risse, dasselbe Muster wie Grade-Property f1 |
| oc-gemini37f | 12/12 | 9/9 | offiziell PASS; keine Risse |
| oc-v4-flash | 12/12 | 9/9 | offiziell PASS (nach Regrade); keine Risse |
| oc-v4-pro | 12/12 | 9/9 | offiziell PASS (nach Regrade); keine Risse |
| qwen36moe-vulkan | 8/12 | 9/9 | offiziell FAIL tests-red; „neu“ sortiert nach Score, CreatedAt nur als Tiebreak (3 Chronologie-Risse) + kein ID-Tiebreak (Ties folgen der Eingabereihenfolge) |
| qwen38-vulkan | nicht baubar | — | keine Abgabe: Paket baut ohne Batterie (base_rc=0), mit Batterie `undefined: RankCandidates`; Nachzügler-Retry mit 64k Kontext steht laut `nachzuegler-a5a6-retry.sh` noch aus |

## Gerissene Tests je Lösung

### codernext-vulkan (Real 11/12)
- `TestZZBatRealTiesInputOrderIndependent` — Modus „neu“, rotierte Anordnung: Position 1 ist `…0001` statt `…0006`.
  Ursache (ranker.go:25): `ModeNew` sortiert stabil nur nach `CreatedAt`; sechs Zwillinge mit gleichem Zeitstempel
  behalten ihre Eingabereihenfolge. Score wird danach aus der Position abgeleitet (`len−i`), trägt die Ordnung also
  korrekt — nur die Ordnung selbst hängt an der Zeilenreihenfolge des Stores. „für dich“ und „top“ brechen Ties per ID.

### muse-vulkan (Real 9/12)
- `TestZZBatRealSingleTopicPool` — „neu“: Position 4 (14.07. 07:00) neuer als Position 3 (13.07. 11:00)
- `TestZZBatRealLargePool200` — „neu“ im 200er-Pool: Position 2 neuer als Position 1 (Nicht-Vorschläge)
- `TestZZBatRealModeNewTiedTimestamps` — „neu“ bei Zeitstempel-Ties: Position 8 (12:00) neuer als Position 7 (03:00)

Ursache (ranking.go, Gewichtstabelle `ModeNew`: freshness 0,45 / quality 0,20 / activity 0,10 / semantic 0,10 plus
Personalisierung): „neu“ ist ein Score-Mix, keine Chronologie. Identisch mit dem offiziellen FAIL (Property f1).

### qwen36moe-vulkan (Real 8/12)
- `TestZZBatRealTiesInputOrderIndependent` — „für dich“, umgekehrte Anordnung: Position 2 ist `…0001` statt `…0006`
- `TestZZBatRealSingleTopicPool` — „neu“: Position 3 (13.07. 16:00) neuer als Position 2 (13.07. 06:00)
- `TestZZBatRealLargePool200` — „neu“: Position 3 neuer als Position 2 (Nicht-Vorschläge)
- `TestZZBatRealModeNewTiedTimestamps` — „neu“ bei Ties: Position 8 (04:00) neuer als Position 7 (03:00)

Ursache (ranking.go:235ff): auch in `ModeNew` primär nach Score, `CreatedAt` nur bei Score-Gleichstand (ε 1e−9); alle
Comparatoren enden auf ActivityCount/CreatedAt ohne ID — perfekte Zwillinge bleiben in Eingabereihenfolge.

### qwen38-vulkan
Nicht baubar: `internal/feed/zz_battery_test.go:175:12: undefined: RankCandidates` — es gibt keine Abgabe
(results.jsonl: `"changed":""`). Baseline-Kompilat ohne Batterie ist grün, d. h. es fehlt die gegradete API, nicht die
Baubarkeit des Pakets.

## Integrität der Workspaces

Alle 12 Läufe: `raw/<label>.wsclean` = `clean`, `raw/<label>.status-diff` = 0 Byte, kein `zz_battery_test.go` und kein
`zz_bench_grade_test.go` unter `runs/` zurückgeblieben (`find` nach dem Lauf leer). Kein Workspace war während des Laufs
in Benutzung (der `pgrep`-Wächter hat nichts übersprungen; die laufenden Benchmarks sind Polyglot/aider-Container auf
anderen Verzeichnissen, llama-server und GPU blieben unangetastet).

## Gesamtbild

- **Pathologische Stufe: 11/11 baubare Lösungen 9/9.** Kein Panic, kein Verlust, kein Hänger — auch nicht bei
  MaxInt-Zählern, ±1e308, Norm-Überlauf, Jahr 1/9999, negativen Seeds, Nil-UUIDs, unbekannten Typen, 3000er-Pool oder
  500 Slugs × 2000 Affinitäten. Die Eingabe-Hygiene ist durch die Bank gut.
- **Score trägt die Ordnung überall** (ScoreCarriesOrder + PageSizeHandOff 11/11): keine Lösung liefert eine Rangfolge,
  die der vorhandene Batcher beim Neusortieren zerstören würde. Diese Invariante war die wichtigste Neuerung gegenüber
  v1 und hätte eine ganze Fehlerklasse (chronologisch sortiert, Score liegen gelassen) sichtbar gemacht — sie kommt
  in keiner Abgabe vor.
- **Die einzigen Risse liegen in der realistischen Stufe:** (a) „neu“ nicht chronologisch bei den beiden offiziell
  durchgefallenen lokalen Lösungen muse und qwen36moe — die Batterie widerspricht keinem Urteil, sie schärft es;
  (b) Eingabe-Anordnungs-Abhängigkeit bei Ties (codernext nur in „neu“, qwen36moe in allen Modi) — strengste Lesart des
  Determinismus-Satzes, bewusst als eigener Test isoliert; die neun übrigen Lösungen brechen Ties per ID und bestehen sie.
- Neu gegenüber v1 vollständig: cc-opus48 (12/12, 9/9) und codernext-vulkan (11/12, 9/9).
