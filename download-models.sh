#!/usr/bin/env bash
# Model download queue for the local agentic-coding lab (2026-08-23)
set -uo pipefail
cd "$HOME/ai-lab/models"
DL() {
  local repo="$1" file="$2"
  echo "=== $(date +%H:%M:%S) START $file"
  aria2c -x8 -s8 -c --file-allocation=falloc --console-log-level=warn --summary-interval=60 \
    -o "$file" "https://huggingface.co/$repo/resolve/main/$file" || { echo "FAILED: $file"; return 1; }
  echo "=== $(date +%H:%M:%S) DONE $file"
}
DL unsloth/Qwen3.8-27B-GGUF mmproj-F16.gguf
DL unsloth/Qwen3.8-27B-GGUF Qwen3.8-27B-UD-IQ4_XS.gguf
DL unsloth/Qwen3.6-35B-A3B-GGUF Qwen3.6-35B-A3B-UD-Q3_K_XL.gguf
DL unsloth/Qwen3-Coder-Next-GGUF Qwen3-Coder-Next-UD-Q3_K_XL.gguf
echo "ALL DOWNLOADS COMPLETE $(date)"
ls -lh
