#!/usr/bin/env bash
# INSTALLIERT 20260825-022854 durch apply-haertung.sh aus grade.v2.sh (A5-Lesart: streng). Vorgaenger: grade.v1.sh
# Staging-Grader v2 für agora-A6-scorer-scratch (Audit 24./25.08.2026, Patch-Pakete P1 + P8;
# Referenz-Skelett audit-report.md 4.1). Aufruf wie grade.sh: bash grade.v2.sh <workspace>;
# Ausgabe-Vertrag unverändert: letzte stdout-Zeile beginnt mit PASS oder FAIL.
#
# Gegenüber grade.sh:
#   - Der Workspace wird NICHT verändert: bewertet wird eine Wegwerf-Kopie unter mktemp -d
#     (trap-Cleanup); $1/.bench-baseline wird ignoriert.
#   - Tamper-Guard: die behaltenen *_test.go des Pakets werden byteweise (cmp) gegen den
#     unberührten Baseline-Workspace workspaces/<task> geprüft — Existenz und Inhalt; kein git,
#     kein modell-beschreibbarer Zustand, fail-closed (F2, F6, F10).
#   - Versteckter Test wird verifiziert einkopiert (cmp); Kollision oder Symlink am Zielpfad
#     ist FAIL (F3).
#   - go test -json unter timeout; PASS nur bei Paket-pass UND einem Pass-Ereignis je erwartetem
#     Test — Baseline-Tests aus `go test -list` einer Kopie der Baseline plus die Test-Namen der
#     versteckten Datei. TestMain-/init-os.Exit-Kaperungen liefern keine Pass-Ereignisse → FAIL
#     (F1, F11). Modell-eigene Testdateien bleiben im Lauf (Brief: die übrigen Tests des Pakets
#     müssen weiterhin durchlaufen); ihr Kaperungspotenzial ist über die Pass-Ereignisse neutral.
#   - Defense-in-depth (P8): modell-berührte .go-Dateien unter agora-backend werden auf
#     Kaperungs-Muster gegrept — nur WARN-Zeilen VOR der Urteilszeile, nie ein Urteil.
#   - set -euo pipefail, absolute Pfade; jeder unerwartete Fehler endet in einer FAIL-Zeile.
#   - Versteckte Datei wählbar über GRADE_HIDDEN (Default grade_test.v2.go): ein bloßer
#     Dateiname wird relativ zum Task-Verzeichnis aufgelöst, sonst gilt er als absoluter Pfad.
#     (Bei A6 gibt es nur eine Lesart; der Schalter ist der Vollständigkeit halber identisch.)
set -euo pipefail
export LC_ALL=C

BENCH=/home/lowelodev/ai-lab/bench
TASK=agora-A6-scorer-scratch
PKG=internal/feed
MODPKG=agora-backend/internal/feed
TASKDIR=$BENCH/tasks/$TASK
PRISTINE=$BENCH/workspaces/$TASK/agora-backend
HIDDEN=${GRADE_HIDDEN:-grade_test.go}
case $HIDDEN in */*) ;; *) HIDDEN=$TASKDIR/$HIDDEN ;; esac

tmp=
cleanup() { if [ -n "$tmp" ]; then rm -rf "$tmp"; fi; }
trap cleanup EXIT
trap 'echo "FAIL grader-error line=$LINENO"; exit 1' ERR
fail() { echo "FAIL $*"; exit 1; }
# brief: bis zu 6 Namen einer Zeilenliste, Rest als +N
brief() {
  local list=$1 n
  n=$(printf '%s\n' "$list" | grep -c . || true)
  printf '%s' "$(printf '%s\n' "$list" | grep . | head -6 | tr '\n' ' ' | sed 's/ $//')"
  if [ "$n" -gt 6 ]; then printf ' +%d' "$((n - 6))"; fi
  return 0
}

[ "$#" -ge 1 ] || fail "no-workspace (usage: grade.v2.sh <workspace>)"
[ -f "$HIDDEN" ] || fail "no-hidden-test $HIDDEN"
[ -d "$PRISTINE/$PKG" ] || fail no-pristine-baseline
ws=$(cd "$1" 2>/dev/null && pwd -P) || fail no-workspace
[ -d "$ws/agora-backend/$PKG" ] || fail no-workspace

# 1 Tamper-Guard gegen die unberührte Baseline (Existenz + Inhalt jeder behaltenen Testdatei)
for f in "$PRISTINE/$PKG"/*_test.go; do
  name=$(basename "$f")
  cmp -s "$f" "$ws/agora-backend/$PKG/$name" || fail "test-file-modified $name"
done

# 2 Wegwerf-Kopien: Abgabe (wird bewertet) und Baseline (nur für die Testliste)
tmp=$(mktemp -d) || fail tmpdir
cp -a "$ws/agora-backend" "$tmp/agora-backend" 2>/dev/null || fail copy-workspace
chmod -R u+w "$tmp/agora-backend" 2>/dev/null || fail copy-workspace
cp -a "$PRISTINE" "$tmp/pristine-backend" 2>/dev/null || fail copy-pristine
tgt=$tmp/agora-backend/$PKG/zz_bench_grade_test.go
if [ -e "$tgt" ] || [ -L "$tgt" ]; then fail grade-file-collision; fi
cp "$HIDDEN" "$tgt" 2>/dev/null && cmp -s "$HIDDEN" "$tgt" || fail grade-file-copy

# 3 Erwartete Tests: Baseline (nur Listen, in der Kopie) + versteckte Datei
hidden_tests=$(grep -oE '^func (Test[A-Za-z0-9_]+)\(' "$HIDDEN" | sed -E 's/^func //; s/\($//' | sort -u || true)
[ -n "$hidden_tests" ] || fail "no-hidden-tests $HIDDEN"
list_out=$(cd "$tmp/pristine-backend" && GOFLAGS="-mod=readonly -buildvcs=false" GOPROXY=off GOTOOLCHAIN=local \
  timeout --signal=SIGKILL 300 go test "./$PKG/" -list '.*' 2>/dev/null) || fail pristine-list-failed
base_tests=$(printf '%s\n' "$list_out" | grep -E '^(Test|Fuzz)[A-Za-z0-9_]*$' || true)
[ -n "$base_tests" ] || fail no-expected-tests
expected=$(printf '%s\n%s\n' "$base_tests" "$hidden_tests" | grep . | sort -u)

# 4 Defense-in-depth (P8): Kaperungs-Muster in modell-berührten Quellen — nur melden
susp=( $'\x16' '=== RUN' '--- PASS' 'os.Exit' 'syscall.Exit' 'testing.Testing' 'TestMain(' 'test.skip' 'test.run' )
while IFS= read -r -d '' f; do
  rel=${f#"$ws/agora-backend/"}
  if [ -f "$PRISTINE/$rel" ] && cmp -s "$f" "$PRISTINE/$rel"; then continue; fi
  for p in "${susp[@]}"; do
    if grep -qF -- "$p" "$f" 2>/dev/null; then
      echo "WARN suspicious-pattern file=$rel pattern=$(printf '%q' "$p")"
    fi
  done
done < <(find "$ws/agora-backend" -type f -name '*.go' -print0 2>/dev/null | sort -z)

# 5 Lauf unter Timeout; PASS nur bei Paket-pass UND Pass-Ereignis je erwartetem Test
out=$(cd "$tmp/agora-backend" && GOFLAGS="-mod=mod -buildvcs=false" GOPROXY=off GOTOOLCHAIN=local \
  timeout --signal=SIGKILL 600 go test "./$PKG/" -count=1 -timeout 300s -json 2>/dev/null) && rc=0 || rc=$?
if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then fail "timeout rc=$rc"; fi
passed=$(printf '%s\n' "$out" | grep -o "\"Action\":\"pass\",\"Package\":\"$MODPKG\",\"Test\":\"[^\"/]*\"" | sed 's/.*"Test":"//; s/"$//' | sort -u || true)
failed=$(printf '%s\n' "$out" | grep -o "\"Action\":\"fail\",\"Package\":\"$MODPKG\",\"Test\":\"[^\"/]*\"" | sed 's/.*"Test":"//; s/"$//' | sort -u || true)
pkg_ok=$(printf '%s\n' "$out" | grep -c "\"Action\":\"pass\",\"Package\":\"$MODPKG\",\"Elapsed\"" || true)
missing=$(comm -23 <(printf '%s\n' "$expected") <(printf '%s\n' "$passed") || true)

if [ "${pkg_ok:-0}" -ge 1 ] && [ -z "$missing" ]; then
  echo PASS
  exit 0
fi
if printf '%s\n' "$out" | grep -q '"Action":"build-fail"\|\[build failed\]\|\[setup failed\]'; then
  echo "FAIL tests-red build-failed rc=$rc"
  exit 1
fi
detail="missing=[$(brief "$missing")]"
if [ -n "$failed" ]; then detail="$detail failed=[$(brief "$failed")]"; fi
if [ "$rc" -ne 0 ]; then detail="$detail rc=$rc"; fi
echo "FAIL tests-red $detail"
exit 1
