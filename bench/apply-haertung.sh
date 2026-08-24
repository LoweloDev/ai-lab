#!/usr/bin/env bash
# apply-haertung.sh — Grader-Haertung (Audit 24./25.08.2026, audit-report.md Abschnitt 4) anwenden.
#
# Aufruf:  bash apply-haertung.sh [--dry-run]
#   --dry-run   nur Schritt 1 (Vorpruefung) + Bericht; veraendert NICHTS, schreibt keinen Marker.
# Umgebung: GRADE_A5=vertrag|streng ueberstimmt die Datei bench/.a5-lesart (Inhalt 'vertrag' => P5
#           Vertragslesart grade_test.v2-vertrag.go; fehlt/leer/'streng' => grade_test.v2.go).
#
# Ablauf (Politik audit-report.md 4.4):
#   0  Schutz: flock (APPLY wartet bis LOCK_WAIT s auf einen fremden Halter); kein laufender
#      run-task*/nachzuegler/nachtkette-Prozess (APPLY wartet bis BUSY_WAIT s auf deren Ende; sonst rc 2,
#      KEIN Marker, damit die Kandidaten-Kette nicht vorzeitig freigegeben wird). Danach schreibt jeder
#      Abbruch — auch ein unerwarteter (set -e) — einen Marker 'ABGEBROCHEN ...'; ein offener Swap wird
#      dabei zurueckgerollt (EXIT-Trap).
#   1  Vorpruefung (Trockenlauf): letzter Eintrag je (Label, Task) in results.jsonl — aufgezeichnetes
#      Urteil ("alt") vs. Staging-Grader grade.v2.sh ("v2", A5 ggf. mit GRADE_HIDDEN=Vertragsdatei).
#      Flips ausserhalb der erwarteten Menge (leer; bei Vertragslesart genau die 8 Audit-Labels in A5
#      FAIL->PASS) => ABBRUCH mit Bericht, nichts anwenden, Marker 'ABGEBROCHEN ...'.
#      Zusaetzlich: der unberuehrte Baseline-Workspace darf unter v2 kein PASS bekommen (Freilos-Check).
#   2  Backup: results.jsonl.bak-audit-<stamp>; grade.sh -> grade.v1.sh; grade_test.go -> grade_test.v1.go.
#   3  Swap: grade.v2.sh -> grade.sh (cp; im Installat zeigt HIDDEN auf grade_test.go statt grade_test.v2.go),
#      grade_test.v2.go -> grade_test.go (A3, A6), A5 je Lesart. Rauchtest: Baseline FAIL, A5-Referenz PASS;
#      bei Fehlschlag automatischer Rollback.
#   4  Regrade aller letzten Eintraege mit dem INSTALLIERTEN grade.sh (absoluter Workspace-Pfad); nur bei
#      Klassenwechsel PASS<->FAIL wird ausschliesslich das grade-Feld der Zeile ersetzt (keine neue Zeile).
#      Log audit-scratch/regrade-<stamp>.log; Flips als Nachtrag in failure-analysis.md und als
#      Eintrag 'suite:<label>/<task>' in dashboard/registry/run-annotations.json.
#   5  Marker bench/.grader-haertung-done = 'OK <stamp> flips=<n>'.
#   Idempotent: liegt grade.v1.sh bereits fuer alle Tasks, werden 2+3 uebersprungen (nur Regrade-Vergleich).
#
# Rollback von Hand:  cp tasks/<t>/grade.v1.sh tasks/<t>/grade.sh; cp tasks/<t>/grade_test.v1.go
#   tasks/<t>/grade_test.go (A5, A6); rm tasks/agora-A3-hls/grade_test.go; cp results.jsonl.bak-audit-<stamp>
#   results.jsonl; rm .grader-haertung-done.
set -euo pipefail
export LC_ALL=C

BENCH=/home/lowelodev/ai-lab/bench
TASKS_DIR=$BENCH/tasks
RUNS=$BENCH/runs
WSDIR=$BENCH/workspaces
RESULTS=$BENCH/results.jsonl
MARKER=$BENCH/.grader-haertung-done
LESART_FILE=$BENCH/.a5-lesart
LOCK=$BENCH/.apply-haertung.lock
ANNOT=/home/lowelodev/ai-lab/dashboard/registry/run-annotations.json
FA=$BENCH/failure-analysis.md
SCRATCH=$BENCH/audit-scratch
WORK=$SCRATCH/haertung/integrator
A5=agora-A5-batcher-scratch
A5_REF=$SCRATCH/a5aud-orig          # legitimer Original-Batcher: positive Kontrolle (muss PASS)
TASKS=(agora-A1-gate agora-A2-jsonld agora-A3-hls agora-A4-feed agora-A5-batcher-scratch
       agora-A6-scorer-scratch aiux-U1-paging aiux-U2-denytools)
GO_HIDDEN_TASKS="agora-A3-hls agora-A5-batcher-scratch agora-A6-scorer-scratch"   # bekommen grade_test.go
GRADE_TIMEOUT=900
EXPECTED_A5_LABELS="agy-37flash cc-opus48 cc-opus5 dsh-v4-flash dsh-v4-pro oc-gemini37f oc-v4-flash oc-v4-pro"
PROC_PATTERN='run-task[A-Za-z0-9_.-]*\.sh|nachzuegler-a5a6-retry\.sh|nachtkette[A-Za-z0-9_.-]*\.sh'
LOCK_WAIT=${APPLY_LOCK_WAIT:-1800}   # APPLY: so lange auf einen fremden Halter (z. B. Trockenlauf) warten (s)
BUSY_WAIT=${APPLY_BUSY_WAIT:-900}    # APPLY: so lange auf auslaufende Suite-Prozesse warten (s), dann rc 2

DRY=0
for a in "$@"; do
  case $a in
    --dry-run) DRY=1 ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) echo "unbekannte Option: $a" >&2; exit 64 ;;
  esac
done

stamp=$(date +%Y%m%d-%H%M%S)
if [ "$DRY" = 1 ]; then RUNDIR=$WORK/dry-$stamp; else RUNDIR=$WORK/apply-$stamp; fi
mkdir -p "$RUNDIR/logs"
REPORT=$RUNDIR/bericht.md
log() { printf '%s %s\n' "$(date '+%H:%M:%S')" "$*"; }
die() { echo "ABBRUCH: $*" >&2; exit "${2:-1}"; }

# Sicherheitsnetz fuer jeden unerwarteten Abbruch (set -e, Signal) NACH der Schutzpruefung: laufender
# Swap wird zurueckgerollt, und es liegt in jedem Fall ein Marker (sonst wartet die Kandidaten-Kette
# ewig bzw. der Waechter schreibt einen inhaltsleeren). Explizite Abbruchpfade schreiben ihren
# eigenen Marker vorher; der Trockenlauf und die Schutzpruefung (rc 2, absichtlich ohne Marker) sind
# ausgenommen.
PHASE=start; PROTECTED=0; SWAP_OPEN=0; FINISHED=0
on_exit() {
  local rc=$?
  [ "$FINISHED" = 1 ] && return 0
  [ "$DRY" = 1 ] && return 0
  [ "$PROTECTED" = 1 ] || return 0
  if [ "$SWAP_OPEN" = 1 ]; then rollback_all || true; fi
  if [ -f "$MARKER" ] && grep -q "^ABGEBROCHEN $stamp" "$MARKER"; then return 0; fi
  local ber=""; [ -f "$REPORT" ] && ber=" bericht=$REPORT"
  echo "ABGEBROCHEN $stamp unerwarteter-abbruch rc=$rc phase=$PHASE log=$RUNDIR/logs$ber" > "$MARKER" || true
  echo "ABBRUCH: unerwarteter Abbruch rc=$rc in Phase '$PHASE' — Marker: $(cat "$MARKER" 2>/dev/null)" >&2
}
trap on_exit EXIT

# ------------------------------------------------------------------ 0 Schutz
exec 9>"$LOCK"
if [ "$DRY" = 1 ]; then
  flock -n 9 || die "apply-haertung.sh laeuft bereits (Lock $LOCK)" 2
else
  # ein Trockenlauf (~2 min) oder ein zweiter Apply darf den unbeaufsichtigten Lauf nicht kippen
  flock -w "$LOCK_WAIT" 9 || die "apply-haertung.sh laeuft bereits (Lock $LOCK, ${LOCK_WAIT}s gewartet)" 2
fi

# Suite-Prozesse: im APPLY-Modus bis zu BUSY_WAIT s auf ihr Ende warten (Waechter startet unmittelbar nach
# 'A5A6-RETRY KOMPLETT' — der letzte run-task kann noch ausklingen), danach rc 2 ohne Marker.
# Eigene Vorfahren ausnehmen: die aufrufende Shell (z. B. ein 'zsh -c "... run-task.sh ...; bash apply-haertung.sh"')
# traegt das Muster sonst selbst in ihrer Kommandozeile und blockiert den Lauf.
ancestors=" $$ "; p=$PPID
while [ -n "$p" ] && [ "$p" -gt 1 ]; do
  ancestors="$ancestors$p "
  p=$(ps -o ppid= -p "$p" 2>/dev/null | tr -d ' ') || break
done
# Ebenso fremde pgrep/grep/pkill-Aufrufe (z. B. ein Monitor, der selbst nach 'run-task.sh' sucht): ihre
# Kommandozeile enthaelt das Muster, sie sind aber keine Suite-Prozesse.
list_busy() {
  pgrep -af "$PROC_PATTERN" | awk -v anc="$ancestors" 'index(anc, " "$1" ")==0 && $2 !~ /(^|\/)(pgrep|pkill|grep|egrep|ps)$/' || true
}
waited=0
while :; do
  busy=$(list_busy)
  [ -n "$busy" ] || break
  [ "$DRY" = 1 ] && break
  if [ "$waited" -eq 0 ]; then
    log "WARN Suite-Prozesse laufen — warte bis zu ${BUSY_WAIT}s auf ihr Ende:"
    printf '%s\n' "$busy" | sed 's/^/    /'
  fi
  [ "$waited" -lt "$BUSY_WAIT" ] || break
  sleep 15; waited=$((waited+15))
done
[ "$waited" -eq 0 ] || log "  gewartet: ${waited}s"
if [ -n "$busy" ]; then
  if [ "$DRY" = 1 ]; then
    log "WARN laufende Suite-Prozesse (Trockenlauf ueberspringt deren Workspaces):"
    printf '%s\n' "$busy" | sed 's/^/    /'
  else
    echo "$busy" >&2
    die "Suite-Prozesse laufen noch (run-task*/nachzuegler/nachtkette) — kein Marker geschrieben" 2
  fi
fi
# (Label, Task)-Paare, die gerade von einem run-task laufen (nur fuer den Trockenlauf relevant)
active_pairs=$(printf '%s\n' "$busy" | awk '{for(i=1;i<=NF;i++) if($i ~ /run-task[A-Za-z0-9_.-]*\.sh$/ && i+2<=NF){print $(i+1)"/"$(i+2)}}' | sort -u || true)
PROTECTED=1; PHASE=vorbedingungen

# ------------------------------------------------------------------ Lesart A5
LESART=${GRADE_A5:-}
if [ -z "$LESART" ] && [ -f "$LESART_FILE" ]; then LESART=$(tr -d '[:space:]' < "$LESART_FILE"); fi
LESART=$(printf '%s' "$LESART" | tr '[:upper:]' '[:lower:]')
case $LESART in
  vertrag) A5_HIDDEN=grade_test.v2-vertrag.go ;;
  ''|streng) LESART=streng; A5_HIDDEN=grade_test.v2.go ;;
  *) die "unbekannte A5-Lesart '$LESART' (erlaubt: vertrag|streng; Quelle GRADE_A5 oder $LESART_FILE)" 3 ;;
esac

# ------------------------------------------------------------------ Vorbedingungen
for t in "${TASKS[@]}"; do
  [ -f "$TASKS_DIR/$t/grade.v2.sh" ] || die "fehlt: $TASKS_DIR/$t/grade.v2.sh"
  [ -d "$WSDIR/$t" ] || die "fehlt: Baseline-Workspace $WSDIR/$t"
done
for t in $GO_HIDDEN_TASKS; do [ -f "$TASKS_DIR/$t/grade_test.v2.go" ] || die "fehlt: $TASKS_DIR/$t/grade_test.v2.go"; done
[ -f "$TASKS_DIR/$A5/$A5_HIDDEN" ] || die "fehlt: $TASKS_DIR/$A5/$A5_HIDDEN"
[ -f "$TASKS_DIR/aiux-U2-denytools/grade.v2.test.js" ] || die "fehlt: grade.v2.test.js (U2)"
[ -f "$RESULTS" ] || die "fehlt: $RESULTS"
[ -f "$ANNOT" ] || die "fehlt: $ANNOT"
command -v python3 >/dev/null || die "python3 fehlt"
command -v go >/dev/null || die "go fehlt"
command -v node >/dev/null || die "node fehlt"

installed_count=0
for t in "${TASKS[@]}"; do [ -f "$TASKS_DIR/$t/grade.v1.sh" ] && installed_count=$((installed_count+1)) || true; done
if [ "$installed_count" -eq "${#TASKS[@]}" ]; then INSTALLED=1
elif [ "$installed_count" -eq 0 ]; then INSTALLED=0
else die "inkonsistenter Zustand: grade.v1.sh liegt fuer $installed_count von ${#TASKS[@]} Tasks — von Hand klaeren"; fi
# Zweiter Aufruf: die installierte A5-Datei muss zur angeforderten Lesart passen, sonst wuerde der Marker
# 'lesart=<neu>' behaupten, waehrend die alte Lesart installiert bleibt (2+3 werden uebersprungen).
if [ "$INSTALLED" = 1 ] && ! cmp -s "$TASKS_DIR/$A5/$A5_HIDDEN" "$TASKS_DIR/$A5/grade_test.go"; then
  die "installiertes $TASKS_DIR/$A5/grade_test.go entspricht nicht der angeforderten A5-Lesart '$LESART' ($A5_HIDDEN). Wechsel streng->vertrag: 'cp $TASKS_DIR/$A5/$A5_HIDDEN $TASKS_DIR/$A5/grade_test.go' und erneut aufrufen (Regrade traegt die 8 erwarteten Flips ein); vertrag->streng: erst Rollback (Kopf dieses Skripts), dann erneut anwenden" 3
fi

log "apply-haertung $stamp  modus=$([ "$DRY" = 1 ] && echo TROCKENLAUF || echo APPLY)  lesart_A5=$LESART ($A5_HIDDEN)  bereits_installiert=$INSTALLED"
log "Arbeitsverzeichnis: $RUNDIR"

# ------------------------------------------------------------------ Hilfsfunktionen
# letzte Zeile je (model, task): "zeilennr<TAB>model<TAB>task<TAB>grade"
last_entries() {
  python3 - "$RESULTS" <<'PY'
import json, sys
last = {}
bad = []
with open(sys.argv[1], encoding='utf-8') as f:
    for i, line in enumerate(f, 1):
        line = line.strip()
        if not line:
            continue
        try:
            d = json.loads(line)
        except json.JSONDecodeError:
            bad.append(i)   # nicht still uebergehen: eine unlesbare juengere Zeile liesse eine aeltere als "letzte" gelten
            continue
        last[(d.get('model'), d.get('task'))] = (i, d.get('grade', ''))
if bad:
    sys.stderr.write(f"WARN results.jsonl: {len(bad)} unlesbare Zeile(n) uebersprungen: {bad[:10]}\n")
for (m, t), (i, g) in sorted(last.items(), key=lambda kv: (str(kv[0][1]), str(kv[0][0]))):
    print(f"{i}\t{m}\t{t}\t{g}")
PY
}

cls() { case $1 in PASS*) echo PASS ;; *) echo FAIL ;; esac; }

# grade_run <script> <ws> <logfile> [VAR=WERT ...]  -> setzt VERDICT, GSECS
grade_run() {
  local script=$1 ws=$2 logf=$3; shift 3
  local t0 t1 rc out
  t0=$(date +%s)
  # </dev/null: der Aufrufer liest gerade aus last_entries (Prozess-Substitution) — kein Grader darf davon zehren
  out=$(cd / && env "$@" timeout --kill-after=30 "$GRADE_TIMEOUT" bash "$script" "$ws" 2>&1 </dev/null) && rc=0 || rc=$?
  t1=$(date +%s)
  printf '%s\n[rc=%s]\n' "$out" "$rc" > "$logf"
  VERDICT=$(printf '%s\n' "$out" | grep . | tail -n 1 || true)
  case $rc in 124|137) VERDICT="FAIL grader-timeout rc=$rc" ;; esac
  case $VERDICT in PASS*|FAIL*) ;; *) VERDICT="FAIL grader-error rc=$rc (${VERDICT:0:80})" ;; esac
  GSECS=$((t1 - t0))
}

# compare_pass <phase> <script-name> : bewertet alle letzten Eintraege, schreibt <RUNDIR>/<phase>.tsv
#   Spalten: zeile label task alt_klasse neu_klasse status alt_grade neu_grade sek
#   status: same | FLIP | SKIP-<grund>
compare_pass() {
  local phase=$1 which=$2
  local tsv=$RUNDIR/$phase.tsv
  : > "$tsv"
  local n=0
  while IFS=$'\t' read -r ln label task alt; do
    [ -n "$label" ] || continue
    n=$((n+1))
    local ws=$RUNS/$label/$task/ws script status neu="" secs=0
    local -a envs=()
    if [ "$which" = v2 ]; then
      script=$TASKS_DIR/$task/grade.v2.sh
      [ "$task" = "$A5" ] && envs=("GRADE_HIDDEN=$A5_HIDDEN")
    else
      script=$TASKS_DIR/$task/grade.sh
    fi
    if ! printf '%s\n' "${TASKS[@]}" | grep -qx -- "$task"; then status="SKIP-unbekannter-task"
    elif [ ! -d "$ws" ]; then status="SKIP-kein-workspace"
    elif [ "$DRY" = 1 ] && printf '%s\n' "$active_pairs" | grep -qx -- "$label/$task"; then status="SKIP-laeuft-gerade"
    else
      mkdir -p "$RUNDIR/logs/$phase/$task"
      grade_run "$script" "$ws" "$RUNDIR/logs/$phase/$task/$label.log" "${envs[@]}"
      neu=$VERDICT; secs=$GSECS
      if [ "$(cls "$alt")" = "$(cls "$neu")" ]; then status=same; else status=FLIP; fi
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$ln" "$label" "$task" "$(cls "$alt")" "$( [ -n "$neu" ] && cls "$neu" || echo -)" "$status" "$alt" "$neu" "$secs" >> "$tsv"
    log "  [$phase $n] $task $label: $status  alt='$alt'  neu='$neu' (${secs}s)"
  done < <(last_entries)
}

# flip_expected <label> <task> <alt_cls> <neu_cls>
flip_expected() {
  [ "$LESART" = vertrag ] || return 1
  [ "$2" = "$A5" ] && [ "$3" = FAIL ] && [ "$4" = PASS ] || return 1
  printf ' %s ' "$EXPECTED_A5_LABELS" | grep -qF -- " $1 "
}

# unexpected_flips <tsv> -> Zeilen der unerwarteten Flips; missing_expected <tsv> -> fehlende erwartete
unexpected_flips() {
  awk -F'\t' '$6=="FLIP"' "$1" | while IFS=$'\t' read -r ln label task ac nc st alt neu secs; do
    flip_expected "$label" "$task" "$ac" "$nc" || printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$ln" "$label" "$task" "$ac" "$nc" "$neu"
  done
}
missing_expected() {
  [ "$LESART" = vertrag ] || return 0
  for l in $EXPECTED_A5_LABELS; do
    # bereits als PASS aufgezeichnet (z. B. beim zweiten Aufruf) zaehlt als erledigt
    awk -F'\t' -v l="$l" -v t="$A5" '$2==l && $3==t && (($6=="FLIP" && $4=="FAIL" && $5=="PASS") || $4=="PASS") {f=1} END{exit f?0:1}' "$1" \
      || echo "$l"
  done
}

table_md() { # table_md <tsv> : Markdown-Tabelle je Task
  printf '| Task | n | gleich | Flips | uebersprungen |\n|---|---|---|---|---|\n'
  for t in "${TASKS[@]}"; do
    awk -F'\t' -v t="$t" '$3==t {n++; if($6=="same")s++; else if($6=="FLIP")f++; else k++} END{printf "| %s | %d | %d | %d | %d |\n", t, n, s, f, k}' "$1"
  done
  awk -F'\t' '{n++; if($6=="same")s++; else if($6=="FLIP")f++; else k++} END{printf "| **Summe** | **%d** | **%d** | **%d** | **%d** |\n", n, s, f, k}' "$1"
}
flips_md() { # flips_md <tsv>
  local rows
  rows=$(awk -F'\t' '$6=="FLIP"' "$1")
  if [ -z "$rows" ]; then echo "_keine Flips_"; return 0; fi
  printf '| Zeile | Label | Task | alt | neu | erwartet | neues Urteil |\n|---|---|---|---|---|---|---|\n'
  printf '%s\n' "$rows" | while IFS=$'\t' read -r ln label task ac nc st alt neu secs; do
    local e=nein; flip_expected "$label" "$task" "$ac" "$nc" && e=ja
    printf '| %s | %s | %s | %s | %s | %s | `%s` |\n' "$ln" "$label" "$task" "$ac" "$nc" "$e" "$neu"
  done
}
skips_md() {
  local rows
  rows=$(awk -F'\t' '$6 ~ /^SKIP/' "$1")
  [ -n "$rows" ] || return 0
  printf '\nUebersprungen:\n\n'
  printf '%s\n' "$rows" | awk -F'\t' '{printf "- %s / %s: %s\n", $2, $3, $6}'
}

# ------------------------------------------------------------------ 1 Vorpruefung
PHASE=vorpruefung
log "Schritt 1: Vorpruefung — aufgezeichnetes Urteil vs. grade.v2.sh"
compare_pass vorpruefung v2

log "Schritt 1b: Freilos-Check — Baseline-Workspaces duerfen unter v2 nicht PASS sein"
freilos=""
for t in "${TASKS[@]}"; do
  envs=(); [ "$t" = "$A5" ] && envs=("GRADE_HIDDEN=$A5_HIDDEN")
  grade_run "$TASKS_DIR/$t/grade.v2.sh" "$WSDIR/$t" "$RUNDIR/logs/baseline-$t.log" "${envs[@]}"
  log "  baseline $t: $VERDICT"
  [ "$(cls "$VERDICT")" = FAIL ] || freilos="$freilos $t"
done

unexp=$(unexpected_flips "$RUNDIR/vorpruefung.tsv")
miss=$(missing_expected "$RUNDIR/vorpruefung.tsv")
n_unexp=$(printf '%s' "$unexp" | grep -c . || true)
n_flip=$(awk -F'\t' '$6=="FLIP"' "$RUNDIR/vorpruefung.tsv" | wc -l)

{
  echo "# apply-haertung $stamp — $([ "$DRY" = 1 ] && echo Trockenlauf || echo Anwendung)"
  echo
  echo "- Lesart A5: **$LESART** ($A5_HIDDEN); bereits installiert: $INSTALLED"
  echo "- results.jsonl: $(wc -l < "$RESULTS") Zeilen, $(last_entries | wc -l) letzte (Label, Task)-Eintraege"
  echo
  echo "## Schritt 1 — Vorpruefung: aufgezeichnetes Urteil vs. grade.v2.sh"
  echo
  table_md "$RUNDIR/vorpruefung.tsv"
  echo
  echo "### Flips"
  echo
  flips_md "$RUNDIR/vorpruefung.tsv"
  skips_md "$RUNDIR/vorpruefung.tsv"
  echo
  echo "### Freilos-Check (Baseline unter v2)"
  echo
  for t in "${TASKS[@]}"; do printf -- '- %s: `%s`\n' "$t" "$(grep . "$RUNDIR/logs/baseline-$t.log" | grep -E '^(PASS|FAIL)' | tail -n 1)"; done
  echo
  echo "### Ergebnis"
  echo
  echo "- Flips gesamt: $n_flip, davon unerwartet: **$n_unexp**"
  [ -z "$miss" ] || echo "- WARN erwartete Flips fehlen (A5 Vertragslesart): $(echo $miss)"
  [ -z "$freilos" ] || echo "- **FEHLER Freilos: Baseline besteht unter v2:**$freilos"
  echo "- Rohdaten: $RUNDIR/vorpruefung.tsv, Grader-Logs unter $RUNDIR/logs/"
} > "$REPORT"

abort_step1=0
[ "$n_unexp" -eq 0 ] || abort_step1=1
[ -z "$freilos" ] || abort_step1=1

if [ "$DRY" = 1 ]; then
  echo; cat "$REPORT"; echo
  if [ "$abort_step1" = 1 ]; then log "TROCKENLAUF: Anwendung wuerde ABGEBROCHEN (unerwartete Flips=$n_unexp, Freilos='$freilos')"; exit 1; fi
  log "TROCKENLAUF: Vorpruefung bestanden — Anwendung wuerde durchlaufen (erwartete Flips=$n_flip)"
  exit 0
fi

if [ "$abort_step1" = 1 ]; then
  {
    echo; echo "## ABGEBROCHEN"; echo
    echo "Unerwartete Flips ($n_unexp) oder Freilos ($freilos) — nichts angewendet. Einzeln pruefen, dann erneut aufrufen."
    [ -z "$unexp" ] || { echo; printf '%s\n' "$unexp" | awk -F'\t' '{printf "- Zeile %s %s / %s: %s -> %s (`%s`)\n", $1,$2,$3,$4,$5,$6}'; }
  } >> "$REPORT"
  echo "ABGEBROCHEN $stamp unerwartete-flips=$n_unexp freilos=[$(echo $freilos)] bericht=$REPORT" > "$MARKER"
  cat "$REPORT"
  die "Vorpruefung nicht bestanden — Marker: $(cat "$MARKER")"
fi
log "Vorpruefung bestanden: Flips=$n_flip (alle erwartet), unerwartet=0"

# ------------------------------------------------------------------ Rollback
rollback_all() {
  log "ROLLBACK der Grader-Installation"
  for t in "${TASKS[@]}"; do
    [ -f "$TASKS_DIR/$t/grade.v1.sh" ] && cp -p "$TASKS_DIR/$t/grade.v1.sh" "$TASKS_DIR/$t/grade.sh" && rm -f "$TASKS_DIR/$t/grade.v1.sh"
    if [ -f "$TASKS_DIR/$t/grade_test.v1.go" ]; then
      cp -p "$TASKS_DIR/$t/grade_test.v1.go" "$TASKS_DIR/$t/grade_test.go"; rm -f "$TASKS_DIR/$t/grade_test.v1.go"
    elif [ "$t" = agora-A3-hls ]; then rm -f "$TASKS_DIR/$t/grade_test.go"; fi
    rm -f "$TASKS_DIR/$t/.grade.sh.new" "$TASKS_DIR/$t/.grade_test.go.new"
  done
  SWAP_OPEN=0
}

# ------------------------------------------------------------------ 2 Backup
PHASE=backup
BAK=$RESULTS.bak-audit-$stamp
cp -p "$RESULTS" "$BAK"
log "Schritt 2: Backup $BAK"
if [ "$INSTALLED" = 0 ]; then
  SWAP_OPEN=1   # ab hier rollt on_exit bei jedem unerwarteten Abbruch zurueck
  for t in "${TASKS[@]}"; do
    cp -p "$TASKS_DIR/$t/grade.sh" "$TASKS_DIR/$t/grade.v1.sh"
    if [ -f "$TASKS_DIR/$t/grade_test.go" ]; then cp -p "$TASKS_DIR/$t/grade_test.go" "$TASKS_DIR/$t/grade_test.v1.go"; fi
  done
  log "  grade.sh -> grade.v1.sh (8x), grade_test.go -> grade_test.v1.go (A5, A6)"

  # ---------------------------------------------------------------- 3 Swap
  PHASE=swap
  log "Schritt 3: Swap grade.v2.sh -> grade.sh, versteckte Tests -> grade_test.go"
  for t in "${TASKS[@]}"; do
    src=$TASKS_DIR/$t/grade.v2.sh; dst=$TASKS_DIR/$t/grade.sh; tmpf=$TASKS_DIR/$t/.grade.sh.new
    # im Installat: HIDDEN auf grade_test.go (nur ausserhalb von Kommentarzeilen), Herkunftszeile nach dem Shebang
    sed -E '/^[[:space:]]*#/! s/grade_test\.v2\.go/grade_test.go/g' "$src" \
      | sed "1a # INSTALLIERT $stamp durch apply-haertung.sh aus grade.v2.sh (A5-Lesart: $LESART). Vorgaenger: grade.v1.sh" > "$tmpf"
    bash -n "$tmpf" || { rm -f "$tmpf"; rollback_all; die "Syntaxfehler im Installat $t"; }
    if grep -vE '^[[:space:]]*#' "$tmpf" | grep -q 'grade_test\.v2\.go'; then rm -f "$tmpf"; rollback_all; die "Installat $t verweist noch auf grade_test.v2.go"; fi
    chmod 755 "$tmpf"; mv -f "$tmpf" "$dst"
    case $t in
      agora-A3-hls|agora-A6-scorer-scratch) hid=grade_test.v2.go ;;
      "$A5") hid=$A5_HIDDEN ;;
      *) hid="" ;;
    esac
    if [ -n "$hid" ]; then
      cp "$TASKS_DIR/$t/$hid" "$TASKS_DIR/$t/.grade_test.go.new" && mv -f "$TASKS_DIR/$t/.grade_test.go.new" "$TASKS_DIR/$t/grade_test.go"
      cmp -s "$TASKS_DIR/$t/$hid" "$TASKS_DIR/$t/grade_test.go" || { rollback_all; die "grade_test.go ($t) nicht identisch mit $hid"; }
      log "  $t: grade.sh <- grade.v2.sh, grade_test.go <- $hid"
    else
      log "  $t: grade.sh <- grade.v2.sh"
    fi
  done

  log "Schritt 3b: Rauchtest des Installats (Baseline FAIL, A5-Referenz PASS)"
  smoke_bad=""
  for t in "${TASKS[@]}"; do
    grade_run "$TASKS_DIR/$t/grade.sh" "$WSDIR/$t" "$RUNDIR/logs/smoke-baseline-$t.log"
    log "  baseline $t: $VERDICT"
    [ "$(cls "$VERDICT")" = FAIL ] || smoke_bad="$smoke_bad baseline:$t"
  done
  if [ -d "$A5_REF/agora-backend" ]; then
    grade_run "$TASKS_DIR/$A5/grade.sh" "$A5_REF" "$RUNDIR/logs/smoke-a5-referenz.log"
    log "  A5-Referenz (a5aud-orig): $VERDICT"
    [ "$(cls "$VERDICT")" = PASS ] || smoke_bad="$smoke_bad a5-referenz"
  else
    log "  WARN A5-Referenz $A5_REF fehlt — positive Kontrolle uebersprungen"
  fi
  if [ -n "$smoke_bad" ]; then
    rollback_all
    echo "ABGEBROCHEN $stamp rauchtest=[$(echo $smoke_bad)] (Rollback ausgefuehrt) bericht=$REPORT" > "$MARKER"
    printf '\n## ABGEBROCHEN im Rauchtest\n\n%s — Installation zurueckgerollt.\n' "$smoke_bad" >> "$REPORT"
    die "Rauchtest fehlgeschlagen:$smoke_bad — Rollback ausgefuehrt"
  fi
  SWAP_OPEN=0   # Installat verifiziert — ab hier bleibt der Grader auch bei spaeterem Abbruch installiert
else
  log "Schritte 2b/3 uebersprungen: grade.v1.sh liegt bereits (zweiter Aufruf) — nur Regrade-Vergleich"
fi

# ------------------------------------------------------------------ 4 Regrade mit installiertem Grader
PHASE=regrade
log "Schritt 4: Regrade aller letzten Eintraege mit dem installierten grade.sh"
compare_pass regrade installed
REGLOG=$SCRATCH/regrade-$stamp.log
{
  echo "# Regrade $stamp — installierter Grader (P1/P2/P4/P8, U2 P6, A5 $LESART), Politik audit-report.md 4.4"
  echo "# Spalten: zeile label task alt_klasse neu_klasse status alt_grade neu_grade sek"
  cat "$RUNDIR/regrade.tsv"
  echo "# same=$(awk -F'\t' '$6=="same"' "$RUNDIR/regrade.tsv" | wc -l) flip=$(awk -F'\t' '$6=="FLIP"' "$RUNDIR/regrade.tsv" | wc -l) skip=$(awk -F'\t' '$6 ~ /^SKIP/' "$RUNDIR/regrade.tsv" | wc -l)"
  echo "# gleiche Klasse, anderer Urteilstext (nicht in results.jsonl uebernommen): $(awk -F'\t' '$6=="same" && $7!=$8' "$RUNDIR/regrade.tsv" | wc -l)"
} > "$REGLOG"

unexp2=$(unexpected_flips "$RUNDIR/regrade.tsv")
if [ -n "$unexp2" ]; then
  # Sollte nach bestandener Vorpruefung nicht vorkommen (gleicher Grader). Nichts in results.jsonl aendern.
  printf '\n## ABGEBROCHEN im Regrade\n\nUnerwartete Flips mit dem installierten Grader (Grader bleibt installiert, results.jsonl unveraendert):\n\n' >> "$REPORT"
  printf '%s\n' "$unexp2" | awk -F'\t' '{printf "- Zeile %s %s / %s: %s -> %s (`%s`)\n", $1,$2,$3,$4,$5,$6}' >> "$REPORT"
  echo "ABGEBROCHEN $stamp regrade-unerwartete-flips=$(printf '%s\n' "$unexp2" | wc -l) (Grader installiert, results.jsonl unveraendert) bericht=$REPORT" > "$MARKER"
  die "Regrade: unerwartete Flips — siehe $REPORT"
fi

# In-Place-Korrektur: nur das grade-Feld der betroffenen Zeile
flips_tsv=$RUNDIR/flips.tsv
awk -F'\t' '$6=="FLIP"' "$RUNDIR/regrade.tsv" > "$flips_tsv"
n_flips=$(wc -l < "$flips_tsv")
if [ "$n_flips" -gt 0 ]; then
  PHASE=results-korrektur   # Grader installiert; results.jsonl wird atomar (tmp + os.replace) ersetzt
  python3 - "$RESULTS" "$flips_tsv" <<'PY'
import json, os, re, sys
results, flips = sys.argv[1], sys.argv[2]
lines = open(results, encoding='utf-8').read().split('\n')
for row in open(flips, encoding='utf-8'):
    row = row.rstrip('\n')
    if not row:
        continue
    ln, label, task, ac, nc, st, alt, neu, secs = row.split('\t')
    i = int(ln) - 1
    d = json.loads(lines[i])
    assert d['model'] == label and d['task'] == task and d['grade'] == alt, (ln, label, task)
    old = '"grade":' + json.dumps(alt, ensure_ascii=False)
    new = '"grade":' + json.dumps(neu, ensure_ascii=False)
    assert lines[i].count(old) == 1, ('grade-Feld nicht eindeutig', ln)
    lines[i] = lines[i].replace(old, new, 1)
    json.loads(lines[i])  # bleibt gueltiges JSON
tmp = results + '.tmp'
open(tmp, 'w', encoding='utf-8').write('\n'.join(lines))
os.replace(tmp, results)
PY
  log "  results.jsonl: $n_flips grade-Felder in place ersetzt (Backup $BAK)"

  # failure-analysis.md Nachtrag
  {
    echo
    echo "## Nachtrag $(date '+%d.%m. %H:%M') — Grader-Haertung angewendet (apply-haertung.sh $stamp)"
    echo "Alle Grader auf das gehaertete Skelett gehoben (audit-report.md 4.1: P1 Go, P2 Node, P4 A6-Properties,"
    echo "P6 U2-Tooltabelle, P8 Warn-Grep; A5-Lesart: $LESART). Alle letzten Eintraege je (Label, Task) neu"
    echo "bewertet (Log: audit-scratch/regrade-$stamp.log, Backup results.jsonl.bak-audit-$stamp)."
    echo "Flips (nur das grade-Feld in place ersetzt, keine neue Zeile):"
    echo
    while IFS=$'\t' read -r ln label task ac nc st alt neu secs; do
      if [ "$task" = "$A5" ] && [ "$LESART" = vertrag ]; then
        echo "- $label / $task: $ac -> $nc (Zeile $ln; P5 Vertragslesart von Property (c), F8). Grund: Brief und"
        echo "  Aufrufer-Vertrag liefern eine sortierte Liste; unter defensiver Lesart FAIL (nur Property c)."
      else
        echo "- $label / $task: $ac -> $nc (Zeile $ln). Neues Urteil: \`$neu\`. Grader-Haertung P1/P2/P4/P6;"
        echo "  Einzelpruefung noetig (audit-report.md 4.2: jede Abweichung ausserhalb P5 ist ein echter Fund)."
      fi
    done < "$flips_tsv"
  } >> "$FA"
  log "  failure-analysis.md: Nachtrag mit $n_flips Flips"

  # run-annotations.json (Format wie Dashboard save_json: indent=1, ensure_ascii=False)
  python3 - "$ANNOT" "$flips_tsv" "$stamp" "$LESART" "$A5" <<'PY'
import json, os, sys
annot_path, flips, stamp, lesart, a5 = sys.argv[1:6]
annot = json.load(open(annot_path, encoding='utf-8'))
for row in open(flips, encoding='utf-8'):
    row = row.rstrip('\n')
    if not row:
        continue
    ln, label, task, ac, nc, st, alt, neu, secs = row.split('\t')
    key = f"suite:{label}/{task}"
    a = dict(annot.get(key) or {})
    if task == a5 and lesart == 'vertrag':
        a['note'] = 'unter defensiver Lesart FAIL (nur Property c)'
        a['patch'] = 'P5'
    else:
        a['note'] = f'Regrade nach Grader-Haertung: {ac} -> {nc}'
        a['patch'] = 'P1/P2/P4/P6'
    a['regrade'] = {'stamp': stamp, 'alt': alt, 'neu': neu, 'zeile': int(ln)}
    annot[key] = a
tmp = annot_path + '.tmp'
with open(tmp, 'w', encoding='utf-8') as f:
    json.dump(annot, f, ensure_ascii=False, indent=1)
os.replace(tmp, annot_path)
PY
  log "  run-annotations.json: $n_flips Eintraege 'suite:<label>/<task>'"
else
  log "  keine Flips — results.jsonl unveraendert"
fi

{
  echo; echo "## Schritt 4 — Regrade mit installiertem grade.sh"; echo
  table_md "$RUNDIR/regrade.tsv"
  echo; echo "### Flips (in results.jsonl uebernommen)"; echo
  flips_md "$RUNDIR/regrade.tsv"
  skips_md "$RUNDIR/regrade.tsv"
  echo; echo "- Regrade-Log: $REGLOG"; echo "- Backup: $BAK"
  echo "- Marker: OK $stamp flips=$n_flips"
} >> "$REPORT"

# ------------------------------------------------------------------ 5 Marker
echo "OK $stamp flips=$n_flips lesart=$LESART" > "$MARKER"
FINISHED=1
log "FERTIG — Marker: $(cat "$MARKER")  Bericht: $REPORT"
