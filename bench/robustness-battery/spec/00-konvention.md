# 00 — Konvention & Regeln (fuer JEDEN Auftrag zuerst lesen)

Du arbeitest im Benchmark-Labor /home/lowelodev/ai-lab (im Container unter demselben Pfad). Schreibrechte hast du NUR unter
/home/lowelodev/ai-lab/bench/robustness-battery/, /home/lowelodev/ai-lab/dashboard/ und /home/lowelodev/ai-lab/bench/audit-scratch/robust/. Alles andere ist read-only und darf nicht veraendert werden —
insbesondere /home/lowelodev/ai-lab/bench/runs (Abgaben der Modelle), /home/lowelodev/ai-lab/bench/workspaces (Baselines), /home/lowelodev/ai-lab/bench/tasks (Grader), results.jsonl.
Parallel laufen echte Benchmarks: keine podman-/llama-/curl-Befehle gegen fremde Dienste. Toolchains: go, node
(KEIN python3, KEIN jq im Container — schreibe Hilfsprogramme in Go oder Node).

## Ziel
Eine "Robustheits-Batterie" je Suite-Task: zusaetzliche, spec-abgeleitete Invarianten-Tests, die auf JEDER
Abgabe eines Tasks laufen und zaehlen, wie viele Haertefaelle die Loesung uebersteht. Zusatz-Metrik neben PASS/FAIL,
aendert keine Urteile, keine Messlatte am besten Modell. Vorbilder: /home/lowelodev/ai-lab/bench/robustness-battery/a5/battery_test.go, /home/lowelodev/ai-lab/bench/robustness-battery/a6/battery_test.go
(lies sie: Struktur, WHY-Kommentare, zwei Stufen), Ergebnisberichte /home/lowelodev/ai-lab/bench/robustness-battery/a5-results.md, /home/lowelodev/ai-lab/bench/robustness-battery/a6-results.md.

## Batterie-Konvention (fest)
Je Task ein Verzeichnis /home/lowelodev/ai-lab/bench/robustness-battery/<task>/ mit battery.json:
{ "task": "<task>", "lang": "go"|"node", "cwd": "<relativ zum ws: agora-backend | runtime/web | agora-web>",
  "install": [ {"src": "battery_test.go", "dst": "internal/feed/zz_battery_test.go"} ],
  "cmd": "go test ./internal/feed/ -run ZZBat -count=1 -json -timeout 110s"  ODER  "node --test --test-reporter=tap test/zz_battery.test.js",
  "real_prefix": "ZZBatReal", "path_prefix": "ZZBatPath", "timeout": 150 }
plus die Testdatei(en) (src relativ zum Batterie-Verzeichnis, dst relativ zu cwd).
- Go: Testfunktionen TestZZBatReal* / TestZZBatPath*. Node (node:test): Testnamen beginnen mit "ZZBatReal " / "ZZBatPath ".
- Real = realistische Kanten, voller Brief-Vertrag. Path = pathologische Eingaben; bestanden heisst nur: kein Crash/Panic/Hang,
  kein Datenverlust, Terminierung (je Test eigener Timeout <= 5 s); KEINE fachliche Deutung erzwingen.
- NUR Invarianten, die der Brief /home/lowelodev/ai-lab/bench/tasks/<task>/prompt.txt impliziert — nie die Form einer Implementierung. Jeder Test mit
  WHY-Kommentar. Deterministisch: kein Zufall, keine Zeitabhaengigkeit, keine Netz-/Dateisystem-Nebenwirkungen.
- Die Batterie benutzt nur Schnittstellen, die schon in der Baseline (/home/lowelodev/ai-lab/bench/workspaces/<task>) existieren und vom Brief gefordert sind,
  damit sie auf JEDER korrekten Loesung laeuft. Sie wiederholt NICHT die Faelle des versteckten Graders (/home/lowelodev/ai-lab/bench/tasks/<task>/grade_test.go,
  grade.test.js, grade.v2.*), sondern ergaenzt Kanten. Modell-eigene Tests werden nicht ausgefuehrt (Go: -run ZZBat; Node: nur die Batterie-Datei).
- Selbsttest IMMER auf Wegwerf-Kopien unter /home/lowelodev/ai-lab/bench/audit-scratch/robust/<task>/ (cp -a des Workspace, Batterie einkopieren, laufen lassen):
  gegen die Baseline (Kern-Real-Tests muessen dort reissen, wo das Problem des Tasks liegt), gegen 2-3 offizielle PASS-Abgaben aus
  /home/lowelodev/ai-lab/bench/results.jsonl (letzter Eintrag je Label; erwartet: ueberwiegend bestanden — reisst eine PASS-Abgabe >40 % der Real-Tests, ist die
  Batterie zu implementierungsnah: nachbessern) und gegen 1 FAIL-Abgabe. Zweimal laufen = identisch. Laufzeit < 60 s.

## Ergebnis-Schema /home/lowelodev/ai-lab/bench/robustness-battery/results.json (schreibt NUR der Runner)
{ "schema": 2, "generated": "<ISO>",
  "batteries": { "<task>": { "lang": "go", "real_total": 12, "path_total": 5, "tests": ["…"] } },
  "results": { "<task>": { "<label>": { "buildable": true, "real_pass": 11, "real_total": 12, "path_pass": 5, "path_total": 5,
      "failed": ["…"], "ws_fingerprint": "<sha256>", "computed": "<ISO>", "seconds": 1.4 }  /* buildable:false => *_pass null, "error": "…" */ } } }
Label = Verzeichnisname unter /home/lowelodev/ai-lab/bench/runs/<label>/<task>/ws. Gesamt-Score je Label (rechnet das Dashboard): real_score = Σ real_pass / Σ real_total
ueber alle Tasks mit buildable:true; n_tasks, n_missing, per_task.

## Abschluss jedes Auftrags
Schreibe /home/lowelodev/ai-lab/bench/robustness-battery/spec/done-<NN>.txt: 5-15 Zeilen — was gebaut, Selbsttest-Zahlen, offene Punkte. Ohne diese Datei gilt der Auftrag als nicht erledigt.
