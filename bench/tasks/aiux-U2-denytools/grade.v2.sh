#!/usr/bin/env bash
# Staging-Grader v2 fuer aiux-U2-denytools (Audit-Pakete P2 + P6). NICHT installiert: grade.sh bleibt
# der aktive Grader, bis die laufenden Ketten fertig sind (audit-report.md, Abschnitt 4).
#
# Aufruf (identisch zu grade.sh):  bash grade.v2.sh <workspace>
# Vertrag: letzte stdout-Zeile beginnt mit "PASS" oder "FAIL <grund>"; Exit 0 nur bei PASS.
# Urteilsformat wie bisher (Dashboard parst es): "PASS pass=N fail=0",
#   "FAIL tests-failing fail=M pass=N", "FAIL too-few-passing pass=N need=K", "FAIL test-file-modified".
#
# Haertungen gegenueber grade.sh (Befund-IDs aus audit-report.md, Abschnitt 2):
#   F10  Workspace-Pfad wird zuerst absolut gemacht; kein cd, bevor Referenzen geprueft sind.
#   F2   Tamper-Guard: ALLE Dateien unter runtime/web/test/ byteweise (cmp) gegen den unberuehrten
#        Baseline-Workspace -- Existenz UND Inhalt, fail-closed. Weder git noch $ws/.bench-baseline.
#   F3   Bewertet wird eine Wegwerf-Kopie (mktemp -d, trap-Cleanup, chmod u+w); die versteckten
#        Testdateien werden in die KOPIE gelegt (Kollision -> FAIL) und nach dem Kopieren per cmp
#        verifiziert. Der Workspace wird nie beschrieben (grade.sh legte die Datei im Workspace an).
#   F4   Parser wie bisher verankert (^ℹ pass N$, tail -1), NEU zusaetzlich node-Exit-Code und
#        exakte Pass-Zahl (46 Baseline + 6 grade.test.js + Tests aus grade.v2.test.js).
#   F13  grade.v2.test.js: Tool-Tabelle ueber alle Familien, Alias-Paare, und jedes zur Laufzeit aus
#        runtime.capabilities().tools gelesene Tool einzeln gesperrt (P6). grade.test.js bleibt.
#   F11  timeout um den node-Lauf.
#   P8   Defense-in-depth-Grep der vom Modell geaenderten/neuen Quellen -- NUR WARN-Zeilen.
set -euo pipefail

BENCH=/home/lowelodev/ai-lab/bench
TASK=aiux-U2-denytools
SUB=runtime/web                                # Unterbaum, der kopiert und bewertet wird
PRISTINE=$BENCH/workspaces/$TASK/$SUB
KEPT_DIR=test                                  # behaltene Tests: alles unter runtime/web/test/
HIDDEN1=$BENCH/tasks/$TASK/grade.test.js       # bisherige 6 Grading-Tests (bleiben unveraendert)
HIDDEN2=$BENCH/tasks/$TASK/grade.v2.test.js    # P6: Tool-Tabelle + Laufzeit-Sweep
HIDDEN1_NAME=deny-tools.grade.test.js
HIDDEN2_NAME=deny-tools.grade-v2.test.js
BASE_PASS=46                                   # Brief: "All 46 existing tests must keep passing"
NODE_TIMEOUT=${BENCH_GRADE_TIMEOUT:-600}      # Sekunden; nur fuer Selbsttests ueberschreibbar (Host-Umgebung)

fail() { echo "FAIL $*"; exit 1; }

# 0 Pfade absolut und existent (F10)
[ -n "${1:-}" ] || fail no-workspace
ws=$(cd "$1" 2>/dev/null && pwd -P) || fail no-workspace
[ -d "$ws/$SUB/$KEPT_DIR" ] || fail no-workspace
[ -d "$PRISTINE/$KEPT_DIR" ] || fail no-pristine-baseline
[ -f "$HIDDEN1" ] && [ -f "$HIDDEN2" ] || fail no-hidden-tests

# Erwartete Pass-Zahl: Baseline + Anzahl Top-Level-Tests der beiden versteckten Dateien
n1=$(grep -cE '^test\(' "$HIDDEN1" || true)
n2=$(grep -cE '^test\(' "$HIDDEN2" || true)
[ "${n1:-0}" -gt 0 ] && [ "${n2:-0}" -gt 0 ] || fail no-hidden-tests
NEED=$((BASE_PASS + n1 + n2))

# 1 Tamper-Guard gegen die unberuehrte Baseline: jede Baseline-Testdatei muss existieren und
#   byteweise identisch sein (F2)
kept=()
while IFS= read -r -d '' f; do kept+=("${f#"$PRISTINE/"}"); done \
  < <(find "$PRISTINE/$KEPT_DIR" -type f -print0 | sort -z)
[ "${#kept[@]}" -gt 0 ] || fail no-pristine-baseline
for f in "${kept[@]}"; do
  cmp -s "$PRISTINE/$f" "$ws/$SUB/$f" || fail "test-file-modified $(basename "$f")"
done

# 2 Defense-in-depth (P8): nur vom Modell geaenderte oder neue Quelldateien, nur WARN-Zeilen
P8_PATTERN='ℹ (pass|fail)|process\.(exit|kill|abort)\b|process\.(stdout|stderr)\.write *=|test-reporter|node:test'
while IFS= read -r -d '' src; do
  rel=${src#"$ws/$SUB/"}
  if [ -f "$PRISTINE/$rel" ] && cmp -s "$PRISTINE/$rel" "$src"; then continue; fi
  grep -nE "$P8_PATTERN" "$src" 2>/dev/null | head -n 5 | while IFS= read -r hit; do
    echo "WARN suspicious-pattern $rel:$hit"
  done || true
done < <(find "$ws/$SUB/src" -type f \( -name '*.js' -o -name '*.mjs' -o -name '*.cjs' -o -name '*.ts' \) -print0 2>/dev/null)

# 3 Wegwerf-Kopie; behaltene Tests verifizieren; versteckte Tests verifiziert einkopieren (F3)
tmp=$(mktemp -d) || fail tmpdir
trap 'rm -rf "$tmp"' EXIT
cp -a "$ws/$SUB" "$tmp/web" 2>/dev/null || fail copy-workspace
chmod -R u+w "$tmp/web" 2>/dev/null || fail copy-workspace
for f in "${kept[@]}"; do
  cmp -s "$PRISTINE/$f" "$tmp/web/$f" || fail "copy-verify $(basename "$f")"
done
for pair in "$HIDDEN1:$HIDDEN1_NAME" "$HIDDEN2:$HIDDEN2_NAME"; do
  src=${pair%%:*}; name=${pair#*:}; tgt="$tmp/web/$KEPT_DIR/$name"
  [ -e "$tgt" ] && fail "grade-file-collision $name"
  cp "$src" "$tgt" 2>/dev/null && cmp -s "$src" "$tgt" || fail "grade-file-copy $name"
done

# 4 Lauf unter Timeout (F11): Baseline-Tests + beide versteckten Dateien, explizit benannt
out=$(cd "$tmp/web" && NO_COLOR=1 FORCE_COLOR=0 \
      timeout --kill-after=30 "$NODE_TIMEOUT" \
      node --test --test-reporter=spec --test-reporter-destination=stdout \
        test/adapter.test.js test/attached.test.js "test/$HIDDEN1_NAME" "test/$HIDDEN2_NAME" 2>&1) \
  && rc=0 || rc=$?
if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then fail "timeout rc=$rc"; fi

# 5 Verankerter Parser (wie grade.sh) + Exit-Code + exakte Zahl (F4)
pass=$(printf '%s\n' "$out" | sed -n 's/^ℹ pass \([0-9][0-9]*\)$/\1/p' | tail -n 1)
failed=$(printf '%s\n' "$out" | sed -n 's/^ℹ fail \([0-9][0-9]*\)$/\1/p' | tail -n 1)
[ -n "$pass" ] && [ -n "$failed" ] || fail "no-test-summary rc=$rc"
[ "$failed" -eq 0 ] || fail "tests-failing fail=$failed pass=$pass"
[ "$pass" -ge "$NEED" ] || fail "too-few-passing pass=$pass need=$NEED"
[ "$pass" -eq "$NEED" ] || fail "pass-count-mismatch pass=$pass need=$NEED"
[ "$rc" -eq 0 ] || fail "node-exit rc=$rc pass=$pass fail=$failed"

echo "PASS pass=$pass fail=$failed"
