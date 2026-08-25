#!/usr/bin/env bash
# Kandidaten-Runde 100-130B-MoE: wartet, bis (a) der Download komplett ist und (b) die
# GPU frei ist (Marker "A5A6-RETRY KOMPLETT" im Retry-Waechter-Log $1), dann je Modell:
# llama-bench-Matrix (Vulkan+ROCm, d 0/10000) -> Server -> volle 8-Task-Suite.
# Kein set -e: ein Fehlschlag darf die Kette nicht reissen. Ergebnisse landen automatisch
# in results.jsonl (Suite-Matrix) und logs/perf-<name>-<backend>.json (Perf-Tab).
RETRY_LOG="${1:?Pfad zum Retry-Waechter-Log}"
cd "$HOME/ai-lab/bench" || exit 1
M="$HOME/ai-lab/models"; L="$HOME/ai-lab/logs"
log() { printf '=== %s %s\n' "$(date +%H:%M)" "$*"; }

while [ ! -f "$M/.kandidaten-download-ok" ] || ! grep -q "A5A6-RETRY KOMPLETT" "$RETRY_LOG" 2>/dev/null; do sleep 300; done
# Grader-Haertung (Audit 24.08.) laeuft nach den Retries; erst danach starten, damit die
# Kandidaten mit denselben (gehaerteten) Gradern bewertet werden wie alle Regrades.
while [ ! -f "$HOME/ai-lab/bench/.grader-haertung-done" ]; do sleep 120; done
# Tobias 25.08. ~02:05: erst die Qwen-Modelle via OpenCode durch Polyglot (Harness-Fairness), dann Kandidaten.
while [ ! -f "$HOME/ai-lab/bench/.oc-lokal-polyglot-done" ]; do sleep 300; done
log "Download komplett + GPU frei + Haertung '$(cat "$HOME/ai-lab/bench/.grader-haertung-done")' + OC-Polyglot lokal '$(cat "$HOME/ai-lab/bench/.oc-lokal-polyglot-done")' — Kandidaten-Runde startet"

serve_and_wait() { # serve_and_wait <serve-name>
  pkill -x llama-server 2>/dev/null; sleep 3
  "$HOME/ai-lab/serve.sh" "$1" vulkan > "$L/server-kandidat-$1.log" 2>&1 &
  for i in $(seq 1 240); do   # 44-GiB-Modelle brauchen laenger zum Laden (bis 8 min)
    curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health 2>/dev/null | grep -q 200 && return 0
    sleep 2
  done
  echo "!! Server $1 kam nicht hoch (siehe $L/server-kandidat-$1.log)"; return 1
}

declare -A FILE=( [mistral4]="Mistral-Small-4-119B-2603.i1-IQ3_XXS.gguf"
                  [qwen35]="Qwen3.5-122B-A10B.i1-IQ3_XXS.gguf"
                  [laguna]="Laguna-S-2.1-IQ3_XXS.gguf" )

for m in mistral4 qwen35 laguna; do
  f="$M/${FILE[$m]}"
  [ -f "$f" ] || { log "$m: Datei fehlt, uebersprungen"; continue; }
  log "$m: llama-bench-Matrix"
  pkill -x llama-server 2>/dev/null; sleep 3
  for be in vulkan rocm; do
    dev=Vulkan0; [ "$be" = rocm ] && dev=ROCm0
    timeout 1500 llama-bench -m "$f" -ngl 99 -fa on -dev "$dev" -ncmoe 40 -b 2048 -ub 2048 \
      -ctk q8_0 -ctv q8_0 -p 2048 -n 128 -d 0,10000 -r 1 -o json \
      > "$L/perf-$m-$be.json" 2> "$L/perf-$m-$be.err"
  done
  log "$m: Suite (8 Tasks, Label $m-vulkan)"
  if serve_and_wait "$m"; then
    ./run-task.sh "$m-vulkan" aiux-U1-paging 1800
    ./run-task.sh "$m-vulkan" agora-A1-gate 1800
    ./run-task.sh "$m-vulkan" agora-A2-jsonld 1800
    ./run-task.sh "$m-vulkan" agora-A3-hls 1800
    ./run-task.sh "$m-vulkan" aiux-U2-denytools 3600
    ./run-task.sh "$m-vulkan" agora-A4-feed 1800
    ./run-task.sh "$m-vulkan" agora-A5-batcher-scratch 3600
    ./run-task.sh "$m-vulkan" agora-A6-scorer-scratch 3600
  fi
done
pkill -x llama-server 2>/dev/null
log "KANDIDATEN-RUNDE KOMPLETT"
