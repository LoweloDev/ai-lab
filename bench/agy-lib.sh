# Gemeinsame Helfer fuer die Antigravity-CLI-(agy)-Bench-Skripte. Wird ge-sourced.
# agy 1.1.19, Login via OAuth (Google-Konto lowelodev@gmail.com).
#
# ---- Auth-Modell (empirisch ermittelt 2026-08-24, strace + agy-Logs) ----
# agy nimmt sein Token NICHT aus ~/.gemini/oauth_creds.json (die Datei ist ein
# gemini-cli-Kompat-Export mit nur cloud-platform/openid/email/profile-Scopes).
# Das CodeAssist-Backend verlangt zusaetzlich die Scopes aicode, cclog und
# experimentsandconfigs; mit dem oauth_creds-Token authentifiziert agy zwar
# ("authenticated successfully"), aber der Eligibility-/Quota-Call gegen
# daily-cloudcode-pa.googleapis.com endet in 403 PERMISSION_DENIED
# ("Eligibility check failed", num_turns=0).
# In einer Umgebung OHNE D-Bus (Container) nutzt agy dateibasierte Token-Storage und
# liest/schreibt  antigravity-cli/antigravity-oauth-token  im Format
#   {"auth_method":"consumer","token":{access_token,token_type,refresh_token,expiry}}
# (vgl. upstream Issue #479; der Lese-Pfad funktioniert in 1.1.19).
#
# Die CREDENTIAL-VORLAGE stammt deshalb aus einem einmaligen INTERAKTIVEN agy-Login
# im Container: dabei ist ein frisches Home nach $AGY_AUTH_SRC (Default
# ~/ai-lab/bench/agy-auth-home) als /home/bench/.gemini gemountet, der Nutzer
# durchlaeuft den OAuth-Copy/Paste-Flow (env SSH_CONNECTION=1 erzwingt ihn), und agy
# legt das korrekt gescopte Token als Datei in der Vorlage ab.
# Jeder Bench-Lauf kopiert die Vorlage frisch (agy_prepare_home); agy darf in der
# Kopie loggen und refreshen, die Vorlage bleibt kanonisch. Ein Refresh in einer
# Kopie erreicht die Vorlage nicht — laeuft das Token der Vorlage irgendwann ab,
# den interaktiven Container-Login einfach wiederholen.
#
# Projekt-Konzept: agy verwaltet Projekte LOKAL. Beim ersten Start legt er ein
# "CLI Project" (id=default-cli-project) unter config/projects/ an und mappt das
# Workspace-Verzeichnis in projects.json ({"projects":{<pfad>:<name>}}). /work wird
# unten vorab eingetragen, damit im Headless-Print-Modus kein Trust-/Projektdialog
# blockiert; --new-project war damit nie noetig.

AGY_AUTH_SRC="${AGY_AUTH_SRC:-$HOME/ai-lab/bench/agy-auth-home}"

# Prueft, ob die Login-Vorlage ein Token enthaelt. rc 0 = ok, sonst Fehlermeldung.
agy_require_auth() {
  if [ ! -s "$AGY_AUTH_SRC/antigravity-cli/antigravity-oauth-token" ]; then
    {
      echo "FEHLER: Login-Vorlage fehlt: $AGY_AUTH_SRC/antigravity-cli/antigravity-oauth-token"
      echo "Einmal den interaktiven agy-Login im Container ausfuehren:"
      echo "  podman run --rm -it --pull=never --userns=keep-id \\"
      echo "    --network=pasta:--map-host-loopback,169.254.1.2 \\"
      echo "    -v \"$AGY_AUTH_SRC:/home/bench/.gemini:Z\" \\"
      echo "    -v \"\$HOME/.local/bin/agy:/usr/local/bin/agy:ro\" \\"
      echo "    -e SSH_CONNECTION=1 agent-bench agy"
      echo "  (Login-URL im Browser oeffnen, Code einfuegen, dann /exit)"
    } >&2
    return 3
  fi
}

# $1 = Zielverzeichnis; wird geloescht und mit einer frischen Kopie der Login-Vorlage
# befuellt. Die grossen/zustandsbehafteten Unterverzeichnisse von antigravity-cli
# (cache, conversations, brain, log, crashes) bleiben aussen vor und werden leer neu
# angelegt — agy legt dort selbst an, was er braucht (empirisch verifiziert; noetig
# sind nur antigravity-oauth-token, settings.json, installation_id, config/,
# projects.json, trustedFolders.json sowie cache/default_project_id.txt und
# cache/onboarding.json).
agy_prepare_home() {
  local dst="$1" d
  agy_require_auth || return 3
  rm -rf "$dst"; mkdir -p "$dst"
  cp -a "$AGY_AUTH_SRC/." "$dst/" || return 1
  for d in cache conversations brain log crashes; do
    rm -rf "$dst/antigravity-cli/$d"; mkdir -p "$dst/antigravity-cli/$d"
  done
  rm -rf "$dst/tmp"; mkdir -p "$dst/tmp"
  # Zwei kleine Cache-Dateien behalten: default_project_id.txt (CodeAssist-Projekt),
  # onboarding.json (Onboarding als erledigt markiert).
  for d in default_project_id.txt onboarding.json; do
    [ -f "$AGY_AUTH_SRC/antigravity-cli/cache/$d" ] && \
      cp -a "$AGY_AUTH_SRC/antigravity-cli/cache/$d" "$dst/antigravity-cli/cache/$d"
  done
  # /work als bekanntes, vertrautes Workspace eintragen (kein Trust-/Projektdialog).
  [ -f "$dst/projects.json" ] || echo '{"projects":{}}' > "$dst/projects.json"
  jq '.projects["/work"] = "work"' "$dst/projects.json" > "$dst/.pj" && mv "$dst/.pj" "$dst/projects.json"
  [ -f "$dst/trustedFolders.json" ] || echo '{}' > "$dst/trustedFolders.json"
  jq '. + {"/work": "TRUST_FOLDER"}' "$dst/trustedFolders.json" > "$dst/.tf" && mv "$dst/.tf" "$dst/trustedFolders.json"
  [ -f "$dst/antigravity-cli/settings.json" ] || echo '{}' > "$dst/antigravity-cli/settings.json"
  jq '.trustedWorkspaces = ((.trustedWorkspaces // []) + ["/work"] | unique)' \
    "$dst/antigravity-cli/settings.json" > "$dst/.st" && mv "$dst/.st" "$dst/antigravity-cli/settings.json"
  chmod 600 "$dst/antigravity-cli/antigravity-oauth-token" 2>/dev/null
  return 0
}

# Modellwahl aus der Umgebung: AGY_MODEL (z.B. gemini-3.7-flash) -> --model,
# AGY_EFFORT (low|medium|high) -> --effort. Ergebnis im globalen Array AGY_ARGS.
# ACHTUNG agy-Eigenheit: sobald --model gesetzt ist, VERLANGT agy auch --effort
# ("--model gemini-3.7-flash requires --effort (available: low, medium, high)");
# ohne beide Flags nimmt agy den in der Vorlage gespeicherten Default (beim Login
# war das "Gemini 3.7 Flash (High)"). AGY_MODEL also immer mit AGY_EFFORT setzen.
agy_model_args() {
  AGY_ARGS=()
  [ -n "${AGY_MODEL:-}" ]  && AGY_ARGS+=(--model "$AGY_MODEL")
  [ -n "${AGY_EFFORT:-}" ] && AGY_ARGS+=(--effort "$AGY_EFFORT")
}
