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
