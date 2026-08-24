#!/usr/bin/env bash
# Variante von run-task.sh fuer API-Modelle via OpenCode im podman-Sandbox-Container.
# Abgeleitet aus run-task.sh (2026-08-24); Unterschiede:
#   - Modell-ID kommt als $1 und geht an `opencode run -m` (z.B. deepseek/deepseek-v4-flash)
#   - Config-Verzeichnis via env OC_CONFIG (Default: opencode-config; fuer API: opencode-config-api)
#   - Label in results.jsonl/runs via env LABEL (Default: Modell-ID mit "/" -> "-")
#   - DEEPSEEK_API_KEY und GEMINI_API_KEY werden in den Container durchgereicht
# Usage: [OC_CONFIG=opencode-config-api] [LABEL=oc-v4-flash] run-task-api.sh <model-id> <task-name> [timeout-seconds]
set -uo pipefail
MODEL_ID="$1"; TASK="$2"; LIMIT="${3:-1200}"
OC_CONFIG="${OC_CONFIG:-opencode-config}"
LABEL="${LABEL:-${MODEL_ID//\//-}}"
BENCH="$HOME/ai-lab/bench"
WS_SRC="$BENCH/workspaces/$TASK"
RUN_DIR="$BENCH/runs/$LABEL/$TASK"
[ -d "$WS_SRC" ] || { echo "unknown task $TASK"; exit 2; }
[ -d "$BENCH/$OC_CONFIG" ] || { echo "unknown config dir $OC_CONFIG"; exit 2; }
rm -rf "$RUN_DIR"; mkdir -p "$RUN_DIR"
cp -r "$WS_SRC" "$RUN_DIR/ws"
git -C "$RUN_DIR/ws" rev-parse HEAD > "$RUN_DIR/ws/.bench-baseline"
PROMPT=$(cat "$BENCH/tasks/$TASK/prompt.txt")

START=$(date +%s.%N)
timeout --signal=SIGTERM --kill-after=30 "$LIMIT" \
  podman run --rm --name "bench-api-$TASK" --pull=never --userns=keep-id \
    --network=pasta:--map-host-loopback,169.254.1.2 \
    -v "$RUN_DIR/ws:/work:Z" \
    -v "$BENCH/$OC_CONFIG:/home/bench/.config/opencode:Z" \
    -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
    -v opencode-cache:/home/bench/.cache:U \
    -v opencode-data:/home/bench/.local:U \
    -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
    -e DEEPSEEK_API_KEY -e GEMINI_API_KEY -e GOOGLE_GENERATIVE_AI_API_KEY \
    agent-bench \
    opencode run -m "$MODEL_ID" --format json "$PROMPT" \
    > "$RUN_DIR/transcript.jsonl" 2> "$RUN_DIR/stderr.log"
RC=$?
END=$(date +%s.%N)
DUR=$(echo "$END $START" | awk '{printf "%.1f", $1-$2}')

GRADE=$(bash "$BENCH/tasks/$TASK/grade.sh" "$RUN_DIR/ws" 2>&1 | tail -1)
CHANGED=$(git -C "$RUN_DIR/ws" diff --stat "$(cat "$RUN_DIR/ws/.bench-baseline")" 2>/dev/null | tail -1)
echo "{\"model\":\"$LABEL\",\"task\":\"$TASK\",\"grade\":\"$GRADE\",\"seconds\":$DUR,\"exit\":$RC,\"changed\":\"$CHANGED\"}" \
  | tee -a "$BENCH/results.jsonl"
