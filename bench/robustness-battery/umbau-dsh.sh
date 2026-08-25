#!/usr/bin/env bash
# Robustheits-Umbau via DeepSeek (Tobias 25.08. ~01:50): neun kleine Auftraege (spec/01..08 + Pruefung
# durch den Host), sequenziell, je Auftrag eine frische Harness-Session im Container.
# HARNESS=oc (Default nach Datenpruefung 25.08. 01:55: OC gleich gut, 1,5x schneller, 35 % guenstiger) oder HARNESS=dsh.
# Sicherheit: ~/ai-lab read-only gemountet, Schreibrechte nur auf robustness-battery/, dashboard/,
# audit-scratch/robust/; .env und ALLE Token-Kopien (agy-auth-home, polyglot-oc/runs, runs/**/cc-home,
# runs/**/agy-home) werden per /dev/null bzw. tmpfs ueberdeckt. Kein podman im Container.
# Wiederholung: fehlt spec/done-NN.txt nach der Session, wird der Auftrag genau einmal wiederholt.
# Usage: [HARNESS=dsh|oc] [DSH_MODEL=deepseek-v4-flash] umbau-dsh.sh [ab-Auftrag NN]
set -uo pipefail
L="$HOME/ai-lab"; B="$L/bench"; R="$B/robustness-battery"; S="$R/spec"
HARNESS="${HARNESS:-oc}"; START="${1:-01}"
LOG="$R/umbau.log"; log() { printf '%s %s\n' "$(date '+%d.%m %H:%M:%S')" "$*" | tee -a "$LOG"; }
[ -n "${DEEPSEEK_API_KEY:-}" ] || { set -a; . "$L/.env"; set +a; }
[ -n "${DEEPSEEK_API_KEY:-}" ] || { echo "DEEPSEEK_API_KEY fehlt"; exit 2; }
mkdir -p "$B/audit-scratch/robust" "$R/sessions"

# Mounts: NUR was gebraucht wird, unter den Original-Pfaden. bench/runs wird als hardgelinkte Sicht
# (nur .../ws, ohne cc-home/agy-home/dsh-home = keine Tokens) eingehaengt; kein .env, keine models/logs.
# (Vorherige Fassung mit 42 tmpfs-Schatten + 584-GB-ro-Baum scheiterte in crun: "No space left on device".)
VIEW="$B/.runs-view"; rm -rf "$VIEW"; mkdir -p "$VIEW"
for ws in "$B"/runs/*/*/ws; do rel="${ws#"$B/runs/"}"; mkdir -p "$VIEW/$(dirname "$rel")"; cp -al "$ws" "$VIEW/$rel" 2>/dev/null; done
P=/home/lowelodev/ai-lab
MOUNTS=(-v "$B/tasks:$P/bench/tasks:ro,Z" -v "$B/workspaces:$P/bench/workspaces:ro,Z"
        -v "$VIEW:$P/bench/runs:ro,Z" -v "$B/results.jsonl:$P/bench/results.jsonl:ro,Z"
        -v "$R:$P/bench/robustness-battery:rw,Z" -v "$L/dashboard:$P/dashboard:rw,Z"
        -v "$B/audit-scratch/robust:$P/bench/audit-scratch/robust:rw,Z"
        -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z")
ENV=(-e DEEPSEEK_API_KEY -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off
     -e GOMODCACHE=/home/bench/go/pkg/mod -e GOCACHE=/tmp/gocache -e HOME=/home/bench)

run_session() { # $1 = NN, $2 = Spec-Datei
  local nn="$1" spec="$2" sdir="$R/sessions/$nn" prompt
  prompt="Lies zuerst /home/lowelodev/ai-lab/bench/robustness-battery/spec/00-konvention.md vollstaendig, dann /home/lowelodev/ai-lab/bench/robustness-battery/spec/$spec, und fuehre den Auftrag darin komplett aus. Halte dich strikt an die Schreibrechte aus der Konvention. Beende mit der done-Datei."
  rm -rf "$sdir"; mkdir -p "$sdir/home"
  log "== Auftrag $nn ($spec) via $HARNESS"
  if [ "$HARNESS" = oc ]; then
    timeout --signal=SIGTERM --kill-after=30 3000 \
      podman run --rm --name "umbau-$nn" --pull=never --userns=keep-id \
        --network=pasta:--map-host-loopback,169.254.1.2 "${MOUNTS[@]}" "${ENV[@]}" \
        -v "$B/opencode-config-api:/home/bench/.config/opencode:Z" \
        -v opencode-cache:/home/bench/.cache:U -v opencode-data:/home/bench/.local:U \
        -w /home/lowelodev/ai-lab \
        agent-bench opencode run -m deepseek/deepseek-v4-flash --format json "$prompt" \
        > "$sdir/transcript.jsonl" 2> "$sdir/stderr.log"
  else
    timeout --signal=SIGTERM --kill-after=30 3000 \
      podman run --rm --name "umbau-$nn" --pull=never --userns=keep-id \
        --network=pasta:--map-host-loopback,169.254.1.2 "${MOUNTS[@]}" "${ENV[@]}" \
        -v "$sdir/home:/home/bench/.dsh:Z" \
        -v dsh-cache:/home/bench/.cache:U -v dsh-data:/home/bench/.local:U \
        -w /home/lowelodev/ai-lab \
        agent-bench-dsh dsh --profile headless "$prompt" \
        > "$sdir/stdout.log" 2> "$sdir/stderr.log"
  fi
  log "   rc=$? done-Datei: $([ -f "$S/done-$nn.txt" ] && echo ja || echo NEIN)"
}

for spec in 01-runner.md 02-batterie-agora-A4-feed.md 03-batterie-agora-A1-gate.md 04-batterie-agora-A3-hls.md \
            05-batterie-aiux-U1-paging.md 06-batterie-agora-A2-jsonld.md 07-batterie-aiux-U2-denytools.md 08-dashboard.md; do
  nn="${spec%%-*}"
  [ "$nn" \< "$START" ] && continue
  [ -f "$S/done-$nn.txt" ] && { log "== Auftrag $nn bereits erledigt (done-Datei) — uebersprungen"; continue; }
  run_session "$nn" "$spec"
  [ -f "$S/done-$nn.txt" ] || { log "   Wiederholung $nn"; run_session "$nn" "$spec"; }
  [ -f "$S/done-$nn.txt" ] || log "!! Auftrag $nn ohne done-Datei nach 2 Versuchen — weiter mit dem naechsten"
done

log "== Host-Nachlauf: run-all.sh ueber alle Batterien"
( cd "$R" && [ -x run-all.sh ] && timeout 1800 ./run-all.sh --force >> "$LOG" 2>&1; echo "run-all rc=$?" ) | tee -a "$LOG"
echo "UMBAU-DSH KOMPLETT ($(ls "$S"/done-*.txt 2>/dev/null | wc -l)/8 done-Dateien)"
