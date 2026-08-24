#!/usr/bin/env bash
# run-battery.sh — Robustheits-Batterie A5 (Zusatz-Metrik, ändert keine PASS/FAIL-Urteile).
# Fährt battery_test.go über alle runs/*/agora-A5-batcher-scratch/ws:
#   1. git status --short sichern (Vorher-Stand — Workspaces sind nicht zwingend clean)
#   2. battery_test.go als agora-backend/internal/feed/zz_battery_test.go einkopieren
#   3. go test ./internal/feed/ -run ZZBat -count=1 -json  (Shell-Timeout 120 s)
#   4. Datei entfernen, git status --short erneut — muss dem Vorher-Stand exakt gleichen
# Vorab läuft ein Baseline-Kompilat OHNE Batterie, um "Modellcode selbst nicht baubar"
# von "Batterie passt nicht auf die gegradete API" unterscheiden zu können.
set -u
HERE=/home/lowelodev/ai-lab/bench/robustness-battery/a5
BATTERY="$HERE/battery_test.go"
RAW="$HERE/raw"
mkdir -p "$RAW"

for wsdir in /home/lowelodev/ai-lab/bench/runs/*/agora-A5-batcher-scratch/ws; do
  label=$(basename "$(dirname "$(dirname "$wsdir")")")
  feed="$wsdir/agora-backend/internal/feed"
  tgt="$feed/zz_battery_test.go"
  echo "=== $label"

  git -C "$wsdir" status --short > "$RAW/$label.pre-status"

  if [ -e "$tgt" ]; then
    echo "COLLISION" > "$RAW/$label.rc"
    echo "$label: zz_battery_test.go existiert bereits — übersprungen"
    continue
  fi

  # Baseline: kompiliert/testet das Paket ohne Batterie? (-run ohne Treffer)
  ( cd "$wsdir/agora-backend" && timeout 90 go test ./internal/feed/ -run ZZZZNOMATCH -count=1 ) \
      > "$RAW/$label.basebuild" 2>&1
  echo "base_rc=$?" >> "$RAW/$label.basebuild"

  cp "$BATTERY" "$tgt"
  ( cd "$wsdir/agora-backend" && timeout 120 go test ./internal/feed/ -run ZZBat -count=1 -timeout 110s -json ) \
      > "$RAW/$label.json" 2> "$RAW/$label.stderr"
  echo "$?" > "$RAW/$label.rc"
  rm -f "$tgt"

  git -C "$wsdir" status --short > "$RAW/$label.post-status"
  if diff -u "$RAW/$label.pre-status" "$RAW/$label.post-status" > "$RAW/$label.status-diff" 2>&1; then
    echo "workspace-clean: JA (git status --short unverändert)"
    echo "clean" > "$RAW/$label.wsclean"
  else
    echo "workspace-clean: NEIN — siehe $RAW/$label.status-diff"
    echo "DIRTY" > "$RAW/$label.wsclean"
  fi
done
echo "=== fertig"
