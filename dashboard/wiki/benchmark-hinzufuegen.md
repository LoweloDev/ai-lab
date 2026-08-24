# Neuen Benchmark-Task bauen

**Frage dieser Seite: Wie baue ich einen neuen Suite-Task, der objektiv und manipulationssicher gradet?**

## Das tasks/-Plugin-Format

Ein Task = zwei Dateien + ein vorbereiteter Workspace. Mehr Schnittstelle gibt es nicht — alles, was so aussieht, taucht automatisch in Runnern und Dashboard auf:

```
bench/tasks/<name>/prompt.txt    # die Aufgabe, die der Agent bekommt
bench/tasks/<name>/grade.sh      # objektives Rot/Grün-Grading
bench/workspaces/<name>/         # Git-Klon, auf dem der Agent arbeitet (nicht im Repo)
```

**Prompt-Stil** (Vorbild `agora-A1-gate`): konkreter Test-Befehl zum Reproduzieren, klares Ziel, harte Regeln explizit — „Do not edit any test files." Die Forensik zeigt: Regeln an den Anfang, mit Konsequenz, und die erlaubte Alternative dazuschreiben („neue Tests in eine NEUE Datei").

## Der grade.sh-Kontrakt

- Aufruf: `grade.sh <workspace-pfad>` — `$1` ist die Arbeitskopie **nach** dem Lauf.
- Nur die **letzte stdout-Zeile** zählt (`tail -1`): beginnt sie mit `PASS`, ist der Lauf grün (`PASS pass=52 fail=0` ist ok). Alles andere ist rot. Diagnose-Ausgaben davor sind erwünscht.
- Der Runner hat vorher `git rev-parse HEAD` nach `$1/.bench-baseline` geschrieben — dein Anker für Diffs.

## Manipulationsschutz-Vorlage

Destilliert aus den echten Gradern (`agora-A1-gate`, `aiux-U2-denytools`):

```bash
#!/usr/bin/env bash
set -u
cd "$1/<projekt>" || { echo "FAIL no-workspace"; exit 1; }
base=$(cat "$1/.bench-baseline" 2>/dev/null)
[ -z "$base" ] && { echo "FAIL no-baseline"; exit 1; }

# 1. Testdateien unverändert? (ein Byte Diff = durchgefallen)
if git -C "$1" diff "$base" -- <pfad/zu/tests>/ | grep -q . ; then
  echo "FAIL test-file-modified"; exit 1
fi

# 2. Versteckte Grading-Tests ERST JETZT hineinkopieren —
#    der Agent hat sie nie gesehen und kann nicht dagegen optimieren:
cp ~/ai-lab/bench/tasks/<name>/grade.test.js <tests>/grade.test.js
out=$(<testbefehl> 2>&1)
rm -f <tests>/grade.test.js

# 3. Ergebnis strukturiert auswerten, nie nur Exit-Code raten:
pass=$(printf '%s\n' "$out" | sed -n 's/^ℹ pass \([0-9]*\)$/\1/p' | tail -1)
fail=$(printf '%s\n' "$out" | sed -n 's/^ℹ fail \([0-9]*\)$/\1/p' | tail -1)
[ -z "$pass" ] || [ -z "$fail" ] && { echo "FAIL no-test-summary"; exit 1; }
[ "$fail" -ne 0 ] && { echo "FAIL tests-failing fail=$fail pass=$pass"; exit 1; }
[ "$pass" -lt <soll> ] && { echo "FAIL too-few-passing pass=$pass need=<soll>"; exit 1; }
echo "PASS pass=$pass fail=$fail"
```

Warum das nicht paranoid ist: am 24.08. sind **zwei** funktional korrekte Läufe genau an Punkt 1 durchgefallen — der Check feuert in der Praxis.

## Workspace vorbereiten

Vorbild `prepare-workspaces.sh`: frischer Git-Klon **ohne Remotes und Hooks**; `deploy/`, `.claude/`, `.github/` entfernen. Geplante Bugs als eigener Commit einbauen. Achtung: das Skript löscht `bench/workspaces/` komplett und baut alle Tasks neu.

## Red/Green-Verifikation — bevor der erste Agent läuft

Ein Grader, der immer PASS oder immer FAIL sagt, misst nichts. Beide Richtungen testen:

```bash
# ROT: unbearbeiteter Workspace muss durchfallen
cp -r ~/ai-lab/bench/workspaces/<name> /tmp/red
git -C /tmp/red rev-parse HEAD > /tmp/red/.bench-baseline
bash ~/ai-lab/bench/tasks/<name>/grade.sh /tmp/red   # erwartet: FAIL ...

# GRÜN: deine Referenzlösung von Hand einbauen, dann:
bash ~/ai-lab/bench/tasks/<name>/grade.sh /tmp/green # erwartet: PASS
# Und einmal absichtlich eine Testdatei anfassen → FAIL test-file-modified
```

## Ausführen

```bash
~/ai-lab/serve.sh qwen38 vulkan
~/ai-lab/bench/run-task.sh qwen38-vulkan <name> [timeout]
```

Die Ergebniszeile landet in `bench/results.jsonl`; der Suite-Tab zeigt den neuen Task sofort als Spalte. Alternativ direkt im Läufe-Tab starten.
