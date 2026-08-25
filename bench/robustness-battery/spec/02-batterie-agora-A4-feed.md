# 02 — Batterie fuer agora-A4-feed

Go, cwd agora-backend, Paket internal/feed. Der Task ist ein Bugfix (gepflanzter Fehler in der Diversity-Penalty von batcher.go). Real: Topic-Mix-Invarianten ueber mehrere Eingabe-Anordnungen, leere/einzelne Topics, gemeinsame Topics an verschiedenen Positionen der TopicSlugs, Live-Kadenz bleibt intakt, Erhalt (nichts verloren/doppelt), Determinismus; Path: leerer Pool, NaN/±Inf-Scores, Duplikat-IDs, riesiger Pool (200). Muss gegen die Baseline-API (batcher.go der Baseline) kompilieren.

Vorgehen:
1. /home/lowelodev/ai-lab/bench/tasks/agora-A4-feed/prompt.txt (der Brief) und den versteckten Grader (/home/lowelodev/ai-lab/bench/tasks/agora-A4-feed/grade.sh, grade_test.go|grade.test.js|grade.v2.*) lesen.
2. Baseline-Code /home/lowelodev/ai-lab/bench/workspaces/agora-A4-feed lesen (Schnittstellen, gegen die die Batterie laeuft).
3. /home/lowelodev/ai-lab/bench/robustness-battery/agora-A4-feed/battery.json + Testdatei nach Konvention (6-12 Real, 3-6 Path, WHY je Test). Falls im Verzeichnis schon Teilstuecke liegen: vervollstaendigen.
4. Selbsttest laut Konvention unter /home/lowelodev/ai-lab/bench/audit-scratch/robust/agora-A4-feed/ (Baseline + 2-3 PASS-Abgaben + 1 FAIL-Abgabe, Labels aus
   /home/lowelodev/ai-lab/bench/results.jsonl: letzte Zeile je model/task; Workspaces /home/lowelodev/ai-lab/bench/runs/<label>/agora-A4-feed/ws). Zahlen notieren, Batterie ggf. nachbessern.
5. done-02.txt (Zahlen je geprueftem Label, real_total/path_total, offene Punkte).
