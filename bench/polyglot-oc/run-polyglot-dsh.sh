#!/usr/bin/env bash
# Polyglot-Benchmark (aider) via DeepSeek Harness (dsh) headless im podman-Container.
# Abgeleitet aus run-polyglot-agy.sh (2026-08-25); Container-Invokation aus run-task-dsh.sh
# (Image agent-bench-dsh, dsh 0.1.1-rc.2, Profil headless). Gleiche 2-Versuche-Semantik:
#   Versuch 1: Instructions (.docs/introduction.md? + instructions.md +
#              instructions.append.md?) + aider-Addendum mit den Solution-Dateinamen.
#   Tests:     python -> pytest, go -> go test ./... (aider TEST_COMMANDS), 180s,
#              vor jedem Testlauf test/editor/invalidator-Dateien zurueckkopieren.
#   Versuch 2: nur wenn rot. dsh headless hat KEINE Session-Fortsetzung (`--resume` kennt
#              nur das tui-Profil; headless: "error: unknown option '--resume'", geprueft
#              25.08. gegen das Image). Darum agy-Muster: Versuch 2 bekommt den vollen
#              Original-Prompt + "Test output:"-Kopfzeile + Tail (~100 Zeilen) der Testausgabe
#              + aider-test_failures-Prompt in einer frischen Session.
#   Score:     tests_outcomes = [v1_gruen, v2_gruen]
# dsh je Versuch: `dsh --profile headless --patch model-patch.yml "<prompt>"`; stdout ist
# NUR die finale Assistant-Nachricht (attempt*.txt), kein JSON-Envelope. Jede Uebung hat ihr
# eigenes dsh-home (-> /home/bench/.dsh), beide Versuche teilen es; die Session-Logs liegen
# darin unter sessions/<cwd-slug>/session-<id>/session.jsonl.zstd (zstd-komprimiert!).
#
# Token-Bilanz (token_stats): aus allen session.jsonl.zstd der Uebung, Events
# type=="assistant/message" -> .data.usage {inputTokens, outputTokens, cacheReadTokens,
# reasoningTokens}. Mapping laut dsh-llm-deepseek/lib/index.js: inputTokens = prompt_tokens
# MINUS Cache-Treffer (disjunkt zu cacheReadTokens); outputTokens = completion_tokens und
# ENTHAELT reasoning_tokens bereits (reasoningTokens ist nur die Teilmenge). Darum:
#   tokens_in  = sum(inputTokens + cacheReadTokens + cacheWriteTokens)
#   tokens_out = sum(outputTokens)            (Reasoning NICHT nochmal addieren)
# Das entspricht der OC-Bilanz (input+cache / output+reasoning).
# Kosten: dsh liefert keine; geschaetzt aus den Tokens mit den models.dev-Preisen (USD je
# 1M), die auch OpenCode in oc-dsflash verrechnet hat (opencode-cache/models.json; das
# API-RUNBOOK nennt nur Pauschalen): v4-flash input 0.14 / cache_read 0.0028 / output 0.28
# (reasoning = output-Preis), v4-pro 0.435 / 0.003625 / 0.87. Andere Modelle: 0, ausser per
# Env DSH_PRICE_IN / DSH_PRICE_CACHE / DSH_PRICE_OUT gesetzt. Off-Peak-Rabatt ist NICHT
# eingerechnet (Listenpreis).
# Prompt-Argument: dsh (commander) liest ein Argument, das mit "-" beginnt, als Option — auch
# hinter "--" (geprueft). Alle Exercise-Prompts beginnen mit "#", Prompt 2 mit dem Original-
# Prompt; run_agent stellt zur Sicherheit "Task:" voran, falls doch ein "-" vorn steht.
#
# Usage: [DSH_MODEL=deepseek-v4-flash] run-polyglot-dsh.sh <label> [--langs python,go] [--limit N]
#   z.B.: source ~/ai-lab/.env && ./run-polyglot-dsh.sh dsh-flash-polyglot
# Env:   DEEPSEEK_API_KEY (Pflicht; wird aus ~/ai-lab/.env nachgeladen, falls nicht exportiert),
#        DSH_MODEL (Default deepseek-v4-flash),
#        T_ATTEMPT1 (240), T_ATTEMPT2 (300), T_TEST (180), DSH_PRICE_* (s.o.)
# Ergebnisse: runs/<label>/<lang>/<name>/{ws,dsh-home,attempt*.txt,attempt*.stderr.log,test*.out,result.json}
#             runs/<label>/run.log, runs/<label>/summary.json, runs/<label>/model-patch.yml
# Bereits vorhandene result.json => Exercise wird uebersprungen (Resume nach Abbruch).
# Abweichung von aider (wie gehabt): .meta/ und .approaches/ werden NICHT ins Workdir
# kopiert (enthalten die Musterloesung).
set -uo pipefail

usage() { echo "usage: $0 <label> [--langs python,go] [--limit N]   (Modell via env DSH_MODEL)" >&2; exit 2; }
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
SRC="$BENCH/aider/polyglot-benchmark"
RUN_DIR="$BENCH/polyglot-oc/runs/$LABEL"
LOG="$RUN_DIR/run.log"
T_ATTEMPT1="${T_ATTEMPT1:-240}" T_ATTEMPT2="${T_ATTEMPT2:-300}" T_TEST="${T_TEST:-180}"
DSH_MODEL="${DSH_MODEL:-deepseek-v4-flash}"
MODEL_ID="dsh/$DSH_MODEL"
DSH_IMAGE="agent-bench-dsh"

# ~/ai-lab/.env hat kein `export` — ein plain `source` in der Shell setzt den Key, reicht ihn
# aber nicht an dieses Skript weiter (Smoke 25.08.). Darum hier selbst mit allexport laden.
if [ -z "${DEEPSEEK_API_KEY:-}" ] && [ -f "$HOME/ai-lab/.env" ]; then
  set -a; . "$HOME/ai-lab/.env"; set +a
fi
[ -n "${DEEPSEEK_API_KEY:-}" ] || { echo "DEEPSEEK_API_KEY nicht gesetzt (fehlt in ~/ai-lab/.env?)" >&2; exit 2; }
command -v zstd >/dev/null 2>&1 || { echo "zstd fehlt (dsh-Session-Logs sind zstd-komprimiert)" >&2; exit 2; }
podman image exists "$DSH_IMAGE" || { echo "Image fehlt: $DSH_IMAGE" >&2; exit 2; }
mkdir -p "$RUN_DIR"

# Preise USD je 1M Tokens (siehe Kopfkommentar)
case "$DSH_MODEL" in
  deepseek-v4-flash*) P_IN=0.14  P_CACHE=0.0028   P_OUT=0.28 ;;
  deepseek-v4-pro*)   P_IN=0.435 P_CACHE=0.003625 P_OUT=0.87 ;;
  *)                  P_IN=0     P_CACHE=0        P_OUT=0 ;;
esac
P_IN="${DSH_PRICE_IN:-$P_IN}" P_CACHE="${DSH_PRICE_CACHE:-$P_CACHE}" P_OUT="${DSH_PRICE_OUT:-$P_OUT}"

# Modell-Override als Patch-Overlay (wie run-task-dsh.sh), einmal je Lauf; immer geschrieben,
# damit das Modell im Lauf explizit ist (Default des headless-Profils ist ohnehin v4-flash).
printf -- '- id: agent-default-model\n  config:\n    provider: deepseek-official\n    model: %s\n' \
  "$DSH_MODEL" > "$RUN_DIR/model-patch.yml"

DSH_VER=$(podman run --rm --pull=never "$DSH_IMAGE" dsh -V 2>/dev/null | tr -d '\r' | head -n1)

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

# dsh-Aufruf im agent-bench-dsh-Container (Muster aus run-task-dsh.sh).
# $1 ws, $2 dsh-home, $3 prompt, $4 timeout-s, $5 transcript (finale Nachricht), $6 stderr-log
run_agent() {
  local ws="$1" home="$2" prompt="$3" tmo="$4" transcript="$5" errlog="$6"
  # Ein Prompt, der mit "-" beginnt, wird von dsh als Option gelesen (auch hinter "--").
  case "$prompt" in -*) prompt="Task:"$'\n'"$prompt" ;; esac
  podman rm -f "$CNAME" >/dev/null 2>&1 || true   # Leiche aus per Timeout gekilltem Vorversuch (rc 125 "name in use", dsflash/alphametics)
  timeout --signal=SIGTERM --kill-after=30 "$tmo" \
    podman run --rm --name "$CNAME" --pull=never --userns=keep-id \
      --network=pasta:--map-host-loopback,169.254.1.2 \
      -v "$ws:/work:Z" \
      -v "$home:/home/bench/.dsh:Z" \
      -v "$RUN_DIR/model-patch.yml:/home/bench/model-patch.yml:ro,Z" \
      -v "$HOME/go/pkg/mod:/home/bench/go/pkg/mod:ro,Z" \
      -v dsh-cache:/home/bench/.cache:U \
      -v dsh-data:/home/bench/.local:U \
      -e GOFLAGS="-mod=mod -buildvcs=false" -e GOPROXY=off \
      -e DEEPSEEK_API_KEY \
      "$DSH_IMAGE" \
      dsh --profile headless --patch /home/bench/model-patch.yml "$prompt" \
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

# Token/Kosten-Summen aus den dsh-Session-Logs der Uebung. $1 dsh-home
# (alle session.jsonl.zstd darunter, also beide Versuche). Abgebrochene/unvollstaendige
# zstd-Streams (Timeout-Kill) liefern die bis dahin vollstaendigen Zeilen; kaputte Zeilen
# werden per fromjson? uebersprungen.
token_stats() {
  local home="$1" f
  { for f in "$home"/sessions/*/*/session.jsonl.zstd; do
      [ -f "$f" ] && zstd -dcq "$f" 2>/dev/null
    done; } \
  | jq -R -s --argjson pin "$P_IN" --argjson pcache "$P_CACHE" --argjson pout "$P_OUT" '
      [ split("\n")[] | fromjson? | select(.type == "assistant/message") | .data.usage // empty ]
      | { i: ([.[] | .inputTokens // 0] | add // 0),
          c: ([.[] | .cacheReadTokens // 0] | add // 0),
          w: ([.[] | .cacheWriteTokens // 0] | add // 0),
          o: ([.[] | .outputTokens // 0] | add // 0) }
      | { tokens_in: (.i + .c + .w), tokens_out: .o,
          cost: ((.i * $pin + (.c + .w) * $pcache + .o * $pout) / 1000000) }' 2>/dev/null \
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
  CNAME="pdsh-$$-${name//[^a-zA-Z0-9_.-]/-}"
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
  if ! mkdir -p "$exdir/dsh-home"; then
    write_result "$exdir" --arg name "$name" --arg lang "$lang" --argjson outcomes '[]' \
      --argjson seconds 0 --argjson attempts 0 --argjson rcs '[]' \
      --arg tok '' --arg err "dsh-home anlegen fehlgeschlagen"; return 0
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
  run_agent "$ws" "$exdir/dsh-home" "$prompt" "$T_ATTEMPT1" \
    "$exdir/attempt1.txt" "$exdir/attempt1.stderr.log"
  rcs="[$?"; attempts=1
  restore_test_files "$exsrc" "$ws"
  if run_tests "$lang" "$ws" "$exdir/test1.out"; then
    outcomes='[true]'
  else
    outcomes='[false,false]'
    # --- Versuch 2: voller Prompt + Test-Tail + aider-test_failures, frische Session ---
    local errtail prompt2
    errtail=$(tail -n 100 "$exdir/test1.out")
    # shellcheck disable=SC2059
    # Kopfzeile "Test output:" vor dem Tail: Go-Testausgaben beginnen mit "--- FAIL", und ein
    # Prompt-Argument, das mit "-" anfaengt, liest jede CLI als Option (s. run-polyglot-claude.sh).
    prompt2="$prompt

Test output:
$errtail$(printf "$TEST_FAILURES_TPL" "$file_list")"
    run_agent "$ws" "$exdir/dsh-home" "$prompt2" "$T_ATTEMPT2" \
      "$exdir/attempt2.txt" "$exdir/attempt2.stderr.log"
    rcs="$rcs,$?"; attempts=2
    restore_test_files "$exsrc" "$ws"
    run_tests "$lang" "$ws" "$exdir/test2.out" && outcomes='[false,true]'
  fi
  rcs="$rcs]"

  end=$(date +%s.%N)
  seconds=$(awk -v a="$start" -v b="$end" 'BEGIN{printf "%.1f", b-a}')
  tok=$(token_stats "$exdir/dsh-home")
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
log "== Start $LABEL: model=$MODEL_ID langs=$LANGS n=$N dsh=${DSH_VER:-?} profile=headless prices=$P_IN/$P_CACHE/$P_OUT USD/M"

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
