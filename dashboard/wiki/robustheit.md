# Robustheit der Abgaben

**Frage dieser Seite: Wie „kantig" sind die Agenten-Abgaben wirklich — zerbrechen sie an unsauberen Eingaben, oder halten sie das Brief-Versprechen in alle Richtungen?**

Die Robustheits-Batterie ist eine **Zusatz-Metrik** über die Suite-Abgaben. Sie ändert **keine Urteile**: PASS/FAIL im Suite-Tab bleibt das einzige Kriterium. Sie beantwortet eine eigene Frage: Hält eine Abgabe den vollen Brief-Vertrag nur auf dem glatten Weg, oder auch unter Druck?

## Wie gemessen wird

Jede Batterie prüft die Abgabe in einem Wegwerf-Workspace (Kopie aus `bench/runs/<label>/<task>/ws`, ohne `.git`/`node_modules`). Pro Task gibt es zwei Prüf-Gruppen:

- **Real** (`ZZBatReal`-Präfix): realistische Kanten **unter vollem Brief-Vertrag** — die Abgabe muss korrekt funktionieren (nicht nur terminieren).
- **Path** (`ZZBatPath`-Präfix): pathologische Eingaben — hier zählt **nur kein Crash/Hang/Verlust** bei ansonsten normalem Lauf, keine Fachdeutung.

Ein Paar heißt **baubar**, wenn der Code überhaupt baut/läuft (sonst `buildable:false` und „n. baubar" in der Zelle). Der **real_score** je Label ist `Σ real_pass / Σ real_total` über alle baubaren Tasks. Spalte **P** zeigt die Path-Quote.

## Wo die Zahlen stehen

- **Dashboard → Suite:** die Zellen tragen eine Mini-Zeile `R 11/12 · P 5/5` (⟳ = veraltet), die rechte Spalte **Robustheit** zeigt den real_score je Label (Prozent + Mini-Balken + `n/8`). Unter dem Gesamtzeit-Graph liegen die Robustheits-Balken; Klick auf eine Zeile klappt die Aufgaben auf. Ein Klick auf eine Zelle listet im Detail die gerissenen Tests.
- **Daten:** `bench/robustness-battery/results.json` (Schema 2: `batteries` + `results` je `(task,label)` inkl. `ws_fingerprint`).
- **Reporter:** `bench/robustness-battery/a5-results.md` und `a6-results.md` (Momentaufnahmen), der Audit-Bericht `bench/audit-report.md` und die Audit-Selbsttests unter `bench/audit-scratch/robust/`.

## Neu berechnen

Über **Läufe → Robustheit neu berechnen** (CPU, kein GPU-Lock). Nur Paare, deren **ws-Fingerprint** (sha256 über die Nicht-Test-Quellen) vom letzten Lauf abweicht, werden neu ausgeführt; `--force` erzwingt alles. Das Dashboard zeigt ⟳ solange ein Paar veraltet ist.

## Falle: ein hoher Score über wenige Tasks

Der real_score zählt **nur baubare Tasks**. Wer zwei Tasks gar nicht erst zum Bauen bringt, wird über die
übrigen sechs gemittelt — und kann so ganz oben stehen, ohne der robusteste zu sein. Der Balken solcher
Labels ist deshalb **gelb** statt blau und trägt ein ⚠ mit der Zahl der weggelassenen Tasks; in der Spalte
**Robustheit** steht dasselbe als `⚠ 6/8`. Vergleiche Prozentwerte nur zwischen Labels mit gleichem `n/8`;
sonst ist die absolute Zahl `R 60/60` gegen `R 78/79` die ehrlichere Auskunft.

(Beispiel Stand 25.08.2026: `qwen38-vulkan` steht mit 100 % oben, hat aber A5 und A6 nicht baubar —
`undefined: BuildBatches` bzw. `undefined: RankCandidates`, also gar keine Abgabe für diese zwei Tasks.)

## Falle: Robustheit ≠ Qualität

Die Batterie ist deterministisch, aber eng: Sie prüft die **vom Brief geforderten Kanten**, nicht die Güte des Entwurfs. Ein Label kann 10/10 Real haben, weil die Brief-Kanten sauber behandelt sind — und trotzdem hässlich gebaut sein. Umgekehrt sagt „n. baubar" nichts über Intelligenz, sondern über den Build-Vertrag. Die ehrlichste Lesart: **Die Batterie misst, ob das Versprechen hält — nicht, ob die Lösung schön ist.**
