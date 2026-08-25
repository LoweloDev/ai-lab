# 06 — Batterie fuer agora-A2-jsonld

Node, cwd agora-web, Vorbild: tests/jsonLdScript.test.mjs. Brief: JSON-LD-Script-Tag-Injection entschaerfen. Real: Vektoren </script>, <!--, -->, U+2028/U+2029, Unicode-Escapes, verschachtelte Objekte/Arrays, Strings mit Anfuehrungszeichen und Backslashes, Idempotenz (zweimal anwenden = einmal), Ausgabe bleibt gueltiges JSON und entspricht semantisch der Eingabe; Path: zyklische Referenzen, riesige Payloads, undefined/BigInt/Date/Function-Werte.

Vorgehen:
1. /home/lowelodev/ai-lab/bench/tasks/agora-A2-jsonld/prompt.txt (der Brief) und den versteckten Grader (/home/lowelodev/ai-lab/bench/tasks/agora-A2-jsonld/grade.sh, grade_test.go|grade.test.js|grade.v2.*) lesen.
2. Baseline-Code /home/lowelodev/ai-lab/bench/workspaces/agora-A2-jsonld lesen (Schnittstellen, gegen die die Batterie laeuft).
3. /home/lowelodev/ai-lab/bench/robustness-battery/agora-A2-jsonld/battery.json + Testdatei nach Konvention (6-12 Real, 3-6 Path, WHY je Test). Falls im Verzeichnis schon Teilstuecke liegen: vervollstaendigen.
4. Selbsttest laut Konvention unter /home/lowelodev/ai-lab/bench/audit-scratch/robust/agora-A2-jsonld/ (Baseline + 2-3 PASS-Abgaben + 1 FAIL-Abgabe, Labels aus
   /home/lowelodev/ai-lab/bench/results.jsonl: letzte Zeile je model/task; Workspaces /home/lowelodev/ai-lab/bench/runs/<label>/agora-A2-jsonld/ws). Zahlen notieren, Batterie ggf. nachbessern.
5. done-06.txt (Zahlen je geprueftem Label, real_total/path_total, offene Punkte).
