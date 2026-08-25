#!/usr/bin/env bash
# Polyglot via OpenCode fuer die lokalen Qwen-Modelle (Tobias 25.08. ~02:05: "die Qwen-Modelle zumindest"):
# Harness-Fairness — bisher liefen die Lokalen nur ueber Aider (Edit-Harness); DeepSeek/Gemini gewannen
# im Agenten-Harness 10-15 Punkte. Reihenfolge nach erwarteter Laufzeit: qwen36moe (35B, ~3,5 h) ->
# codernext (80B, ~5 h) -> qwen38 (27B, ~7 h). Wartet auf die Grader-Haertung (GPU frei nach den Retries);
# die Kandidaten-Kette wartet ihrerseits auf den Marker OC-LOKAL-POLYGLOT KOMPLETT (Datei unten).
# Labels: oc-<modell>-polyglot -> erscheinen automatisch im Polyglot-Tab (oc:<label>).
# Usage: oc-lokal-kette.sh [modelle...]   (Default: qwen36moe codernext qwen38)
cd "$HOME/ai-lab/bench/polyglot-oc" || exit 1
B="$HOME/ai-lab/bench"; L="$HOME/ai-lab/logs"; MARK="$B/.oc-lokal-polyglot-done"
MODELS=("${@:-qwen36moe}" ); [ $# -eq 0 ] && MODELS=(qwen36moe codernext qwen38)
log() { printf '=== %s %s\n' "$(date +%H:%M)" "$*"; }

while [ ! -f "$B/.grader-haertung-done" ]; do sleep 120; done
log "Haertung '$(cat "$B/.grader-haertung-done")' — OC-Polyglot lokal startet: ${MODELS[*]}"

serve_and_wait() {
  pkill -x llama-server 2>/dev/null; sleep 3
  "$HOME/ai-lab/serve.sh" "$1" vulkan > "$L/server-ocpoly-$1.log" 2>&1 &
  for i in $(seq 1 240); do
    curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health 2>/dev/null | grep -q 200 && return 0
    sleep 2
  done
  echo "!! Server $1 kam nicht hoch"; return 1
}

# Zeitbremse je Modell (Tobias 25.08. ~02:15: "wenn 27B wieder so aus dem Ruder laeuft, abbrechen"):
# nach Ablauf wird der Lauf gekillt, die fertigen Uebungen per --summary-only zusammengefasst und im
# Dashboard als "Zeitlimit, n/73" annotiert (sichtbar, weil das Teilergebnis eine gueltige Stichprobe ist).
declare -A CAP=( [qwen36moe]=$((6*3600)) [codernext]=$((6*3600)) [qwen38]=$((4*3600)) )
for m in "${MODELS[@]}"; do
  log "$m: Server starten"
  serve_and_wait "$m" || continue
  lbl="oc-$m-polyglot"
  # Lokale Modelle sind langsamer als die Cloud: Versuchs-Timeouts 600 s statt 240/300.
  OC_CONFIG=opencode-config T_ATTEMPT1=600 T_ATTEMPT2=600 \
    timeout --signal=SIGTERM --kill-after=60 "${CAP[$m]:-21600}" \
    ./run-polyglot-oc.sh llamacpp/local "$lbl" > "runs/$lbl.launch.log" 2>&1
  rc=$?
  if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then
    podman rm -f $(podman ps -aq --filter name=poc-) >/dev/null 2>&1
    OC_CONFIG=opencode-config ./run-polyglot-oc.sh llamacpp/local "$lbl" --summary-only >> "runs/$lbl.launch.log" 2>&1
    n=$(find "runs/$lbl" -name result.json | wc -l)
    python3 - "$lbl" "$n" <<'PY'
import json,sys
p='/home/lowelodev/ai-lab/dashboard/registry/run-annotations.json'; d=json.load(open(p))
lbl,n=sys.argv[1],sys.argv[2]
d['oc:'+lbl]={"label":f"{lbl} (Zeitlimit, {n}/73)","hidden":False,
              "note":f"Lauf nach Zeitlimit abgebrochen ({n}/73 Uebungen); Teilergebnis = gueltige Stichprobe, nicht mit 73er-Laeufen gleichsetzen."}
json.dump(d,open(p,'w'),ensure_ascii=False,indent=1)
PY
    log "$m: ZEITLIMIT nach ${CAP[$m]}s — Teilergebnis $n/73 zusammengefasst"
  else
    log "$m: fertig (rc=$rc) $(jq -c '.overall' "runs/$lbl/summary.json" 2>/dev/null)"
  fi
done
pkill -x llama-server 2>/dev/null
echo "OK $(date +%d.%m-%H:%M) ${MODELS[*]}" > "$MARK"
log "OC-LOKAL-POLYGLOT KOMPLETT"
