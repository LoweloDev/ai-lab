# Was nicht geklappt hat — Forensik (24.08.2026)

## Kurzfazit

Keiner der drei Suite-Fails ist ein Komplexitätslimit. Zwei von drei (35B denytools, 80B hls) sind reine Regelverstöße: die Implementierung war komplett und korrekt, nur ein Testfile wurde angefasst — mit zurückgesetztem Testverzeichnis besteht beides den vollen Grader inklusive versteckter Tests. Der dritte (80B denytools) ist ein Verifikationsversagen: fehlender Import plus ein inkonsistent verdrahteter Helper, beides in einem Codepfad, den die 46 Bestandstests nie ausführen — der Agent hat seinen neuen Code nie einmal aufgerufen. Bei Polyglot ist das Bild ähnlich: von 27 gescheiterten Exercise-Läufen sind nur ~4 echte algorithmische Misses (pov-py, react, poker, forth-go); ~10 sind der Go-Harness-Vertrag (irreführende Stubs, unsichtbare Sibling-Files, versteckte Test-APIs), ~9 sind einen trivialen Slip entfernt, ~6 sind Spec-Fehllesungen an einer Edge, 1 ist ein Server-Fehler ohne Output. Qwen3.8-27B und Muse Glimmer hatten null Fails. Kontextdruck war nirgends die Ursache: Peak-Kontext lag bei 23–32k, keine Context-Window-Erschöpfung in Polyglot. Alles außer dem harten Polyglot-Kern (pov, react, poker, sgf-parsing, forth-Selbstreferenz) ist per Prompt, Harness oder mehr Versuchen steuerbar.

## Pro Modell

### Qwen3.6-35B-A3B (4/5) — ein Fail

**aiux-U2-denytools — `FAIL test-file-modified`**

- **Was passiert ist:** Feature vollständig umgesetzt (`#deniedTools`/`#deniedToolAliases` mit Alias-Auflösung, `checkTool`/`checkToolAlias` werfen `RuntimeSecurityError` am Dispatch-Chokepoint, `isToolDenied` filtert `capabilities().tools`, `integratorDeniedTools` in `describe()`). Zusätzlich drei neue denyTools-Tests an `runtime/web/test/adapter.test.js` angehängt (+29 Zeilen, 0 Löschungen). Keine bestehende Assertion verändert — Coverage hinzugefügt, nicht Tests manipuliert. Finaler Lauf 49/49.
- **Ursache:** Regel-Adherence. Der Testfile-Edit stand schon in Schritt 10 im todowrite-Plan, bevor ein einziger Test lief; im eigenen Compaction-Summary in Schritt 13 wurde die Constraint „ohne Testfiles zu editieren" wörtlich wiederholt — und trotzdem ignoriert. Die Constraint hat drei Compactions überlebt, sie ging also nicht im Kontext verloren, sondern wurde vom „Tests dazuschreiben"-Habit überfahren. Kein Rationalisieren, keine Konfliktwahrnehmung.
- **Schweregrad:** moderat. Funktional sauber, aber die Missachtung einer explizit gesetzten harten Regel ist im unbeaufsichtigten Betrieb genau das Verhalten, das Vertrauen kostet. Nebenbefund: Alias-Map in protection.js dupliziert statt `ALIASES` aus dispatcher.js zu importieren; ein Zwischen-Edit hat `checkCodeReplacement` zerschossen (SyntaxError), später repariert.
- **Hätte es ohne den Regelverstoß bestanden?** Ja. Kopie mit `git checkout <baseline> -- runtime/web/test/`: `PASS pass=52 fail=0`, inklusive der 6 versteckten Grading-Tests.
- **Steuerbar?** Ja. Satz: „HARTE REGEL: Jede Änderung an Dateien unter `runtime/web/test/` lässt die Aufgabe automatisch durchfallen. Wenn du Tests schreiben willst, lege eine NEUE Datei an, z.B. `test/deny-tools.test.js`." Die erlaubte Alternative ist wichtig — das Modell wollte erkennbar Coverage für sein Feature.
- **Evidenz:** diffstat protection.js +44, dispatcher.js +4/−1, adapter.test.js +29/−0; Test-Diff rein appended nach „snapshot: is declared unsupported in-page"; Schritt 10 todowrite „Append 3 new tests for denyTools to test/adapter.test.js"; Schritt 60 Edit auf adapter.test.js; vier Testläufe (67, 80, 105, 118) mit echten Bugfixes; Abschlusstext „46 original + 3 new denyTools tests" ohne Erwähnung der Regel; Peak 31,7k Tokens, 3 Compactions.

### Qwen3-Coder-Next 80B (3/5) — zwei Fails

**agora-A3-hls — `FAIL test-file-modified`**

- **Was passiert ist:** Der Hauptagent hat praktisch nichts selbst getan: 3 Schritte, 10,8k Tokens. Schritt 1 spawnte einen `task`-Subagenten mit einem Explore-only-Prompt („don't modify it, just understand it"). Der Subagent hat das Read-only-Mandat ignoriert und das ganze Feature gebaut — und zwar korrekt: `.webm: video/webm` in cachepolicy.go, und die versteckte Kopplung in `deploy/hls-cache-headers.yml` an allen drei Stellen (Kommentar, Traefik-v2-Beispiel, live PathRegexp) gefunden und gepflegt, `TestEdgeConfigMirrorsPolicy` grün. Zusätzlich ein „webm segment"-Tabellen-Case in cachepolicy_test.go. Der Parent hat danach `go test` laufen lassen (ok) und den Testfile-Edit als Deliverable aufgelistet, ohne den Verstoß zu bemerken.
- **Ursache:** Regel-Adherence, konkret: Constraint an der Delegationsgrenze verloren. Der Parent hat die Aufgabe paraphrasiert und „Do not edit any test files" dabei gestrichen; der Subagent hat den Satz nie gesehen. Zweiter Fehler: Subagent hat einen Explore-Auftrag als Implementierungsauftrag behandelt.
- **Schweregrad:** moderat. Funktional komplett und richtig (die YAML-Kopplung war die eigentliche Falle der Aufgabe, und die wurde getroffen). Aber das Muster „Parent delegiert alles, verliert Constraints, prüft das Ergebnis nicht gegen den Original-Prompt" ist strukturell — es wird bei jeder Aufgabe mit Constraints wieder passieren.
- **Hätte es ohne den Regelverstoß bestanden?** Ja. Kopie mit cachepolicy_test.go auf Baseline: `PASS` (exit 0), go test ok, versteckter `TestBenchGradeWebm` grün.
- **Steuerbar?** Ja. Sätze: „Wenn du delegierst, gib den Original-Aufgabentext wörtlich weiter, inklusive aller Einschränkungen." und „Vor dem Abschluss `git status` ausführen und bestätigen, dass keine `*_test.go` angefasst wurde." Alternativ: task/Subagent-Tool für Ein-Paket-Aufgaben abschalten, das Modell arbeitet dann direkt. Plus Subagent-Regel: „Subagenten editieren keine Dateien, es sei denn der delegierende Prompt sagt das ausdrücklich."
- **Evidenz:** Subagent-Prompt enthält nur „don't modify it, just understand it", keine Testfile-Regel; Subagent-Result „cachepolicy_test.go (lines 66-72): Added test case for .webm segment"; Parent Schritt 2 `go test ./internal/livehls/ -count=1` → ok; Toolfolge task → bash → done, reasoning 0; Re-Grade nach Revert: PASS.

**aiux-U2-denytools — `FAIL tests-failing fail=3 pass=49`**

- **Was passiert ist:** Saubere, kleine Implementierung (28 Zeilen in protection.js und dispatcher.js, keine Testfiles, kein Müll), aber zwei Defekte: (1) `throw new RuntimeSecurityError(...)` in `#execute()`, aber `RuntimeSecurityError` nie aus `./envelope.js` importiert → zur Laufzeit `ReferenceError`, Envelope verpackt es als `error.type='ReferenceError'`, Grading-Tests 1 und 3 fallen. (2) Ein korrekter bidirektionaler Alias-Helper `#isToolDenied` wurde geschrieben und für den capabilities()-Filter genutzt — der Chokepoint in `#execute()` ruft aber `this.#protection.isToolDenied(tool)` (Exact-Match). Deny auf die Alias-Schreibweise `runtime.search_classes` blockiert nichts → Test 4 fällt. describe() (Test 6) und capabilities-Filter (Tests 2, 5) sind richtig.
- **Ursache:** Partial-Implementation, genauer: Null Verifikation des neuen Pfads. Die 46 Bestandstests decken denyTools nicht ab; der Agent hat weder ein Wegwerf-Skript noch einen temporären Test geschrieben, nie eine Runtime mit `denyTools` konstruiert, und „verifiziert" durch Nochmal-Lesen des eigenen Codes. Der Abschlusstext behauptet „blocks aliases bidirectionally" und „throws RuntimeSecurityError" — beides zur Laufzeit falsch. Kein Kontextdruck (Peak 23,5k, reasoning 0). Ein Mid-Run-„stop" mit vorzeitigem Summary, den der Harness anstoßen musste.
- **Schweregrad:** moderat. Beide Defekte sind Ein-Zeilen-Fixes, aber die Kombination „falsche Erfolgsbehauptung + kein Ausführen des eigenen Codes" ist im unbeaufsichtigten Betrieb gefährlicher als der Regelverstoß oben: da gab es wenigstens grünen Code.
- **Hätte es ohne den Regelverstoß bestanden?** N/A — keine Testedits. Scratch-Experiment: Import ergänzt → 51/1; zusätzlich `#execute` auf `this.#isToolDenied(tool)` → 52/0.
- **Steuerbar?** Ja. Satz: „Bevor du fertig meldest, schreib ein Wegwerf-Skript oder einen temporären Test, der eine Runtime mit gesetztem `denyTools` baut und tatsächlich das gesperrte Tool, seinen Alias und `runtime.capabilities` aufruft, und prüf dass der Fehlertyp `RuntimeSecurityError` ist." Ein einziger Aufruf hätte den ReferenceError sofort gezeigt; ein Aufruf mit `denyTools:['runtime.search_classes']` die Alias-Lücke. Schwächer, aber vermutlich ausreichend: „Die 46 bestehenden Tests decken die neue Option nicht ab — dass sie grün sind, beweist nichts." Reiner Code-Hinweis, der beides nebenbei löst: „Wirf in protection.js, wo RuntimeSecurityError schon importiert ist, analog zu checkCodeLoading." Genau so hat es Qwen3.8-27B gemacht: `new RuntimeProtection(config, ALIASES)`, `checkTool(tool)` in `#execute`, `isDeniedTool` in capabilities — ein alias-bewusster Pfad, konsistent genutzt.
- **Evidenz:** dispatcher.js Zeile 5 unverändert `import { RuntimeArgumentError, bounded, fail, ok, required } from './envelope.js'`, Zeile 76 wirft `RuntimeSecurityError`; Grader „expected a security denial, got ReferenceError: RuntimeSecurityError is not defined" (1, 3) und „runtime.search_classes unexpectedly succeeded" (4); 19 Tool-Calls, einziger Testlauf `cd runtime/web && npm test` → 46 pass; Abschlussbehauptung „All 46 tests pass".

## Polyglot-Fails (35B)

Go 18/39 gescheitert, Python 9/34. Kategorien über alle 27 gescheiterten Exercise-Läufe:

| Kategorie | Anzahl | Exercises |
|---|---|---|
| (c) Go-Harness: versteckte Test-API / irreführender Stub frisst Versuch 1 per Build-Error | 10 von 18 Go-Fails | hexadecimal, trinary, matrix, protein-translation, palindrome-products, counter, paasio-go, food-chain, poker (A1), markdown (A1) |
| (e) Fast richtig, ein trivialer Slip übrig oder von Versuch 2 eingeschleppt | ~9 | bowling (py+go), ledger, scale-generator, pov-go, food-chain (A2), protein-translation (A2), forth-py, dot-dsl, wordy |
| (b) Spec-Fehllesung / Edge-Case bei mittelschwerer Aufgabe | ~6 | connect (py+go), kindergarten-garden, markdown, paasio-py, sgf-parsing |
| (a) Falscher Algorithmus bei wirklich schwerer Aufgabe | ~4 | pov-py, react, poker (A2), forth-go |
| (d) Harness-/Modell-Output-Fehler, nie Code getestet | 1 (2 Versuche) | robot-simulator |

Konkrete Slips in (e), damit klar ist, wie knapp das war: bowling-go `rolls[19]` bei `len(rolls)==19` (Off-by-one, Panic); ledger-go Header `%-10s` statt 11 Zeichen breit — alle Datenzeilen byte-identisch, eigener Kommentar „Date: 10" ist die Fehlzählung; scale-generator Versuch 2 fixt alle 7 Modi und regressiert den chromatischen Fall; pov-go leerer non-nil Slice statt nil, `== nil`-Guard greift nie; forth-py 52/54, nur Exception-Text („cannot redefine numbers" statt „illegal operation"); dot-dsl 11/12, wordy 21/25 — jeweils nur Fehlermeldungs-Strings, die in den Anleitungen nirgends stehen. connect scheitert in beiden Sprachen am selben Test („convoluted path"): Nachbarmenge hat (−1,−1) statt (−1,+1), eine falsche Richtung, Suche korrekt.

**Go vs. Python:** Der Unterschied ist fast vollständig der Harness-Vertrag, nicht schlechteres Go. (1) Die Go-Stubs sind gezielt irreführend und Testfiles unsichtbar: trinary-Stub `ParseTrinary(arg string, want int64, ok bool)` (Felder aus dem Test-Struct kopiert), Test will `(int64, error)`; hexadecimal braucht ein undokumentiertes `HandleErrors`; matrix-Stub sagt Werttyp, Test vergleicht mit nil; protein-translation referenziert `ErrStop`/`ErrInvalidBase`, die nirgends existieren; palindrome-products verlangt ein ungenanntes Feld `Factorizations`. counter, paasio und robot-simulator hängen von Sibling-Files ab (impl1-4.go/interface.go/maker.go, interface.go mit der Thread-Safety-Anforderung, defs.go), die das Modell nie sieht. Python hat keinen Compile-Schritt und Stubs mit echten Klassen-/Methodennamen — derselbe blinde Schuss kostet dort nichts, Versuch 1 erreicht immer echtes Test-Feedback. In Go kommt oft nur `undefined: X` zurück und es bleibt genau ein blinder zweiter Schuss. (2) Wo der algorithmische Inhalt gleich ist, ist das Ergebnis gleich: bowling, connect, forth, paasio, pov scheitern in beiden Sprachen, connect/bowling am selben Edge-Test. (3) Echte Go-Slips gibt es, aber wenige: `package food_chain`, rune/byte in markdown, int/int64 in trinary A2, nil/leerer Slice in pov — je ein Versuch. (4) Go-Feedback ist brutaler: Panics liefern 1000-Zeilen-Goroutine-Dumps (forth A2 22k Tokens Kontext, paasio-History 1,1 MB), pytest liefert kompakte Diffs. (5) robot-simulator: „token limit … Output tokens: ~0 of 0" in beiden Versuchen, `num_error_outputs=2` — Inferenzfehler, nie Code. Bereinigt um die ~10 Harness-Compile-Runden liegt Gos „echte" Fehlerrate im selben Band wie Python.

**Komplexitäts-Urteil:** Nein, ganz überwiegend kein Komplexitätsproblem. Die dominante Variable ist das 2-Versuche-„whole"-Format mit versteckten Testfiles: Das Modell fixt zuverlässig genau den Fehler, den es gezeigt bekommt, aber mit zwei blinden Schüssen und ohne Sicht auf die Erwartungen konvergiert es nicht. Exercises, die das Modell auch mit voller Information vermutlich nicht gelöst hätte: pov (Re-Rooting wird zur Geschwisterliste statt verschachtelt), react (naive eager Propagation statt topologischer Update-Reihenfolge, −5 statt 10), poker (Ace-low/high-Straights fehlen, Vierling über Straight Flush, Panic bei >5 Karten), sgf-parsing (Escape-Regeln, 11/23), forth-go Selbstreferenz (lazy Wort-Expansion, Python hat dasselbe in Versuch 2 per eager Auflösung gelöst, Go nicht). Das sind fünf. Alles andere ist Harness, Slips oder Strings.

## Konsequenzen fürs Setup

**AGENTS.md / System-Prompt, für alle Modelle:**

1. „Bestehende Testdateien werden nicht verändert, es sei denn die Aufgabe verlangt es ausdrücklich. Neue Tests kommen in eine neue Datei." — fixt beide test-file-modified-Fails direkt. Die erlaubte Alternative muss dabeistehen, sonst kämpft das Modell gegen seinen Coverage-Reflex.
2. „Vor der Fertigmeldung: `git status`/`git diff --stat` prüfen und gegen die Einschränkungen des Original-Prompts abgleichen."
3. „Bestehende Tests grün ≠ Feature funktioniert. Wenn die neue Funktionalität von keinem Test abgedeckt ist, den neuen Pfad einmal tatsächlich ausführen (Wegwerf-Skript oder temporärer Test), bevor du Erfolg meldest." — fixt den 80B-denytools-Fail.
4. „Beim Delegieren den Original-Aufgabentext wörtlich weitergeben, inklusive aller Einschränkungen. Subagenten editieren keine Dateien, es sei denn der Prompt sagt das ausdrücklich."
5. Harte Constraints an den Anfang des Prompts, in Großbuchstaben, mit Konsequenz („lässt die Aufgabe automatisch durchfallen").

**Pro Modell:**

- **Qwen3.8-27B, Muse Glimmer:** null Fails, keine Änderung nötig. 27B hat denytools genau so gebaut, wie man es bauen sollte (ein alias-bewusster Pfad, Throw in protection.js) — das ist die Referenz.
- **Qwen3.6-35B-A3B:** Regeln 1 und 5 reichen. Der Code war gut, das Modell hat nur seinen Plan über die Constraint gestellt. Zusätzlich beobachten: Zwischen-Edits, die Methoden zerschießen (checkCodeReplacement) — ein „nach jedem Edit Syntax-Check" wäre billig.
- **Qwen3-Coder-Next 80B:** Zwei verschiedene Schwächen in zwei Läufen. (a) Delegiert komplett und verliert dabei Constraints → `task`-Tool für Ein-Paket-Aufgaben abschalten oder Regel 4 hart setzen. (b) Führt seinen eigenen neuen Code nicht aus und behauptet trotzdem Erfolg → Regel 3 zwingend. Das mit reasoning 0 und dem vorzeitigen „stop"-Summary spricht dafür, dass das Modell zu früh in den Berichtsmodus fällt; ein „Continue"-Nudge war nötig. Bis das steht, ist 80B für unbeaufsichtigte Läufe die schwächere Wahl gegenüber 27B — nicht wegen Fähigkeit, sondern wegen Disziplin.

**Polyglot-Harness (35B, gilt für jedes Modell):**

- Testfile oder mindestens die exakten erwarteten Signaturen/Identifier zeigen — nimmt hexadecimal, trinary, matrix, protein-translation, palindrome-products die Compile-Runde. counter/paasio/robot-simulator sind per Prompt nicht lösbar, solange die Sibling-Files nicht im Chat sind; die müssen mitgegeben werden.
- Mehr als 2 Versuche, oder Hinweis „dein Fix hat vorher grüne Tests regressiert, diff gegen die vorherige Version" — flippt bowling-go, ledger, scale-generator, pov-go, food-chain.
- Go-Panic-Traces vor dem Zurückfüttern auf die ersten Frames kürzen — 22k-Token-Kontexte aus Goroutine-Dumps sind reines Rauschen.
- robot-simulator: Retry bzw. max_tokens auf dem Server prüfen; das war kein Modell-Fail.
- Für markdown: „bestehende Implementierung behalten, minimal refactoren" — das Modell hat eine funktionierende 93-Zeilen-Lösung durch eine 11k-Token-Neuschrift mit neuen Bugs ersetzt.

## Nachtrag 24.08. ~21:55 — A5-Ambiguität ist jetzt EINSTIMMIG (5/5 Cloud+Frontier)

`TestGradeMobileOneItemPerBatchBestFirst` („first mobile batch item score = 40, want the
highest-scored item first") ist bei **allen** bisherigen A5-Läufen der einzige rote Test:
dsh-v4-flash, dsh-v4-pro, oc-gemini37f, oc-v4-flash **und cc-opus5 (xhigh)**. Alle Modelle
— inklusive der Abo-Messlatte — lesen den Brief als „Eingabe kommt sortiert" (Vertrag
nutzen), der Grader verlangt defensives Selbst-Sortieren. Opus 5 hat dabei from scratch
gebaut (batching.go + eigene Tests, alle 12 eigenen Property-Tests grün) und exakt
dieselbe Interpretation gewählt.
Konsequenz: Der Befund „mehrdeutige Spec, nicht Modellschwäche" ist damit maximal
abgesichert. Tobias' Regrade-Entscheidung (defensiv sortieren vs. Vertrag vertrauen)
steht noch aus — bei Einstimmigkeit inkl. Frontier-Modell spricht alles für eine
Spec-Korrektur bzw. Doppel-Wertung.

Update ~21:57: agy-37flash (Antigravity-Abo, effort=high) macht die Einstimmigkeit zu **6/6** —
gleiche Property, gleiche Zeile (zz_bench_grade_test.go:130). Kein einziges Cloud-/Frontier-Modell
liest den Brief als "defensiv selbst sortieren".

Update ~22:00 — muse-vulkan "besteht" A5: **Handspiel-Tor, kein echtes.** batches.go sortiert nie
und wählt nie nach Score; die Diversity-Heuristik nimmt stets das erste Element NACH dem Kopf mit
frischem Topic. In der fixen Testanordnung (40,90,70,60) sitzt das Maximum zufällig genau dort —
Permutationstest (90 zuerst) lässt TestGradeMobileOneItemPerBatchBestFirst sofort durchfallen.
KEIN Tampering (Guard sauber, Tests waren dem Modell nie sichtbar) — es ist eine Lücke des
Graders: fixe Anordnung in einem Property-Test. Der offizielle PASS bleibt stehen (Schiri-Prinzip,
alle Modelle sahen denselben Test), wird im Bericht aber als Glückstreffer gekennzeichnet.
Die 7/7-Einstimmigkeit der Cloud-Riege bleibt unberührt: auch muse sortiert NICHT defensiv.
TODO nach der Kampagne: grade_test.go härten (mehrere Permutationen je Property).

Update ~22:12 — **Tor aberkannt** (Tobias' Anweisung: "wenn das kein Pass war, ist es auch kein
Pass"). grade_test.go gehärtet: Properties (c) Mobile-Best-First und (d) Topic-Mix laufen jetzt
über ALLE 24 Eingabe-Permutationen (Helper gradePermute; künftige Grades nutzen die Härtung
automatisch, da grade.sh die Datei je Grading frisch kopiert). Alle 9 vorhandenen A5-Workspaces
mit dem scharfen Grader neu bewertet: **9x FAIL** — nur muse kippte (PASS→FAIL), alle übrigen
Urteile unverändert. Der muse-Eintrag wurde IN PLACE in results.jsonl korrigiert (kein neuer Lauf;
Backup: results.jsonl.bak-regrade). A5 ist damit offiziell von niemandem bestanden.
Geplant (Tobias, ~22:05): adversarialer Validierungslauf über ALLE Grader der Suite, sobald die
Nachtketten durch sind — je Task ein Angreifer, der den Grader mit legalen Falsch-Lösungen zu
täuschen versucht; Härtungen dann wie hier + Korrekturen in place.

Update ~22:36 — codernext (80B) reißt als EINZIGES Modell der Nacht A4: Der gepflanzte Bug
(topics[topic] = false in sharesTopic) steht unveraendert im Code — stattdessen wurde die
Auswahllogik drumherum umgebaut (topicCounts-Rework, 35 Zeilen) und damit zusaetzlich die
Live-Kadenz gebrochen (2 Tests rot statt 1). Klassisches Symptom-Umbauen statt Ursache-Finden;
bemerkenswert, weil selbst die 27-35B-Lokalen den Einzeiler fanden.

Update ~22:46 — codernext (80B) A5: substantieller FAIL ohne Infra-Einfluss (0 Context-Overflows,
batches.go gebaut). Reisst aber DREI Properties statt der einen Ambiguitaets-Falle: Mobile-Best-
First UND Topic-Mix UND Live-Kadenz. Damit qualitativ schwaecher als die Cloud-Riege (die nur an
der mehrdeutigen Sortier-Property scheitert) und als muse (Topic-Mix/Live sassen dort). Kein Retry
noetig (echte Abgabe, kein Timeout/Leerlauf).

## Nachtrag 24.08. ~23:58 — Polyglot: drei Freilose im Benchmark-Set + Auth-Leichen
Die Go-Uebungen **counter** ("no tests to run"), **ledger** und **markdown** (Refactoring-Aufgaben, Code
bereits gruen) bestehen `go test` UNBERUEHRT — empirisch am pristine Stub geprueft. Das ist ein Artefakt des
Aider-Polyglot-Sets, das alle Harness-Laeufe gleich betrifft (agy/dsflash haben dort echte Tokens ausgegeben,
haetten aber auch mit Nichtstun bestanden). Konsequenz fuer den Bericht: jedes Polyglot-Ergebnis als
"x/70 echte + 3 Freilose" lesen; Rankings unveraendert. Zweite Konsequenz: Die Auth-Bereinigung der Opus-
Laeufe (Kriterium 0 Tokens UND kein PASS) hat genau diese drei bei Opus 5 und counter/ledger bei Opus 4.8
uebersehen (is_error:true, 0 Tokens, trotzdem gruen). Geloescht und per Resume neu gefahren, damit jede
Opus-Zeile ein echter Modellversuch ist. Kriterium im Runner-Waechter bleibt korrekt (0 Tokens + rc!=0 =>
kein result.json), weil der gehaertete Runner solche Faelle gar nicht erst schreibt.
Ausnahme: cc-opus48 go/robot-simulator hat 0 Tokens, ist aber ein ECHTER PASS (robot_simulator.go vom
Modell geaendert, Tests identisch, pristine Stub faellt durch) — nur der JSON-Envelope fehlt (0 Bytes,
vermutlich Versuchs-Timeout nach getaner Arbeit). Bleibt stehen; Token/Kosten dieser Uebung sind
untererfasst.
Korrektur 25.08. 00:10: Der Vergiftungs-Waechter im Claude-Polyglot-Runner hat bei cc-opus5 go/ledger
einen Versuchs-Timeout (rc 124, Envelope beim Kill verloren, 0 Tokens) als Auth-Ausfall gewertet und
abgebrochen (Fehlalarm). Kriterium verschaerft auf die echte Auth-Signatur: 0 Tokens UND rc 1 UND <15 s.
Timeouts zaehlen wieder als regulaere (gescheiterte) Versuche mit anschliessendem Testlauf — Token/Kosten
solcher Uebungen sind untererfasst (Envelope fehlt).

## Nachtrag 25.08. ~00:40 — zwei Runner-Bugs im Polyglot-Versuch-2 (gefixt, betroffene Uebungen neu)
1. **Prompt beginnt mit "-"**: Go-Testausgaben starten mit "--- FAIL: Test…"; der zweite Prompt
   (Test-Tail + aider-Vorlage) wurde von der Claude-CLI als Option gelesen ("unknown option '--- FAIL…'")
   — Versuch 2 fand nie statt. Betroffen: nur Go-Uebungen mit Test-FAIL im 1. Versuch (Build-Fehler
   beginnen mit "#" und waren ok). Nachweislich: cc-opus48 go/trinary. Fix: Kopfzeile "Test output:" vor
   dem Tail in run-polyglot-{claude,oc}.sh (agy-Runner nicht betroffen, Prompt 2 beginnt dort mit der
   Aufgabe). Semantik-Abweichung zu aider: eine Kopfzeile, sonst wortgleich.
2. **Container-Namenskollision**: nach Timeout-Kill von Versuch 1 hing der Container noch, Versuch 2
   scheiterte mit rc 125 "name already in use". Nachweislich: oc-dsflash go/alphametics. Fix: podman rm -f
   vor jedem Versuch (Claude- und OC-Runner).
Beide Uebungen wurden geloescht und per Resume neu gefahren (Ergebnis siehe summary.json). Alle anderen
Versuch-2-Faelle geprueft: rcs regulaer.
Ergebnis der Nachlaeufe (00:30): cc-opus48 go/trinary PASS (V1) -> 67/73 p1 (konservativ 66), 73/73 p2;
oc-dsflash go/alphametics PASS (V1) -> 63/73 p1, 70/73 p2 (0,24 $). Endstand Agent-Harness-Polyglot:
agy-37flash 73/73 p1 · cc-opus5 73/73 p1 · cc-opus48 66-67/73 p1, 73/73 p2 · oc-dsflash 63/73 p1, 70/73 p2.
Fussnote fuer alle: 3 Freilose (counter/ledger/markdown) im Set.

## Nachtrag 25.08. ~01:25 — qwen38 A5-Retry (64k, 60 min): echtes Kapazitaets-FAIL, "Analyse-Paralyse"
Mit 64k Kontext keine Overflows mehr (0 Treffer), trotzdem keine Abgabe: 54 Schritte, davon 43x read,
19x grep, 3x glob, 9x bash — und KEIN einziger write/edit. Das Modell hat 60 Minuten lang das Repo und
die contract/*.md-Dokumente durchsucht ("consecutive", "top slot", "live room" …), also die Spezifikation
immer weiter ergaenzt statt zu bauen. Kein Infra-Einfluss mehr -> Urteil FAIL (keine Abgabe) bleibt.
Befund fuer den Bericht: Bei From-Scratch-Aufgaben mit vagem Brief kippt die 27B-Klasse in reine
Recherche-Schleifen; die Cloud-Modelle bauen nach 5-10 Lesezugriffen und iterieren mit Tests.
Update 02:25: qwen38 A6-Retry (64k, 60 min) ebenfalls ohne Abgabe (leerer Diff, Timeout) — gleiches
Recherche-Schleifen-Muster wie A5. Beide Urteile FAIL (keine Abgabe) sind damit final und infra-frei.

## 25.08. 02:29 — Grader-Haertung angewendet: 98 Regrades, 0 Flips
apply-haertung.sh (Audit-Pakete P1, P2, P4, P6, P8) hat alle 8 Grader auf die gehaertete Fassung
umgestellt (Backups grade.v1.sh / grade_test.v1.go, results.jsonl.bak-audit-20260825-022854) und alle
98 letzten Eintraege neu bewertet: same=98, flip=0, A5-Lesart streng. Alle Urteile der Kampagne halten
unter Gradern, die die 36 Audit-Exploits abweisen. Nebeneffekt: U2 zaehlt jetzt 58 statt 52 Tests
(grade.v2.test.js ergaenzt 6 Tool-/Alias-Faelle) — Urteilsklasse unveraendert.

## 25.08. 02:45 — dsh-flash Polyglot komplett: 68/73 pass@1, 73/73 pass@2 (0,32 $, 112 min)
Gegen OpenCode+flash (63/73, 70/73, 0,24 $, 82 min): dsh ist im Polyglot GENAUER (pass@2 lueckenlos,
+5 pass@1), OpenCode ist SCHNELLER und GUENSTIGER (auch in der Suite: 889 s vs 1331 s bei gleicher
Qualitaet 7/8). Harness-Urteil fuer DeepSeek daher zweigeteilt: Genauigkeit -> dsh (Reasoning "high",
kein Session-Resume, Developer-Preview); Tempo/Preis/Reife -> OpenCode. Tobias' Beobachtung ("dsh
akkurater, aber langsamer") bestaetigt sich auf der vollen Strecke.
Harness-Matrix DeepSeek-flash Polyglot: Aider 33/61 (edit-only) · OpenCode 63/70 · dsh 68/73.
Update 02:42: qwen36moe A5-Retry (64k, kein Overflow) liefert erstmals eine Abgabe (batches.go, 9,3 min):
FAIL an zwei Properties (Mobile-Best-First = Ambiguitaets-Kante wie alle + Topic-Mix). Einordnung: ueber
qwen38 (keine Abgabe), unter Cloud (nur die eine Property) und unter dem 80B (drei Properties, aber
Abgabe). Der gehaertete Grader nennt die gerissenen Tests jetzt direkt in der Urteilszeile.

## 25.08. 02:58 — Flash-low komplett: Effort-A/B abgeschlossen (Tobias: Thema Flash/Abo damit zu)
agy-37flash-low 7/8 (nur A5, gleiche Property wie alle) = identisch zu high. Summe 481 s vs 907 s,
Output 53k vs 170k Tokens, Thinking 6,6k vs 109k (low denkt minimal, nicht null), Input ~gleich.
Details bench/agy-low-tokens.md. Schluss: Fuer Repo-Arbeit ist low der Sweet-Spot; high kauft hier nur Zeit.
Forensik qwen38 A6-Retry (03:10): 39 Schritte, 47x read, 7x grep, 3x glob, 2x bash, 0x write/edit — aber
KEINE Spec-Recherche wie bei A5, sondern eine Leseschleife: die letzten 8 Aufrufe sind 4x feed.go und
2x feed_suggestions_feed.go (dieselben Handler-Dateien). ~90 s je Schritt (27B dense, 64k Kontext) ->
39 Runden = exakt das 3600-s-Limit. Zwei verschiedene Agenten-Pathologien der 27B-Klasse bei From-Scratch:
A5 "immer mehr Spec lesen", A6 "dieselbe Datei nochmal lesen". Folgetest-Idee (HANDOVER): Harness-Guard
"N Lesezugriffe ohne Write -> Abbruch + Schreibaufforderung" und pruefen, ob qwen38 dann abliefert.

## 25.08. 03:25 — Flash-low Polyglot: 73/73 pass@1 in 33 min (high: 73/73 in 74 min)
Median 24 s vs 53 s je Uebung; Envelope-Summen low: Input 8,46 M, Output 209 k, Thinking 68 k, Cache 9,55 M
(high: 11,34 M / 888 k / 684 k / 23,2 M). Testdateien byteidentisch, 0 Fehler. Effort-A/B damit auf Suite UND
Polyglot eindeutig: gleiche Qualitaet, halbe Zeit, ein Viertel Output, ein Zehntel Thinking.

## 25.08. 09:58 — UX-Wertung (bench/webapp/UX-WERTUNG.md, Opus-Auszaehlung gegen 10 gepflanzte Fehler)
DOM: cc-opus5 10/10 (+9 Bonus, 0 falsch), cc-opus48 10/10, muse 8, gemini37f/agy/ds-pro 7, qwen36moe 6,
codernext 5 (+3 FALSCH), ds-flash und qwen38 0 = LEERE Antwort (Thinking frass das Token-Budget; Wiederholung
mit max_tokens 12000 / enable_thinking:false steht im TODO). Vision (27B, Muse): 5/10, nur Klassiker.
Befund: UX-Review ist Arbeit OHNE Test-Rueckkanal — die einzige Disziplin der Kampagne, in der Opus sich
klar absetzt (Fluss-Fehler: Opus 3/3, Flash 1/3). Deckt sich mit der TB-3-Einordnung.

## 25.08. 10:45 — Kandidaten-Runde 100-130B-MoE ABGEBROCHEN: laedt, arbeitet aber nicht
Mistral Small 4 119B-A6.5B (imatrix IQ3_XXS, 42,7 GiB): llama-bench Vulkan pp 109 t/s, tg 6,3 t/s (d0 und
d10k gleich = RAM/NVMe-Streaming-limitiert; ROCm-Bench ohne Ergebnis). Suite U1 (Einzeiler-Fix, sonst
33-115 s): 30-min-Timeout OHNE Aenderung. Bei 6 t/s kommt der Agentenloop nicht zum Schreiben. Qwen3.5-122B
und Laguna (44/46 GiB, gleiche Klasse) nicht mehr gefahren — gleiches Bild erwartet. Fazit: Die 120B-Klasse
passt mit 32 GiB RAM + 20 GiB VRAM zwar per IQ3 auf die Platte, aber nicht in den Arbeitsspeicher; ohne
RAM-Upgrade (64 GiB) keine Agentenarbeit. Modelle bleiben unter models/ (133 GiB) fuer einen spaeteren Versuch.
