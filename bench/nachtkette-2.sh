#!/usr/bin/env bash
# Nachtkette 2 (25.08. ~01:40): Lokale A4/A5/A6 + 80B-Komplettprogramm.
# Kein set -e: ein fehlgeschlagener Task darf die Kette nicht reissen.
cd "$HOME/ai-lab/bench" || exit 1
M="$HOME/ai-lab/models"; L="$HOME/ai-lab/logs"

serve_and_wait() { # serve_and_wait <serve-name>
  pkill -x llama-server 2>/dev/null; sleep 2
  "$HOME/ai-lab/serve.sh" "$1" vulkan > "$L/server-nacht-$1.log" 2>&1 &
  for i in $(seq 1 120); do
    curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health 2>/dev/null | grep -q 200 && return 0
    sleep 2
  done
  echo "!! Server $1 kam nicht hoch"; return 1
}

suite3() { # suite3 <label>
  ./run-task.sh "$1" agora-A4-feed 900
  ./run-task.sh "$1" agora-A5-batcher-scratch 1800
  ./run-task.sh "$1" agora-A6-scorer-scratch 1800
}

echo "=== $(date +%H:%M) Phase 1: Lokale A4/A5/A6"
serve_and_wait qwen38-vision && suite3 qwen38-vulkan
serve_and_wait muse          && suite3 muse-vulkan
serve_and_wait qwen36moe     && suite3 qwen36moe-vulkan

echo "=== $(date +%H:%M) Phase 2: 80B llama-bench-Matrix (Server aus)"
pkill -x llama-server 2>/dev/null; sleep 2
llama-bench -m "$M/Qwen3-Coder-Next-UD-Q3_K_XL.gguf" -ngl 99 -fa on -dev Vulkan0 \
  -ncmoe 26 -b 2048 -ub 2048 -ctk q8_0 -ctv q8_0 \
  -p 2048 -n 128 -d 0,10000,32000 -r 1 -o json > "$L/perf-codernext-vulkan.json" 2> "$L/perf-codernext-vulkan.err"
llama-bench -m "$M/Qwen3-Coder-Next-UD-Q3_K_XL.gguf" -ngl 99 -fa on -dev ROCm0 \
  -ncmoe 26 -b 2048 -ub 2048 -ctk q8_0 -ctv q8_0 \
  -p 2048 -n 128 -d 0,10000,32000 -r 1 -o json > "$L/perf-codernext-rocm.json" 2> "$L/perf-codernext-rocm.err"

echo "=== $(date +%H:%M) Phase 3: 80B Suite A4/A5/A6 + Apps/UX"
if serve_and_wait codernext; then
  suite3 codernext-vulkan
  cd webapp
  timeout 400 python3 dom-agent.py codernext info
  timeout 500 python3 dom-agent.py codernext form
  timeout 600 python3 ux-dom-test.py codernext
  cd ..
  echo "=== $(date +%H:%M) Phase 4: 80B Polyglot (laeuft bis in den Vormittag)"
  "$HOME/ai-lab/bench/aider/run-polyglot-subset.sh" > /dev/null 2>&1
fi
echo "=== $(date +%H:%M) NACHTKETTE-2 KOMPLETT"
