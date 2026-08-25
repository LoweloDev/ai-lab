# 03 — Batterie fuer agora-A1-gate

Go, cwd agora-backend, Paket internal/aiwork. Brief: Gate/Begrenzung fuer AI-Arbeit. Real: Grenzwerte (Limit 0/1/n), Freigabe nach Abschluss, korrektes Verhalten bei Kontext-Abbruch, keine Goroutine-/Slot-Leaks nach vielen Zyklen — DETERMINISTISCH ohne Sleep-basierte Assertions (Kanaele/WaitGroups statt Timing); Path: negative Limits, Cancel vor Start, Doppelfreigabe, nil-Kontext.

Vorgehen:
1. /home/lowelodev/ai-lab/bench/tasks/agora-A1-gate/prompt.txt (der Brief) und den versteckten Grader (/home/lowelodev/ai-lab/bench/tasks/agora-A1-gate/grade.sh, grade_test.go|grade.test.js|grade.v2.*) lesen.
2. Baseline-Code /home/lowelodev/ai-lab/bench/workspaces/agora-A1-gate lesen (Schnittstellen, gegen die die Batterie laeuft).
3. /home/lowelodev/ai-lab/bench/robustness-battery/agora-A1-gate/battery.json + Testdatei nach Konvention (6-12 Real, 3-6 Path, WHY je Test). Falls im Verzeichnis schon Teilstuecke liegen: vervollstaendigen.
4. Selbsttest laut Konvention unter /home/lowelodev/ai-lab/bench/audit-scratch/robust/agora-A1-gate/ (Baseline + 2-3 PASS-Abgaben + 1 FAIL-Abgabe, Labels aus
   /home/lowelodev/ai-lab/bench/results.jsonl: letzte Zeile je model/task; Workspaces /home/lowelodev/ai-lab/bench/runs/<label>/agora-A1-gate/ws). Zahlen notieren, Batterie ggf. nachbessern.
5. done-03.txt (Zahlen je geprueftem Label, real_total/path_total, offene Punkte).
