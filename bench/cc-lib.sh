# Gemeinsame Helfer fuer die Claude-Code-(cc)-Bench-Skripte. Wird ge-sourced.
# claude 2.1.241 (Host-Binary, ro nach /usr/local/bin/claude gemountet),
# Login via Max-Subscription-OAuth aus ~/.claude/.credentials.json.
#
# Der Container bekommt pro Lauf ein MINIMALES, frisch gebautes Config-Verzeichnis
# (cc-home), gemountet rw nach /home/bench/.claude. Das echte ~/.claude wird NIE
# gemountet — es enthaelt persoenliche Memory, History, Hooks und Plugins.
# Inhalt des cc-home:
#   .credentials.json — Kopie der Host-OAuth-Credentials. Ein Token-Refresh im
#                       Container schreibt in die Kopie; der Host bleibt kanonisch.
#   settings.json     — minimal, von uns erzeugt (kein Autoupdate, keine Telemetrie).
#   .claude.json      — Onboarding-/Bypass-Flags vorab gesetzt. Liegt IM cc-home,
#                       weil CLAUDE_CONFIG_DIR=/home/bench/.claude gesetzt wird —
#                       sonst schriebe claude nach /home/bench/.claude.json
#                       (ausserhalb des Mounts) und die Flags griffen nicht.
#
# Nicht-interaktive Stolpersteine (validiert 2026-08-24, claude 2.1.241):
#   - bypassPermissionsModeAccepted=true noetig: ohne das Flag stuft der Print-Modus
#     --dangerously-skip-permissions auf den Default-Modus herab ("bypass requires
#     accepting the disclaimer interactively first") und Tool-Aufrufe scheitern.
#   - hasCompletedOnboarding=true + Projekt-Trust fuer /work unterdruecken alle
#     restlichen Erststart-Dialoge.
#   - Effort: CLAUDE_CODE_EFFORT_LEVEL (low|medium|high|xhigh|max) wird per
#     -e durchgereicht; Runner-Default CC_EFFORT=xhigh (cc_effort_env).

CC_BIN="$(readlink -f "$HOME/.local/bin/claude")"   # Symlink -> versionierte Binary

# Token-Lebensdauer-Falle (Vorfall 24.08. ~23:00, 101 Polyglot-Uebungen vergiftet):
# Ein OAuth-Refresh IM Container landet nur in der cc-home-Kopie; der Host behaelt den
# alten Token, jeder folgende Lauf startet mit einem abgelaufenen Token und der Print-
# Modus scheitert in 2 s mit "Login expired" -> als FAIL gezaehlt. Darum:
#   cc_token_ok      — prueft expiresAt am Host (mit 10-min-Puffer) -> rc 0/1
#   cc_sync_back     — Container-Refresh zurueck auf den Host, wenn dort ein neuerer
#                      Token liegt (nur claudeAiOauth-Block, atomar, mode 600)
#   cc_prepare_home  — ruft cc_token_ok; ist der Token abgelaufen, wird 1x versucht,
#                      aus der zuletzt gesyncten Kopie zu heilen, sonst rc 3 (Auth) —
#                      die Runner brechen dann ab statt Muell zu produzieren.
cc_token_ok() {
  python3 - "$HOME/.claude/.credentials.json" <<'EOF'
import json,sys,time
try:
    o=json.load(open(sys.argv[1])).get('claudeAiOauth',{})
    exp=o.get('expiresAt') or 0
    sys.exit(0 if exp/1000 > time.time()+600 else 1)
except Exception:
    sys.exit(1)
EOF
}

cc_sync_back() {  # $1 = cc-home, dessen .credentials.json evtl. neuer ist als der Host
  local src="$1/.credentials.json" host="$HOME/.claude/.credentials.json"
  [ -f "$src" ] || return 0
  python3 - "$src" "$host" <<'EOF'
import json,sys,os,tempfile
src,host=sys.argv[1],sys.argv[2]
try:
    s=json.load(open(src)); h=json.load(open(host))
except Exception:
    sys.exit(0)
so=s.get('claudeAiOauth') or {}; ho=h.get('claudeAiOauth') or {}
if (so.get('expiresAt') or 0) <= (ho.get('expiresAt') or 0): sys.exit(0)
h['claudeAiOauth']=so
fd,tmp=tempfile.mkstemp(dir=os.path.dirname(host)); os.write(fd,json.dumps(h).encode()); os.close(fd)
os.chmod(tmp,0o600); os.replace(tmp,host)
print("cc-lib: OAuth-Token vom Container auf den Host zurueckgesynct", file=sys.stderr)
EOF
}

cc_prepare_home() {  # $1 = Zielverzeichnis; wird geloescht und frisch befuellt
  local dst="$1"
  [ -f "$HOME/.claude/.credentials.json" ] || { echo "~/.claude/.credentials.json fehlt" >&2; return 1; }
  if ! cc_token_ok; then
    echo "cc-lib: Host-OAuth-Token abgelaufen/kurz vor Ablauf — Lauf wird NICHT gestartet (rc 3). Bitte 'claude' interaktiv oeffnen oder /login." >&2
    return 3
  fi
  rm -rf "$dst"; mkdir -p "$dst"
  install -m 600 "$HOME/.claude/.credentials.json" "$dst/.credentials.json" || return 1
  cat > "$dst/settings.json" <<'EOF'
{
  "includeCoAuthoredBy": false,
  "env": {
    "DISABLE_AUTOUPDATER": "1",
    "DISABLE_ERROR_REPORTING": "1",
    "DISABLE_TELEMETRY": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
EOF
  cat > "$dst/.claude.json" <<'EOF'
{
  "hasCompletedOnboarding": true,
  "bypassPermissionsModeAccepted": true,
  "projects": {
    "/work": {
      "hasTrustDialogAccepted": true,
      "hasCompletedProjectOnboarding": true
    }
  }
}
EOF
}

# Effort-Default des Runners: CC_EFFORT (Default xhigh) -> CLAUDE_CODE_EFFORT_LEVEL,
# sofern der Aufrufer CLAUDE_CODE_EFFORT_LEVEL nicht schon selbst exportiert hat.
cc_effort_env() {
  export CLAUDE_CODE_EFFORT_LEVEL="${CLAUDE_CODE_EFFORT_LEVEL:-${CC_EFFORT:-xhigh}}"
}
