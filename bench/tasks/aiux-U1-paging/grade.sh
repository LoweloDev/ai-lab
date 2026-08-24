#!/usr/bin/env bash
set -u
cd "$1/runtime/web" || { echo "FAIL no-workspace"; exit 1; }
base=$(cat "$1/.bench-baseline")
if git -C "$1" diff "$base" -- runtime/web/test/ | grep -q . ; then echo "FAIL test-file-modified"; exit 1; fi
out=$(node --test test/adapter.test.js test/attached.test.js 2>&1)
pass=$(echo "$out" | grep -oP 'ℹ pass \K\d+' | head -1)
fail=$(echo "$out" | grep -oP 'ℹ fail \K\d+' | head -1)
[ "${fail:-1}" = "0" ] && [ "${pass:-0}" = "46" ] && echo PASS || { echo "FAIL tests (pass=$pass fail=$fail)"; exit 1; }
