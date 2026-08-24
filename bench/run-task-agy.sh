#!/usr/bin/env bash
# Variante von run-task-dsh.sh fuer die Antigravity-CLI (agy) im Image agent-bench.
# agy 1.1.19: Host-Binary ro nach /usr/local/bin/agy gemountet. Login per OAuth aus
# einer frischen Kopie der Login-Vorlage ~/ai-lab/bench/agy-auth-home (agy-lib.sh;
# die Vorlage entsteht durch einen einmaligen interaktiven agy-Login im Container
# und bleibt kanonisch; die Kopie liegt in $RUN_DIR/agy-home, ueberlebt den Lauf
# inkl. agy-Logs).
# Unterschiede zu run-task-dsh.sh:
#   - Kommando: agy -p "<prompt>" --output-format json --dangerously-skip-permissions
#     --print-timeout 18m; stdout ist ein JSON-Envelope (status/response/num_turns/
#     usage/session_id) -> $RUN_DIR/transcript.json
#   - Modellwahl via env AGY_MODEL (z.B. gemini-3.7-flash) -> --model,
#     AGY_EFFORT (low|medium|high) -> --effort
#   - eigene Volumes agy-cache/agy-data fuer ~/.cache und ~/.local
# Usage: [AGY_MODEL=gemini-3.7-flash] [AGY_EFFORT=low] run-task-agy.sh <model-label> <task-name> [timeout-seconds]
set -uo pipefail
MODEL="$1"; TASK="$2"; LIMIT="${3:-1200}"
BENCH="$HOME/ai-lab/bench"
# shellcheck source=agy-lib.sh
source "$BENCH/agy-lib.sh"
WS_SRC="$BENCH/workspaces/$TASK"
RUN_DIR="$BENCH/runs/$MODEL/$TASK"
[ -d "$WS_SRC" ] || { echo "unknown task $TASK"; exit 2; }
rm -rf "$RUN_DIR"; mkdir -p "$RUN_DIR"
cp -r "$WS_SRC" "$RUN_DIR/ws"
git -C "$RUN_DIR/ws" rev-parse HEAD > "$RUN_DIR/ws/.bench-baseline"
PROMPT=$(cat "$BENCH/tasks/$TASK/prompt.txt")
agy_prepare_home "$RUN_DIR/agy-home" || { echo "agy-home Kopie fehlgeschlagen"; exit 2; }
agy_model_args

START=$(date +%s.%N)
timeout --signal=SIGTERM --kill-after=30 "$LIMIT" \
  podman run --rm --name "bench-agy-$TASK" --pull=never --userns=keep-id \
    --network=pasta:--map-host-loopback,169.254.1.2 \
    -v "$RUN_DIR/ws:/work:Z" \
    -v "$RUN_DIR/agy-home:/home/bench/.gemini:Z" \
    -v "$HOME/.local/bin/agy:/usr/local/bin/agy:ro" \
    -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
    -v agy-cache:/home/bench/.cache:U \
    -v agy-data:/home/bench/.local:U \
    -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
    agent-bench \
    agy -p "$PROMPT" --output-format json --dangerously-skip-permissions \
      --print-timeout 18m ${AGY_ARGS[@]+"${AGY_ARGS[@]}"} \
    > "$RUN_DIR/transcript.json" 2> "$RUN_DIR/stderr.log"
RC=$?
END=$(date +%s.%N)
DUR=$(echo "$END $START" | awk '{printf "%.1f", $1-$2}')

GRADE=$(bash "$BENCH/tasks/$TASK/grade.sh" "$RUN_DIR/ws" 2>&1 | tail -1)
CHANGED=$(git -C "$RUN_DIR/ws" diff --stat "$(cat "$RUN_DIR/ws/.bench-baseline")" 2>/dev/null | tail -1)
echo "{\"model\":\"$MODEL\",\"task\":\"$TASK\",\"grade\":\"$GRADE\",\"seconds\":$DUR,\"exit\":$RC,\"changed\":\"$CHANGED\"}" \
  | tee -a "$BENCH/results.jsonl"
