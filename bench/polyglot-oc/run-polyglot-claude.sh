#!/usr/bin/env bash
# Polyglot-Benchmark (aider) via Claude Code headless im podman-Sandbox-Container.
# Adaption von run-polyglot-oc.sh: OpenCode-Aufruf ersetzt durch die claude-Container-
# Invokation aus run-task-claude.sh/cc-lib.sh (Host-Binary 2.1.241, Max-Subscription-
# OAuth, minimales frisches cc-home pro Exercise). Semantik unveraendert:
#   Versuch 1: Instructions (.docs/introduction.md? + instructions.md + instructions.append.md?)
#              + aider-Addendum mit den Solution-Dateinamen.
#   Tests:     python -> `pytest`, go -> `go test ./...`, cwd = Exercise-Dir, Timeout 180s.
#              Vor jedem Testlauf werden test/editor/invalidator-Dateien zurueckkopiert.
#   Versuch 2: nur wenn rot. Tail (~100 Zeilen) der Testausgabe + aider-test_failures-Prompt,
#              gleiche Claude-Session wird fortgesetzt (--resume <session_id>), Tests erneut.
#   Score:     tests_outcomes = [versuch1_gruen, versuch2_gruen]
# Unterschiede zu run-polyglot-oc.sh:
#   - claude -p ... --output-format json: stdout ist EIN JSON-Envelope pro Versuch
#     (attempt1.json/attempt2.json statt *.jsonl); Token/Kosten aus .usage/.total_cost_usd
#     (tokens_in = input + cache_read + cache_creation; tokens_out = output_tokens).
#   - Session-Fortsetzung: .session_id aus dem Envelope -> --resume; dafuer teilt sich
#     das Exercise EIN cc-home ueber beide Versuche (Session-Log liegt darin).
#   - Kein API-Key noetig; Auth aus der cc-home-Kopie von ~/.claude/.credentials.json.
#   - Effort: CC_EFFORT (Default xhigh) -> -e CLAUDE_CODE_EFFORT_LEVEL.
#   - Timeouts per Env ueberschreibbar: T_ATTEMPT1 (240), T_ATTEMPT2 (300), T_TEST (180).
# Abweichung von aider (wie gehabt): .meta/ und .approaches/ werden NICHT ins Workdir
# kopiert (enthalten die Musterloesung).
#
# Usage: run-polyglot-claude.sh <model-id> <label> [--langs python,go] [--limit N]
#   z.B.: ./run-polyglot-claude.sh claude-opus-4-8 cc-opus48-polyglot --limit 5
# Ergebnisse: runs/<label>/<lang>/<name>/{ws,cc-home,attempt*.json,test*.out,result.json}
#             runs/<label>/run.log, runs/<label>/summary.json
# Bereits vorhandene result.json => Exercise wird uebersprungen (Resume nach Abbruch).
set -uo pipefail

usage() { echo "usage: $0 <model-id> <label> [--langs python,go] [--limit N]" >&2; exit 2; }
[ $# -ge 2 ] || usage
MODEL_ID="$1"; LABEL="$2"; shift 2
LANGS="python,go"; LIMIT=0
while [ $# -gt 0 ]; do
  case "$1" in
    --langs) LANGS="$2"; shift 2 ;;
    --limit) LIMIT="$2"; shift 2 ;;
    *) usage ;;
  esac
done

BENCH="$HOME/ai-lab/bench"
# shellcheck source=../cc-lib.sh
source "$BENCH/cc-lib.sh"
cc_effort_env
SRC="$BENCH/aider/polyglot-benchmark"
RUN_DIR="$BENCH/polyglot-oc/runs/$LABEL"
LOG="$RUN_DIR/run.log"
T_ATTEMPT1="${T_ATTEMPT1:-240}" T_ATTEMPT2="${T_ATTEMPT2:-300}" T_TEST="${T_TEST:-180}"

[ -f "$HOME/.claude/.credentials.json" ] || { echo "~/.claude/.credentials.json fehlt" >&2; exit 2; }
mkdir -p "$RUN_DIR"

log() { printf '%s %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$LOG"; }

# aider prompts.py, wortgleich
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

# Claude-Code-Aufruf im agent-bench-Container (Muster aus run-task-claude.sh).
# $1 ws, $2 cc-home, $3 prompt, $4 timeout, $5 transcript(json), $6 stderr-log,
# $7 session-id ("" = neue Session)
run_agent() {
  local ws="$1" cchome="$2" prompt="$3" tmo="$4" transcript="$5" errlog="$6" sid="$7"
  local args=(-p --output-format json --dangerously-skip-permissions --model "$MODEL_ID")
  [ -n "$sid" ] && args+=(--resume "$sid")
  podman rm -f "$CNAME" >/dev/null 2>&1 || true   # Leiche aus per Timeout gekilltem Vorversuch (rc 125 "name in use", dsflash/alphametics)
  timeout --signal=SIGTERM --kill-after=30 "$tmo" \
    podman run --rm --name "$CNAME" --pull=never --userns=keep-id \
      --network=pasta:--map-host-loopback,169.254.1.2 \
      -v "$ws:/work:Z" \
      -v "$cchome:/home/bench/.claude:Z" \
      -v "$CC_BIN:/usr/local/bin/claude:ro" \
      -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
      -v cc-cache:/home/bench/.cache:U \
      -v cc-data:/home/bench/.local:U \
      -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
      -e CLAUDE_CONFIG_DIR=/home/bench/.claude \
      -e CLAUDE_CODE_EFFORT_LEVEL \
      agent-bench claude "${args[@]}" "$prompt" \
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

# Token/Kosten-Summen aus den claude-JSON-Envelopes (ein Envelope je Versuch)
token_stats() {
  jq -s '{ tokens_in:  ([.[] | (.usage.input_tokens // 0) + (.usage.cache_read_input_tokens // 0)
                              + (.usage.cache_creation_input_tokens // 0)] | add // 0),
           tokens_out: ([.[] | (.usage.output_tokens // 0)] | add // 0),
           cost:       ([.[] | (.total_cost_usd // 0)] | add // 0) }' "$@" 2>/dev/null \
    || echo '{"tokens_in":0,"tokens_out":0,"cost":0}'
}

write_result() { # $1 exdir; restliche Argumente an jq -n
  local exdir="$1"; shift
  jq -n "$@" '{name:$name, lang:$lang, tests_outcomes:$outcomes, seconds:$seconds,
               attempts:$attempts, agent_rcs:$rcs, error:$err}
              + ($tok | fromjson? // {tokens_in:0,tokens_out:0,cost:0})' \
    > "$exdir/result.json"
}

run_exercise() { # $1 lang, $2 name; schreibt result.json; rc 3 = Auth/Infra-Abbruch (kein result.json)
  local lang="$1" name="$2"
  local exsrc="$SRC/$lang/exercises/practice/$name"
  local exdir="$RUN_DIR/$lang/$name" ws="$RUN_DIR/$lang/$name/ws"
  CNAME="pcc-$$-${name//[^a-zA-Z0-9_.-]/-}"
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
  cc_prepare_home "$exdir/cc-home"; local prc=$?
  if [ "$prc" -eq 3 ]; then
    # Auth-Ausfall: KEIN result.json schreiben (waere ein vergifteter FAIL); Lauf abbrechen.
    log "!! AUTH: Host-OAuth-Token abgelaufen — Lauf gestoppt bei $lang/$name. Nach /login einfach neu starten (Resume)."
    return 3
  elif [ "$prc" -ne 0 ]; then
    write_result "$exdir" --arg name "$name" --arg lang "$lang" --argjson outcomes '[]' \
      --argjson seconds 0 --argjson attempts 0 --argjson rcs '[]' \
      --arg tok '' --arg err "cc-home Bootstrap fehlgeschlagen"; return 0
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
  run_agent "$ws" "$exdir/cc-home" "$prompt" "$T_ATTEMPT1" "$exdir/attempt1.json" "$exdir/attempt1.stderr.log" ""
  rcs="[$?"; attempts=1
  restore_test_files "$exsrc" "$ws"
  if run_tests "$lang" "$ws" "$exdir/test1.out"; then
    outcomes='[true]'
  else
    outcomes='[false,false]'
    # --- Versuch 2: Session fortsetzen, Test-Tail + aider-test_failures ---
    local sid errtail prompt2
    sid=$(jq -r '.session_id // empty' "$exdir/attempt1.json" 2>/dev/null)
    errtail=$(tail -n 100 "$exdir/test1.out")
    # shellcheck disable=SC2059
    # Kopfzeile davor: Go-Testausgaben beginnen mit "--- FAIL", und ein Prompt-Argument, das mit
    # "-" anfaengt, liest jede CLI als Option ("unknown option '--- FAIL...'", Opus-4.8/trinary 25.08.).
    prompt2="Test output:
$errtail$(printf "$TEST_FAILURES_TPL" "$file_list")"
    # ohne fortsetzbare Session (Crash/Timeout in V1) fehlt der Kontext -> Aufgabe voranstellen
    [ -z "$sid" ] && prompt2="$prompt

$prompt2"
    run_agent "$ws" "$exdir/cc-home" "$prompt2" "$T_ATTEMPT2" "$exdir/attempt2.json" "$exdir/attempt2.stderr.log" "$sid"
    rcs="$rcs,$?"; attempts=2
    restore_test_files "$exsrc" "$ws"
    run_tests "$lang" "$ws" "$exdir/test2.out" && outcomes='[false,true]'
  fi
  rcs="$rcs]"

  end=$(date +%s.%N)
  seconds=$(awk -v a="$start" -v b="$end" 'BEGIN{printf "%.1f", b-a}')
  tok=$(token_stats "$exdir"/attempt*.json)
  cc_sync_back "$exdir/cc-home"
  # Vergiftungs-Wächter (Auth-Signatur): 0 Tokens + rc 1 + unter 15 s => kein Modellversuch,
  # sondern "Login expired" o.ae. Kein result.json (Resume holt die Uebung nach), Abbruch.
  # NICHT bei Timeout (rc 124/137): das ist ein legitimer, gescheiterter Modellversuch, dessen
  # Envelope beim Kill verloren geht (Fehlalarm 25.08. 00:04 bei go/ledger).
  if [ "$(jq -r '.tokens_in + .tokens_out' <<<"$tok" 2>/dev/null)" = "0" ] \
     && grep -qE '(^|,)1(,|$)' <<<"${rcs//[\[\]]/}" && ! grep -qE '(^|,)(0|124|137)(,|$)' <<<"${rcs//[\[\]]/}" \
     && awk -v s="$seconds" 'BEGIN{exit !(s<15)}'; then
    log "!! INFRA: $lang/$name lieferte 0 Tokens bei rc=$rcs (Auth/Infra?) — kein Ergebnis geschrieben, Lauf gestoppt."
    tail -3 "$exdir/attempt1.stderr.log" 2>/dev/null | sed 's/^/     /' >&2
    return 3
  fi
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
log "== Start $LABEL: model=$MODEL_ID langs=$LANGS n=$N effort=$CLAUDE_CODE_EFFORT_LEVEL claude=2.1.241"

i=0
for entry in "${LIST[@]}"; do
  i=$((i+1)); lang="${entry%%/*}"; name="${entry#*/}"
  exdir="$RUN_DIR/$lang/$name"
  if [ -f "$exdir/result.json" ] && jq -e . "$exdir/result.json" >/dev/null 2>&1; then
    log "[$i/$N] $entry uebersprungen (result.json vorhanden)"
    continue
  fi
  run_exercise "$lang" "$name"; xrc=$?
  if [ "$xrc" -eq 3 ]; then
    log "== ABBRUCH (Auth/Infra) bei [$i/$N] $entry — bisherige Ergebnisse bleiben, Neustart setzt hier fort."
    ABORTED=1; break
  fi
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
