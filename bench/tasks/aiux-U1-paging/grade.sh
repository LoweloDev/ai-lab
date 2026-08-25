#!/usr/bin/env bash
# INSTALLIERT 20260825-022854 durch apply-haertung.sh aus grade.v2.sh (A5-Lesart: streng). Vorgaenger: grade.v1.sh
# Staging-Grader v2 fuer aiux-U1-paging (Audit-Paket P2). NICHT installiert: grade.sh bleibt der
# aktive Grader, bis die laufenden Ketten fertig sind (audit-report.md, Abschnitt 4).
#
# Aufruf (identisch zu grade.sh):  bash grade.v2.sh <workspace>
# Vertrag: letzte stdout-Zeile beginnt mit "PASS" oder "FAIL <grund>"; Exit 0 nur bei PASS.
# Urteilsformat wie bisher: "PASS" bzw. "FAIL tests (pass=N fail=M rc=R)".
#
# Haertungen gegenueber grade.sh (Befund-IDs aus audit-report.md, Abschnitt 2):
#   F10  Workspace-Pfad wird zuerst absolut gemacht; kein cd, bevor Referenzen geprueft sind.
#   F2   Tamper-Guard: ALLE Dateien unter runtime/web/test/ byteweise (cmp) gegen den unberuehrten
#        Baseline-Workspace -- Existenz UND Inhalt, fail-closed. Weder git noch $ws/.bench-baseline
#        werden gelesen. Zusaetzliche Modell-Dateien im Testverzeichnis werden nicht bewertet (der
#        Lauf nennt die Testdateien explizit; node --test fuehrt nichts anderes aus).
#   F3   Bewertet wird eine Wegwerf-Kopie (mktemp -d, trap-Cleanup, chmod u+w).
#   F4   Verankerter Summary-Parser: nur "^ℹ pass N$" / "^ℹ fail N$", davon die LETZTE Zeile
#        (immer der node-Elternprozess), plus node-Exit-Code und exakte Pass-Zahl (46).
#   F11  timeout um den node-Lauf.
#   P8   Defense-in-depth-Grep der vom Modell geaenderten/neuen Quellen -- NUR WARN-Zeilen.
set -euo pipefail

BENCH=/home/lowelodev/ai-lab/bench
TASK=aiux-U1-paging
SUB=runtime/web                                # Unterbaum, der kopiert und bewertet wird
PRISTINE=$BENCH/workspaces/$TASK/$SUB
KEPT_DIR=test                                  # behaltene Tests: alles unter runtime/web/test/
TEST_CMD=(node --test --test-reporter=spec --test-reporter-destination=stdout test/adapter.test.js test/attached.test.js)
EXPECT_PASS=46                                 # Brief: "All 46 tests must pass"
NODE_TIMEOUT=${BENCH_GRADE_TIMEOUT:-600}      # Sekunden; nur fuer Selbsttests ueberschreibbar (Host-Umgebung)

fail() { echo "FAIL $*"; exit 1; }

# 0 Pfade absolut und existent (F10)
[ -n "${1:-}" ] || fail no-workspace
ws=$(cd "$1" 2>/dev/null && pwd -P) || fail no-workspace
[ -d "$ws/$SUB/$KEPT_DIR" ] || fail no-workspace
[ -d "$PRISTINE/$KEPT_DIR" ] || fail no-pristine-baseline

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

# 3 Wegwerf-Kopie des Unterbaums; behaltene Tests in der Kopie nochmals verifizieren (F3)
tmp=$(mktemp -d) || fail tmpdir
trap 'rm -rf "$tmp"' EXIT
cp -a "$ws/$SUB" "$tmp/web" 2>/dev/null || fail copy-workspace
chmod -R u+w "$tmp/web" 2>/dev/null || fail copy-workspace
for f in "${kept[@]}"; do
  cmp -s "$PRISTINE/$f" "$tmp/web/$f" || fail "copy-verify $(basename "$f")"
done

# 4 Lauf unter Timeout (F11)
out=$(cd "$tmp/web" && NO_COLOR=1 FORCE_COLOR=0 \
      timeout --kill-after=30 "$NODE_TIMEOUT" "${TEST_CMD[@]}" 2>&1) && rc=0 || rc=$?
if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then fail "timeout rc=$rc"; fi

# 5 Verankerter Parser + Exit-Code + exakte Zahl (F4)
pass=$(printf '%s\n' "$out" | sed -n 's/^ℹ pass \([0-9][0-9]*\)$/\1/p' | tail -n 1)
failed=$(printf '%s\n' "$out" | sed -n 's/^ℹ fail \([0-9][0-9]*\)$/\1/p' | tail -n 1)
[ -n "$pass" ] && [ -n "$failed" ] || fail "no-test-summary rc=$rc"
if [ "$rc" -ne 0 ] || [ "$failed" -ne 0 ] || [ "$pass" -ne "$EXPECT_PASS" ]; then
  fail "tests (pass=$pass fail=$failed rc=$rc)"
fi

echo PASS
