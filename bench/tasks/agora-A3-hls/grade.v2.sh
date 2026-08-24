#!/usr/bin/env bash
# grade.v2.sh — gehaerteter Grader fuer agora-A3-hls (Staging, Patch-Paket P1 aus audit-report.md 4.1).
# Aufruf: bash grade.v2.sh <workspace>      letzte stdout-Zeile: PASS | FAIL <grund>
#
# Gegenueber grade.sh:
#   * Tamper-Guard: Kept-Tests byteweise gegen den unberuehrten Baseline-Workspace unter workspaces/
#     (Existenz + Inhalt); kein git, kein $ws/.bench-baseline (F2, F6, F10).
#   * Bewertung in einer Wegwerf-Kopie des Workspaces ohne .git (mktemp -d, trap) — der Workspace wird
#     nie veraendert; der versteckte Test wird verifiziert (cp + cmp) als zz_bench_grade_test.go einkopiert,
#     Kollision = FAIL (F3).
#   * go test -json unter timeout; PASS nur bei Paket-pass UND Pass-Ereignis je erwartetem Test.
#     Erwartet = Baseline-Tests aus `go test -list` im Pristine (+ Namen aus HIDDEN = grade_test.v2.go) (F1, F11).
#   * Modell-eigene Testdateien bleiben im Lauf (Brief: "die uebrigen Tests des Pakets muessen weiterhin
#     durchlaufen"); ihre Kaperungsmoeglichkeiten sind ueber die Pass-Ereignisse neutralisiert.
#   * P8: Defense-in-depth-Grep der Modell-Quellen — nur WARN-Zeilen VOR der Urteilszeile.
set -euo pipefail
export LC_ALL=C

BENCH=/home/lowelodev/ai-lab/bench
TASK=agora-A3-hls
MODULE=agora-backend          # Modulname in go.mod == Verzeichnisname im Workspace
PKG=internal/livehls
HIDDEN=$BENCH/tasks/$TASK/grade_test.v2.go   # A3: versteckter Test, aus dem Heredoc des alten grade.sh ausgelagert (bei Promotion -> grade_test.go)
PRISTINE=$BENCH/workspaces/$TASK/$MODULE

fail() { echo "FAIL $*"; exit 1; }
tmp=""
cleanup() {
  if [ -n "$tmp" ] && [ -d "$tmp" ]; then
    chmod -R u+rwX -- "$tmp" 2>/dev/null || true
    rm -rf -- "$tmp" 2>/dev/null || true
  fi
}
trap cleanup EXIT

[ -n "${1:-}" ] || fail usage
ws=$(cd -- "$1" 2>/dev/null && pwd -P) || fail no-workspace
[ -d "$ws/$MODULE/$PKG" ] || fail no-workspace
[ -d "$PRISTINE/$PKG" ] || fail no-pristine-baseline
if [ -n "$HIDDEN" ] && [ ! -f "$HIDDEN" ]; then fail no-hidden-test; fi

# 1 Tamper-Guard gegen die unberuehrte Baseline (Existenz + Inhalt; cmp rc 2 = Datei fehlt)
for f in "$PRISTINE/$PKG"/*_test.go; do
  [ -f "$f" ] || continue
  name=$(basename -- "$f")
  cmp -s -- "$f" "$ws/$MODULE/$PKG/$name" || fail "test-file-modified $name"
done

# 2 Wegwerf-Kopie des GANZEN Workspaces ohne .git (Tests lesen Nachbarverzeichnisse, z. B. A3
#   edgeconfig_test.go -> ../../../deploy/hls-cache-headers.yml). Nichts darf aus der Kopie heraus zeigen.
tmp=$(mktemp -d) || fail tmpdir
copy=$tmp/ws
mkdir -- "$copy" || fail tmpdir
while IFS= read -r -d '' e; do
  [ "$(basename -- "$e")" = ".git" ] && continue
  cp -a -- "$e" "$copy/" 2>/dev/null || fail copy-workspace
done < <(find "$ws" -mindepth 1 -maxdepth 1 -print0)
[ -d "$copy/$MODULE/$PKG" ] || fail copy-workspace
chmod -R u+rwX -- "$copy" 2>/dev/null || fail copy-workspace
while IFS= read -r -d '' l; do
  case "$(realpath -m -- "$l")" in
    "$copy"/*) ;;
    *) fail "symlink-escape ${l#"$copy/"}" ;;
  esac
done < <(find "$copy" -type l -print0)
if [ -n "$HIDDEN" ]; then
  tgt="$copy/$MODULE/$PKG/zz_bench_grade_test.go"
  if [ -e "$tgt" ] || [ -L "$tgt" ]; then fail grade-file-collision; fi
  cp -- "$HIDDEN" "$tgt" 2>/dev/null && cmp -s -- "$HIDDEN" "$tgt" || fail grade-file-copy
fi

# 3 Erwartete Tests: Baseline (nur listen, nicht ausfuehren; readonly, damit Pristine unberuehrt bleibt)
#   plus Namen aus der versteckten Datei. Benchmarks werden nicht erwartet (laufen ohne -bench nicht).
expected_base=$(cd -- "$PRISTINE" && env GOFLAGS="-mod=readonly -buildvcs=false" GOPROXY=off GOTOOLCHAIN=local \
  timeout --signal=SIGKILL 300 go test "./$PKG/" -list '.*' 2>/dev/null \
  | grep -E '^(Test|Example|Fuzz)[A-Za-z0-9_]*$') || fail pristine-list
expected_hidden=""
if [ -n "$HIDDEN" ]; then
  expected_hidden=$(grep -oE '^func (Test|Example|Fuzz)[A-Za-z0-9_]*\(' -- "$HIDDEN" | sed -E 's/^func //; s/\($//') \
    || fail hidden-list
fi
expected=$(printf '%s\n%s\n' "$expected_base" "$expected_hidden" | grep -E '^(Test|Example|Fuzz)' | sort -u || true)
[ -n "$expected" ] || fail no-expected-tests

# 4 Lauf unter Timeout (aeusserer SIGKILL-Timeout faengt auch init()-Haenger vor dem Go-Watchdog)
out="$tmp/go-test.json"; err="$tmp/go-test.err"; rc=0
(cd -- "$copy/$MODULE" && env GOFLAGS="-mod=mod -buildvcs=false" GOPROXY=off GOTOOLCHAIN=local \
  timeout --signal=SIGKILL 600 go test "./$PKG/" -count=1 -timeout 300s -json) >"$out" 2>"$err" || rc=$?
if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then fail timeout; fi

# Importpfad des Pakets in den Ereignissen = Modulpfad aus go.mod + PKG (Fallback: Verzeichnisname)
modpath=$(awk '$1=="module"{print $2; exit}' -- "$copy/$MODULE/go.mod" 2>/dev/null || true)
pkgpath="${modpath:-$MODULE}/$PKG"
passed=$(grep -o "\"Action\":\"pass\",\"Package\":\"$pkgpath\",\"Test\":\"[^\"/]*\"" -- "$out" \
  | sed 's/.*"Test":"//; s/"$//' | sort -u || true)
pkg_ok=$(grep -cF "\"Action\":\"pass\",\"Package\":\"$pkgpath\",\"Elapsed\"" -- "$out" || true)
missing=$(comm -23 <(printf '%s\n' "$expected") <(printf '%s\n' "$passed") || true)

# 5 P8 Defense-in-depth: Modell-Quellen (neue oder gegenueber Pristine geaenderte .go-Dateien) auf
#   Kaperungsmuster pruefen — nur melden, nie werten.
warn_re='=== RUN|--- PASS|os\.Exit\(|syscall\.Exit\(|testing\.Testing\(|func TestMain\(|test\.skip|test\.run'
while IFS= read -r -d '' f; do
  rel=${f#"$ws/$MODULE/"}
  if [ -f "$PRISTINE/$rel" ] && cmp -s -- "$f" "$PRISTINE/$rel"; then continue; fi
  hits=$( { grep -naoE "$warn_re" -- "$f" || true; grep -naoF $'\x16' -- "$f" | sed 's/\x16/<0x16>/' || true; } \
    | sort -u | head -n 5 | tr '\n' ' ')
  if [ -n "$hits" ]; then echo "WARN hijack-pattern $rel: $hits"; fi
done < <(find "$ws/$MODULE" -name '*.go' -type f -print0)

# 6 Urteil
echo "INFO go-test rc=$rc pkg-pass=${pkg_ok:-0} expected=$(printf '%s\n' "$expected" | grep -c .) passed=$(printf '%s\n' "$passed" | grep -c . || true) stderr=$(head -c 300 -- "$err" | tr '\n' ' ')"
if [ "$rc" -eq 0 ] && [ "${pkg_ok:-0}" -ge 1 ] && [ -z "$missing" ]; then
  echo PASS
else
  fail "tests-red missing=[$(printf '%s\n' "$missing" | tr '\n' ' ' | sed 's/ $//')]"
fi
