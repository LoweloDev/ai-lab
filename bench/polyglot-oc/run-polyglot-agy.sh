#!/usr/bin/env bash
# Polyglot-Benchmark (aider) via Antigravity-CLI (agy) headless im podman-Container.
# Abgeleitet aus run-polyglot-oc.sh (2026-08-24); gleiche 2-Versuche-Semantik:
#   Versuch 1: Instructions (.docs/introduction.md? + instructions.md +
#              instructions.append.md?) + aider-Addendum mit den Solution-Dateinamen.
#   Tests:     python -> pytest, go -> go test ./... (aider TEST_COMMANDS), 180s,
#              vor jedem Testlauf test/editor/invalidator-Dateien zurueckkopieren.
#   Versuch 2: nur wenn rot. agy hat im Print-Modus keine fortsetzbare Session wie
#              OpenCode (-s <id>); Versuch 2 bekommt darum den vollen Prompt + Tail
#              (~100 Zeilen) der Testausgabe + aider-test_failures-Prompt in einer
#              frischen Konversation (entspricht dem OC-Fallback ohne Session).
#   Score:     tests_outcomes = [v1_gruen, v2_gruen]
# agy je Versuch: frisches `agy -p ... --output-format json` im Exercise-Container;
# /home/bench/.gemini ist die pro Exercise EINMAL angelegte Kopie der Login-Vorlage
# (agy-lib.sh), beide Versuche teilen sie. Tokens aus dem JSON-Envelope (usage.*).
#
# Usage: [AGY_MODEL=gemini-3.7-flash] [AGY_EFFORT=low] run-polyglot-agy.sh <label> [--langs python,go] [--limit N]
# Ergebnisse: runs/<label>/<lang>/<name>/{ws,agy-home,attempt*.json,test*.out,result.json}
#             runs/<label>/run.log, runs/<label>/summary.json
# Bereits vorhandene result.json => Exercise wird uebersprungen (Resume nach Abbruch).
set -uo pipefail

usage() { echo "usage: $0 <label> [--langs python,go] [--limit N]   (Modell via env AGY_MODEL/AGY_EFFORT)" >&2; exit 2; }
[ $# -ge 1 ] || usage
LABEL="$1"; shift
LANGS="python,go"; LIMIT=0
while [ $# -gt 0 ]; do
  case "$1" in
    --langs) LANGS="$2"; shift 2 ;;
    --limit) LIMIT="$2"; shift 2 ;;
    *) usage ;;
  esac
done

BENCH="$HOME/ai-lab/bench"
# shellcheck source=../agy-lib.sh
source "$BENCH/agy-lib.sh"
SRC="$BENCH/aider/polyglot-benchmark"
RUN_DIR="$BENCH/polyglot-oc/runs/$LABEL"
LOG="$RUN_DIR/run.log"
T_ATTEMPT1=240 T_ATTEMPT2=300 T_TEST=180
PT_ATTEMPT1=3m30s PT_ATTEMPT2=4m30s      # agy --print-timeout, unter dem harten timeout
MODEL_ID="agy/${AGY_MODEL:-default}${AGY_EFFORT:+@${AGY_EFFORT}}"

agy_require_auth || exit 2
agy_model_args
mkdir -p "$RUN_DIR"

log() { printf '%s %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$LOG"; }

# aider prompts.py, wortgleich (wie run-polyglot-oc.sh)
ADDENDUM_TPL='
####

Use the above instructions to modify the supplied files: %s
Don'\''t change the names of existing functions or classes, as they may be referenced from other code like unit tests, etc.
Only use standard libraries, don'\''t suggest installing any packages.
'
TEST_FAILURES_TPL='
####

See the testing errors above.
The tests are correct, don'\''t try and change them.
Fix the code in %s to resolve the errors.
'

# agy-Aufruf im agent-bench-Container.
# $1 ws, $2 agy-home, $3 prompt, $4 timeout-s, $5 print-timeout, $6 transcript, $7 stderr-log
run_agent() {
  local ws="$1" home="$2" prompt="$3" tmo="$4" ptmo="$5" transcript="$6" errlog="$7"
  timeout --signal=SIGTERM --kill-after=30 "$tmo" \
    podman run --rm --name "$CNAME" --pull=never --userns=keep-id \
      --network=pasta:--map-host-loopback,169.254.1.2 \
      -v "$ws:/work:Z" \
      -v "$home:/home/bench/.gemini:Z" \
      -v "$HOME/.local/bin/agy:/usr/local/bin/agy:ro" \
      -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
      -v agy-cache:/home/bench/.cache:U \
      -v agy-data:/home/bench/.local:U \
      -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
      agent-bench \
      agy -p "$prompt" --output-format json --dangerously-skip-permissions \
        --print-timeout "$ptmo" ${AGY_ARGS[@]+"${AGY_ARGS[@]}"} \
      > "$transcript" 2> "$errlog"
}

# Testlauf im aider-bench-Container. $1 lang, $2 ws, $3 outfile; rc 0 = gruen
run_tests() {
  local lang="$1" ws="$2" out="$3" cmd rc
  case "$lang" in
    python) cmd='pytest' ;;                  # aider TEST_COMMANDS[".py"]
    go)     cmd='go test ./...' ;;           # aider TEST_COMMANDS[".go"]
    *) echo "unbekannte Sprache $lang" > "$out"; return 3 ;;
  esac
  timeout --signal=SIGTERM --kill-after=20 "$T_TEST" \
    podman run --rm --name "$CNAME-t" --pull=never --network=none \
      -v "$ws:/ex:Z" -w /ex \
      -e HOME=/tmp -e GOFLAGS="-buildvcs=false" -e GOPROXY=off \
      -e GOCACHE=/tmp/gocache -e GOPATH=/tmp/go \
      aider-bench sh -c "$cmd" > "$out" 2>&1
  rc=$?
  [ $rc -eq 124 ] && echo "Tests timed out!" >> "$out"
  # aider cleanup_test_output: Timing-Angaben entfernen
  sed -i -E 's/\bin [0-9]+\.[0-9]+s\b//g' "$out"
  return $rc
}

# test/editor/invalidator-Dateien unveraendert aus der Quelle zuruecksetzen
restore_test_files() {
  local exsrc="$1" ws="$2" f
  jq -r '.files | ((.test // []) + (.editor // []) + (.invalidator // []))[]' \
      "$exsrc/.meta/config.json" | while IFS= read -r f; do
    [ -f "$exsrc/$f" ] || continue
    mkdir -p "$ws/$(dirname "$f")"
    cp -f "$exsrc/$f" "$ws/$f"
  done
}

# Token-Summen aus den agy-JSON-Envelopes (usage.input_tokens usw.); Kosten = 0
# (Consumer-Abo, agy meldet keine Kosten).
token_stats() {
  jq -s '{ tokens_in:  ([.[] | (.usage.input_tokens // 0) + (.usage.cache_read_tokens // 0)] | add // 0),
           tokens_out: ([.[] | (.usage.output_tokens // 0) + (.usage.thinking_tokens // 0)] | add // 0),
           cost: 0 }' "$@" 2>/dev/null \
    || echo '{"tokens_in":0,"tokens_out":0,"cost":0}'
}

write_result() { # $1 exdir; restliche Argumente an jq -n
  local exdir="$1"; shift
  jq -n "$@" '{name:$name, lang:$lang, tests_outcomes:$outcomes, seconds:$seconds,
               attempts:$attempts, agent_rcs:$rcs, error:$err}
              + ($tok | fromjson? // {tokens_in:0,tokens_out:0,cost:0})' \
    > "$exdir/result.json"
}

run_exercise() { # $1 lang, $2 name; schreibt result.json, gibt nie != 0 zurueck
  local lang="$1" name="$2"
  local exsrc="$SRC/$lang/exercises/practice/$name"
  local exdir="$RUN_DIR/$lang/$name" ws="$RUN_DIR/$lang/$name/ws"
  CNAME="pagy-$$-${name//[^a-zA-Z0-9_.-]/-}"
  local start end seconds outcomes attempts rcs tok err="null"

  rm -rf "$exdir"; mkdir -p "$exdir"
  start=$(date +%s.%N)

  if [ ! -f "$exsrc/.meta/config.json" ]; then
    write_result "$exdir" --arg name "$name" --arg lang "$lang" --argjson outcomes '[]' \
      --argjson seconds 0 --argjson attempts 0 --argjson rcs '[]' \
      --arg tok '' --arg err "config.json fehlt: $exsrc"; return 0
  fi
  if ! cp -a "$exsrc" "$ws" || ! rm -rf "$ws/.meta" "$ws/.approaches"; then
    write_result "$exdir" --arg name "$name" --arg lang "$lang" --argjson outcomes '[]' \
      --argjson seconds 0 --argjson attempts 0 --argjson rcs '[]' \
      --arg tok '' --arg err "Workspace-Kopie fehlgeschlagen"; return 0
  fi
  if ! agy_prepare_home "$exdir/agy-home"; then
    write_result "$exdir" --arg name "$name" --arg lang "$lang" --argjson outcomes '[]' \
      --argjson seconds 0 --argjson attempts 0 --argjson rcs '[]' \
      --arg tok '' --arg err "agy-home Kopie fehlgeschlagen (Login-Vorlage?)"; return 0
  fi

  # Instructions + file_list wie aider (Basenames der solution-Dateien)
  local file_list instructions prompt
  file_list=$(jq -r '[.files.solution[] | split("/")[-1]] | join(" ")' "$exsrc/.meta/config.json")
  instructions=""
  [ -f "$exsrc/.docs/introduction.md" ] && instructions+=$(cat "$exsrc/.docs/introduction.md")$'\n'
  instructions+=$(cat "$exsrc/.docs/instructions.md")$'\n'
  [ -f "$exsrc/.docs/instructions.append.md" ] && instructions+=$(cat "$exsrc/.docs/instructions.append.md")$'\n'
  # shellcheck disable=SC2059
  prompt="$instructions$(printf "$ADDENDUM_TPL" "$file_list")"

  # --- Versuch 1 ---
  run_agent "$ws" "$exdir/agy-home" "$prompt" "$T_ATTEMPT1" "$PT_ATTEMPT1" \
    "$exdir/attempt1.json" "$exdir/attempt1.stderr.log"
  rcs="[$?"; attempts=1
  restore_test_files "$exsrc" "$ws"
  if run_tests "$lang" "$ws" "$exdir/test1.out"; then
    outcomes='[true]'
  else
    outcomes='[false,false]'
    # --- Versuch 2: voller Prompt + Test-Tail + aider-test_failures, frische Konversation ---
    local errtail prompt2
    errtail=$(tail -n 100 "$exdir/test1.out")
    # shellcheck disable=SC2059
    prompt2="$prompt

$errtail$(printf "$TEST_FAILURES_TPL" "$file_list")"
    run_agent "$ws" "$exdir/agy-home" "$prompt2" "$T_ATTEMPT2" "$PT_ATTEMPT2" \
      "$exdir/attempt2.json" "$exdir/attempt2.stderr.log"
    rcs="$rcs,$?"; attempts=2
    restore_test_files "$exsrc" "$ws"
    run_tests "$lang" "$ws" "$exdir/test2.out" && outcomes='[false,true]'
  fi
  rcs="$rcs]"

  end=$(date +%s.%N)
  seconds=$(awk -v a="$start" -v b="$end" 'BEGIN{printf "%.1f", b-a}')
  tok=$(token_stats "$exdir"/attempt*.json)
  write_result "$exdir" --arg name "$name" --arg lang "$lang" --argjson outcomes "$outcomes" \
    --argjson seconds "$seconds" --argjson attempts "$attempts" --argjson rcs "$rcs" \
    --arg tok "$tok" --argjson err "$err"
  return 0
}

# ---- Exercise-Liste (alphabetisch je Sprache, wie ls) ----
LIST=()
for lang in ${LANGS//,/ }; do
  d="$SRC/$lang/exercises/practice"
  [ -d "$d" ] || { echo "unbekannte Sprache: $lang" >&2; exit 2; }
  while IFS= read -r n; do LIST+=("$lang/$n"); done < <(ls -1 "$d")
done
[ "$LIMIT" -gt 0 ] && LIST=("${LIST[@]:0:$LIMIT}")
N=${#LIST[@]}

RUN_START=$(date +%s)
log "== Start $LABEL: model=$MODEL_ID langs=$LANGS n=$N auth=$AGY_AUTH_SRC"

i=0
for entry in "${LIST[@]}"; do
  i=$((i+1)); lang="${entry%%/*}"; name="${entry#*/}"
  exdir="$RUN_DIR/$lang/$name"
  if [ -f "$exdir/result.json" ] && jq -e . "$exdir/result.json" >/dev/null 2>&1; then
    log "[$i/$N] $entry uebersprungen (result.json vorhanden)"
    continue
  fi
  run_exercise "$lang" "$name"
  log "[$i/$N] $entry $(jq -r '"outcomes=\(.tests_outcomes) \(.seconds)s tok=\(.tokens_in)/\(.tokens_out) rcs=\(.agent_rcs)"
        + (if .error then " FEHLER: \(.error)" else "" end)' "$exdir/result.json")"
done

# ---- Summary ueber alle vorhandenen result.json dieses Labels ----
RUN_WALL=$(( $(date +%s) - RUN_START ))
find "$RUN_DIR" -mindepth 3 -maxdepth 3 -name result.json -print0 | xargs -0 -r jq -s \
  --arg label "$LABEL" --arg model "$MODEL_ID" --arg date "$(date -Is)" --argjson wall "$RUN_WALL" '
  def agg: {n: length,
    pass1: ([.[] | select(.tests_outcomes[0] == true)] | length),
    pass2: ([.[] | select(any(.tests_outcomes[]?; . == true))] | length)}
    | . + {pass1_rate: (if .n>0 then (.pass1/.n*1000|round/1000) else null end),
           pass2_rate: (if .n>0 then (.pass2/.n*1000|round/1000) else null end)};
  {label: $label, model: $model, generated: $date,
   overall: agg,
   per_lang: (group_by(.lang) | map({key: .[0].lang, value: (. | agg)}) | from_entries),
   errors: [.[] | select(.error) | "\(.lang)/\(.name): \(.error)"],
   totals: {wall_seconds_sum: ([.[].seconds] | add // 0 | (.*10|round/10)),
            run_wall_seconds: $wall,
            tokens_in: ([.[].tokens_in] | add // 0),
            tokens_out: ([.[].tokens_out] | add // 0),
            cost: ([.[].cost] | add // 0)}}' \
  > "$RUN_DIR/summary.json"
log "== Fertig ($RUN_WALL s). Summary: $RUN_DIR/summary.json"
jq . "$RUN_DIR/summary.json"
