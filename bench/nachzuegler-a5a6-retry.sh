#!/usr/bin/env bash
# Wartet auf "NACHTKETTE-2 KOMPLETT" im Ketten-Log ($1) und wiederholt danach alle
# lokalen A5/A6-Läufe, die per Timeout (exit 137) ODER ohne Abgabe (leerer Diff)
# endeten, mit 3600 s Limit. qwen38 bekommt 64k Kontext (32k war nachweislich zu
# klein: "Context size has been exceeded" im Server-Log, 24.08. ~22:15).
# Gleiches Label: der neue Versuch überholt den Timeout in Dashboard/results.jsonl.
# Tobias' Auftrag 24.08. ~22:05 ("wenn nichts gebaut wurde müssen wir das wiederholen").
OUT="${1:?Pfad zum Nachtketten-Log}"
cd "$HOME/ai-lab/bench" || exit 1
L="$HOME/ai-lab/logs"

while ! grep -q "NACHTKETTE-2 KOMPLETT" "$OUT" 2>/dev/null; do sleep 300; done
echo "=== $(date +%H:%M) Nachtkette fertig — pruefe A5/A6-Timeouts"

declare -A SERVE=( [qwen38-vulkan]=qwen38-vision [muse-vulkan]=muse
                   [qwen36moe-vulkan]=qwen36moe [codernext-vulkan]=codernext )
declare -A EXTRA=( [qwen38-vulkan]="-c 65536" [qwen36moe-vulkan]="-c 65536" [codernext-vulkan]="-c 65536" )

for lbl in qwen38-vulkan muse-vulkan qwen36moe-vulkan codernext-vulkan; do
  todo=()
  for task in agora-A5-batcher-scratch agora-A6-scorer-scratch; do
    last=$(grep "\"model\":\"$lbl\",\"task\":\"$task\"" results.jsonl | tail -1)
    [ -n "$last" ] && echo "$last" | grep -Eq '"exit":137|"changed":""' && todo+=("$task")
  done
  [ ${#todo[@]} -eq 0 ] && continue
  echo "=== $(date +%H:%M) $lbl: Retry fuer ${todo[*]}"
  pkill -x llama-server 2>/dev/null; sleep 2
  "$HOME/ai-lab/serve.sh" "${SERVE[$lbl]}" vulkan ${EXTRA[$lbl]:-} > "$L/server-retry-${SERVE[$lbl]}.log" 2>&1 &
  up=0
  for i in $(seq 1 120); do
    curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health 2>/dev/null | grep -q 200 && { up=1; break; }
    sleep 2
  done
  [ "$up" = 1 ] || { echo "!! Server ${SERVE[$lbl]} kam nicht hoch — ueberspringe $lbl"; continue; }
  for task in "${todo[@]}"; do ./run-task.sh "$lbl" "$task" 3600; done
done
pkill -x llama-server 2>/dev/null
echo "A5A6-RETRY KOMPLETT"
