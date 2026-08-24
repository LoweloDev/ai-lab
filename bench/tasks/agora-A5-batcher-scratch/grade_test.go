package feed

// Bench grading for agora-A5-batcher-scratch. This file is copied into
// agora-backend/internal/feed/ as zz_bench_grade_test.go at grade time and removed
// afterwards. Every property below is deliberately design-agnostic: it encodes only
// behavior that any reasonable reading of the product brief implies (variety, live
// cadence, viewport pacing, conservation, empty-input safety) and never the shape of
// one particular implementation. All identifiers are grade-prefixed to avoid colliding
// with helpers the agent may have defined in their own new test files.

import (
	"testing"

	"github.com/google/uuid"
)

func gradeID(seed byte) uuid.UUID {
	var id uuid.UUID
	id[0] = seed
	return id
}

func gradeItem(seed byte, itemType ItemType, score float64, topic string) RankedItem {
	candidate := Candidate{
		ID:         gradeID(seed),
		Type:       itemType,
		Title:      "grade item",
		TopicSlugs: []string{topic},
	}
	if itemType == ItemLiveRoom {
		candidate.LiveIsActive = true
	}
	return RankedItem{Candidate: candidate, Score: score}
}

func gradeDesktopOptions(pageSize int) BatchOptions {
	return BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportDesktop,
		UserConfidence: 0.9,
		PageSize:       pageSize,
	}
}

func gradeMixedPool() []RankedItem {
	return []RankedItem{
		gradeItem(1, ItemTextDebate, 100, "ai"),
		gradeItem(2, ItemLiveRoom, 95, "ai"),
		gradeItem(3, ItemTextDebate, 90, "housing"),
		gradeItem(4, ItemTextDebate, 85, "culture"),
		gradeItem(5, ItemLiveRoom, 80, "science"),
		gradeItem(6, ItemTextDebate, 75, "history"),
		gradeItem(7, ItemSuggestion, 70, "law"),
	}
}

// Property (a) — conservation. WHY: the brief demands that nothing may be lost and
// nothing shown twice, so every ranked item appears in exactly one batch and no batch
// is an empty page.
func TestGradeConservation(t *testing.T) {
	pool := gradeMixedPool()
	batches := BuildBatches(pool, gradeDesktopOptions(3))

	seen := make(map[uuid.UUID]int)
	total := 0
	for index, batch := range batches {
		if len(batch.Items) == 0 {
			t.Fatalf("batch %d is empty; an empty page has nothing to show", index)
		}
		for _, item := range batch.Items {
			seen[item.Candidate.ID]++
			total++
		}
	}
	if total != len(pool) {
		t.Fatalf("batches contain %d items in total, input had %d; nothing may be lost or duplicated", total, len(pool))
	}
	for _, item := range pool {
		if seen[item.Candidate.ID] != 1 {
			t.Fatalf("input item %s appears %d times across batches, want exactly 1", item.Candidate.ID, seen[item.Candidate.ID])
		}
	}
}

// Property (b) — empty input. WHY: the brief says the batcher must not crash when there
// is nothing to show; with no items there are no pages to assemble, so no batches with
// content may come back (a panic fails this test on its own).
func TestGradeEmptyInputProducesNoBatches(t *testing.T) {
	if batches := BuildBatches(nil, gradeDesktopOptions(3)); len(batches) != 0 {
		t.Fatalf("nil input produced %d batches, want none", len(batches))
	}
	empty := BuildBatches([]RankedItem{}, BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportMobile,
		UserConfidence: 0.1,
		PageSize:       3,
	})
	if len(empty) != 0 {
		t.Fatalf("empty input produced %d batches, want none", len(empty))
	}
}

// Property (c) — mobile pacing. WHY: the brief says phones show one item at a time, so
// every mobile batch holds exactly one item; and a ranked feed reasonably leads with
// its strongest item, so the first batch carries the maximum-score item (all scores are
// distinct here, so no tie-handling is assumed).
func TestGradeMobileOneItemPerBatchBestFirst(t *testing.T) {
	items := []RankedItem{
		gradeItem(1, ItemTextDebate, 40, "ai"),
		gradeItem(2, ItemTextDebate, 90, "housing"),
		gradeItem(3, ItemTextDebate, 70, "culture"),
		gradeItem(4, ItemLiveRoom, 60, "science"),
	}
	batches := BuildBatches(items, BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportMobile,
		UserConfidence: 0.9,
		PageSize:       3,
	})

	if len(batches) != len(items) {
		t.Fatalf("mobile batch count = %d, want %d (one item per page, nothing lost)", len(batches), len(items))
	}
	for index, batch := range batches {
		if len(batch.Items) != 1 {
			t.Fatalf("mobile batch %d has %d items, want exactly 1", index, len(batch.Items))
		}
	}
	if batches[0].Items[0].Candidate.ID != gradeID(2) {
		t.Fatalf("first mobile batch item score = %v, want the highest-scored item first", batches[0].Items[0].Score)
	}
}

// Property (d) — topic variety. WHY: the brief says a page must feel varied and the same
// topic must not fill a page when alternatives exist; with three top items on one topic
// and one alternative in the pool, the first desktop page must mix in a second topic.
func TestGradeDesktopPageMixesTopicsWhenAlternativeExists(t *testing.T) {
	items := []RankedItem{
		gradeItem(1, ItemTextDebate, 100, "climate"),
		gradeItem(2, ItemTextDebate, 99, "climate"),
		gradeItem(3, ItemTextDebate, 98, "climate"),
		gradeItem(4, ItemTextDebate, 97, "housing"),
	}
	batches := BuildBatches(items, gradeDesktopOptions(3))

	if len(batches) == 0 {
		t.Fatalf("expected at least one batch")
	}
	first := batches[0]
	if len(first.Items) < 2 {
		t.Fatalf("first desktop batch has %d item(s); the brief promises a small handful per desktop page", len(first.Items))
	}
	topics := make(map[string]bool)
	for _, item := range first.Items {
		if len(item.Candidate.TopicSlugs) > 0 {
			topics[item.Candidate.TopicSlugs[0]] = true
		}
	}
	if len(topics) < 2 {
		t.Fatalf("first desktop batch covers only topics %v although an alternative topic was available", topics)
	}
}

// Property (e) — live cadence. WHY: the brief says live rooms must not hog the top slot
// of consecutive pages outside the live mode. Tolerance: a pair of consecutive live-top
// batches is only a failure if a non-live item was still available when the later top
// slot was filled (i.e. a non-live item appears in that batch or any later one), so a
// forced all-live tail passes.
func TestGradeNoBackToBackLiveTopSlotOutsideLiveMode(t *testing.T) {
	items := []RankedItem{
		gradeItem(1, ItemLiveRoom, 100, "ai"),
		gradeItem(2, ItemTextDebate, 95, "economics"),
		gradeItem(3, ItemLiveRoom, 94, "culture"),
		gradeItem(4, ItemLiveRoom, 93, "science"),
		gradeItem(5, ItemTextDebate, 50, "history"),
	}
	batches := BuildBatches(items, gradeDesktopOptions(3))

	for i := 0; i+1 < len(batches); i++ {
		if len(batches[i].Items) == 0 || len(batches[i+1].Items) == 0 {
			continue // empty batches are property (a)'s problem
		}
		if batches[i].Items[0].Candidate.Type != ItemLiveRoom ||
			batches[i+1].Items[0].Candidate.Type != ItemLiveRoom {
			continue
		}
		for j := i + 1; j < len(batches); j++ {
			for _, item := range batches[j].Items {
				if item.Candidate.Type != ItemLiveRoom {
					t.Fatalf("batches %d and %d both put a live room on top although a non-live item was still available", i, i+1)
				}
			}
		}
	}
}

// Property (f) — page size. WHY: the brief says desktop shows a small handful per page
// and the caller hands the page size over in BatchOptions, so no desktop batch may
// exceed the configured page size (checked for two configured sizes).
func TestGradeDesktopBatchesRespectConfiguredPageSize(t *testing.T) {
	for _, pageSize := range []int{2, 3} {
		batches := BuildBatches(gradeMixedPool(), gradeDesktopOptions(pageSize))
		for index, batch := range batches {
			if len(batch.Items) > pageSize {
				t.Fatalf("pageSize=%d: batch %d has %d items, must not exceed the configured page size", pageSize, index, len(batch.Items))
			}
		}
	}
}
