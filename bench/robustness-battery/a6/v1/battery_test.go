package feed

// Robustness battery for agora-A6-scorer-scratch (ZUSATZ-Metrik, aendert keine
// PASS/FAIL-Urteile). This file is copied into agora-backend/internal/feed/ as
// zz_battery_test.go at battery time and removed afterwards.
//
// Every invariant below is derived ONLY from the product brief (prompt.txt),
// never from the shape of any particular implementation:
//   - the ranking is called as RankCandidates(candidates, profile, options) and
//     feeds the downstream page assembly, so it must be a reordering of the
//     pool: no candidate lost, none invented ("Verlust" is measured on DISTINCT
//     IDs so that collapsing pathological duplicate-ID input is NOT counted as
//     loss - keeping duplicates and deduplicating are both legitimate readings);
//   - "Gleiche Eingaben muessen dieselbe Reihenfolge ergeben" => two calls with
//     identical inputs must agree exactly, even on perfect score ties;
//   - "'neu' ist schlicht chronologisch nach Erscheinen, das Neueste zuerst" =>
//     in ModeNew, CreatedAt must be non-increasing. All pools used for this
//     assertion set UpdatedAt = CreatedAt (the grade file does the same), so a
//     solution reading "Erscheinen" as last update is judged identically; the
//     order WITHIN a timestamp tie stays a free design choice;
//   - "Ein Live-Raum, in dem gerade wirklich etwas passiert, verdient einen
//     Schubs gegenueber einem, der nur herumsteht" => an active live room must
//     beat its identical idle twin even inside a pool of nothing but live rooms.
//
// Two tiers, recognizable by test name:
//   ZZBatReal* - realistic edges (score ties, empty feed, single topic, pure
//                live pool, ~200-item pools, pool-size boundaries 0/1/200).
//   ZZBatPath* - pathological edges (negative/NaN/Inf signals, duplicate IDs,
//                zero/future timestamps, vector-shape mismatch, unknown mode).
//                Pass criterion is deliberately weak: no panic, no loss, no
//                endless loop (per-call watchdog). NO particular business
//                interpretation of garbage input is enforced.
//
// NOTE on PageSize: the graded API RankCandidates/RankOptions carries no
// PageSize (paging lives in BatchOptions of the separate A5 batcher), so the
// "PageSize 0/negative" edge is not expressible against this contract. The
// pool-size boundaries 0 / 1 / 200 stand in for it.
//
// NOTE on input order: wherever a relative order is asserted, the expected
// LOSER precedes the winner in the input slice, so an input-order-preserving
// no-op cannot fake a pass. Every call gets a freshly built pool, because an
// implementation may legitimately sort its input slice in place.
//
// All identifiers are zzBat/ZZBat-prefixed to avoid colliding with helpers the
// solutions define in their own test files.

import (
	"math"
	"runtime/debug"
	"testing"
	"time"

	"github.com/google/uuid"
)

var zzBatNow = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

// zzBatCallTimeout bounds one RankCandidates call. 13 tests x a handful of
// calls each stays far below the runner's 120s budget even if the first call
// of every test hangs.
const zzBatCallTimeout = 8 * time.Second

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

// zzBatDebate is the ceteris-paribus baseline candidate; tests vary exactly
// the signal under test.
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

// zzBatRank runs RankCandidates on a watchdog goroutine: a panic or a call
// that does not return within zzBatCallTimeout fails the single test instead
// of crashing or hanging the whole binary.
func zzBatRank(t *testing.T, label string, candidates []Candidate, profile UserProfile, options RankOptions) []RankedItem {
	t.Helper()
	type zzBatOutcome struct {
		items    []RankedItem
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
		done <- zzBatOutcome{items: RankCandidates(candidates, profile, options)}
	}()
	select {
	case out := <-done:
		if out.panicked != nil {
			t.Fatalf("%s: RankCandidates panicked: %v\n%s", label, out.panicked, out.stack)
		}
		return out.items
	case <-time.After(zzBatCallTimeout):
		t.Fatalf("%s: RankCandidates did not return within %v (endless loop?)", label, zzBatCallTimeout)
	}
	return nil
}

// zzBatAssertConservation: ranking is a reordering, not a filter. Every
// distinct input ID must appear in the output, nothing may be invented, and no
// ID may appear more often than in the input. With unique input IDs this is an
// exact one-to-one check; with duplicate-ID input it permits deduplication.
func zzBatAssertConservation(t *testing.T, label string, in []Candidate, out []RankedItem) {
	t.Helper()
	inCounts := map[uuid.UUID]int{}
	for _, candidate := range in {
		inCounts[candidate.ID]++
	}
	outCounts := map[uuid.UUID]int{}
	for _, item := range out {
		outCounts[item.Candidate.ID]++
	}
	for id := range inCounts {
		if outCounts[id] == 0 {
			t.Fatalf("%s: candidate %s missing from ranked output (loss)", label, id)
		}
	}
	for id, count := range outCounts {
		if inCounts[id] == 0 {
			t.Fatalf("%s: ranked output contains invented candidate %s", label, id)
		}
		if count > inCounts[id] {
			t.Fatalf("%s: candidate %s appears %d times in output but only %d times in input", label, id, count, inCounts[id])
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

// zzBatAssertChronological: in ModeNew, CreatedAt must never increase down the
// list. Order within a timestamp tie is a free design choice.
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

// zzBatAssertChronologicalNonSuggestions: in a MIXED pool two brief clauses can
// collide for AI suggestions in mode "neu": "'neu' ist schlicht chronologisch"
// versus "KI-Vorschlaege sind Wuerze, kein Hauptgericht" (a brand-new
// suggestion at the very top of "neu" would displace human debates). Keeping
// suggestions strictly chronological and containing them are BOTH defensible
// readings, so mixed-pool chronology is asserted over the non-suggestion items
// only; conservation still guarantees the suggestions are not dropped. Pools
// consisting purely of human items use the strict check above.
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

// ---------------------------------------------------------------------------
// ZZBatReal — realistic edges
// ---------------------------------------------------------------------------

// WHY: an empty pool is an everyday case (fresh instance, exhausted feed, hard
// filters upstream). Reordering an empty pool is an empty ranking - in every
// documented mode, with and without a profile, without panic.
func TestZZBatRealEmptyPool(t *testing.T) {
	for _, mode := range []Mode{ModeForYou, ModeNew, ModeTop} {
		for poolName, pool := range map[string][]Candidate{"nil": nil, "empty": {}} {
			for profileName, profile := range map[string]UserProfile{"zero": {}, "rich": zzBatRichProfile()} {
				label := "empty pool (" + poolName + " slice, " + profileName + " profile, mode " + string(mode) + ")"
				ranked := zzBatRank(t, label, pool, profile, zzBatOpts(mode))
				if len(ranked) != 0 {
					t.Fatalf("%s: empty input produced %d ranked items", label, len(ranked))
				}
			}
		}
	}
}

// WHY: a pool of one must come back as exactly that one item in every mode.
// The downstream page assembly expects the scored list; a ranker that drops
// the only candidate empties the feed.
func TestZZBatRealSingleCandidate(t *testing.T) {
	for _, mode := range []Mode{ModeForYou, ModeNew, ModeTop} {
		single := zzBatDebate(1)
		ranked := zzBatRank(t, "single candidate, mode "+string(mode), []Candidate{single}, zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, "single candidate, mode "+string(mode), []Candidate{single}, ranked)
	}
}

// WHY: perfectly identical twins (only the ID differs) are the hardest score
// tie. The brief demands identical inputs -> identical order, or the feed
// jumps when the next page loads: two calls on the same tied pool must agree
// exactly, and no twin may be lost or doubled.
func TestZZBatRealScoreTies(t *testing.T) {
	pool := func() []Candidate {
		var candidates []Candidate
		for seed := uint16(1); seed <= 8; seed++ {
			candidates = append(candidates, zzBatDebate(seed))
		}
		return candidates
	}
	profile := func() UserProfile { return UserProfile{Confidence: 0.5} }
	for _, mode := range []Mode{ModeForYou, ModeNew, ModeTop} {
		label := "score ties, mode " + string(mode)
		first := zzBatRank(t, label+" (first call)", pool(), profile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), first)
		second := zzBatRank(t, label+" (second call)", pool(), profile(), zzBatOpts(mode))
		zzBatAssertSameOrder(t, label, first, second)
	}
}

// WHY: a niche user sees pools where every candidate carries the one topic the
// profile cares about - the topic signal cancels out completely. The pool must
// still be conserved, the order deterministic, and "neu" chronological.
func TestZZBatRealSingleTopicPool(t *testing.T) {
	pool := func() []Candidate {
		var candidates []Candidate
		for index := 0; index < 12; index++ {
			candidate := zzBatDebate(uint16(100 + index))
			candidate.TopicSlugs = []string{"kultur"}
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
}

// WHY: during a big event the pool can be nothing but live rooms. The brief's
// live nudge (a room where something actually happens beats one just standing
// around) must hold inside a pure live pool too; nothing lost, order
// deterministic. The idle twin precedes the active twin in the input.
func TestZZBatRealPureLivePool(t *testing.T) {
	idleTwin := uint16(201)
	activeTwin := uint16(202)
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

	first := zzBatRank(t, "pure live pool (first call)", pool(), UserProfile{}, zzBatOpts(ModeForYou))
	zzBatAssertConservation(t, "pure live pool", pool(), first)
	zzBatAssertAbove(t, "pure live pool: live nudge", first, zzBatID(activeTwin), zzBatID(idleTwin))
	second := zzBatRank(t, "pure live pool (second call)", pool(), UserProfile{}, zzBatOpts(ModeForYou))
	zzBatAssertSameOrder(t, "pure live pool", first, second)
}

func zzBatLargePool() []Candidate {
	topics := []string{"kultur", "wohnungsbau", "ki-sicherheit", "geschichte"}
	var pool []Candidate
	for index := 0; index < 200; index++ {
		candidate := zzBatDebate(uint16(1000 + index))
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

// WHY: a production pool is a couple hundred candidates of mixed type. Ranking
// must stay an exact permutation (200 in, 200 out, each exactly once), stay
// deterministic across calls, finish promptly (watchdog), and "neu" must stay
// chronological at scale.
func TestZZBatRealLargePool200(t *testing.T) {
	first := zzBatRank(t, "large pool, for_you (first call)", zzBatLargePool(), zzBatRichProfile(), zzBatOpts(ModeForYou))
	zzBatAssertConservation(t, "large pool, for_you", zzBatLargePool(), first)
	second := zzBatRank(t, "large pool, for_you (second call)", zzBatLargePool(), zzBatRichProfile(), zzBatOpts(ModeForYou))
	zzBatAssertSameOrder(t, "large pool, for_you", first, second)

	chronological := zzBatRank(t, "large pool, new", zzBatLargePool(), zzBatRichProfile(), zzBatOpts(ModeNew))
	zzBatAssertConservation(t, "large pool, new", zzBatLargePool(), chronological)
	zzBatAssertChronologicalNonSuggestions(t, "large pool, new", chronological)

	top := zzBatRank(t, "large pool, top", zzBatLargePool(), zzBatRichProfile(), zzBatOpts(ModeTop))
	zzBatAssertConservation(t, "large pool, top", zzBatLargePool(), top)
}

// WHY: batch imports and migrations produce many identical CreatedAt values.
// "neu" must stay non-increasing across those ties, keep everyone, and stay
// deterministic - even under a profile that pulls hard on the OLDER items
// (personalization must not leak into the chronological mode).
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

// ---------------------------------------------------------------------------
// ZZBatPath — pathological edges.
// Pass criterion: no panic, no loss, termination. Nothing more.
// ---------------------------------------------------------------------------

// WHY: upstream bugs hand out negative counts, scores and affinities. The
// ranker sits in the request path and must survive them; what score they yield
// is not prescribed.
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
		plain := zzBatDebate(405)
		return []Candidate{negQuality, negActivity, negLive, negBalance, plain}
	}
	profile := func() UserProfile {
		return UserProfile{
			TopicAffinities:   map[string]float64{"kultur": -5},
			CreatorAffinities: map[uuid.UUID]float64{zzBatUser(9): -2},
			FormatAffinities:  map[ItemType]float64{ItemTextDebate: -3},
			Confidence:        -1,
		}
	}
	for _, mode := range []Mode{ModeForYou, ModeNew, ModeTop} {
		label := "negative signals, mode " + string(mode)
		ranked := zzBatRank(t, label, pool(), profile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
}

// WHY: a broken embedding backend or a bad division upstream produces NaN and
// +/-Inf. Sorting on NaN keys silently corrupts comparisons and score-threshold
// filters silently drop entries - the ranker must neither panic nor lose
// candidates. What score NaN input yields is not prescribed.
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
		nanBalance := zzBatDebate(505)
		nanBalance.SideBalance = math.NaN()
		plain := zzBatDebate(506)
		return []Candidate{nanQuality, posInf, negInf, nanVector, nanBalance, plain}
	}
	profile := func() UserProfile {
		return UserProfile{
			InterestVector:  []float64{math.NaN(), 0, 0, 0},
			TopicAffinities: map[string]float64{"kultur": math.NaN()},
			Confidence:      math.NaN(),
		}
	}
	for _, mode := range []Mode{ModeForYou, ModeNew, ModeTop} {
		label := "NaN/Inf signals, mode " + string(mode)
		ranked := zzBatRank(t, label, pool(), profile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
}

// WHY: retries and sloppy joins duplicate candidates. No panic, and the
// distinct candidate must survive; whether duplicates are kept or collapsed is
// a legitimate design choice - conservation is measured on distinct IDs.
func TestZZBatPathDuplicateIDs(t *testing.T) {
	pool := func() []Candidate {
		exactDupe := zzBatDebate(601)
		conflictingDupeA := zzBatDebate(602)
		conflictingDupeA.Title = "version A"
		conflictingDupeB := zzBatDebate(602)
		conflictingDupeB.Title = "version B"
		conflictingDupeB.Type = ItemLiveRoom
		conflictingDupeB.LiveIsActive = true
		plain := zzBatDebate(603)
		return []Candidate{exactDupe, exactDupe, conflictingDupeA, conflictingDupeB, plain}
	}
	for _, mode := range []Mode{ModeForYou, ModeNew, ModeTop} {
		label := "duplicate IDs, mode " + string(mode)
		ranked := zzBatRank(t, label, pool(), UserProfile{}, zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
}

// WHY: zero-value timestamps (bad rows) and clock skew (CreatedAt after Now)
// happen; age and decay arithmetic must not panic, hang, or drop them. A zero
// options.Now is the harshest variant of the same hazard.
func TestZZBatPathZeroAndFutureTimes(t *testing.T) {
	pool := func() []Candidate {
		zeroTimes := zzBatDebate(701)
		zeroTimes.CreatedAt = time.Time{}
		zeroTimes.UpdatedAt = time.Time{}
		future := zzBatDebate(702)
		future.CreatedAt = zzBatNow.Add(48 * time.Hour)
		future.UpdatedAt = zzBatNow.Add(72 * time.Hour)
		plain := zzBatDebate(703)
		return []Candidate{zeroTimes, future, plain}
	}
	for _, mode := range []Mode{ModeForYou, ModeNew, ModeTop} {
		label := "zero/future times, mode " + string(mode)
		ranked := zzBatRank(t, label, pool(), UserProfile{}, zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
	zeroNow := zzBatRank(t, "zero options.Now", pool(), zzBatRichProfile(), RankOptions{Mode: ModeForYou})
	zzBatAssertConservation(t, "zero options.Now", pool(), zeroNow)
}

// WHY: model or version drift produces vectors of differing dimensionality,
// nil vectors, and all-zero vectors (norm 0 - a division hazard in cosine
// code). None of that may panic or drop candidates.
func TestZZBatPathVectorShapeMismatch(t *testing.T) {
	pool := func() []Candidate {
		nilVector := zzBatDebate(801)
		shortVector := zzBatDebate(802)
		shortVector.Vector = []float64{1, 0}
		longVector := zzBatDebate(803)
		longVector.Vector = []float64{0, 1, 0, 0, 0, 0, 0, 1}
		zeroVector := zzBatDebate(804)
		zeroVector.Vector = []float64{0, 0, 0, 0}
		matching := zzBatDebate(805)
		matching.Vector = []float64{1, 0, 0, 0}
		return []Candidate{nilVector, shortVector, longVector, zeroVector, matching}
	}
	withVector := zzBatRank(t, "vector shapes, profile dim 4", pool(), zzBatRichProfile(), zzBatOpts(ModeForYou))
	zzBatAssertConservation(t, "vector shapes, profile dim 4", pool(), withVector)

	zeroProfileVector := zzBatRichProfile()
	zeroProfileVector.InterestVector = []float64{0, 0, 0, 0}
	withZero := zzBatRank(t, "vector shapes, zero profile vector", pool(), zeroProfileVector, zzBatOpts(ModeForYou))
	zzBatAssertConservation(t, "vector shapes, zero profile vector", pool(), withZero)

	nilProfileVector := zzBatRichProfile()
	nilProfileVector.InterestVector = nil
	withNil := zzBatRank(t, "vector shapes, nil profile vector", pool(), nilProfileVector, zzBatOpts(ModeForYou))
	zzBatAssertConservation(t, "vector shapes, nil profile vector", pool(), withNil)
}

// WHY: the mode string arrives from the API surface; an unknown or empty mode
// must not panic, hang, or lose the pool - what order it yields is free. The
// fully zero RankOptions is the harshest such call.
func TestZZBatPathUnknownModeZeroOptions(t *testing.T) {
	pool := func() []Candidate {
		var candidates []Candidate
		for seed := uint16(901); seed <= 905; seed++ {
			candidates = append(candidates, zzBatDebate(seed))
		}
		return candidates
	}
	for _, mode := range []Mode{Mode("definitely-not-a-mode"), Mode("")} {
		label := "unknown mode " + string(mode)
		ranked := zzBatRank(t, label, pool(), zzBatRichProfile(), zzBatOpts(mode))
		zzBatAssertConservation(t, label, pool(), ranked)
	}
	zeroOptions := zzBatRank(t, "zero RankOptions", pool(), zzBatRichProfile(), RankOptions{})
	zzBatAssertConservation(t, "zero RankOptions", pool(), zeroOptions)
}
