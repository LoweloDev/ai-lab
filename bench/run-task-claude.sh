#!/usr/bin/env bash
# Variante von run-task-agy.sh fuer Claude Code (claude, Host-Binary 2.1.241)
# im Image agent-bench. Auth via Max-Subscription-OAuth: cc-lib.sh baut pro Lauf
# ein minimales frisches /home/bench/.claude (Kopie NUR von .credentials.json
# plus erzeugte settings.json/.claude.json; das echte ~/.claude wird nie gemountet).
# Unterschiede zu run-task-agy.sh:
#   - Kommando: claude -p "<prompt>" --output-format json
#     --dangerously-skip-permissions --model <model-id>; stdout ist ein
#     JSON-Envelope (type/subtype/is_error/result/usage/modelUsage/session_id)
#     -> $RUN_DIR/transcript.json
#   - Modell-ID ist Pflicht-Argument (claude-opus-5, claude-opus-4-8, ...)
#   - Effort: CC_EFFORT (Default xhigh) -> -e CLAUDE_CODE_EFFORT_LEVEL
#   - CLAUDE_CONFIG_DIR=/home/bench/.claude, damit auch .claude.json
#     (Onboarding-/Bypass-Flags) im gemounteten cc-home liegt
#   - eigene Volumes cc-cache/cc-data fuer ~/.cache und ~/.local
# Usage: [CC_EFFORT=xhigh] run-task-claude.sh <model-id> <label> <task-name> [timeout-seconds]
#   z.B.: run-task-claude.sh claude-opus-4-8 cc-opus48 aiux-U1-paging
set -uo pipefail
[ $# -ge 3 ] || { echo "usage: $0 <model-id> <label> <task-name> [timeout-seconds]" >&2; exit 2; }
MODEL_ID="$1"; LABEL="$2"; TASK="$3"; LIMIT="${4:-1200}"
BENCH="$HOME/ai-lab/bench"
# shellcheck source=cc-lib.sh
source "$BENCH/cc-lib.sh"
WS_SRC="$BENCH/workspaces/$TASK"
RUN_DIR="$BENCH/runs/$LABEL/$TASK"
[ -d "$WS_SRC" ] || { echo "unknown task $TASK"; exit 2; }
rm -rf "$RUN_DIR"; mkdir -p "$RUN_DIR"
cp -r "$WS_SRC" "$RUN_DIR/ws"
git -C "$RUN_DIR/ws" rev-parse HEAD > "$RUN_DIR/ws/.bench-baseline"
PROMPT=$(cat "$BENCH/tasks/$TASK/prompt.txt")
cc_prepare_home "$RUN_DIR/cc-home"; PRC=$?
[ "$PRC" -eq 3 ] && { echo "AUTH: OAuth-Token abgelaufen — $LABEL/$TASK NICHT gestartet (kein results.jsonl-Eintrag)"; exit 3; }
[ "$PRC" -ne 0 ] && { echo "cc-home Bootstrap fehlgeschlagen"; exit 2; }
cc_effort_env
printf '{"model_id":"%s","label":"%s","effort":"%s","claude":"2.1.241"}\n' \
  "$MODEL_ID" "$LABEL" "$CLAUDE_CODE_EFFORT_LEVEL" > "$RUN_DIR/meta.json"

START=$(date +%s.%N)
timeout --signal=SIGTERM --kill-after=30 "$LIMIT" \
  podman run --rm --name "bench-cc-$TASK" --pull=never --userns=keep-id \
    --network=pasta:--map-host-loopback,169.254.1.2 \
    -v "$RUN_DIR/ws:/work:Z" \
    -v "$RUN_DIR/cc-home:/home/bench/.claude:Z" \
    -v "$CC_BIN:/usr/local/bin/claude:ro" \
    -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
    -v cc-cache:/home/bench/.cache:U \
    -v cc-data:/home/bench/.local:U \
    -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
    -e CLAUDE_CONFIG_DIR=/home/bench/.claude \
    -e CLAUDE_CODE_EFFORT_LEVEL \
    agent-bench \
    claude -p "$PROMPT" --output-format json --dangerously-skip-permissions \
      --model "$MODEL_ID" \
    > "$RUN_DIR/transcript.json" 2> "$RUN_DIR/stderr.log"
RC=$?
END=$(date +%s.%N)
DUR=$(echo "$END $START" | awk '{printf "%.1f", $1-$2}')
cc_sync_back "$RUN_DIR/cc-home"
# Vergiftungs-Wächter: Envelope fehlt/leer + rc!=0 + <15 s => Auth/Infra, kein Modellversuch.
if [ "$RC" -ne 0 ] && ! jq -e '.usage // .result' "$RUN_DIR/transcript.json" >/dev/null 2>&1 \
   && awk -v d="$DUR" 'BEGIN{exit !(d<15)}'; then
  echo "INFRA: $LABEL/$TASK endete nach ${DUR}s ohne Envelope (rc=$RC) — vermutlich Auth. KEIN Eintrag in results.jsonl."
  tail -3 "$RUN_DIR/stderr.log" 2>/dev/null; exit 3
fi

GRADE=$(bash "$BENCH/tasks/$TASK/grade.sh" "$RUN_DIR/ws" 2>&1 | tail -1)
CHANGED=$(git -C "$RUN_DIR/ws" diff --stat "$(cat "$RUN_DIR/ws/.bench-baseline")" 2>/dev/null | tail -1)
echo "{\"model\":\"$LABEL\",\"task\":\"$TASK\",\"grade\":\"$GRADE\",\"seconds\":$DUR,\"exit\":$RC,\"changed\":\"$CHANGED\"}" \
  | tee -a "$BENCH/results.jsonl"
