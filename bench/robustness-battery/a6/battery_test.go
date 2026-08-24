package feed

// Robustness battery v2 for agora-A6-scorer-scratch (ZUSATZ-Metrik: it never changes
// a PASS/FAIL verdict). This file is copied into agora-backend/internal/feed/ as
// zz_battery_test.go at battery time and removed afterwards.
//
// Every invariant below is derived ONLY from the product brief (prompt.txt) and from
// the hand-off the brief names explicitly; never from the shape of any particular
// implementation. Model: the property structure of tasks/agora-A6-scorer-scratch/
// grade_test.go (WHY comment per test, ceteris-paribus pools, expected loser first in
// the input so an input-order-preserving no-op cannot fake a pass).
//
// The brief-level contract the battery leans on:
//   - RankCandidates feeds "die Seiten-Zusammenstellung danach", which "erwartet eine
//     fertig bewertete, sortierte Liste". That consumer exists in the package
//     (BuildBatches in batcher.go, untouched by every solution) and it RE-SORTS the
//     list by RankedItem.Score before paging. Consequences the ranker must honour:
//       * it is a reordering of the pool: no candidate lost, none invented, the
//         candidate arrives intact (Type unchanged) — "loss" is measured on DISTINCT
//         IDs so collapsing pathological duplicate-ID input is NOT counted as loss;
//       * the order it decides on must be carried by Score (non-increasing, never
//         NaN) — otherwise the page assembly destroys it before any user sees it;
//       * PageSize lives only in that consumer (BatchOptions); the PageSize edges are
//         therefore expressed end-to-end (RankCandidates -> BuildBatches) — the only
//         place the brief's contract has a page size at all.
//   - "Gleiche Eingaben muessen dieselbe Reihenfolge ergeben — sonst springt der Feed
//     beim Nachladen": two calls with identical inputs must agree exactly, even on
//     perfect score ties; a reload that receives the same pool in a different row
//     order is the same input in the product's sense (row order is not a signal the
//     brief knows), so the order must not depend on it either — the strictest reading
//     of that clause, kept in its own test so it stays attributable.
//   - "'neu' ist schlicht chronologisch nach Erscheinen, das Neueste zuerst": in
//     ModeNew CreatedAt must be non-increasing. Every pool used for that assertion sets
//     UpdatedAt = CreatedAt (as the grade file does), so a solution reading
//     "Erscheinen" as the last update is judged identically; the order WITHIN a
//     timestamp tie stays a free design choice. In MIXED pools two brief clauses can
//     collide for AI suggestions ("schlicht chronologisch" vs. "Wuerze, kein
//     Hauptgericht"), so mixed-pool chronology is asserted over non-suggestion items
//     only; conservation still guarantees the suggestions are not dropped.
//   - "Ein Live-Raum, in dem gerade wirklich etwas passiert, verdient einen Schubs":
//     an active live room beats its identical idle twin even inside a pool of nothing
//     but live rooms, with and without a profile.
//   - "KI-Vorschlaege ... duerfen eine gesunde, aktive menschliche Debatte von
//     vergleichbarer Relevanz nie verdraengen": holds many-vs-many inside a realistic
//     pool, and containment must never turn into deletion when the pool is nothing
//     but suggestions.
//   - "Die Feed-Modi muessen sich vernuenftig verhalten": the package knows five modes;
//     the brief specifies three. For ModeHot/ModeLive only the weak invariants are
//     asserted (no panic, conservation, determinism, Score carries the order).
//
// Two tiers, recognizable by test name:
//   ZZBatReal* — realistic edges (empty feed, single candidate, perfect score ties,
//                input-order independence, single-topic pool, pure live pool, pure
//                suggestion pool, suggestion containment at scale, 200-item mixed
//                pool, timestamp ties in "neu", Score-carries-order, PageSize hand-off
//                1/2/3/=pool/50 through the given page assembly).
//   ZZBatPath* — pathological edges (negative signals incl. negative seeds, NaN/±Inf,
//                extreme magnitudes, duplicate IDs, zero/future timestamps, vector
//                shape mismatch, unknown mode/type and degenerate fields, PageSize
//                0/negative through the page assembly, 3000-item pool and huge
//                cardinalities). Pass criterion is deliberately weak: no panic, no
//                loss, termination (per-call watchdog). NO particular business
//                interpretation of garbage input is enforced.
//
// Every call gets a freshly built pool (an implementation may legitimately sort its
// input in place); the one test that deliberately reuses a slice says so. All
// identifiers are zzBat/ZZBat-prefixed to avoid colliding with helpers the solutions
// define in their own test files.

import (
	"fmt"
	"math"
	"runtime/debug"
	"testing"
	"time"

	"github.com/google/uuid"
)

var zzBatNow = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

// Watchdog budget: a test ends at its FIRST timeout (t.Fatalf), so the worst case is
// one timeout per test — 20 x 4s + 1 x 15s stays below the runner's 110s go-test
// timeout even if every single test hangs on its first call.
const (
	zzBatCallTimeout = 4 * time.Second
	zzBatHugeTimeout = 15 * time.Second
)

var (
	zzBatBriefModes = []Mode{ModeForYou, ModeNew, ModeTop}
	zzBatAllModes   = []Mode{ModeForYou, ModeNew, ModeTop, ModeHot, ModeLive}
)

func zzBatID(seed uint16) uuid.UUID {
	var id uuid.UUID
	id[0] = 0xBA
	id[14] = byte(seed >> 8)
	id[15] = byte(seed)
	return id
}

func zzBatUser(seed byte) uuid.UUID {
	var id uuid.UUID
	id[0] = 0x5B
	id[15] = seed
	return id
}

// zzBatDebate is the ceteris-paribus baseline candidate; tests vary exactly the
// signal under test.
func zzBatDebate(seed uint16) Candidate {
	return Candidate{
		ID:            zzBatID(seed),
		Type:          ItemTextDebate,
		Title:         "bat debate",
		Thesis:        "bat thesis",
		CreatedAt:     zzBatNow.Add(-24 * time.Hour),
		UpdatedAt:     zzBatNow.Add(-3 * time.Hour),
		ActivityCount: 10,
		QualityScore:  50,
	}
}

func zzBatOpts(mode Mode) RankOptions {
	return RankOptions{Mode: mode, Now: zzBatNow, Seed: 7}
}

func zzBatRichProfile() UserProfile {
	user := zzBatUser(1)
	return UserProfile{
		UserID:             &user,
		FollowedUserIDs:    []uuid.UUID{zzBatUser(1)},
		FollowedTopicSlugs: []string{"ki-sicherheit"},
		TopicAffinities:    map[string]float64{"ki-sicherheit": 0.9, "kultur": 0.4, "wohnungsbau": -0.2},
		CreatorAffinities:  map[uuid.UUID]float64{zzBatUser(1): 0.8},
		FormatAffinities:   map[ItemType]float64{ItemLiveRoom: 0.3},
		InterestVector:     []float64{1, 0, 0, 0},
		Confidence:         0.8,
	}
}

// zzBatGuard runs fn on a watchdog goroutine: a panic or a call that does not return
// within timeout fails the single test instead of crashing or hanging the binary.
func zzBatGuard(t *testing.T, label string, timeout time.Duration, fn func()) {
	t.Helper()
	type zzBatOutcome struct {
		panicked interface{}
		stack    string
	}
	done := make(chan zzBatOutcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- zzBatOutcome{panicked: r, stack: string(debug.Stack())}
			}
		}()
		fn()
		done <- zzBatOutcome{}
	}()
	select {
	case out := <-done:
		if out.panicked != nil {
			t.Fatalf("%s: panicked: %v\n%s", label, out.panicked, out.stack)
		}
	case <-time.After(timeout):
		t.Fatalf("%s: did not return within %v (endless loop?)", label, timeout)
	}
}

func zzBatRankTimeout(t *testing.T, label string, candidates []Candidate, profile UserProfile, options RankOptions, timeout time.Duration) []RankedItem {
	t.Helper()
	var ranked []RankedItem
	zzBatGuard(t, label+": RankCandidates", timeout, func() {
		ranked = RankCandidates(candidates, profile, options)
	})
	return ranked
}

func zzBatRank(t *testing.T, label string, candidates []Candidate, profile UserProfile, options RankOptions) []RankedItem {
	t.Helper()
	return zzBatRankTimeout(t, label, candidates, profile, options, zzBatCallTimeout)
}

// zzBatBatches hands the ranked list to the given page assembly (baseline code, not
// the solution's) under the same watchdog.
func zzBatBatches(t *testing.T, label string, ranked []RankedItem, options BatchOptions) []Batch {
	t.Helper()
	var batches []Batch
	zzBatGuard(t, label+": BuildBatches", zzBatCallTimeout, func() {
		batches = BuildBatches(ranked, options)
	})
	return batches
}

func zzBatFlatten(batches []Batch) []RankedItem {
	var items []RankedItem
	for _, batch := range batches {
		items = append(items, batch.Items...)
	}
	return items
}

// zzBatAssertConservation: ranking is a reordering, not a filter. Every distinct
// input ID must appear in the output, nothing may be invented, no ID may appear more
// often than in the input, and a uniquely identified candidate must arrive with its
// Type intact (the page assembly keys layout decisions on it). With unique input IDs
// this is an exact one-to-one check; with duplicate-ID input it permits deduplication.
func zzBatAssertConservation(t *testing.T, label string, in []Candidate, out []RankedItem) {
	t.Helper()
	inCounts := map[uuid.UUID]int{}
	inTypes := map[uuid.UUID]ItemType{}
	for _, candidate := range in {
		inCounts[candidate.ID]++
		inTypes[candidate.ID] = candidate.Type
	}
	outCounts := map[uuid.UUID]int{}
	for _, item := range out {
		id := item.Candidate.ID
		outCounts[id]++
		if inCounts[id] == 0 {
			t.Fatalf("%s: ranked output contains invented candidate %s", label, id)
		}
		if outCounts[id] > inCounts[id] {
			t.Fatalf("%s: candidate %s appears %d times in output but only %d times in input", label, id, outCounts[id], inCounts[id])
		}
		if inCounts[id] == 1 && item.Candidate.Type != inTypes[id] {
			t.Fatalf("%s: candidate %s came back as type %q instead of %q (the candidate must reach the page assembly intact)",
				label, id, item.Candidate.Type, inTypes[id])
		}
	}
	for id := range inCounts {
		if outCounts[id] == 0 {
			t.Fatalf("%s: candidate %s missing from ranked output (loss)", label, id)
		}
	}
}

func zzBatAssertSameOrder(t *testing.T, label string, first, second []RankedItem) {
	t.Helper()
	if len(first) != len(second) {
		t.Fatalf("%s: same inputs produced different lengths: %d vs %d", label, len(first), len(second))
	}
	for index := range first {
		if first[index].Candidate.ID != second[index].Candidate.ID {
			t.Fatalf("%s: same inputs produced a different order at position %d: %s vs %s",
				label, index, first[index].Candidate.ID, second[index].Candidate.ID)
		}
	}
}

func zzBatPos(t *testing.T, label string, ranked []RankedItem, id uuid.UUID) int {
	t.Helper()
	for index, item := range ranked {
		if item.Candidate.ID == id {
			return index
		}
	}
	t.Fatalf("%s: candidate %s is missing from the ranked output", label, id)
	return -1
}

func zzBatAssertAbove(t *testing.T, label string, ranked []RankedItem, winner, loser uuid.UUID) {
	t.Helper()
	winnerPos := zzBatPos(t, label, ranked, winner)
	loserPos := zzBatPos(t, label, ranked, loser)
	if winnerPos >= loserPos {
		t.Fatalf("%s: expected %s (pos %d) above %s (pos %d)", label, winner, winnerPos, loser, loserPos)
	}
}

// zzBatAssertChronological: in ModeNew, CreatedAt must never increase down the list.
// Order within a timestamp tie is a free design choice.
func zzBatAssertChronological(t *testing.T, label string, ranked []RankedItem) {
	t.Helper()
	for index := 1; index < len(ranked); index++ {
		previous := ranked[index-1].Candidate.CreatedAt
		current := ranked[index].Candidate.CreatedAt
		if current.After(previous) {
			t.Fatalf("%s: mode new must be non-increasing in CreatedAt, but position %d (%s) is newer than position %d (%s)",
				label, index, current, index-1, previous)
		}
	}
}

// zzBatAssertChronologicalNonSuggestions: mixed-pool variant, see header.
func zzBatAssertChronologicalNonSuggestions(t *testing.T, label string, ranked []RankedItem) {
	t.Helper()
	var humans []RankedItem
	for _, item := range ranked {
		if item.Candidate.Type != ItemSuggestion {
			humans = append(humans, item)
		}
	}
	zzBatAssertChronological(t, label+" (non-suggestion items)", humans)
}

// zzBatAssertScoreCarriesOrder: the given page assembly re-sorts by Score, so the
// ranker's order only reaches the user if Score is non-increasing along it and never
// NaN (NaN comparisons are all false and silently corrupt that sort).
func zzBatAssertScoreCarriesOrder(t *testing.T, label string, ranked []RankedItem) {
	t.Helper()
	for index, item := range ranked {
		if math.IsNaN(item.Score) {
			t.Fatalf("%s: position %d (%s) carries a NaN score; the page assembly sorts by Score and NaN corrupts that sort",
				label, index, item.Candidate.ID)
		}
		if index > 0 && item.Score > ranked[index-1].Score {
			t.Fatalf("%s: the order must be carried by Score (the given page assembly re-sorts by it), but position %d scores %.6g above position %d with %.6g",
				label, index, item.Score, index-1, ranked[index-1].Score)
		}
	}
}

// zzBatPermute returns the pool in one of four deterministic input orders.
func zzBatPermute(pool []Candidate, variant int) []Candidate {
	n := len(pool)
	out := make([]Candidate, 0, n)
	switch variant {
	case 1: // reversed
		for index := n - 1; index >= 0; index-- {
			out = append(out, pool[index])
		}
	case 2: // stride 3
		for offset := 0; offset < 3; offset++ {
			for index := offset; index < n; index += 3 {
				out = append(out, pool[index])
			}
		}
	case 3: // rotated by half
		for index := 0; index < n; index++ {
			out = append(out, pool[(index+n/2)%n])
		}
	default: // identity
		out = append(out, pool...)
	}
	return out
}

// zzBatMixedPool is a realistic mixed pool of the given size: four topics on an
// orthonormal basis, debates/live rooms/suggestions, varied freshness, activity and
// quality, some followed creators. UpdatedAt = CreatedAt so the chronology of "neu"
// is unambiguous.
func zzBatMixedPool(size int, firstSeed uint16) []Candidate {
	topics := []string{"kultur", "wohnungsbau", "ki-sicherheit", "geschichte"}
	pool := make([]Candidate, 0, size)
	for index := 0; index < size; index++ {
		candidate := zzBatDebate(firstSeed + uint16(index))
		candidate.CreatedAt = zzBatNow.Add(-time.Duration(index*37) * time.Minute)
		candidate.UpdatedAt = candidate.CreatedAt
		candidate.ActivityCount = (index * 13) % 97
		candidate.QualityScore = float64((index * 29) % 101)
		candidate.TopicSlugs = []string{topics[index%len(topics)]}
		vector := make([]float64, 4)
		vector[index%4] = 1
		candidate.Vector = vector
		switch {
		case index%7 == 3:
			candidate.Type = ItemSuggestion
		case index%5 == 2:
			candidate.Type = ItemLiveRoom
			candidate.LiveIsActive = index%2 == 0
			candidate.LiveParticipant = index % 23
		}
		if index%11 == 5 {
			creator := zzBatUser(1)
			candidate.CreatorID = &creator
		}
		pool = append(pool, candidate)
	}
	return pool
}

func zzBatLargePool() []Candidate { return zzBatMixedPool(200, 1000) }

// ---------------------------------------------------------------------------
// ZZBatReal — realistic edges
// ---------------------------------------------------------------------------

// WHY: an empty pool is an everyday case (fresh instance, exhausted feed, hard
// filters upstream). Reordering an empty pool is an empty ranking — in every mode
// the package knows, with and without a profile, without panic.
func TestZZBatRealEmptyPool(t *testing.T) {
	pools := []struct {
		name  string
		items []Candidate
	}{{"nil", nil}, {"empty", []Candidate{}}}
	profiles := []struct {
		name    string
		profile UserProfile
	}{{"zero", UserProfile{}}, {"rich", zzBatRichProfile()}}
	for _, mode := range zzBatAllModes {
		for _, pool := range pools {
			for _, prof := range profiles {
				label := fmt.Sprintf("empty pool (%s slice, %s profile, mode %q)", pool.name, prof.name, mode)
				ranked := zzBatRank(t, label, pool.items, prof.profile, zzBatOpts(mode))
				if len(ranked) != 0 {
					t.Fatalf("%s: empty input produced %d ranked items", label, len(ranked))
				}
			}
		}
	}
}

// WHY: a pool of one must come back as exactly that one item, intact and with a
// usable score, in every mode: the page assembly expects the scored list, and a
// ranker that drops or NaN-scores the only candidate empties the feed.
func TestZZBatRealSingleCandidate(t *testing.T) {
	for _, mode := range zzBatAllModes {
		for _, prof := range []struct {
			name    string
			profile UserProfile
		}{{"zero", UserProfile{}}, {"rich", zzBatRichProfile()}} {
			label := fmt.Sprintf("single candidate (%s profile, mode %q)", prof.name, mode)
			single := zzBatDebate(1)
			single.TopicSlugs = []string{"ki-sicherheit"}
			single.Vector = []float64{1, 0, 0, 0}
			ranked := zzBatRank(t, label, []Candidate{single}, prof.profile, zzBatOpts(mode))
			zzBatAssertConservation(t, label, []Candidate{single}, ranked)
			zzBatAssertScoreCarriesOrder(t, label, ranked)
		}
	}
}

// WHY: perfectly identical twins (only the ID differs) are the hardest score tie.
// The brief demands identical inputs -> identical order, or the feed jumps when the
// next page loads: two calls on the same tied pool must agree exactly, no twin may be
// lost or doubled — and a caller that keeps ONE cached pool and calls twice (page 1,
// page 2) must get the same order even if the ranker sorted that slice in place.
func TestZZBatRealScoreTies(t *testing.T) {
	pool := func() []Candidate {
		var candidates []Candidate
		for seed := uint16(1); seed <= 8; seed++ {
			candidates = append(candidates, zzBatDebate(seed))
		}
		return candidates
	}
	profile := func() UserProfile { return UserProfile{Confidence: 0.5} }
	for _, mode := range zzBatBriefModes {
		label := fmt.Sprintf("score ties, mode %q", mode)
		first := zzBatRank(t, label+" (first call)", pool(), profile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), first)
		second := zzBatRank(t, label+" (second call, fresh pool)", pool(), profile(), zzBatOpts(mode))
		zzBatAssertSameOrder(t, label+" (fresh pool)", first, second)

		shared := pool()
		third := zzBatRank(t, label+" (shared slice, call 1)", shared, profile(), zzBatOpts(mode))
		fourth := zzBatRank(t, label+" (shared slice, call 2)", shared, profile(), zzBatOpts(mode))
		zzBatAssertSameOrder(t, label+" (shared slice vs fresh)", first, third)
		zzBatAssertSameOrder(t, label+" (shared slice called twice)", third, fourth)
	}
}

// WHY: "sonst springt der Feed beim Nachladen" — a reload receives the same pool,
// but nothing guarantees the store hands it over in the same row order (the brief
// knows no such signal). If perfect twins or near-ties are ordered by arrival, the
// feed reshuffles on reload. Strictest reading of the determinism clause; kept in its
// own test so it stays attributable. Six perfect twins plus four distinct items,
// ranked from four deterministic input orders, must yield one identical order.
func TestZZBatRealTiesInputOrderIndependent(t *testing.T) {
	pool := func() []Candidate {
		var candidates []Candidate
		for seed := uint16(1); seed <= 6; seed++ {
			candidates = append(candidates, zzBatDebate(seed))
		}
		quality := zzBatDebate(7)
		quality.QualityScore = 90
		quality.ActivityCount = 50
		newest := zzBatDebate(8)
		newest.CreatedAt = zzBatNow.Add(-1 * time.Hour)
		newest.UpdatedAt = newest.CreatedAt
		live := zzBatDebate(9)
		live.Type = ItemLiveRoom
		live.LiveIsActive = true
		live.LiveParticipant = 12
		personal := zzBatDebate(10)
		personal.TopicSlugs = []string{"ki-sicherheit"}
		personal.Vector = []float64{1, 0, 0, 0}
		return append(candidates, quality, newest, live, personal)
	}
	for _, mode := range zzBatBriefModes {
		label := fmt.Sprintf("input-order independence, mode %q", mode)
		reference := zzBatRank(t, label+" (identity order)", zzBatPermute(pool(), 0), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), reference)
		for variant, name := range map[int]string{1: "reversed", 2: "stride-3", 3: "rotated"} {
			ranked := zzBatRank(t, label+" ("+name+" order)", zzBatPermute(pool(), variant), zzBatRichProfile(), zzBatOpts(mode))
			zzBatAssertSameOrder(t, label+" ("+name+" order vs identity)", reference, ranked)
		}
	}
}

// WHY: a niche user sees pools where every candidate carries the one topic the
// profile cares about — the topic signal cancels out completely. The pool must
// still be conserved, the order deterministic, and "neu" chronological.
func TestZZBatRealSingleTopicPool(t *testing.T) {
	pool := func() []Candidate {
		var candidates []Candidate
		for index := 0; index < 12; index++ {
			candidate := zzBatDebate(uint16(100 + index))
			candidate.TopicSlugs = []string{"kultur"}
			candidate.Vector = []float64{0, 0, 1, 0}
			candidate.CreatedAt = zzBatNow.Add(-time.Duration(index+1) * 5 * time.Hour)
			candidate.UpdatedAt = candidate.CreatedAt
			candidate.ActivityCount = (index * 7) % 30
			candidate.QualityScore = float64((index * 17) % 90)
			candidates = append(candidates, candidate)
		}
		return candidates
	}
	profile := func() UserProfile {
		return UserProfile{
			FollowedTopicSlugs: []string{"kultur"},
			TopicAffinities:    map[string]float64{"kultur": 0.9},
			InterestVector:     []float64{0, 0, 1, 0},
			Confidence:         0.8,
		}
	}

	forYouFirst := zzBatRank(t, "single topic, for_you (first call)", pool(), profile(), zzBatOpts(ModeForYou))
	zzBatAssertConservation(t, "single topic, for_you", pool(), forYouFirst)
	forYouSecond := zzBatRank(t, "single topic, for_you (second call)", pool(), profile(), zzBatOpts(ModeForYou))
	zzBatAssertSameOrder(t, "single topic, for_you", forYouFirst, forYouSecond)

	chronological := zzBatRank(t, "single topic, new", pool(), profile(), zzBatOpts(ModeNew))
	zzBatAssertConservation(t, "single topic, new", pool(), chronological)
	zzBatAssertChronological(t, "single topic, new", chronological)

	top := zzBatRank(t, "single topic, top", pool(), profile(), zzBatOpts(ModeTop))
	zzBatAssertConservation(t, "single topic, top", pool(), top)
}

// WHY: during a big event the pool can be nothing but live rooms. The brief's live
// nudge (a room where something actually happens beats one just standing around)
// must hold inside a pure live pool too, with and without a profile; nothing lost,
// order deterministic. The idle twin precedes the active twin in the input.
func TestZZBatRealPureLivePool(t *testing.T) {
	idleTwin := zzBatID(201)
	activeTwin := zzBatID(202)
	pool := func() []Candidate {
		var candidates []Candidate
		for index := 0; index < 10; index++ {
			candidate := zzBatDebate(uint16(201 + index))
			candidate.Type = ItemLiveRoom
			candidate.LiveIsActive = index >= 4 && index%2 == 1
			candidate.LiveParticipant = 3 + index
			candidates = append(candidates, candidate)
		}
		// Positions 0 and 1 are the ceteris-paribus pair: identical twins that
		// differ ONLY in LiveIsActive, idle one first in the input.
		candidates[0].LiveIsActive = false
		candidates[0].LiveParticipant = 9
		candidates[1].LiveIsActive = true
		candidates[1].LiveParticipant = 9
		return candidates
	}
	for _, prof := range []struct {
		name    string
		profile UserProfile
	}{{"zero", UserProfile{}}, {"rich", zzBatRichProfile()}} {
		label := "pure live pool, " + prof.name + " profile"
		first := zzBatRank(t, label+" (first call)", pool(), prof.profile, zzBatOpts(ModeForYou))
		zzBatAssertConservation(t, label, pool(), first)
		zzBatAssertAbove(t, label+": live nudge", first, activeTwin, idleTwin)
		second := zzBatRank(t, label+" (second call)", pool(), prof.profile, zzBatOpts(ModeForYou))
		zzBatAssertSameOrder(t, label, first, second)
	}
}

// WHY: when human supply is thin the pool can be nothing but AI suggestions.
// "Wuerze, kein Hauptgericht" bounds suggestions RELATIVE to human debates; it never
// licenses dropping them — a ranker that contains suggestions by deleting them
// serves an empty feed. All of them must come back, deterministically, in every
// brief mode.
func TestZZBatRealAllSuggestionsPool(t *testing.T) {
	pool := func() []Candidate {
		topics := []string{"ki-sicherheit", "kultur", "wohnungsbau"}
		var candidates []Candidate
		for index := 0; index < 10; index++ {
			candidate := zzBatDebate(uint16(250 + index))
			candidate.Type = ItemSuggestion
			candidate.TopicSlugs = []string{topics[index%3]}
			vector := make([]float64, 4)
			vector[index%3] = 1
			candidate.Vector = vector
			candidate.CreatedAt = zzBatNow.Add(-time.Duration(index+1) * 20 * time.Minute)
			candidate.UpdatedAt = candidate.CreatedAt
			candidate.ActivityCount = 0
			candidate.QualityScore = float64((index * 11) % 40)
			candidates = append(candidates, candidate)
		}
		return candidates
	}
	for _, mode := range zzBatBriefModes {
		label := fmt.Sprintf("all-suggestions pool, mode %q", mode)
		first := zzBatRank(t, label+" (first call)", pool(), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), first)
		second := zzBatRank(t, label+" (second call)", pool(), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertSameOrder(t, label, first, second)
	}
}

// WHY: the grade file proves containment one-vs-one; a real pool carries several
// suggestions at once. Five maximally on-interest, brand-new, inactive suggestions
// versus five healthy, active human debates on the same topic and vector, inside a
// pool of twenty off-interest fillers (whose positions are free): no suggestion may
// sit above any of those debates. Suggestions come first in the input.
func TestZZBatRealSuggestionsContainedAtScale(t *testing.T) {
	debateIDs := map[uuid.UUID]bool{}
	pool := func() []Candidate {
		var candidates []Candidate
		for index := 0; index < 5; index++ {
			suggestion := zzBatDebate(uint16(2001 + index))
			suggestion.Type = ItemSuggestion
			suggestion.TopicSlugs = []string{"ki-sicherheit"}
			suggestion.Vector = []float64{1, 0, 0, 0}
			suggestion.CreatedAt = zzBatNow.Add(-time.Duration(5+index) * time.Minute)
			suggestion.UpdatedAt = suggestion.CreatedAt
			suggestion.ActivityCount = 0
			suggestion.QualityScore = 0
			candidates = append(candidates, suggestion)
		}
		topics := []string{"wohnungsbau", "kultur", "geschichte"}
		for index := 0; index < 20; index++ {
			filler := zzBatDebate(uint16(2101 + index))
			filler.TopicSlugs = []string{topics[index%3]}
			vector := make([]float64, 4)
			vector[1+index%3] = 1
			filler.Vector = vector
			filler.CreatedAt = zzBatNow.Add(-time.Duration(index+1) * 2 * time.Hour)
			filler.UpdatedAt = zzBatNow.Add(-time.Duration(index+1) * 30 * time.Minute)
			filler.ActivityCount = (index * 7) % 50
			filler.QualityScore = float64((index * 23) % 100)
			if index%4 == 1 {
				filler.Type = ItemLiveRoom
				filler.LiveIsActive = index%8 == 1
				filler.LiveParticipant = 2 + index
			}
			candidates = append(candidates, filler)
		}
		for index := 0; index < 5; index++ {
			debate := zzBatDebate(uint16(2201 + index))
			debate.TopicSlugs = []string{"ki-sicherheit"}
			debate.Vector = []float64{1, 0, 0, 0}
			debate.CreatedAt = zzBatNow.Add(-time.Duration(6+index) * time.Hour)
			debate.UpdatedAt = zzBatNow.Add(-1*time.Hour - time.Duration(index)*10*time.Minute)
			debate.ActivityCount = 40 + index
			debate.QualityScore = 80
			debateIDs[debate.ID] = true
			candidates = append(candidates, debate)
		}
		return candidates
	}
	profile := UserProfile{
		InterestVector:  []float64{1, 0, 0, 0},
		TopicAffinities: map[string]float64{"ki-sicherheit": 1.0},
		Confidence:      0.8,
	}
	ranked := zzBatRank(t, "suggestion containment at scale", pool(), profile, zzBatOpts(ModeForYou))
	zzBatAssertConservation(t, "suggestion containment at scale", pool(), ranked)

	worstDebate, bestSuggestion := -1, len(ranked)
	var worstDebateID, bestSuggestionID uuid.UUID
	for position, item := range ranked {
		switch {
		case item.Candidate.Type == ItemSuggestion:
			if position < bestSuggestion {
				bestSuggestion, bestSuggestionID = position, item.Candidate.ID
			}
		case debateIDs[item.Candidate.ID]:
			if position > worstDebate {
				worstDebate, worstDebateID = position, item.Candidate.ID
			}
		}
	}
	if worstDebate > bestSuggestion {
		t.Fatalf("suggestion containment at scale: suggestion %s (pos %d) sits above healthy human debate %s (pos %d) of the same relevance",
			bestSuggestionID, bestSuggestion, worstDebateID, worstDebate)
	}
}

// WHY: a production pool is a couple hundred candidates of mixed type. Ranking must
// stay an exact permutation (200 in, 200 out, each exactly once), stay deterministic
// across calls, finish promptly (watchdog) in every mode the package knows, and
// "neu" must stay chronological at scale.
func TestZZBatRealLargePool200(t *testing.T) {
	for _, mode := range zzBatAllModes {
		label := fmt.Sprintf("large pool, mode %q", mode)
		first := zzBatRank(t, label+" (first call)", zzBatLargePool(), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, zzBatLargePool(), first)
		second := zzBatRank(t, label+" (second call)", zzBatLargePool(), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertSameOrder(t, label, first, second)
		if mode == ModeNew {
			zzBatAssertChronologicalNonSuggestions(t, label, first)
		}
	}
}

// WHY: batch imports and migrations produce many identical CreatedAt values. "neu"
// must stay non-increasing across those ties, keep everyone, and stay deterministic
// — even under a profile that pulls hard on the OLDER items (personalization must
// not leak into the chronological mode).
func TestZZBatRealModeNewTiedTimestamps(t *testing.T) {
	pool := func() []Candidate {
		followed := zzBatUser(1)
		var candidates []Candidate
		for index := 0; index < 40; index++ {
			candidate := zzBatDebate(uint16(300 + index))
			candidate.CreatedAt = zzBatNow.Add(-time.Duration(index/4) * time.Hour)
			candidate.UpdatedAt = candidate.CreatedAt
			if index >= 32 {
				// The oldest group gets the full personalization pull.
				candidate.TopicSlugs = []string{"ki-sicherheit"}
				candidate.Vector = []float64{1, 0, 0, 0}
				candidate.CreatorID = &followed
			}
			candidates = append(candidates, candidate)
		}
		return candidates
	}
	first := zzBatRank(t, "tied timestamps, new (first call)", pool(), zzBatRichProfile(), zzBatOpts(ModeNew))
	zzBatAssertConservation(t, "tied timestamps, new", pool(), first)
	zzBatAssertChronological(t, "tied timestamps, new", first)
	second := zzBatRank(t, "tied timestamps, new (second call)", pool(), zzBatRichProfile(), zzBatOpts(ModeNew))
	zzBatAssertSameOrder(t, "tied timestamps, new", first, second)
}

// WHY: the brief hands the result to the existing page assembly, which "erwartet eine
// fertig bewertete, sortierte Liste" and (see batcher.go) re-sorts it by Score
// before paging. An order that is not carried by Score — chronological "neu" with
// personalization scores left in, a containment pass that moves items without
// re-scoring, NaN scores — is destroyed before any user sees it. Checked on a mixed
// 40-item pool in every mode, with and without a profile.
func TestZZBatRealScoreCarriesOrder(t *testing.T) {
	for _, mode := range zzBatAllModes {
		for _, prof := range []struct {
			name    string
			profile UserProfile
		}{{"zero", UserProfile{}}, {"rich", zzBatRichProfile()}} {
			label := fmt.Sprintf("score carries order (%s profile, mode %q)", prof.name, mode)
			ranked := zzBatRank(t, label, zzBatMixedPool(40, 3000), prof.profile, zzBatOpts(mode))
			zzBatAssertConservation(t, label, zzBatMixedPool(40, 3000), ranked)
			zzBatAssertScoreCarriesOrder(t, label, ranked)
		}
	}
}

// WHY: PageSize exists only in the consumer the brief names (BuildBatches /
// BatchOptions), so the page-size edges are checked end-to-end: the ranked list
// paged at 1, 2, 3, =pool and 50 per page must reach the user complete (nothing lost
// or doubled across pages, no empty page), and the first mobile page (one item) must
// show the item the ranker put first — or one it scored identically.
func TestZZBatRealPageSizeHandOff(t *testing.T) {
	const poolSize = 30
	for _, mode := range zzBatBriefModes {
		label := fmt.Sprintf("page-size hand-off, mode %q", mode)
		ranked := zzBatRank(t, label, zzBatMixedPool(poolSize, 3100), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, zzBatMixedPool(poolSize, 3100), ranked)
		for _, pageSize := range []int{1, 2, 3, poolSize, 50} {
			pageLabel := fmt.Sprintf("%s, desktop page size %d", label, pageSize)
			batches := zzBatBatches(t, pageLabel, ranked, BatchOptions{Mode: mode, Viewport: ViewportDesktop, UserConfidence: 0.8, PageSize: pageSize})
			for index, batch := range batches {
				if len(batch.Items) == 0 {
					t.Fatalf("%s: page %d is empty", pageLabel, index)
				}
			}
			zzBatAssertConservation(t, pageLabel+" (all pages)", zzBatMixedPool(poolSize, 3100), zzBatFlatten(batches))
		}
		mobile := zzBatBatches(t, label+", mobile", ranked, BatchOptions{Mode: mode, Viewport: ViewportMobile, UserConfidence: 0.8, PageSize: 1})
		if len(mobile) == 0 || len(mobile[0].Items) == 0 {
			t.Fatalf("%s: mobile paging produced no first page", label)
		}
		lead := mobile[0].Items[0]
		if lead.Candidate.ID != ranked[0].Candidate.ID && !(lead.Score == ranked[0].Score) {
			t.Fatalf("%s: the first mobile page shows %s (score %.6g), but the ranker put %s (score %.6g) first — the order did not survive the hand-off",
				label, lead.Candidate.ID, lead.Score, ranked[0].Candidate.ID, ranked[0].Score)
		}
	}
}

// ---------------------------------------------------------------------------
// ZZBatPath — pathological edges.
// Pass criterion: no panic, no loss, termination. Nothing more.
// ---------------------------------------------------------------------------

// WHY: upstream bugs hand out negative counts, scores, affinities, confidence — and
// the rotation seed is a plain int64 that may be negative or minimal (a modulo on it
// yields a negative index). The ranker sits in the request path and must survive
// all of that; what score they yield is not prescribed.
func TestZZBatPathNegativeSignals(t *testing.T) {
	pool := func() []Candidate {
		negQuality := zzBatDebate(401)
		negQuality.QualityScore = -50
		negActivity := zzBatDebate(402)
		negActivity.ActivityCount = -10
		negLive := zzBatDebate(403)
		negLive.Type = ItemLiveRoom
		negLive.LiveIsActive = true
		negLive.LiveParticipant = -3
		negBalance := zzBatDebate(404)
		negBalance.SideBalance = -2.5
		negVector := zzBatDebate(405)
		negVector.Vector = []float64{-1, -1, -1, -1}
		plain := zzBatDebate(406)
		return []Candidate{negQuality, negActivity, negLive, negBalance, negVector, plain}
	}
	profile := func() UserProfile {
		return UserProfile{
			TopicAffinities:   map[string]float64{"kultur": -5},
			CreatorAffinities: map[uuid.UUID]float64{zzBatUser(9): -2},
			FormatAffinities:  map[ItemType]float64{ItemTextDebate: -3},
			InterestVector:    []float64{-1, 0, 0, 0},
			Confidence:        -1,
		}
	}
	for _, mode := range zzBatBriefModes {
		for _, seed := range []int64{7, -1, math.MinInt64, math.MaxInt64} {
			label := fmt.Sprintf("negative signals, mode %q, seed %d", mode, seed)
			ranked := zzBatRank(t, label, pool(), profile(), RankOptions{Mode: mode, Now: zzBatNow, Seed: seed})
			zzBatAssertConservation(t, label, pool(), ranked)
		}
	}
}

// WHY: a broken embedding backend or a bad division upstream produces NaN and
// +/-Inf. Sorting on NaN keys silently corrupts comparisons and score-threshold
// filters silently drop entries — the ranker must neither panic nor lose candidates.
// What score NaN input yields is not prescribed.
func TestZZBatPathNaNAndInfSignals(t *testing.T) {
	pool := func() []Candidate {
		nanQuality := zzBatDebate(501)
		nanQuality.QualityScore = math.NaN()
		posInf := zzBatDebate(502)
		posInf.QualityScore = math.Inf(1)
		negInf := zzBatDebate(503)
		negInf.QualityScore = math.Inf(-1)
		nanVector := zzBatDebate(504)
		nanVector.Vector = []float64{math.NaN(), 0, 0, 0}
		infVector := zzBatDebate(505)
		infVector.Vector = []float64{math.Inf(1), 0, math.Inf(-1), 0}
		nanBalance := zzBatDebate(506)
		nanBalance.SideBalance = math.NaN()
		allNaN := zzBatDebate(507)
		allNaN.QualityScore = math.NaN()
		allNaN.SideBalance = math.NaN()
		allNaN.Vector = []float64{math.NaN(), math.NaN(), math.NaN(), math.NaN()}
		plain := zzBatDebate(508)
		return []Candidate{nanQuality, posInf, negInf, nanVector, infVector, nanBalance, allNaN, plain}
	}
	profiles := []struct {
		name    string
		profile UserProfile
	}{
		{"NaN profile", UserProfile{
			InterestVector:    []float64{math.NaN(), 0, 0, 0},
			TopicAffinities:   map[string]float64{"kultur": math.NaN()},
			CreatorAffinities: map[uuid.UUID]float64{zzBatUser(1): math.NaN()},
			FormatAffinities:  map[ItemType]float64{ItemTextDebate: math.NaN()},
			Confidence:        math.NaN(),
		}},
		{"Inf profile", UserProfile{
			InterestVector:  []float64{math.Inf(1), 0, 0, math.Inf(-1)},
			TopicAffinities: map[string]float64{"kultur": math.Inf(1)},
			Confidence:      math.Inf(1),
		}},
		{"rich profile", zzBatRichProfile()},
	}
	for _, mode := range zzBatBriefModes {
		for _, prof := range profiles {
			label := fmt.Sprintf("NaN/Inf signals, %s, mode %q", prof.name, mode)
			ranked := zzBatRank(t, label, pool(), prof.profile, zzBatOpts(mode))
			zzBatAssertConservation(t, label, pool(), ranked)
		}
	}
}

// WHY: overflow hazards — maximal ints, float extremes, vectors whose norm overflows
// to +Inf, timestamps at the edges of the calendar (duration arithmetic saturates),
// a reference time at the epoch or in the far future. None of it may panic, hang or
// drop candidates.
func TestZZBatPathExtremeMagnitudes(t *testing.T) {
	pool := func() []Candidate {
		maxCounts := zzBatDebate(601)
		maxCounts.Type = ItemLiveRoom
		maxCounts.LiveIsActive = true
		maxCounts.ActivityCount = math.MaxInt
		maxCounts.LiveParticipant = math.MaxInt
		minCounts := zzBatDebate(602)
		minCounts.ActivityCount = math.MinInt
		minCounts.LiveParticipant = math.MinInt
		hugeQuality := zzBatDebate(603)
		hugeQuality.QualityScore = 1e308
		tinyQuality := zzBatDebate(604)
		tinyQuality.QualityScore = -1e308
		hugeVector := zzBatDebate(605)
		hugeVector.Vector = []float64{1e200, 1e200, 1e200, 1e200}
		hugeBalance := zzBatDebate(606)
		hugeBalance.SideBalance = 1e308
		ancient := zzBatDebate(607)
		ancient.CreatedAt = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
		ancient.UpdatedAt = ancient.CreatedAt
		farFuture := zzBatDebate(608)
		farFuture.CreatedAt = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
		farFuture.UpdatedAt = farFuture.CreatedAt
		plain := zzBatDebate(609)
		return []Candidate{maxCounts, minCounts, hugeQuality, tinyQuality, hugeVector, hugeBalance, ancient, farFuture, plain}
	}
	profile := func() UserProfile {
		return UserProfile{
			InterestVector:  []float64{1e200, 1e200, 1e200, 1e200},
			TopicAffinities: map[string]float64{"kultur": 1e308},
			Confidence:      1e308,
		}
	}
	nows := []struct {
		name string
		now  time.Time
	}{{"bench now", zzBatNow}, {"epoch", time.Unix(0, 0).UTC()}, {"year 9999", time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)}}
	for _, mode := range zzBatBriefModes {
		for _, now := range nows {
			label := fmt.Sprintf("extreme magnitudes, mode %q, now=%s", mode, now.name)
			ranked := zzBatRank(t, label, pool(), profile(), RankOptions{Mode: mode, Now: now.now, Seed: 7})
			zzBatAssertConservation(t, label, pool(), ranked)
		}
	}
}

// WHY: retries and sloppy joins duplicate candidates. No panic, and the distinct
// candidate must survive; whether duplicates are kept or collapsed is a legitimate
// design choice — conservation is measured on distinct IDs.
func TestZZBatPathDuplicateIDs(t *testing.T) {
	pool := func() []Candidate {
		exactDupe := zzBatDebate(701)
		conflictingDupeA := zzBatDebate(702)
		conflictingDupeA.Title = "version A"
		conflictingDupeB := zzBatDebate(702)
		conflictingDupeB.Title = "version B"
		conflictingDupeB.Type = ItemLiveRoom
		conflictingDupeB.LiveIsActive = true
		plain := zzBatDebate(703)
		var pool []Candidate
		pool = append(pool, exactDupe, exactDupe, exactDupe, conflictingDupeA, conflictingDupeB, plain)
		return pool
	}
	for _, mode := range zzBatBriefModes {
		label := fmt.Sprintf("duplicate IDs, mode %q", mode)
		ranked := zzBatRank(t, label, pool(), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
}

// WHY: zero-value timestamps (bad rows), UpdatedAt before CreatedAt, and clock skew
// (CreatedAt after Now) happen; age and decay arithmetic must not panic, hang, or
// drop them. A zero options.Now is the harshest variant of the same hazard.
func TestZZBatPathZeroAndFutureTimes(t *testing.T) {
	pool := func() []Candidate {
		zeroTimes := zzBatDebate(801)
		zeroTimes.CreatedAt = time.Time{}
		zeroTimes.UpdatedAt = time.Time{}
		future := zzBatDebate(802)
		future.CreatedAt = zzBatNow.Add(48 * time.Hour)
		future.UpdatedAt = zzBatNow.Add(72 * time.Hour)
		inverted := zzBatDebate(803)
		inverted.CreatedAt = zzBatNow.Add(-1 * time.Hour)
		inverted.UpdatedAt = zzBatNow.Add(-40 * time.Hour)
		zeroUpdated := zzBatDebate(804)
		zeroUpdated.UpdatedAt = time.Time{}
		plain := zzBatDebate(805)
		return []Candidate{zeroTimes, future, inverted, zeroUpdated, plain}
	}
	for _, mode := range zzBatBriefModes {
		label := fmt.Sprintf("zero/future times, mode %q", mode)
		ranked := zzBatRank(t, label, pool(), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
	zeroNow := zzBatRank(t, "zero options.Now", pool(), zzBatRichProfile(), RankOptions{Mode: ModeForYou})
	zzBatAssertConservation(t, "zero options.Now", pool(), zeroNow)
}

// WHY: model or version drift produces vectors of differing dimensionality, nil
// vectors, and all-zero vectors (norm 0 — a division hazard in cosine code), on the
// candidate side and on the profile side. None of that may panic or drop candidates.
func TestZZBatPathVectorShapeMismatch(t *testing.T) {
	pool := func() []Candidate {
		nilVector := zzBatDebate(901)
		emptyVector := zzBatDebate(902)
		emptyVector.Vector = []float64{}
		oneDim := zzBatDebate(903)
		oneDim.Vector = []float64{1}
		shortVector := zzBatDebate(904)
		shortVector.Vector = []float64{1, 0}
		longVector := zzBatDebate(905)
		longVector.Vector = []float64{0, 1, 0, 0, 0, 0, 0, 1}
		zeroVector := zzBatDebate(906)
		zeroVector.Vector = []float64{0, 0, 0, 0}
		matching := zzBatDebate(907)
		matching.Vector = []float64{1, 0, 0, 0}
		return []Candidate{nilVector, emptyVector, oneDim, shortVector, longVector, zeroVector, matching}
	}
	profiles := []struct {
		name   string
		vector []float64
	}{
		{"dim 4", []float64{1, 0, 0, 0}},
		{"dim 8", []float64{0, 0, 0, 0, 0, 0, 0, 1}},
		{"dim 1", []float64{1}},
		{"zero", []float64{0, 0, 0, 0}},
		{"empty", []float64{}},
		{"nil", nil},
	}
	for _, prof := range profiles {
		profile := zzBatRichProfile()
		profile.InterestVector = prof.vector
		label := "vector shapes, profile vector " + prof.name
		ranked := zzBatRank(t, label, pool(), profile, zzBatOpts(ModeForYou))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
}

// WHY: the mode string and item type arrive from the API surface and the store; an
// unknown or empty mode, an unknown or empty type, ads with and without promotion
// data, empty strings where slugs and titles should be, nil creator pointers next to
// creator affinities, nil UUIDs in the follow list — none of it may panic, hang, or
// lose the pool. What order it yields is free. The fully zero RankOptions is the
// harshest such call.
func TestZZBatPathUnknownModeAndDegenerateFields(t *testing.T) {
	pool := func() []Candidate {
		unknownType := zzBatDebate(1001)
		unknownType.Type = ItemType("video")
		emptyType := zzBatDebate(1002)
		emptyType.Type = ItemType("")
		adNoPromotion := zzBatDebate(1003)
		adNoPromotion.Type = ItemAd
		adWithPromotion := zzBatDebate(1004)
		adWithPromotion.Type = ItemAd
		adWithPromotion.Promotion = &PromotionInfo{CampaignID: zzBatUser(5), Label: "", Placement: "", Goal: ""}
		emptyStrings := zzBatDebate(1005)
		emptyStrings.Title = ""
		emptyStrings.Thesis = ""
		emptyStrings.TopicSlugs = []string{"", "kultur", ""}
		nilCreator := zzBatDebate(1006)
		nilCreator.CreatorID = nil
		nilID := zzBatDebate(1007)
		nilID.CreatorID = &uuid.Nil
		plain := zzBatDebate(1008)
		return []Candidate{unknownType, emptyType, adNoPromotion, adWithPromotion, emptyStrings, nilCreator, nilID, plain}
	}
	profile := func() UserProfile {
		profile := zzBatRichProfile()
		profile.UserID = nil
		profile.FollowedTopicSlugs = append(profile.FollowedTopicSlugs, "")
		profile.FollowedUserIDs = append(profile.FollowedUserIDs, uuid.Nil)
		profile.TopicAffinities[""] = 1
		profile.CreatorAffinities[uuid.Nil] = 1
		profile.FormatAffinities[ItemType("")] = 1
		profile.FormatAffinities[ItemType("video")] = -1
		return profile
	}
	for _, mode := range append([]Mode{Mode("definitely-not-a-mode"), Mode("")}, zzBatAllModes...) {
		label := fmt.Sprintf("degenerate fields, mode %q", mode)
		ranked := zzBatRank(t, label, pool(), profile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
	zeroOptions := zzBatRank(t, "zero RankOptions", pool(), profile(), RankOptions{})
	zzBatAssertConservation(t, "zero RankOptions", pool(), zeroOptions)
}

// WHY: PageSize exists only in the consumer the brief names; a zero or negative page
// size arriving there — fed with the ranker's output, including output produced
// from NaN input — must not panic and must not lose a candidate across pages.
// (The page assembly is baseline code, so this edge cannot separate solutions
// unless a ranker's output breaks the paging; it is kept for the completeness of the
// PageSize edge the battery mandate asks for.)
func TestZZBatPathPageSizeZeroNegative(t *testing.T) {
	nanPool := func() []Candidate {
		var candidates []Candidate
		for index := 0; index < 6; index++ {
			candidate := zzBatDebate(uint16(1101 + index))
			if index%2 == 0 {
				candidate.QualityScore = math.NaN()
			}
			candidates = append(candidates, candidate)
		}
		return candidates
	}
	inputs := []struct {
		name string
		pool func() []Candidate
	}{
		{"mixed pool", func() []Candidate { return zzBatMixedPool(12, 1200) }},
		{"NaN pool", nanPool},
	}
	for _, mode := range zzBatBriefModes {
		for _, input := range inputs {
			ranked := zzBatRank(t, fmt.Sprintf("page size 0/negative, %s, mode %q", input.name, mode), input.pool(), zzBatRichProfile(), zzBatOpts(mode))
			zzBatAssertConservation(t, fmt.Sprintf("page size 0/negative, %s, mode %q", input.name, mode), input.pool(), ranked)
			for _, pageSize := range []int{0, -1, -1000} {
				for _, viewport := range []string{ViewportDesktop, ViewportMobile} {
					label := fmt.Sprintf("page size %d, %s, %s, mode %q", pageSize, viewport, input.name, mode)
					batches := zzBatBatches(t, label, ranked, BatchOptions{Mode: mode, Viewport: viewport, UserConfidence: 0.8, PageSize: pageSize})
					zzBatAssertConservation(t, label+" (all pages)", input.pool(), zzBatFlatten(batches))
				}
			}
		}
	}
}

// WHY: termination at scale — a 3000-candidate pool (fifteen times the realistic
// size) and huge cardinalities on a single candidate and profile (500 topic slugs,
// 2000 affinities, 500 followed users) must return within the watchdog and keep
// every candidate. Complexity is not prescribed; only that it terminates promptly.
func TestZZBatPathHugePoolAndCardinalities(t *testing.T) {
	for _, mode := range zzBatBriefModes {
		label := fmt.Sprintf("3000-candidate pool, mode %q", mode)
		ranked := zzBatRankTimeout(t, label, zzBatMixedPool(3000, 5000), zzBatRichProfile(), zzBatOpts(mode), zzBatHugeTimeout)
		zzBatAssertConservation(t, label, zzBatMixedPool(3000, 5000), ranked)
	}

	wide := func() []Candidate {
		pool := zzBatMixedPool(50, 9000)
		slugs := make([]string, 0, 500)
		for index := 0; index < 500; index++ {
			slugs = append(slugs, fmt.Sprintf("topic-%d", index))
		}
		for index := range pool {
			pool[index].TopicSlugs = slugs
		}
		return pool
	}
	profile := zzBatRichProfile()
	for index := 0; index < 2000; index++ {
		profile.TopicAffinities[fmt.Sprintf("topic-%d", index)] = float64(index%7) / 7
	}
	for index := 0; index < 500; index++ {
		profile.FollowedUserIDs = append(profile.FollowedUserIDs, zzBatUser(byte(index%251)))
		profile.FollowedTopicSlugs = append(profile.FollowedTopicSlugs, fmt.Sprintf("topic-%d", index))
	}
	ranked := zzBatRankTimeout(t, "huge cardinalities", wide(), profile, zzBatOpts(ModeForYou), zzBatHugeTimeout)
	zzBatAssertConservation(t, "huge cardinalities", wide(), ranked)
}
