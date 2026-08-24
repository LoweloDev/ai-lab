#!/usr/bin/env bash
# Wartet auf "A5A6-RETRY KOMPLETT" (Retry-Waechter-Log $1) — dann sind alle Suite-Laeufe der
# Nacht durch und kein run-task laeuft mehr. Danach:
#   - liegt bench/.haertung-freigabe (setzt Claude nach dem Review des Apply-Skripts):
#       bash bench/apply-haertung.sh  (setzt selbst .grader-haertung-done mit OK/ABGEBROCHEN)
#   - sonst: .grader-haertung-done = UEBERSPRUNGEN (Kandidaten-Kette laeuft mit alten Gradern)
# Idempotent; Ausgabe nach logs/haertung-waechter.log.
RETRY_LOG="${1:?Pfad zum Retry-Waechter-Log}"
B="$HOME/ai-lab/bench"; LOG="$HOME/ai-lab/logs/haertung-waechter.log"
log() { printf '%s %s\n' "$(date '+%d.%m %H:%M:%S')" "$*" | tee -a "$LOG"; }

while ! grep -q "A5A6-RETRY KOMPLETT" "$RETRY_LOG" 2>/dev/null; do sleep 300; done
log "Retries komplett — pruefe Haertungs-Freigabe"
[ -f "$B/.grader-haertung-done" ] && { log "Marker existiert bereits: $(cat "$B/.grader-haertung-done")"; exit 0; }

if [ -f "$B/.haertung-freigabe" ] && [ -x "$B/apply-haertung.sh" ]; then
  log "Freigabe vorhanden — wende Haertung an"
  # rc 2 = "beschaeftigt"/Lock (Apply hat selbst bis 15 min gewartet): NICHT abbrechen, sondern
  # bis zu 6x alle 10 min erneut versuchen (Review-Befund 25.08. 01:05). Erst danach Marker setzen.
  for versuch in 1 2 3 4 5 6; do
    bash "$B/apply-haertung.sh" >> "$LOG" 2>&1; rc=$?
    log "apply-haertung.sh Versuch $versuch rc=$rc, Marker: $(cat "$B/.grader-haertung-done" 2>/dev/null || echo FEHLT)"
    [ -f "$B/.grader-haertung-done" ] && break
    [ "$rc" -eq 2 ] || break
    sleep 600
  done
  [ -f "$B/.grader-haertung-done" ] || echo "ABGEBROCHEN rc=$rc nach $versuch Versuchen (kein Marker vom Skript)" > "$B/.grader-haertung-done"
else
  log "Keine Freigabe (oder kein apply-haertung.sh) — Haertung uebersprungen, Kandidaten laufen mit alten Gradern"
  echo "UEBERSPRUNGEN $(date +%d.%m-%H:%M)" > "$B/.grader-haertung-done"
fi
echo "HAERTUNG-WAECHTER FERTIG"
