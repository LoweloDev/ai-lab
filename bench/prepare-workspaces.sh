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

# Task A4: planted regression in the feed/personalization batcher (complexity test).
# sharesTopic populates its map with `false` -> topic diversity penalty is dead code;
# TestDiversityPenaltyPreventsSingleTopicBatch goes red. Single root cause, compiles.
mk_ws agora-A4-feed "$AGORA"
cd "$BASE/agora-A4-feed"
python3 - << 'PYEOF'
p = 'agora-backend/internal/feed/batcher.go'
s = open(p).read()
old = "\tfor _, topic := range left.TopicSlugs {\n\t\ttopics[topic] = true\n\t}"
new = "\tfor _, topic := range left.TopicSlugs {\n\t\ttopics[topic] = false\n\t}"
assert old in s, 'PLANT A4 FAILED'
open(p, 'w').write(s.replace(old, new))
PYEOF
git -c user.email=bench@local -c user.name=bench commit --quiet -am "bench: task state"

# Task A5: design-from-scratch batcher. The batcher implementation AND its tests are
# removed; the agent must design BuildBatches from a vague product brief and is graded
# by hidden property tests (tasks/agora-A5-batcher-scratch/grade_test.go). History MUST
# be stripped to a single baseline commit: with a normal clone the deleted batcher.go
# would be one `git log --all -- '*batcher*'` away. batcher_test.go can go with it —
# its helpers (rankedCandidate/batchTopics/candidateIDsFromRanked) are used nowhere
# else; testCandidate lives in scorer_test.go and stays.
mk_ws agora-A5-batcher-scratch "$AGORA"
cd "$BASE/agora-A5-batcher-scratch"
rm agora-backend/internal/feed/batcher.go agora-backend/internal/feed/batcher_test.go
rm -rf .git
git init --quiet
git checkout --quiet -b bench/local-agent   # branch must exist BEFORE the commit
git -c user.email=bench@local -c user.name=bench add -A
git -c user.email=bench@local -c user.name=bench commit --quiet -m "bench: workspace baseline"
[ "$(git rev-list --count HEAD)" -eq 1 ] || { echo "PLANT A5 FAILED: history not stripped"; exit 1; }
if git log --all --oneline -- '*batcher*' | grep -q . ; then echo "PLANT A5 FAILED: batcher recoverable"; exit 1; fi

# Task A6: design-from-scratch personalization scorer. The scoring core AND its tests
# are removed; the agent must design RankCandidates from a vague product brief and is
# graded by hidden behavioral property tests (tasks/agora-A6-scorer-scratch/grade_test.go)
# built on exact orthonormal fake embedding vectors — no real embedding model anywhere.
# Kept-file surgery so the rest of the package stands alone:
#   - preferences.go used scorer.go's clamp01 -> inlined as math.Min(totalWeight/8, 1)
#     (identical semantics: totalWeight sums Abs values, never negative). Keeps helper
#     names like clamp01 free for the agent AND lets a verbatim restore of the original
#     scorer.go still compile (validation path).
#   - scorer_test.go owned shared fixtures (testNow/testID/testCandidate) used by
#     batcher_test.go, events_test.go and preferences_test.go -> moved verbatim into a
#     new kept fixtures_test.go, so all kept tests compile and stay tamper-guarded.
# batcher.go/batcher_test.go STAY: A6 is the ranking heart, not the page batching.
# History MUST be stripped to a single baseline commit: with a normal clone the deleted
# scorer.go would be one `git log --all -- '*scorer*'` away.
mk_ws agora-A6-scorer-scratch "$AGORA"
cd "$BASE/agora-A6-scorer-scratch"
rm agora-backend/internal/feed/scorer.go agora-backend/internal/feed/scorer_test.go
python3 - << 'PYEOF'
p = 'agora-backend/internal/feed/preferences.go'
s = open(p).read()
old = "\t\tConfidence:        clamp01(totalWeight / 8),"
new = "\t\tConfidence:        math.Min(totalWeight/8, 1),"
assert old in s, 'PLANT A6 FAILED: clamp01 call not found'
open(p, 'w').write(s.replace(old, new))
PYEOF
cat > agora-backend/internal/feed/fixtures_test.go << 'EOF'
package feed

// Shared test fixtures for the feed package's tests: a fixed clock, deterministic
// IDs, and a neutral baseline candidate, used across the package's test files.

import (
	"time"

	"github.com/google/uuid"
)

var testNow = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

func testID(seed byte) uuid.UUID {
	var id uuid.UUID
	id[15] = seed
	return id
}

func testCandidate(seed byte, itemType ItemType, title string) Candidate {
	return Candidate{
		ID:              testID(seed),
		Type:            itemType,
		Title:           title,
		Thesis:          title + " thesis",
		CreatedAt:       testNow.Add(-24 * time.Hour),
		UpdatedAt:       testNow.Add(-2 * time.Hour),
		ActivityCount:   5,
		QualityScore:    50,
		LiveParticipant: 0,
	}
}
EOF
rm -rf .git
git init --quiet
git checkout --quiet -b bench/local-agent   # branch must exist BEFORE the commit
git -c user.email=bench@local -c user.name=bench add -A
git -c user.email=bench@local -c user.name=bench commit --quiet -m "bench: workspace baseline"
[ "$(git rev-list --count HEAD)" -eq 1 ] || { echo "PLANT A6 FAILED: history not stripped"; exit 1; }
if git log --all --oneline -- '*scorer*' | grep -q . ; then echo "PLANT A6 FAILED: scorer recoverable"; exit 1; fi

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
