#!/usr/bin/env bash
# run-all.sh — Generischer Robustheits-Batterie-Runner (Auftrag 01).
#
# Faehrt die Batterien aus robustness-battery/<task>/battery.json ueber alle
# vorhandenen Abgaben (runs/*/<task>/ws) und schreibt results.json (Schema 2).
# Es wird IMMER auf Wegwerf-Kopien (os.MkdirTemp) gerechnet — die Original-Workspaces
# unter bench/runs/ bleiben unangetastet (ro-Mount).
#
# Aufrufbeispiele:
#   ./run-all.sh                     # nur Paare berechnen, deren Fingerprint neu ist
#   ./run-all.sh --force             # wirklich alle Paare neu berechnen
#   ./run-all.sh --task agora-A6-scorer-scratch
#   ./run-all.sh --label qwen36moe-vulkan
#   ./run-all.sh --force --jobs 4    # Parallelitaet
#
# Der Runner liegt als eigenes Go-Modul (Modulname battery, nur Standardbibliothek)
# unter cmd/battery/. Wegen des verschachtelten go.mod springt das Skript dorthin
# und startet `go run .`; GOFLAGS=-mod=mod, GOPROXY=off und ein geteilter GOCACHE
# unter $HOME/.cache/gocache-battery machen die Laeufe offline und wiederholbar.
# Log je (task,label)-Paar: run-all.log (append).
set -euo pipefail
export GOFLAGS=-mod=mod
export GOPROXY=off
export GOCACHE=$HOME/.cache/gocache-battery
cd /home/lowelodev/ai-lab/bench/robustness-battery/cmd/battery
go run . "$@"
