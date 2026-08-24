#!/usr/bin/env bash
# Start llama-server for one of the lab models.
# Usage: serve.sh <qwen38|qwen36moe|codernext> [vulkan|rocm] [extra llama-server args...]
set -euo pipefail
M="$HOME/ai-lab/models"
MODEL="${1:?model: qwen38|qwen36moe|codernext}"
BACKEND="${2:-vulkan}"
shift; [ $# -gt 0 ] && shift
case "$BACKEND" in
  vulkan) DEV=Vulkan0 ;;
  rocm)   DEV=ROCm0 ;;
  *) echo "backend must be vulkan|rocm"; exit 2 ;;
esac

COMMON=(--host 127.0.0.1 --port 8080 --api-key sk-local --jinja -fa on
        --cache-reuse 256 -t 8 --device "$DEV" --metrics)

case "$MODEL" in
  qwen38)
    exec llama-server "${COMMON[@]}" \
      -m "$M/Qwen3.8-27B-UD-IQ4_XS.gguf" \
      -ngl 99 -c 32768 -ctk q8_0 -ctv q8_0 "$@" ;;
  qwen38-vision)
    exec llama-server "${COMMON[@]}" \
      -m "$M/Qwen3.8-27B-UD-IQ4_XS.gguf" --mmproj "$M/mmproj-F16.gguf" \
      -ngl 99 -c 16384 -ctk q8_0 -ctv q8_0 "$@" ;;
  qwen36moe)
    exec llama-server "${COMMON[@]}" \
      -m "$M/Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf" \
      -ngl 99 -c 32768 -ctk q8_0 -ctv q8_0 "$@" ;;
  muse)
    exec llama-server "${COMMON[@]}" \
      -m "$M/Muse-Glimmer-30B-UD-Q4_K_XL.gguf" \
      -ngl 99 -c 32768 -ctk q8_0 -ctv q8_0 "$@" ;;
  muse-vision)
    exec llama-server "${COMMON[@]}" \
      -m "$M/Muse-Glimmer-30B-UD-Q4_K_XL.gguf" --mmproj "$M/mmproj-Muse-Glimmer-30B-Q8_0.gguf" \
      -ngl 99 -c 16384 -ctk q8_0 -ctv q8_0 "$@" ;;
  codernext)
    # 80B-A3B MoE: attention+shared experts on GPU, routed experts in RAM/mmap.
    # --n-cpu-moe tuned so VRAM stays under ~19GB.
    exec llama-server "${COMMON[@]}" \
      -m "$M/Qwen3-Coder-Next-UD-Q3_K_XL.gguf" \
      -ngl 99 --n-cpu-moe 26 -c 32768 -ctk q8_0 -ctv q8_0 \
      -b 2048 -ub 2048 "$@" ;;
  *) echo "unknown model $MODEL"; exit 2 ;;
esac
