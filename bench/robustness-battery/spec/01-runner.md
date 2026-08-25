# 01 — Generischer Batterie-Runner (in Go)

Baue unter /home/lowelodev/ai-lab/bench/robustness-battery/cmd/battery/ ein Go-Programm (eigenes go.mod, Modulname battery, nur Standardbibliothek) und den Wrapper
/home/lowelodev/ai-lab/bench/robustness-battery/run-all.sh (bash, chmod +x: `cd /home/lowelodev/ai-lab/bench/robustness-battery && go run ./cmd/battery "$@"`; GOFLAGS=-mod=mod GOPROXY=off setzen; GOCACHE auf /tmp/gocache-battery).
Funktion von `battery [--force] [--task T] [--label L] [--jobs 2]`:
1. Liest alle /home/lowelodev/ai-lab/bench/robustness-battery/*/battery.json (Verzeichnisse ohne battery.json ueberspringen). Lege ZUERST battery.json fuer
   /home/lowelodev/ai-lab/bench/robustness-battery/agora-A5-batcher-scratch/ und /home/lowelodev/ai-lab/bench/robustness-battery/agora-A6-scorer-scratch/ an (Kopie der battery_test.go aus /home/lowelodev/ai-lab/bench/robustness-battery/a5/ bzw. /home/lowelodev/ai-lab/bench/robustness-battery/a6/ ins jeweilige
   Task-Verzeichnis; cmd/install wie in deren run-battery.sh; a5/ und a6/ bleiben liegen).
2. Fuer jeden Task alle /home/lowelodev/ai-lab/bench/runs/*/<task>/ws finden. Fingerprint je ws: sha256 ueber sortierte Pfad+Inhalt aller Nicht-Test-Quelldateien
   unter <cwd> (Go: *.go ohne _test.go; Node: *.js/*.mjs/*.ts ohne Verzeichnisse test/, tests/, node_modules/). Nur (task,label) berechnen,
   die in results.json fehlen oder deren Fingerprint abweicht; --force alles.
3. Je Paar: Workspace nach os.MkdirTemp kopieren (node_modules NICHT kopieren, sondern Symlink auf das Original), install-Dateien einkopieren,
   cmd in <cwd> mit Timeout (battery.json.timeout Sekunden, Prozessgruppe killen) ausfuehren, Ergebnis parsen:
   Go: go test -json Events (Action pass/fail/skip je Test, Build-Fehler => buildable:false, error = erste Fehlerzeile);
   Node TAP: Zeilen "ok N - Name" / "not ok N - Name" (Subtests einruecken beachten; Startfehler => buildable:false).
   Zaehlen nach real_prefix/path_prefix; failed = Liste der gerissenen Testnamen. tmp danach loeschen (auch bei Fehler).
4. results.json atomar schreiben (tmp + rename), Schema 2; "generated" nur aendern, wenn wirklich neu berechnet wurde; "batteries"
   aus den Testnamen der Batterie-Datei ableiten (Go: Regex ^func (TestZZBat…)\(; Node: Regex test\(['"]ZZBat… ). Parallelitaet: --jobs (Default 2).
   Log nach /home/lowelodev/ai-lab/bench/robustness-battery/run-all.log (append, eine Zeile je Paar).
5. Test: `./run-all.sh` jetzt ausfuehren (a5/a6 ueber alle vorhandenen Labels), Ergebnis mit /home/lowelodev/ai-lab/bench/robustness-battery/a5-results.md / a6-results.md vergleichen
   (gleiche Zahlen je Label? Abweichungen in done-01.txt erklaeren). Zweiter Aufruf ohne --force: < 2 s, results.json unveraendert.
   Pruefe mit `git -C /home/lowelodev/ai-lab/bench/runs/<label>/<task>/ws status --short` (ro-Mount — darf ohnehin nichts schreiben), dass nichts zurueckbleibt.
Kopfkommentar in run-all.sh mit Aufrufbeispielen. Dann done-01.txt.
