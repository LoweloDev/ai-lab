#!/usr/bin/env bash
# INSTALLIERT 20260825-022854 durch apply-haertung.sh aus grade.v2.sh (A5-Lesart: streng). Vorgaenger: grade.v1.sh
# Staging-Grader v2 fuer agora-A2-jsonld (Audit-Paket P2). NICHT installiert: grade.sh bleibt der
# aktive Grader, bis die laufenden Ketten fertig sind (audit-report.md, Abschnitt 4).
#
# Aufruf (identisch zu grade.sh):  bash grade.v2.sh <workspace>
# Vertrag: letzte stdout-Zeile beginnt mit "PASS" oder "FAIL <grund>"; Exit 0 nur bei PASS.
#
# Haertungen gegenueber grade.sh (Befund-IDs aus audit-report.md, Abschnitt 2):
#   F10  Workspace-Pfad wird zuerst absolut gemacht; kein cd, bevor Referenzen geprueft sind.
#   F2   Tamper-Guard: die behaltene Spec wird byteweise (cmp) gegen den unberuehrten Baseline-
#        Workspace unter workspaces/<task> geprueft -- Existenz UND Inhalt, fail-closed. Weder git
#        noch $ws/.bench-baseline werden gelesen (beides liegt im Einflussbereich des Modells).
#   F3   Bewertet wird eine Wegwerf-Kopie (mktemp -d, trap-Cleanup, chmod u+w). Der Workspace wird
#        weder beschrieben noch bleibt etwas zurueck.
#   F4   Verankerter Summary-Parser: nur Zeilen "^ℹ pass N$" / "^ℹ fail N$", davon die LETZTE
#        (die stammt immer vom node-Elternprozess, nie aus Modell-Code), plus node-Exit-Code und
#        exakte Pass-Zahl statt ">= 4".
#   F7   POSITIVE Verdrahtungspruefung: in der ld+json-Script-Zeile muss __html den Helper
#        jsonLdScriptBody(...) aufrufen (oder eine Variable, die direkt daraus zugewiesen wurde);
#        Negativ-Grep auf "JSON.stringify(jsonLd" OHNE schliessende Klammer.
#   F11  timeout um den node-Lauf.
#   P8   Defense-in-depth-Grep der vom Modell geaenderten/neuen Quellen -- NUR als WARN-Zeilen vor
#        der Urteilszeile, nie als Urteil.
#
# P7-Hinweis (F14) -- bewusst nur als Kommentar, keine Aenderung im Aufruf:
#   Die Spec importiert src/lib/jsonLdScript.ts direkt in Node (Type-Stripping). Nicht loeschbare
#   TS-Syntax im Helper (enum, Parameter-Properties, namespace, Decorators) ist dort ein SyntaxError,
#   obwohl Next.js sie baut. Abhilfe waere ein Satz im Brief ("nur loeschbare TS-Syntax, Node laedt
#   die Datei direkt") oder `--experimental-transform-types` im node-Aufruf. Hier NICHT aktiviert:
#   der Grader laeuft exakt das Kommando, das der Brief dem Modell nennt -- das Modell konnte den
#   Fehler also selbst sehen. Wer das Flag setzt, muss den Brief mitziehen (Suite-Version, 4.4/6).
set -euo pipefail

BENCH=/home/lowelodev/ai-lab/bench
TASK=agora-A2-jsonld
SUB=agora-web                                  # Unterbaum, der kopiert und bewertet wird
PRISTINE=$BENCH/workspaces/$TASK/$SUB
KEPT_TESTS=(tests/jsonLdScript.test.mjs)       # behaltene Tests laut Brief ("Do not edit the test file")
TEST_CMD=(node --test --test-reporter=spec --test-reporter-destination=stdout tests/jsonLdScript.test.mjs)
EXPECT_PASS=4                                  # genau die 4 Tests der Spec
PAGE='src/app/debate/[id]/page.tsx'
HELPER=jsonLdScriptBody
NODE_TIMEOUT=${BENCH_GRADE_TIMEOUT:-600}      # Sekunden; nur fuer Selbsttests ueberschreibbar (Host-Umgebung)

fail() { echo "FAIL $*"; exit 1; }

# 0 Pfade absolut und existent (F10)
[ -n "${1:-}" ] || fail no-workspace
ws=$(cd "$1" 2>/dev/null && pwd -P) || fail no-workspace
[ -d "$ws/$SUB/tests" ] || fail no-workspace
[ -f "$ws/$SUB/$PAGE" ] || fail no-workspace
[ -d "$PRISTINE/tests" ] || fail no-pristine-baseline

# 1 Tamper-Guard gegen die unberuehrte Baseline: Existenz + Inhalt (F2)
for f in "${KEPT_TESTS[@]}"; do
  [ -f "$PRISTINE/$f" ] || fail "no-pristine-baseline $f"
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
done < <(find "$ws/$SUB/src" -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.js' -o -name '*.jsx' -o -name '*.mjs' -o -name '*.cjs' \) -print0 2>/dev/null)

# 3 Wegwerf-Kopie des Unterbaums; behaltene Tests in der Kopie nochmals verifizieren (F3)
tmp=$(mktemp -d) || fail tmpdir
trap 'rm -rf "$tmp"' EXIT
cp -a "$ws/$SUB" "$tmp/$SUB" 2>/dev/null || fail copy-workspace
chmod -R u+w "$tmp/$SUB" 2>/dev/null || fail copy-workspace
for f in "${KEPT_TESTS[@]}"; do
  cmp -s "$PRISTINE/$f" "$tmp/$SUB/$f" || fail "copy-verify $(basename "$f")"
done

# 4 Lauf unter Timeout (F11); Ausgabe komplett einsammeln
out=$(cd "$tmp/$SUB" && NO_COLOR=1 FORCE_COLOR=0 \
      timeout --kill-after=30 "$NODE_TIMEOUT" "${TEST_CMD[@]}" 2>&1) && rc=0 || rc=$?
if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then fail "timeout rc=$rc"; fi

# 5 Verankerter Parser: letzte echte Summary-Zeile des Elternprozesses + Exit-Code (F4)
pass=$(printf '%s\n' "$out" | sed -n 's/^ℹ pass \([0-9][0-9]*\)$/\1/p' | tail -n 1)
failed=$(printf '%s\n' "$out" | sed -n 's/^ℹ fail \([0-9][0-9]*\)$/\1/p' | tail -n 1)
[ -n "$pass" ] && [ -n "$failed" ] || fail "no-test-summary rc=$rc"
if [ "$rc" -ne 0 ] || [ "$failed" -ne 0 ] || [ "$pass" -ne "$EXPECT_PASS" ]; then
  fail "tests-red (pass=$pass fail=$failed rc=$rc)"
fi

# 6 Verdrahtung in der Seite (F7): positiv UND negativ, am unveraenderten Original im Workspace
page="$ws/$SUB/$PAGE"
flat=$(tr -s ' \n\t\r' ' ' < "$page")
# Negativ: der nackte Aufruf darf in keiner Schreibweise stehen (auch nicht mit null,0 / as ...)
if grep -q 'JSON.stringify(jsonLd' "$page"; then fail "not-wired-into-page (JSON.stringify(jsonLd still present)"; fi
# Positiv: jede ld+json-Script-Zeile muss __html aus dem Helper speisen
exprs=$(printf '%s' "$flat" | grep -oE 'application/ld\+json[^<>]*__html: *[^}]*' | sed -E 's/.*__html: *//; s/ *$//' || true)
[ -n "$exprs" ] || fail "not-wired-into-page (no ld+json script with __html found)"
while IFS= read -r expr; do
  [ -n "$expr" ] || continue
  case "$expr" in
    "$HELPER("*) continue ;;
  esac
  # Zwischenvariable erlaubt, wenn sie direkt aus dem Helper zugewiesen wird: const x = jsonLdScriptBody(
  if printf '%s' "$expr" | grep -qE '^[A-Za-z_$][A-Za-z0-9_$]*$' \
     && printf '%s' "$flat" | grep -qE "(const|let|var) +$expr *(:[^=]*)?= *$HELPER\("; then
    continue
  fi
  fail "not-wired-into-page (__html: $expr)"
done <<< "$exprs"
# Herkunft des Helpers nur melden, nicht werten (Barrel-Re-Exports waeren legitim)
printf '%s' "$flat" | grep -qE "import[^;]*\b$HELPER\b[^;]*from *['\"][^'\"]*lib/jsonLdScript['\"]" \
  || echo "WARN helper-import-not-from-lib/jsonLdScript"

echo PASS
