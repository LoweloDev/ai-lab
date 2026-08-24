#!/usr/bin/env bash
# Variante von run-task.sh fuer den DeepSeek Harness (dsh) im Image agent-bench-dsh.
# dsh 0.1.1-rc.2 (developer preview, im Image gepinnt), headless-Profil.
# Abgeleitet aus run-task.sh (2026-08-24); Unterschiede:
#   - Image agent-bench-dsh statt agent-bench; Kommando: dsh --profile headless "<prompt>"
#   - DEEPSEEK_API_KEY wird in den Container durchgereicht (dsh liest den Key aus der Umgebung)
#   - Modellwahl via env DSH_MODEL (z.B. DSH_MODEL=deepseek-v4-pro). Default des headless-Profils
#     ist deepseek-v4-flash (Plugin agent-default-model, provider deepseek-official). Der Override
#     laeuft ueber eine --patch-Overlay-Datei; per `dsh --dump-config` offline verifiziert (24.08.).
#   - ~/.dsh des Containers wird nach $RUN_DIR/dsh-home gemountet, damit die Session-JSONL-Logs
#     (dsh-home/sessions/) den Lauf ueberleben. Eigene Cache-Volumes (dsh-cache/dsh-data),
#     um den OpenCode-Zustand nicht zu vermischen.
# Usage: [DSH_MODEL=deepseek-v4-pro] run-task-dsh.sh <model-label> <task-name> [timeout-seconds]
set -uo pipefail
MODEL="$1"; TASK="$2"; LIMIT="${3:-1200}"
BENCH="$HOME/ai-lab/bench"
WS_SRC="$BENCH/workspaces/$TASK"
RUN_DIR="$BENCH/runs/$MODEL/$TASK"
[ -d "$WS_SRC" ] || { echo "unknown task $TASK"; exit 2; }
rm -rf "$RUN_DIR"; mkdir -p "$RUN_DIR/dsh-home"
cp -r "$WS_SRC" "$RUN_DIR/ws"
git -C "$RUN_DIR/ws" rev-parse HEAD > "$RUN_DIR/ws/.bench-baseline"
PROMPT=$(cat "$BENCH/tasks/$TASK/prompt.txt")

# Optionaler Modell-Override als Patch-Overlay (Eintraege werden per id gemerged).
DSH_EXTRA=()
if [ -n "${DSH_MODEL:-}" ]; then
  printf -- '- id: agent-default-model\n  config:\n    provider: deepseek-official\n    model: %s\n' \
    "$DSH_MODEL" > "$RUN_DIR/model-patch.yml"
  DSH_EXTRA=(--patch /home/bench/model-patch.yml)
fi

START=$(date +%s.%N)
timeout --signal=SIGTERM --kill-after=30 "$LIMIT" \
  podman run --rm --name "bench-dsh-$TASK" --pull=never --userns=keep-id \
    --network=pasta:--map-host-loopback,169.254.1.2 \
    -v "$RUN_DIR/ws:/work:Z" \
    -v "$RUN_DIR/dsh-home:/home/bench/.dsh:Z" \
    ${DSH_MODEL:+-v "$RUN_DIR/model-patch.yml:/home/bench/model-patch.yml:ro,Z"} \
    -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
    -v dsh-cache:/home/bench/.cache:U \
    -v dsh-data:/home/bench/.local:U \
    -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
    -e DEEPSEEK_API_KEY \
    agent-bench-dsh \
    dsh --profile headless ${DSH_EXTRA[@]+"${DSH_EXTRA[@]}"} "$PROMPT" \
    > "$RUN_DIR/transcript.log" 2> "$RUN_DIR/stderr.log"
RC=$?
END=$(date +%s.%N)
DUR=$(echo "$END $START" | awk '{printf "%.1f", $1-$2}')

GRADE=$(bash "$BENCH/tasks/$TASK/grade.sh" "$RUN_DIR/ws" 2>&1 | tail -1)
CHANGED=$(git -C "$RUN_DIR/ws" diff --stat "$(cat "$RUN_DIR/ws/.bench-baseline")" 2>/dev/null | tail -1)
echo "{\"model\":\"$MODEL\",\"task\":\"$TASK\",\"grade\":\"$GRADE\",\"seconds\":$DUR,\"exit\":$RC,\"changed\":\"$CHANGED\"}" \
  | tee -a "$BENCH/results.jsonl"
