#!/usr/bin/env bash
set -u
cd "$1/agora-web" || { echo "FAIL no-workspace"; exit 1; }
base=$(cat "$1/.bench-baseline")
if git diff "$base" -- tests/jsonLdScript.test.mjs | grep -q . ; then echo "FAIL test-file-modified"; exit 1; fi
out=$(node --test tests/jsonLdScript.test.mjs 2>&1)
pass=$(echo "$out" | grep -oP 'ℹ pass \K\d+' | head -1)
fail=$(echo "$out" | grep -oP 'ℹ fail \K\d+' | head -1)
[ "${fail:-1}" = "0" ] && [ "${pass:-0}" -ge 4 ] || { echo "FAIL tests-red (pass=$pass fail=$fail)"; exit 1; }
if grep -q 'JSON.stringify(jsonLd)' 'src/app/debate/[id]/page.tsx'; then echo "FAIL not-wired-into-page"; exit 1; fi
echo PASS
