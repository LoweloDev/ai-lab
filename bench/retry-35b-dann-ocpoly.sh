#!/usr/bin/env bash
# 25.08. 02:30: Der 35B-A5-Retry starb an einer podman-Namenskollision (exit 125). Nachholen, sobald die
# Grader-Haertung durch ist (davor darf kein run-task laufen), danach die OC-Polyglot-Kette starten.
B="$HOME/ai-lab/bench"; L="$HOME/ai-lab/logs"
while [ ! -f "$B/.grader-haertung-done" ]; do sleep 120; done
echo "=== $(date +%H:%M) Haertung '$(cat "$B/.grader-haertung-done")' — 35B-A5-Retry (64k, 60 min)"
podman rm -f bench-agora-A5-batcher-scratch >/dev/null 2>&1
pkill -x llama-server 2>/dev/null; sleep 3
"$HOME/ai-lab/serve.sh" qwen36moe vulkan -c 65536 > "$L/server-retry2-qwen36moe.log" 2>&1 &
for i in $(seq 1 120); do curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health 2>/dev/null | grep -q 200 && break; sleep 2; done
( cd "$B" && ./run-task.sh qwen36moe-vulkan agora-A5-batcher-scratch 3600 )
pkill -x llama-server 2>/dev/null
echo "=== $(date +%H:%M) Retry fertig — OC-Polyglot-Kette startet"
exec "$B/polyglot-oc/oc-lokal-kette.sh"
