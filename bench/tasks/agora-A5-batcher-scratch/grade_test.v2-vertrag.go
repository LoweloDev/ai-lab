package feed

// Bench grading for agora-A5-batcher-scratch — Vertragslesart (Option A, audit-report.md 4.3,
// Patch-Paket P5). Inhaltlich identisch zu audit-scratch/synth/a5-grade-v2_test.go:
//   - Property (c) in Vertragslesart: RankCandidates liefert absteigend sortiert, der Batcher
//     übernimmt die Eingabeordnung als Rangfolge (kein defensives Selbst-Sortieren, F8)
//   - Property (b) auf die eigene Begründung zurückgeschnitten (kein Batch MIT Inhalt, F12)
//   - Property (e2): Live-Kadenz bei reichlich Text, unter allen 720 Anordnungen (F9)
// NICHT Standard: grade.v2.sh lädt diese Datei nur, wenn GRADE_HIDDEN darauf zeigt
// (Umschaltung durch das Apply-Skript, sobald die Entscheidung P5 gefallen ist).

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

// Property (b) — entschärft: keine Seite MIT Inhalt (eine leere Leerzustands-Seite ist erlaubt).
func TestGradeEmptyInputProducesNoBatches(t *testing.T) {
	for _, batch := range BuildBatches(nil, gradeDesktopOptions(3)) {
		if len(batch.Items) != 0 {
			t.Fatalf("nil input produced a batch with %d items, want no content", len(batch.Items))
		}
	}
	for _, batch := range BuildBatches([]RankedItem{}, BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportMobile,
		UserConfidence: 0.1,
		PageSize:       3,
	}) {
		if len(batch.Items) != 0 {
			t.Fatalf("empty input produced a batch with %d items, want no content", len(batch.Items))
		}
	}
}

func gradePermute(n int, fn func(order []int)) {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	var rec func(k int)
	rec = func(k int) {
		if k == n {
			cp := append([]int(nil), order...)
			fn(cp)
			return
		}
		for i := k; i < n; i++ {
			order[k], order[i] = order[i], order[k]
			rec(k + 1)
			order[k], order[i] = order[i], order[k]
		}
	}
	rec(0)
}

func TestGradeMobileOneItemPerBatchBestFirst(t *testing.T) {
	items := []RankedItem{
		gradeItem(2, ItemTextDebate, 90, "housing"),
		gradeItem(3, ItemTextDebate, 70, "culture"),
		gradeItem(4, ItemLiveRoom, 60, "science"),
		gradeItem(1, ItemTextDebate, 40, "ai"),
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

func TestGradeDesktopPageMixesTopicsWhenAlternativeExists(t *testing.T) {
	base := []RankedItem{
		gradeItem(1, ItemTextDebate, 100, "climate"),
		gradeItem(2, ItemTextDebate, 99, "climate"),
		gradeItem(3, ItemTextDebate, 98, "climate"),
		gradeItem(4, ItemTextDebate, 97, "housing"),
	}
	gradePermute(len(base), func(order []int) {
		items := make([]RankedItem, len(base))
		for i, idx := range order {
			items[i] = base[idx]
		}
		batches := BuildBatches(items, gradeDesktopOptions(3))

		if len(batches) == 0 {
			t.Fatalf("input order %v: expected at least one batch", order)
		}
		first := batches[0]
		if len(first.Items) < 2 {
			t.Fatalf("input order %v: first desktop batch has %d item(s); the brief promises a small handful per desktop page", order, len(first.Items))
		}
		topics := make(map[string]bool)
		for _, item := range first.Items {
			if len(item.Candidate.TopicSlugs) > 0 {
				topics[item.Candidate.TopicSlugs[0]] = true
			}
		}
		if len(topics) < 2 {
			t.Fatalf("input order %v: first desktop batch covers only topics %v although an alternative topic was available", order, topics)
		}
	})
}

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
			continue
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

// Property (e2) — Live-Kadenz bei reichlich Text. WHY: Die Pro-Seite-Kappung ("höchstens
// ein Live-Raum je Seite") hilft hier nicht; mit vier Text-Beiträgen im Pool darf Seite 2
// nicht wieder mit einem Live-Raum beginnen. Unter allen 720 Anordnungen.
func TestGradeNoBackToBackLiveTopSlotWhenTextIsPlentiful(t *testing.T) {
	base := []RankedItem{
		gradeItem(1, ItemLiveRoom, 100, "a"),
		gradeItem(2, ItemTextDebate, 95, "b"),
		gradeItem(3, ItemTextDebate, 94, "c"),
		gradeItem(4, ItemLiveRoom, 93, "d"),
		gradeItem(5, ItemTextDebate, 92, "e"),
		gradeItem(6, ItemTextDebate, 91, "f"),
	}
	gradePermute(len(base), func(order []int) {
		items := make([]RankedItem, len(base))
		for i, idx := range order {
			items[i] = base[idx]
		}
		batches := BuildBatches(items, gradeDesktopOptions(3))
		for i := 0; i+1 < len(batches); i++ {
			if len(batches[i].Items) == 0 || len(batches[i+1].Items) == 0 {
				continue
			}
			if batches[i].Items[0].Candidate.Type == ItemLiveRoom &&
				batches[i+1].Items[0].Candidate.Type == ItemLiveRoom {
				t.Fatalf("input order %v: batches %d and %d both put a live room on top although four text debates were available", order, i, i+1)
			}
		}
	})
}

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
