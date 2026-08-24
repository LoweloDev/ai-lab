#!/usr/bin/env bash
# UX-Review (DOM, ohne Vision) via Antigravity-CLI: derselbe Prompt+HTML-Payload wie
# ux-dom-test.py, aber als einzelner headless agy-Aufruf im agent-bench-Container.
# Die Antwort (.response des JSON-Envelopes) landet in runs-ux/<label>-dom.md im
# gleichen Format wie bei ux-dom-test.py.
# Usage: [AGY_MODEL=gemini-3.7-flash] [AGY_EFFORT=low] ux-dom-agy.sh <label>
set -uo pipefail
[ $# -ge 1 ] || { echo "usage: $0 <label>   (Modell via env AGY_MODEL/AGY_EFFORT)" >&2; exit 2; }
LABEL="$1"
DIR="$(cd "$(dirname "$0")" && pwd)"
BENCH="$HOME/ai-lab/bench"
# shellcheck source=../agy-lib.sh
source "$BENCH/agy-lib.sh"
agy_require_auth || exit 2
agy_model_args

# Prompt wortgleich mit ux-dom-test.py
PROMPT='Du bist ein UX-Reviewer. Unten der komplette Quelltext dreier Seiten einer kleinen Shop-Website. Nenne die konkreten UX-Probleme als nummerierte Liste — pro Problem: Seite, was falsch ist, warum es Nutzer behindert. Nur echte Probleme, keine Geschmacksfragen.

'
SRC=""
for p in index produkte kontakt; do
  SRC+="===== $p.html =====
$(cat "$DIR/site/$p.html")
"
done

WORK="$DIR/runs-ux/.agy-$LABEL"
rm -rf "$WORK"; mkdir -p "$WORK/ws"
agy_prepare_home "$WORK/agy-home" || exit 2

T0=$(date +%s.%N)
timeout --signal=SIGTERM --kill-after=30 900 \
  podman run --rm --name "ux-agy-$LABEL" --pull=never --userns=keep-id \
    --network=pasta:--map-host-loopback,169.254.1.2 \
    -v "$WORK/ws:/work:Z" \
    -v "$WORK/agy-home:/home/bench/.gemini:Z" \
    -v "$HOME/.local/bin/agy:/usr/local/bin/agy:ro" \
    -v agy-cache:/home/bench/.cache:U \
    -v agy-data:/home/bench/.local:U \
    agent-bench \
    agy -p "$PROMPT$SRC" --output-format json --dangerously-skip-permissions \
      --print-timeout 12m ${AGY_ARGS[@]+"${AGY_ARGS[@]}"} \
    > "$WORK/transcript.json" 2> "$WORK/stderr.log"
RC=$?
DT=$(awk -v a="$T0" -v b="$(date +%s.%N)" 'BEGIN{printf "%.1f", b-a}')

OUT=$(jq -r '.response // empty' "$WORK/transcript.json" 2>/dev/null)
if [ -z "$OUT" ]; then
  echo "FEHLER: keine Antwort (agy rc=$RC, status=$(jq -r '.status // "?"' "$WORK/transcript.json" 2>/dev/null), error=$(jq -r '.error // "?"' "$WORK/transcript.json" 2>/dev/null)); Details: $WORK" >&2
  exit 1
fi
printf '# UX-Findings (DOM, ohne Vision) %s (%ss)\n\n%s\n' "$LABEL" "$DT" "$OUT" \
  > "$DIR/runs-ux/$LABEL-dom.md"
echo "$LABEL-dom: ${DT}s, ${#OUT} Zeichen -> runs-ux/$LABEL-dom.md (agy rc=$RC)"
