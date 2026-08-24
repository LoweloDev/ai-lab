#!/usr/bin/env bash
# Trimmed perf matrix: pp2048 + tg128 at depths, 1 rep. ~3-5 min per combo.
set -uo pipefail
M="$HOME/ai-lab/models"
L="$HOME/ai-lab/logs"
run() { # run <name> <dev> <modelfile> <depths> [extra args...]
  local name="$1" dev="$2" file="$3" depths="$4"; shift 4
  echo "=== $(date +%H:%M:%S) $name"
  llama-bench -m "$file" -ngl 99 -fa on -dev "$dev" \
    -p 2048 -n 128 -d "$depths" -r 1 --progress \
    "$@" -o json > "$L/perf-$name.json" 2>"$L/perf-$name.err" \
    && echo OK || echo "FAILED (see $L/perf-$name.err)"
}
run qwen38-vulkan-d32 Vulkan0 "$M/Qwen3.8-27B-UD-IQ4_XS.gguf" 32000 -ctk q8_0 -ctv q8_0
run qwen38-rocm   ROCm0   "$M/Qwen3.8-27B-UD-IQ4_XS.gguf" 0,10000,32000 -ctk q8_0 -ctv q8_0
run qwen36moe-vulkan Vulkan0 "$M/Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf" 0,10000,32000 -ctk q8_0 -ctv q8_0
run qwen36moe-rocm   ROCm0   "$M/Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf" 0,10000,32000 -ctk q8_0 -ctv q8_0
echo "perf bench 2 done"
