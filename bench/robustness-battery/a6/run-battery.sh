#!/usr/bin/env bash
# run-battery.sh — Robustheits-Batterie A6 v2 (ZUSATZ-Metrik, aendert keine PASS/FAIL-Urteile).
# Faehrt battery_test.go ueber alle runs/*/agora-A6-scorer-scratch/ws (oder ueber den
# Label-Glob in $1):
#   0. Sicherheitsstopp, falls gerade ein Prozess (Benchmark/podman) den Workspace benutzt
#   1. git status --short VORHER festhalten (Workspaces sind nicht zwingend clean)
#   2. Baseline-Kompilat OHNE Batterie (-run ohne Treffer), um "Modellcode nicht baubar"
#      von "gegradete API fehlt" unterscheiden zu koennen
#   3. battery_test.go als agora-backend/internal/feed/zz_battery_test.go einkopieren
#   4. go test ./internal/feed/ -run ZZBat -count=1 -json (Shell-Timeout 120 s, -timeout 110 s)
#   5. Datei ENTFERNEN, git status --short NACHHER — muss dem Vorher-Stand byte-gleich sein
#   6. summarize.py schreibt logs/<label>.log und druckt eine RESULT-Zeile
# Rohdaten je Label unter raw/ (.json .stderr .rc .basebuild .pre-status .post-status .status-diff .wsclean).
set -u
BENCH=/home/lowelodev/ai-lab/bench
HERE="$BENCH/robustness-battery/a6"
BATTERY="$HERE/battery_test.go"
RAW="$HERE/raw"
LOGDIR="$HERE/logs"
mkdir -p "$RAW" "$LOGDIR"

# Kein Netz, kein Schreiben an go.mod/go.sum (Default -mod=readonly bleibt).
export GOPROXY=off
export GOFLAGS=-buildvcs=false

LABEL_GLOB=${1:-*}

for ws in "$BENCH"/runs/$LABEL_GLOB/agora-A6-scorer-scratch/ws; do
  [ -d "$ws" ] || continue
  label=$(basename "$(dirname "$(dirname "$ws")")")
  feed="$ws/agora-backend/internal/feed"
  tgt="$feed/zz_battery_test.go"
  echo "=== $label"

  # 0. Workspace gerade in Benutzung (laufender Benchmark, podman-Mount)? Dann nicht anfassen.
  # (pgrep schliesst sich selbst aus — ein ps|grep wuerde sein eigenes Muster treffen.)
  if pgrep -f -- "runs/$label/agora-A6-scorer-scratch" >/dev/null; then
    echo "IN-USE" > "$RAW/$label.rc"
    echo "RESULT $label status=uebersprungen detail=workspace-in-benutzung guard=n/a"
    continue
  fi
  if [ ! -d "$feed" ]; then
    echo "NO-FEED" > "$RAW/$label.rc"
    echo "RESULT $label status=nicht-baubar detail=kein-feed-paket guard=n/a"
    continue
  fi
  if [ -e "$tgt" ]; then
    echo "COLLISION" > "$RAW/$label.rc"
    echo "RESULT $label status=uebersprungen detail=zz_battery_test.go-existiert-schon guard=n/a"
    continue
  fi

  # 1. Vorher-Stand
  git -C "$ws" status --short > "$RAW/$label.pre-status"

  # 2. Baseline-Kompilat ohne Batterie
  ( cd "$ws/agora-backend" && timeout 90 go test ./internal/feed/ -run ZZZZNOMATCH -count=1 ) \
      > "$RAW/$label.basebuild" 2>&1
  echo "base_rc=$?" >> "$RAW/$label.basebuild"

  # 3./4. Batterie
  cp "$BATTERY" "$tgt"
  ( cd "$ws/agora-backend" && timeout 120 go test ./internal/feed/ -run ZZBat -count=1 -timeout 110s -json ) \
      > "$RAW/$label.json" 2> "$RAW/$label.stderr"
  echo "$?" > "$RAW/$label.rc"

  # 5. Entfernen und Unversehrtheit belegen
  rm -f "$tgt"
  git -C "$ws" status --short > "$RAW/$label.post-status"
  if [ ! -e "$tgt" ] && diff -u "$RAW/$label.pre-status" "$RAW/$label.post-status" > "$RAW/$label.status-diff" 2>&1; then
    echo "clean" > "$RAW/$label.wsclean"
  else
    echo "DIRTY" > "$RAW/$label.wsclean"
    echo "!! $label: Workspace NICHT unveraendert — siehe $RAW/$label.status-diff"
  fi

  # 6. Auswertung
  python3 "$HERE/summarize.py" --result "$label"
done
echo "=== fertig"
