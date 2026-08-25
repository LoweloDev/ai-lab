#!/usr/bin/env bash
set -u
cd "$1/agora-backend" || { echo "FAIL no-workspace"; exit 1; }
base=$(cat "$1/.bench-baseline")
if git diff "$base" -- internal/aiwork/gate_test.go | grep -q . ; then echo "FAIL test-file-modified"; exit 1; fi
if go test ./internal/aiwork/ -count=1 >/dev/null 2>&1; then echo PASS; else echo "FAIL tests-red"; exit 1; fi
