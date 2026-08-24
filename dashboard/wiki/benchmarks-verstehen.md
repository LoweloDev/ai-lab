# Benchmarks verstehen

**Frage dieser Seite: Was misst jeder Benchmark-Typ, was misst er NICHT — und wie liest man die Zahlen, ohne sich zu belügen?**

## Die vier Benchmark-Arten

| Benchmark | Misst | Misst NICHT |
|---|---|---|
| **Suite** (5 Tasks) | Agentic End-to-End: Bug in echtem Repo finden, Feature bauen, Tests grün kriegen, **Regeln einhalten** („Do not edit any test files") | Wissensbreite, Algorithmik pur, Geschwindigkeit isoliert |
| **Polyglot** (aider, 73 Übungen python+go) | Code-Generierung mit minimalem Feedback: 2 Versuche, „whole"-Format, Testdateien unsichtbar | Tool-Use, Repo-Navigation, Regel-Adherence — es gibt keine Regeln zu brechen |
| **DOM/App-Steuerung** (`dom-agent.py`) | Website bedienen über Text-Snapshots (Info finden, Formular ausfüllen) — ohne Vision, objektiv verifiziert | visuelles Verständnis, Layout-Probleme |
| **UX-Tests** (`ux-dom-test.py`, `ux-vision-test.py`) | Findet das Modell die **10 absichtlich eingebauten Fehler** der Testseite (Ground Truth: `bench/webapp/UX-FLAWS.md`)? DOM-Variante liest Quelltext, Vision-Variante 5 Screenshots | Geschmacksfragen; die DOM-Variante kann Rendering-Fehler (Überlappung) prinzipiell nicht sehen |

Die 5 Suite-Tasks: `agora-A1-gate` (geplanteter Bug, Go), `agora-A2-jsonld` (Security-Fix, Next.js), `agora-A3-hls` (Feature mit versteckter YAML-Kopplung), `aiux-U1-paging` (geplanteter Bug, JS), `aiux-U2-denytools` (Feature mit 6 versteckten Grading-Tests).

## Lehre 1: Regelverstoß ≠ Unfähigkeit

Die Forensik (`bench/failure-analysis.md`) hat alle drei lokalen Suite-Fails seziert. **Keiner war ein Komplexitätslimit:**

- **35B, U2:** Feature komplett und korrekt — aber 3 eigene Tests an eine bestehende Testdatei angehängt → `FAIL test-file-modified`. Mit zurückgesetztem Testverzeichnis: `PASS pass=52 fail=0`.
- **80B, A3:** Subagent baute das Feature korrekt (inkl. der eigentlichen Falle, der YAML-Kopplung) — aber der Parent hatte beim Delegieren die Testfile-Regel verschluckt. Nach Revert der Testdatei: PASS.
- **80B, U2:** der einzige echte Defekt — fehlender Import + inkonsistenter Helper, weil das Modell seinen neuen Codepfad **nie einmal ausgeführt** hat („All 46 tests pass" beweist nichts über neue Features).

„Effektive Fähigkeit" ohne Regelverstöße: 35B wäre 5/5, 80B 4/5. Ein `FAIL test-file-modified` sagt also: Disziplinproblem, per Prompt-Regel steuerbar — nicht „Modell zu dumm".

## Lehre 2: Leere Antworten = Thinking-Budget, kein dummes Modell

Wenn `content` leer zurückkommt, hat meist das Reasoning die `max_tokens` aufgefressen (qwen38 im UX-DOM-Test: Thinking fraß die 6000) oder der Kontext war zu klein (muse-vision: 5 Screenshots sprengen 16k). Erst Budget/Kontext prüfen, dann übers Modell urteilen — [Troubleshooting](troubleshooting.md).

## Lehre 3: pass@2 richtig lesen

Polyglot-Headline ist **`pass_rate_2`**: Versuch 1 ist blind (nur Aufgabentext + Stub), Versuch 2 bekommt die Testfehler zurück. `tests_outcomes` pro Übung: `[true]` = pass@1, `[false, true]` = pass@2. Die Testdateien selbst sieht das Modell **nie** — in Go heißt das: irreführende Stubs und versteckte Test-APIs fressen Versuch 1 oft als reinen Build-Error. Von 27 gescheiterten 35B-Übungsläufen waren nur **~4 echte algorithmische Misses**; ~10 waren der Go-Harness-Vertrag, ~9 einen trivialen Slip entfernt, ~6 Spec-Fehllesungen, 1 ein Serverfehler. Bereinigt liegt Gos „echte" Fehlerrate im selben Band wie Python. Ein Polyglot-Prozentwert vergleicht also Modelle fair **untereinander**, ist aber kein absolutes Fähigkeitsmaß.

## Zahlen lesen — die Kurzregeln

- Suite: **letzte Zeile pro Modell+Task** in `results.jsonl` zählt (Wiederholungen überschreiben logisch).
- `grade` immer zusammen mit `exit` und `changed` lesen: `FAIL` + leeres `changed` + Fehler in `stderr.log` = Infrastrukturproblem, kein Messergebnis.
- Echte Beispiele: 35B löst A1 in 23,5 s, 27B braucht 78,5 s — beide PASS. Tempo und Qualität sind getrennte Achsen.
- Details zum Format: [Ergebnisse lesen](ergebnisse-lesen.md).
