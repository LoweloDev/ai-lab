#!/usr/bin/env bash
set -u
cd "$1/agora-backend" || { echo "FAIL no-workspace"; exit 1; }
base=$(cat "$1/.bench-baseline")
if git diff "$base" -- internal/livehls/cachepolicy_test.go internal/livehls/edgeconfig_test.go | grep -q . ; then echo "FAIL test-file-modified"; exit 1; fi
cat > internal/livehls/bench_grade_test.go <<'EOF'
package livehls

import "testing"

func TestBenchGradeWebm(t *testing.T) {
	if ct := ContentTypeForKey("live/room/seg_1.webm"); ct != "video/webm" {
		t.Fatalf("webm content type: got %q", ct)
	}
	if c := ClassifyKey("live/room/seg_1.webm"); c != ClassSegment {
		t.Fatalf("webm class: got %q", c)
	}
}
EOF
ok=1
go test ./internal/livehls/ -count=1 >/dev/null 2>&1 || ok=0
rm -f internal/livehls/bench_grade_test.go
[ $ok = 1 ] && echo PASS || { echo "FAIL tests-red"; exit 1; }
