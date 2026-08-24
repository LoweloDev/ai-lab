#!/usr/bin/env bash
# Prepares sandboxed benchmark workspaces from the two local repos.
# Originals in ~/Projects are NEVER touched. Each task gets its own fresh copy
# with: no git remotes, no hooks, a bench branch, and (where needed) a planted bug.
set -euo pipefail
BASE="$HOME/ai-lab/bench/workspaces"
rm -rf "$BASE"
mkdir -p "$BASE"

mk_ws() { # mk_ws <name> <src-repo>
  local name="$1" src="$2" dst="$BASE/$1"
  git clone --quiet "file://$src" "$dst"
  cd "$dst"
  git remote remove origin
  rm -rf .git/hooks; mkdir .git/hooks
  # Strip anything the agent must never see or run
  rm -rf deploy/scripts .claude .github 2>/dev/null || true
  git checkout --quiet -b bench/local-agent
  git -c user.email=bench@local -c user.name=bench add -A
  git -c user.email=bench@local -c user.name=bench commit --quiet -m "bench: workspace baseline" --allow-empty
}

# ---------- agora tasks ----------
AGORA="$HOME/Projects/agora-debate"

# Task A1: planted bug in the aiwork gate (priority tie-break)
mk_ws agora-A1-gate "$AGORA"
cd "$BASE/agora-A1-gate"
sed -i 's/queued.priority > g.waiting\[best\].priority/queued.priority >= g.waiting[best].priority/' \
  agora-backend/internal/aiwork/gate.go
grep -q 'priority >= g.waiting' agora-backend/internal/aiwork/gate.go || { echo "PLANT A1 FAILED"; exit 1; }
git -c user.email=bench@local -c user.name=bench commit --quiet -am "bench: task state"

# Task A2: JSON-LD script breakout (revert the fix, keep the failing test as spec)
mk_ws agora-A2-jsonld "$AGORA"
cd "$BASE/agora-A2-jsonld"
git revert --no-commit --no-edit 9ccc391 2>/dev/null
git checkout 9ccc391 -- agora-web/tests/jsonLdScript.test.mjs
git -c user.email=bench@local -c user.name=bench commit --quiet -m "bench: task state"

# Task A3: HLS cache policy .webm feature (no plant needed - additive feature task)
mk_ws agora-A3-hls "$AGORA"

# ---------- ai-ux-framework tasks ----------
AIUX="$HOME/Projects/ai-ux-framework"

# Task U1: planted paging bug in the web adapter's describe_type
mk_ws aiux-U1-paging "$AIUX"
cd "$BASE/aiux-U1-paging"
sed -i 's/const end = Math.min(members.length, offset + limit);/const end = Math.min(members.length, offset + limit - 1);/' \
  runtime/web/src/members.js
grep -q 'offset + limit - 1' runtime/web/src/members.js || { echo "PLANT U1 FAILED"; exit 1; }
git -c user.email=bench@local -c user.name=bench commit --quiet -am "bench: task state"

# Task U2: denyTools feature in the web adapter (additive feature task, no plant)
mk_ws aiux-U2-denytools "$AIUX"

echo "--- workspaces ready:"
ls -d "$BASE"/*
