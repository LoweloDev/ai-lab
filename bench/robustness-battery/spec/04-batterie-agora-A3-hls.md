# 04 — Batterie fuer agora-A3-hls

Go, cwd agora-backend, Paket internal/livehls. Brief: Cache-Policy/Key-Klassifikation fuer HLS-Auslieferung. Real: alle Segment-/Playlist-/Init-Muster inkl. Query-Strings, Gross-/Kleinschreibung, Unterverzeichnisse, die im Brief genannten Grenzfaelle der Klassifikation, Konsistenz Policy<->Klasse; Path: leere Keys, sehr lange Keys, Sonderzeichen/Unicode, Pfad-Traversal-Strings, doppelte Slashes.

Vorgehen:
1. /home/lowelodev/ai-lab/bench/tasks/agora-A3-hls/prompt.txt (der Brief) und den versteckten Grader (/home/lowelodev/ai-lab/bench/tasks/agora-A3-hls/grade.sh, grade_test.go|grade.test.js|grade.v2.*) lesen.
2. Baseline-Code /home/lowelodev/ai-lab/bench/workspaces/agora-A3-hls lesen (Schnittstellen, gegen die die Batterie laeuft).
3. /home/lowelodev/ai-lab/bench/robustness-battery/agora-A3-hls/battery.json + Testdatei nach Konvention (6-12 Real, 3-6 Path, WHY je Test). Falls im Verzeichnis schon Teilstuecke liegen: vervollstaendigen.
4. Selbsttest laut Konvention unter /home/lowelodev/ai-lab/bench/audit-scratch/robust/agora-A3-hls/ (Baseline + 2-3 PASS-Abgaben + 1 FAIL-Abgabe, Labels aus
   /home/lowelodev/ai-lab/bench/results.jsonl: letzte Zeile je model/task; Workspaces /home/lowelodev/ai-lab/bench/runs/<label>/agora-A3-hls/ws). Zahlen notieren, Batterie ggf. nachbessern.
5. done-04.txt (Zahlen je geprueftem Label, real_total/path_total, offene Punkte).
