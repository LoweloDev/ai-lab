# 07 — Batterie fuer aiux-U2-denytools

Node, cwd runtime/web, Vorbild: test/deny-tools.grade.test.js und grade.v2.test.js. Brief: denyTools zweischichtig — Layer 2 darf Layer 1 NIE lockern. Real: generische Alias-Paare (nicht nur search_types/search_classes), Gross-/Kleinschreibung, Kombinationen beider Layer, Reihenfolge-Unabhaengigkeit, zur Laufzeit gewaehltes Tool, Layer-1-Sperre bleibt trotz Layer-2-Freigabe; Path: leere/undefined Listen, Duplikate, Nicht-Strings, sehr lange Listen.

Vorgehen:
1. /home/lowelodev/ai-lab/bench/tasks/aiux-U2-denytools/prompt.txt (der Brief) und den versteckten Grader (/home/lowelodev/ai-lab/bench/tasks/aiux-U2-denytools/grade.sh, grade_test.go|grade.test.js|grade.v2.*) lesen.
2. Baseline-Code /home/lowelodev/ai-lab/bench/workspaces/aiux-U2-denytools lesen (Schnittstellen, gegen die die Batterie laeuft).
3. /home/lowelodev/ai-lab/bench/robustness-battery/aiux-U2-denytools/battery.json + Testdatei nach Konvention (6-12 Real, 3-6 Path, WHY je Test). Falls im Verzeichnis schon Teilstuecke liegen: vervollstaendigen.
4. Selbsttest laut Konvention unter /home/lowelodev/ai-lab/bench/audit-scratch/robust/aiux-U2-denytools/ (Baseline + 2-3 PASS-Abgaben + 1 FAIL-Abgabe, Labels aus
   /home/lowelodev/ai-lab/bench/results.jsonl: letzte Zeile je model/task; Workspaces /home/lowelodev/ai-lab/bench/runs/<label>/aiux-U2-denytools/ws). Zahlen notieren, Batterie ggf. nachbessern.
5. done-07.txt (Zahlen je geprueftem Label, real_total/path_total, offene Punkte).
