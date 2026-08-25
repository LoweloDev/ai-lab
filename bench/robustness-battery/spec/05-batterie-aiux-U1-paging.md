# 05 — Batterie fuer aiux-U1-paging

Node, cwd runtime/web, Vorbild fuer Aufruf: test/adapter.test.js (node:test). Brief: Paging/Seitenlogik im Web-Adapter. Real: leere Listen, Seitengrenzen (0, 1, =n, n+1, riesig), Off-by-one an Seitenraendern, stabile Reihenfolge, Gesamtzahl konsistent; Path: negative/NaN/undefined/null-Seiten, absurde Seitengroessen, Nicht-Arrays.

Vorgehen:
1. /home/lowelodev/ai-lab/bench/tasks/aiux-U1-paging/prompt.txt (der Brief) und den versteckten Grader (/home/lowelodev/ai-lab/bench/tasks/aiux-U1-paging/grade.sh, grade_test.go|grade.test.js|grade.v2.*) lesen.
2. Baseline-Code /home/lowelodev/ai-lab/bench/workspaces/aiux-U1-paging lesen (Schnittstellen, gegen die die Batterie laeuft).
3. /home/lowelodev/ai-lab/bench/robustness-battery/aiux-U1-paging/battery.json + Testdatei nach Konvention (6-12 Real, 3-6 Path, WHY je Test). Falls im Verzeichnis schon Teilstuecke liegen: vervollstaendigen.
4. Selbsttest laut Konvention unter /home/lowelodev/ai-lab/bench/audit-scratch/robust/aiux-U1-paging/ (Baseline + 2-3 PASS-Abgaben + 1 FAIL-Abgabe, Labels aus
   /home/lowelodev/ai-lab/bench/results.jsonl: letzte Zeile je model/task; Workspaces /home/lowelodev/ai-lab/bench/runs/<label>/aiux-U1-paging/ws). Zahlen notieren, Batterie ggf. nachbessern.
5. done-05.txt (Zahlen je geprueftem Label, real_total/path_total, offene Punkte).
