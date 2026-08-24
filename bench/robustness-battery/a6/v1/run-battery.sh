#!/usr/bin/env bash
# run-battery.sh — Robustheits-Batterie A6 (ZUSATZ-Metrik, aendert keine PASS/FAIL-Urteile).
# Faehrt battery_test.go ueber alle runs/*/agora-A6-scorer-scratch/ws:
#   1. git status --short VORHER festhalten
#   2. battery_test.go als agora-backend/internal/feed/zz_battery_test.go kopieren
#   3. go test -run ZZBat -count=1 -v (Timeout 120s aussen, 100s innen)
#   4. Datei ENTFERNEN, git status --short NACHHER festhalten, Gleichheit pruefen
# Ausgabe: eine RESULT-Zeile je Workspace + vollstaendige Logs unter logs/<label>.log
set -u

BENCH=/home/lowelodev/ai-lab/bench
BATTERY_DIR="$BENCH/robustness-battery/a6"
BATTERY="$BATTERY_DIR/battery_test.go"
LOGDIR="$BATTERY_DIR/logs"
mkdir -p "$LOGDIR"

# Optionales Argument: Label-Glob (Default: alle Runs).
LABEL_GLOB=${1:-*}

for ws in "$BENCH"/runs/$LABEL_GLOB/agora-A6-scorer-scratch/ws; do
  label=$(basename "$(dirname "$(dirname "$ws")")")
  feed="$ws/agora-backend/internal/feed"
  tgt="$feed/zz_battery_test.go"
  log="$LOGDIR/$label.log"

  if [ ! -d "$feed" ]; then
    echo "RESULT $label status=nicht-baubar detail=kein-feed-paket guard=n/a"
    continue
  fi
  if [ -e "$tgt" ]; then
    echo "RESULT $label status=uebersprungen detail=zz_battery_test.go-existiert-schon guard=n/a"
    continue
  fi

  before=$(git -C "$ws" status --short)

  cp "$BATTERY" "$tgt"
  ( cd "$ws/agora-backend" && timeout 120 go test ./internal/feed/ -run ZZBat -count=1 -v -timeout 100s ) >"$log" 2>&1
  rc=$?
  rm -f "$tgt"

  after=$(git -C "$ws" status --short)
  if [ "$before" = "$after" ] && [ ! -e "$tgt" ]; then
    guard="workspace-unveraendert"
  else
    guard="WORKSPACE-VERAENDERT"
    {
      echo "--- git status VORHER:"
      echo "$before"
      echo "--- git status NACHHER:"
      echo "$after"
    } >>"$log"
  fi

  # Build-Fehler: kein einziges Testresultat, aber Compile-Diagnose im Log.
  if ! grep -q -- '^--- \(PASS\|FAIL\): TestZZBat' "$log"; then
    if grep -qE '\[build failed\]|undefined:|cannot find|syntax error' "$log"; then
      echo "RESULT $label status=nicht-baubar rc=$rc guard=$guard"
      continue
    fi
    echo "RESULT $label status=keine-testresultate rc=$rc guard=$guard"
    continue
  fi

  real_pass=$(grep -c '^--- PASS: TestZZBatReal' "$log" || true)
  real_fail=$(grep -c '^--- FAIL: TestZZBatReal' "$log" || true)
  path_pass=$(grep -c '^--- PASS: TestZZBatPath' "$log" || true)
  path_fail=$(grep -c '^--- FAIL: TestZZBatPath' "$log" || true)
  failed=$(grep -oE '^--- FAIL: TestZZBat[A-Za-z0-9]+' "$log" | awk '{print $3}' | paste -sd, -)

  echo "RESULT $label status=gelaufen real=$real_pass/$((real_pass + real_fail)) path=$path_pass/$((path_pass + path_fail)) failed=[${failed:-—}] rc=$rc guard=$guard"
done
