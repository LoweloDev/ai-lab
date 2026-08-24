#!/usr/bin/env bash
# UX-Flaw-Erkennung OHNE Vision via Claude Code headless: Ein-Schuss-Review des
# rohen HTML/CSS-Quelltexts. Shell-Adaption von ux-dom-test.py (identischer Prompt,
# identisches Ausgabeformat runs-ux/<label>-dom.md), Aufruf wie run-task-claude.sh:
# claude 2.1.241 (Host-Binary) im agent-bench-Container, Max-Subscription-OAuth aus
# frischem minimalem cc-home (cc-lib.sh), leeres /work — der Quelltext steht komplett
# im Prompt, Werkzeuge braucht der Lauf nicht.
# Effort: CC_EFFORT (Default xhigh) -> -e CLAUDE_CODE_EFFORT_LEVEL.
# Usage: [CC_EFFORT=xhigh] ux-dom-claude.sh <model-id> <label> [timeout-seconds=900]
#   z.B.: ./ux-dom-claude.sh claude-opus-4-8 cc-opus48
# Ergebnisse: runs-ux/<label>-dom.md (Findings), runs-ux/<label>-dom.json (Envelope)
set -uo pipefail
[ $# -ge 2 ] || { echo "usage: $0 <model-id> <label> [timeout-seconds]" >&2; exit 2; }
MODEL_ID="$1"; LABEL="$2"; LIMIT="${3:-900}"
DIR="$(cd "$(dirname "$0")" && pwd)"
BENCH="$HOME/ai-lab/bench"
# shellcheck source=../cc-lib.sh
source "$BENCH/cc-lib.sh"
cc_effort_env

# Prompt wortgleich zu ux-dom-test.py
PROMPT='Du bist ein UX-Reviewer. Unten der komplette Quelltext dreier Seiten einer kleinen Shop-Website. Nenne die konkreten UX-Probleme als nummerierte Liste — pro Problem: Seite, was falsch ist, warum es Nutzer behindert. Nur echte Probleme, keine Geschmacksfragen.

'
for p in index produkte kontakt; do
  [ -f "$DIR/site/$p.html" ] || { echo "fehlt: $DIR/site/$p.html" >&2; exit 2; }
  PROMPT+="===== $p.html ====="$'\n'"$(cat "$DIR/site/$p.html")"$'\n'
done

RUNS="$DIR/runs-ux"
mkdir -p "$RUNS"
WORK="$RUNS/$LABEL-dom-work"
rm -rf "$WORK"; mkdir -p "$WORK/ws"
cc_prepare_home "$WORK/cc-home"; PRC=$?
[ "$PRC" -eq 3 ] && { echo "AUTH: OAuth-Token abgelaufen — UX-Review $LABEL nicht gestartet" >&2; exit 3; }
[ "$PRC" -ne 0 ] && { echo "cc-home Bootstrap fehlgeschlagen" >&2; exit 2; }

START=$(date +%s.%N)
timeout --signal=SIGTERM --kill-after=30 "$LIMIT" \
  podman run --rm --name "ux-cc-$LABEL" --pull=never --userns=keep-id \
    --network=pasta:--map-host-loopback,169.254.1.2 \
    -v "$WORK/ws:/work:Z" \
    -v "$WORK/cc-home:/home/bench/.claude:Z" \
    -v "$CC_BIN:/usr/local/bin/claude:ro" \
    -v cc-cache:/home/bench/.cache:U \
    -v cc-data:/home/bench/.local:U \
    -e CLAUDE_CONFIG_DIR=/home/bench/.claude \
    -e CLAUDE_CODE_EFFORT_LEVEL \
    agent-bench \
    claude -p "$PROMPT" --output-format json --dangerously-skip-permissions \
      --model "$MODEL_ID" \
    > "$RUNS/$LABEL-dom.json" 2> "$WORK/stderr.log"
RC=$?
END=$(date +%s.%N)
DT=$(awk -v a="$START" -v b="$END" 'BEGIN{printf "%.1f", b-a}')
cc_sync_back "$WORK/cc-home"

OUT=$(jq -r 'if .is_error then "FEHLER: \(.result)" else .result // "" end' \
  "$RUNS/$LABEL-dom.json" 2>/dev/null)
[ -n "$OUT" ] || OUT="FEHLER: kein Envelope (rc=$RC, siehe $WORK/stderr.log)"
{
  printf '# UX-Findings (DOM, ohne Vision) %s (%ss)\n\n' "$LABEL" "$DT"
  printf '%s\n' "$OUT"
} > "$RUNS/$LABEL-dom.md"
echo "$LABEL-dom: ${DT}s, ${#OUT} Zeichen, rc=$RC -> runs-ux/$LABEL-dom.md"
