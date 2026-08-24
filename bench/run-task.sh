#!/usr/bin/env bash
# Run one benchmark task with one model through OpenCode inside the podman sandbox.
# Usage: run-task.sh <model-label> <task-name> [timeout-seconds]
# The model actually used is whatever llama-server on :8080 currently serves.
set -uo pipefail
MODEL="$1"; TASK="$2"; LIMIT="${3:-1200}"
BENCH="$HOME/ai-lab/bench"
WS_SRC="$BENCH/workspaces/$TASK"
RUN_DIR="$BENCH/runs/$MODEL/$TASK"
[ -d "$WS_SRC" ] || { echo "unknown task $TASK"; exit 2; }
rm -rf "$RUN_DIR"; mkdir -p "$RUN_DIR"
cp -r "$WS_SRC" "$RUN_DIR/ws"
git -C "$RUN_DIR/ws" rev-parse HEAD > "$RUN_DIR/ws/.bench-baseline"
PROMPT=$(cat "$BENCH/tasks/$TASK/prompt.txt")

START=$(date +%s.%N)
timeout --signal=SIGTERM --kill-after=30 "$LIMIT" \
  podman run --rm --name "bench-$TASK" --pull=never --userns=keep-id \
    --network=pasta:--map-host-loopback,169.254.1.2 \
    -v "$RUN_DIR/ws:/work:Z" \
    -v "$BENCH/opencode-config:/home/bench/.config/opencode:Z" \
    -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
    -v opencode-cache:/home/bench/.cache:U \
    -v opencode-data:/home/bench/.local:U \
    -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
    agent-bench \
    opencode run -m llamacpp/local --format json "$PROMPT" \
    > "$RUN_DIR/transcript.jsonl" 2> "$RUN_DIR/stderr.log"
RC=$?
END=$(date +%s.%N)
DUR=$(echo "$END $START" | awk '{printf "%.1f", $1-$2}')

GRADE=$(bash "$BENCH/tasks/$TASK/grade.sh" "$RUN_DIR/ws" 2>&1 | tail -1)
CHANGED=$(git -C "$RUN_DIR/ws" diff --stat "$(cat "$RUN_DIR/ws/.bench-baseline")" 2>/dev/null | tail -1)
echo "{\"model\":\"$MODEL\",\"task\":\"$TASK\",\"grade\":\"$GRADE\",\"seconds\":$DUR,\"exit\":$RC,\"changed\":\"$CHANGED\"}" \
  | tee -a "$BENCH/results.jsonl"
