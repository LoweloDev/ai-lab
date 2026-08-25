#!/usr/bin/env bash
# Grader for agora-A5-batcher-scratch. Workspace dir is $1; the runner wrote the baseline
# git SHA into $1/.bench-baseline. The workspace ships WITHOUT internal/feed/batcher.go
# (and without batcher_test.go); the agent designs BuildBatches from the product brief.
# PASS iff the kept existing test files are untouched and the package suite is green with
# the hidden property tests copied in. The agent's own NEW test files are untracked, so
# they never trip the tamper guard — but they do run, and must pass too.
set -u
cd "$1/agora-backend" || { echo "FAIL no-workspace"; exit 1; }

base=$(cat "$1/.bench-baseline" 2>/dev/null)
if [ -z "$base" ]; then echo "FAIL no-baseline"; exit 1; fi

# Tamper guard: the existing (kept) feed test files must be byte-identical to baseline;
# a deleted kept test file also shows up here.
if git -C "$1" diff "$base" -- 'agora-backend/internal/feed/*_test.go' | grep -q . ; then
  echo "FAIL test-file-modified"
  exit 1
fi

tgt=internal/feed/zz_bench_grade_test.go
if [ -e "$tgt" ]; then echo "FAIL grade-file-collision"; exit 1; fi
cp /home/lowelodev/ai-lab/bench/tasks/agora-A5-batcher-scratch/grade_test.go "$tgt"
go test ./internal/feed/ -count=1 >/dev/null 2>&1
rc=$?
rm -f "$tgt"

if [ "$rc" -eq 0 ]; then echo PASS; else echo "FAIL tests-red"; exit 1; fi
