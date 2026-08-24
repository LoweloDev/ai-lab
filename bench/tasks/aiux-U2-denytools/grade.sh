#!/usr/bin/env bash
# Grader for aiux-U2-denytools. Workspace dir is $1; the runner wrote the baseline git SHA
# into $1/.bench-baseline. PASS iff no test file was modified, no test fails, and the pass
# count reaches 46 existing tests + 6 grading tests = 52.
set -u
cd "$1/runtime/web" || { echo "FAIL no-workspace"; exit 1; }

base=$(cat "$1/.bench-baseline" 2>/dev/null)
if [ -z "$base" ]; then echo "FAIL no-baseline"; exit 1; fi

if git -C "$1" diff "$base" -- runtime/web/test/ | grep -q . ; then
  echo "FAIL test-file-modified"
  exit 1
fi

cp /home/lowelodev/ai-lab/bench/tasks/aiux-U2-denytools/grade.test.js test/deny-tools.grade.test.js
out=$(node --test test/adapter.test.js test/attached.test.js test/deny-tools.grade.test.js 2>&1)
rm -f test/deny-tools.grade.test.js

pass=$(printf '%s\n' "$out" | sed -n 's/^ℹ pass \([0-9][0-9]*\)$/\1/p' | tail -n 1)
fail=$(printf '%s\n' "$out" | sed -n 's/^ℹ fail \([0-9][0-9]*\)$/\1/p' | tail -n 1)

if [ -z "$pass" ] || [ -z "$fail" ]; then echo "FAIL no-test-summary"; exit 1; fi
if [ "$fail" -ne 0 ]; then echo "FAIL tests-failing fail=$fail pass=$pass"; exit 1; fi
if [ "$pass" -lt 52 ]; then echo "FAIL too-few-passing pass=$pass need=52"; exit 1; fi

echo "PASS pass=$pass fail=$fail"
exit 0
