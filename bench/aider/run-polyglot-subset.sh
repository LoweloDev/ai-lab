#!/usr/bin/env bash
# Aider polyglot benchmark — python+go subset against the LOCAL llama.cpp server.
#
# Prereqs (already prepared):
#   - image "aider-bench" built via:  cd aider && podman build -f ../Dockerfile.podman -t aider-bench .
#   - llama.cpp server on http://127.0.0.1:8080/v1 (api key sk-local), single model loaded
#   - pasta networking maps host loopback to 169.254.1.2 inside the container
#
# Results land in:  aider/tmp.benchmarks/<YYYY-MM-DD-HH-MM-SS>--<run name>
# Stats are printed at the end and can be re-generated any time with:
#   podman run --rm -v .../aider:/aider -v .../aider/tmp.benchmarks:/benchmarks \
#     -e AIDER_BENCHMARK_DIR=/benchmarks -w /aider aider-bench \
#     ./benchmark/benchmark.py --stats tmp.benchmarks/<run dir>
set -euo pipefail

BENCH_ROOT="/home/lowelodev/ai-lab/bench/aider"
AIDER_DIR="$BENCH_ROOT/aider"
POLYGLOT_DIR="$BENCH_ROOT/polyglot-benchmark"

RUN_NAME="local-qwen-py-go-$(date +%Y%m%d-%H%M%S)"
LANGS="python,go"          # 34 python + 39 go = 73 exercises
THREADS=1                  # local server: strictly serial
MODEL="openai/local"       # litellm -> OPENAI_API_BASE, model id "local" (llama.cpp ignores it)
EDIT_FORMAT="whole"

LOG="$BENCH_ROOT/run-$RUN_NAME.log"
echo "Run name : $RUN_NAME"
echo "Log      : $LOG"
echo "Results  : $AIDER_DIR/tmp.benchmarks/*--$RUN_NAME"

podman run --rm \
  --network=pasta:--map-host-loopback,169.254.1.2 \
  --memory=12g --memory-swap=12g \
  -v "$AIDER_DIR":/aider \
  -v "$AIDER_DIR/tmp.benchmarks":/benchmarks \
  -v "$POLYGLOT_DIR":/benchmarks/polyglot-benchmark \
  -e OPENAI_API_BASE=http://169.254.1.2:8080/v1 \
  -e OPENAI_API_KEY=sk-local \
  -e AIDER_DOCKER=1 \
  -e AIDER_BENCHMARK_DIR=/benchmarks \
  -w /aider \
  aider-bench \
  ./benchmark/benchmark.py "$RUN_NAME" \
    --model "$MODEL" \
    --edit-format "$EDIT_FORMAT" \
    --threads "$THREADS" \
    --languages "$LANGS" \
    --read-model-settings /aider/local-model-settings.yml \
    --exercises-dir polyglot-benchmark \
    --new \
  2>&1 | tee "$LOG"

# Final stats summary (yaml record, pass_rate_2 is the headline number)
podman run --rm \
  -v "$AIDER_DIR":/aider \
  -v "$AIDER_DIR/tmp.benchmarks":/benchmarks \
  -e AIDER_BENCHMARK_DIR=/benchmarks \
  -w /aider \
  aider-bench \
  ./benchmark/benchmark.py --stats "$RUN_NAME" \
  2>&1 | tee -a "$LOG"
