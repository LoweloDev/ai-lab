#!/usr/bin/env bash
# Raw performance matrix: model x backend x context depth.
# Writes JSON results to ~/ai-lab/logs/perf-<name>.json
set -uo pipefail
M="$HOME/ai-lab/models"
L="$HOME/ai-lab/logs"
run() { # run <name> <dev> <modelfile> [extra llama-bench args...]
  local name="$1" dev="$2" file="$3"; shift 3
  echo "=== $(date +%H:%M:%S) $name"
  llama-bench -m "$file" -ngl 99 -fa on -dev "$dev" \
    -p 2048 -n 128 -pg 8192,256 -d 0,10000,32000 -r 2 --progress \
    "$@" -o json > "$L/perf-$name.json" 2>"$L/perf-$name.err" \
    && echo OK || echo "FAILED (see $L/perf-$name.err)"
}
case "${1:-all}" in
  qwen38|all)
    run qwen38-vulkan Vulkan0 "$M/Qwen3.8-27B-UD-IQ4_XS.gguf" -ctk q8_0 -ctv q8_0
    run qwen38-rocm   ROCm0   "$M/Qwen3.8-27B-UD-IQ4_XS.gguf" -ctk q8_0 -ctv q8_0
    [ "${1:-all}" = qwen38 ] && exit 0 ;;&
  qwen36moe|all)
    run qwen36moe-vulkan Vulkan0 "$M/Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf" -ctk q8_0 -ctv q8_0
    run qwen36moe-rocm   ROCm0   "$M/Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf" -ctk q8_0 -ctv q8_0
    [ "${1:-all}" = qwen36moe ] && exit 0 ;;&
  codernext|all)
    run codernext-vulkan Vulkan0 "$M/Qwen3-Coder-Next-UD-Q3_K_XL.gguf" -ncmoe 40 -b 2048 -ub 2048 -ctk q8_0 -ctv q8_0
    run codernext-rocm   ROCm0   "$M/Qwen3-Coder-Next-UD-Q3_K_XL.gguf" -ncmoe 40 -b 2048 -ub 2048 -ctk q8_0 -ctv q8_0
    ;;
esac
echo "perf bench done"
