package feed

// Bench grading for agora-A6-scorer-scratch. This file is copied into
// agora-backend/internal/feed/ as zz_bench_grade_test.go at grade time and removed
// afterwards. Every property below is deliberately design-agnostic: it encodes only
// behavior that any reasonable reading of the product brief implies (semantic
// proximity, follow lift, learned topic affinity, freshness/liveliness, the live
// nudge, suggestion containment, mode sanity, determinism) and never the shape of
// one particular formula. All comparisons are ceteris-paribus and RELATIVE (never
// absolute scores) with maximal signal contrast, so any sane weighting passes.
// Embedding vectors come from an offline, deterministic fake provider that maps
// texts onto an orthonormal basis (cosine exactly 1 or exactly 0) — no real
// embedding model, no network, anywhere. All identifiers are zzBench-prefixed to
// avoid colliding with helpers the agent may have defined in their own test files.

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

var zzBenchNow = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

// zzBenchFakeEmbedder is a deterministic, offline EmbeddingProvider: every known
// text maps to a fixed axis of an orthonormal basis, so cosine similarity between
// any two bench vectors is exactly 1 (same text) or exactly 0 (different texts).
type zzBenchFakeEmbedder struct{}

func (zzBenchFakeEmbedder) Name() string  { return "bench-fake" }
func (zzBenchFakeEmbedder) Model() string { return "bench-orthonormal-v1" }
func (zzBenchFakeEmbedder) Dims() int     { return 4 }

func (zzBenchFakeEmbedder) Vectorize(text string) ([]float64, error) {
	axes := map[string]int{
		"ki-sicherheit": 0,
		"wohnungsbau":   1,
		"kultur":        2,
		"geschichte":    3,
	}
	axis, ok := axes[text]
	if !ok {
		return nil, fmt.Errorf("zzBenchFakeEmbedder: unknown bench text %q", text)
	}
	vector := make([]float64, 4)
	vector[axis] = 1
	return vector, nil
}

var _ EmbeddingProvider = zzBenchFakeEmbedder{}

func zzBenchVector(t *testing.T, text string) []float64 {
	t.Helper()
	vector, err := zzBenchFakeEmbedder{}.Vectorize(text)
	if err != nil {
		t.Fatalf("fake embedding failed: %v", err)
	}
	return vector
}

func zzBenchID(seed byte) uuid.UUID {
	var id uuid.UUID
	id[0] = 0xB6
	id[15] = seed
	return id
}

func zzBenchUser(seed byte) uuid.UUID {
	var id uuid.UUID
	id[0] = 0x5E
	id[15] = seed
	return id
}

// zzBenchDebate is the ceteris-paribus baseline: each test varies exactly the signal
// under test and nothing else, so only that signal can explain a rank difference.
func zzBenchDebate(seed byte) Candidate {
	return Candidate{
		ID:            zzBenchID(seed),
		Type:          ItemTextDebate,
		Title:         "bench debate",
		Thesis:        "bench thesis",
		CreatedAt:     zzBenchNow.Add(-24 * time.Hour),
		UpdatedAt:     zzBenchNow.Add(-3 * time.Hour),
		ActivityCount: 10,
		QualityScore:  50,
	}
}

func zzBenchRank(candidates []Candidate, profile UserProfile, mode Mode) []RankedItem {
	return RankCandidates(candidates, profile, RankOptions{Mode: mode, Now: zzBenchNow, Seed: 7})
}

func zzBenchPos(t *testing.T, ranked []RankedItem, id uuid.UUID) int {
	t.Helper()
	for index, item := range ranked {
		if item.Candidate.ID == id {
			return index
		}
	}
	t.Fatalf("candidate %s is missing from the ranked output; ranking must not drop candidates", id)
	return -1
}

func zzBenchAssertAbove(t *testing.T, ranked []RankedItem, winner, loser uuid.UUID, why string) {
	t.Helper()
	winnerPos := zzBenchPos(t, ranked, winner)
	loserPos := zzBenchPos(t, ranked, loser)
	if winnerPos >= loserPos {
		t.Fatalf("%s: expected %s (pos %d) above %s (pos %d)", why, winner, winnerPos, loser, loserPos)
	}
}

// NOTE on input order: in every property the expected LOSER comes first in the input
// slice, so a no-op or naive scorer that preserves input order on ties fails loudly.

// Property (a) — semantic proximity. WHY: the brief says what lies contentually close
// to the person's interests (that is what the vectors are for) belongs on top.
func TestZZBenchSemanticProximityOutranksOrthogonal(t *testing.T) {
	loser := zzBenchDebate(1)
	loser.Vector = zzBenchVector(t, "wohnungsbau")
	winner := zzBenchDebate(2)
	winner.Vector = zzBenchVector(t, "ki-sicherheit")

	profile := UserProfile{
		InterestVector: zzBenchVector(t, "ki-sicherheit"),
		Confidence:     0.8,
	}

	ranked := zzBenchRank([]Candidate{loser, winner}, profile, ModeForYou)
	zzBenchAssertAbove(t, ranked, winner.ID, loser.ID,
		"semantic proximity: the interest-matching vector must beat an orthogonal twin")
}

// Property (b) — follow lift. WHY: the brief says people I follow count; two identical
// candidates differing only in creator must rank the followed creator's one first.
func TestZZBenchFollowedCreatorOutranksUnfollowedTwin(t *testing.T) {
	followed := zzBenchUser(1)
	stranger := zzBenchUser(2)

	loser := zzBenchDebate(1)
	loser.CreatorID = &stranger
	winner := zzBenchDebate(2)
	winner.CreatorID = &followed

	profile := UserProfile{
		FollowedUserIDs: []uuid.UUID{followed},
		Confidence:      0.8,
	}

	ranked := zzBenchRank([]Candidate{loser, winner}, profile, ModeForYou)
	zzBenchAssertAbove(t, ranked, winner.ID, loser.ID,
		"follow lift: the followed creator's candidate must beat an unfollowed twin")
}

// Property (c) — learned topic affinity. WHY: the brief says topics I keep engaging
// with grow on me; a strong recorded affinity for a topic must lift that topic's
// candidate above an otherwise equal candidate on an unrecorded topic.
func TestZZBenchLearnedTopicAffinityLifts(t *testing.T) {
	loser := zzBenchDebate(1)
	loser.TopicSlugs = []string{"wohnungsbau"}
	winner := zzBenchDebate(2)
	winner.TopicSlugs = []string{"ki-sicherheit"}

	profile := UserProfile{
		TopicAffinities: map[string]float64{"ki-sicherheit": 0.9},
		Confidence:      0.8,
	}

	ranked := zzBenchRank([]Candidate{loser, winner}, profile, ModeForYou)
	zzBenchAssertAbove(t, ranked, winner.ID, loser.ID,
		"learned topic affinity: the engaged-with topic must beat an equal twin without it")
}

// Property (d) — freshness and liveliness. WHY: the brief says fresh and lively beats
// old and dormant; a newer, more active candidate must outrank its stale, inactive twin.
func TestZZBenchFreshActiveOutranksStaleTwin(t *testing.T) {
	loser := zzBenchDebate(1)
	loser.TopicSlugs = []string{"kultur"}
	loser.CreatedAt = zzBenchNow.Add(-30 * 24 * time.Hour)
	loser.UpdatedAt = zzBenchNow.Add(-30 * 24 * time.Hour)
	loser.ActivityCount = 0

	winner := zzBenchDebate(2)
	winner.TopicSlugs = []string{"kultur"}
	winner.CreatedAt = zzBenchNow.Add(-2 * time.Hour)
	winner.UpdatedAt = zzBenchNow.Add(-1 * time.Hour)
	winner.ActivityCount = 60

	ranked := zzBenchRank([]Candidate{loser, winner}, UserProfile{}, ModeForYou)
	zzBenchAssertAbove(t, ranked, winner.ID, loser.ID,
		"freshness: newer and more active must beat a stale, inactive twin")
}

// Property (d2) — live nudge. WHY: the brief says a live room where something is
// actually happening right now deserves a nudge over one that just stands around.
func TestZZBenchActiveLiveRoomOutranksIdleTwin(t *testing.T) {
	loser := zzBenchDebate(1)
	loser.Type = ItemLiveRoom
	loser.LiveIsActive = false
	loser.LiveParticipant = 9

	winner := zzBenchDebate(2)
	winner.Type = ItemLiveRoom
	winner.LiveIsActive = true
	winner.LiveParticipant = 9

	ranked := zzBenchRank([]Candidate{loser, winner}, UserProfile{}, ModeForYou)
	zzBenchAssertAbove(t, ranked, winner.ID, loser.ID,
		"live nudge: an actually-live room must beat an identical idle one")
}

// Property (e) — suggestion containment. WHY: the brief says AI suggestions are
// seasoning, not the main course: even a maximally on-interest, brand-new suggestion
// must never crowd out a healthy, active human debate of comparable relevance.
func TestZZBenchSuggestionNeverCrowdsOutHealthyDebate(t *testing.T) {
	suggestion := zzBenchDebate(1)
	suggestion.Type = ItemSuggestion
	suggestion.TopicSlugs = []string{"ki-sicherheit"}
	suggestion.Vector = zzBenchVector(t, "ki-sicherheit")
	suggestion.CreatedAt = zzBenchNow.Add(-5 * time.Minute)
	suggestion.UpdatedAt = zzBenchNow.Add(-5 * time.Minute)
	suggestion.ActivityCount = 0
	suggestion.QualityScore = 0

	debate := zzBenchDebate(2)
	debate.TopicSlugs = []string{"ki-sicherheit"}
	debate.Vector = zzBenchVector(t, "ki-sicherheit")
	debate.CreatedAt = zzBenchNow.Add(-6 * time.Hour)
	debate.UpdatedAt = zzBenchNow.Add(-1 * time.Hour)
	debate.ActivityCount = 40
	debate.QualityScore = 80

	profile := UserProfile{
		InterestVector:  zzBenchVector(t, "ki-sicherheit"),
		TopicAffinities: map[string]float64{"ki-sicherheit": 1.0},
		Confidence:      0.8,
	}

	ranked := zzBenchRank([]Candidate{suggestion, debate}, profile, ModeForYou)
	zzBenchAssertAbove(t, ranked, debate.ID, suggestion.ID,
		"suggestion containment: a healthy human debate must beat even a perfect AI suggestion")
}

// Property (f1) — mode sanity, new. WHY: the brief says "neu" is plainly chronological
// (newest first) — even a maximal personalization pull on the oldest item must not
// reorder it.
func TestZZBenchModeNewIsChronological(t *testing.T) {
	followed := zzBenchUser(1)

	oldest := zzBenchDebate(1)
	oldest.CreatedAt = zzBenchNow.Add(-72 * time.Hour)
	oldest.UpdatedAt = oldest.CreatedAt
	oldest.TopicSlugs = []string{"ki-sicherheit"}
	oldest.Vector = zzBenchVector(t, "ki-sicherheit")
	oldest.CreatorID = &followed

	middle := zzBenchDebate(2)
	middle.CreatedAt = zzBenchNow.Add(-24 * time.Hour)
	middle.UpdatedAt = middle.CreatedAt
	middle.TopicSlugs = []string{"wohnungsbau"}
	middle.Vector = zzBenchVector(t, "geschichte")

	newest := zzBenchDebate(3)
	newest.CreatedAt = zzBenchNow.Add(-1 * time.Hour)
	newest.UpdatedAt = newest.CreatedAt
	newest.TopicSlugs = []string{"kultur"}
	newest.Vector = zzBenchVector(t, "kultur")

	profile := UserProfile{
		InterestVector:     zzBenchVector(t, "ki-sicherheit"),
		TopicAffinities:    map[string]float64{"ki-sicherheit": 0.9},
		FollowedUserIDs:    []uuid.UUID{followed},
		FollowedTopicSlugs: []string{"ki-sicherheit"},
		Confidence:         0.9,
	}

	ranked := zzBenchRank([]Candidate{oldest, newest, middle}, profile, ModeNew)
	newestPos := zzBenchPos(t, ranked, newest.ID)
	middlePos := zzBenchPos(t, ranked, middle.ID)
	oldestPos := zzBenchPos(t, ranked, oldest.ID)
	if !(newestPos < middlePos && middlePos < oldestPos) {
		t.Fatalf("mode new must order strictly by recency; got positions newest=%d middle=%d oldest=%d",
			newestPos, middlePos, oldestPos)
	}
}

// Property (f2) — mode sanity, top. WHY: the brief says "top" shows what has proven
// itself — quality beats mere novelty there.
func TestZZBenchModeTopFavorsProvenQuality(t *testing.T) {
	freshWeak := zzBenchDebate(1)
	freshWeak.CreatedAt = zzBenchNow.Add(-30 * time.Minute)
	freshWeak.UpdatedAt = freshWeak.CreatedAt
	freshWeak.QualityScore = 35
	freshWeak.ActivityCount = 2

	durable := zzBenchDebate(2)
	durable.CreatedAt = zzBenchNow.Add(-30 * 24 * time.Hour)
	durable.UpdatedAt = durable.CreatedAt
	durable.QualityScore = 96
	durable.ActivityCount = 80

	ranked := zzBenchRank([]Candidate{freshWeak, durable}, UserProfile{}, ModeTop)
	zzBenchAssertAbove(t, ranked, durable.ID, freshWeak.ID,
		"mode top: proven quality must beat mere novelty")
}

// Property (f3) — personalization is actually wired. WHY: the brief says "für dich"
// must visibly sort differently from "neu" for a person with a profile: a strongly
// personal but somewhat older item leads the personal feed, while "neu" stays
// chronological on the very same input.
func TestZZBenchPersonalizedModeDiffersFromNew(t *testing.T) {
	followed := zzBenchUser(1)

	freshStranger := zzBenchDebate(1)
	freshStranger.CreatedAt = zzBenchNow.Add(-1 * time.Hour)
	freshStranger.UpdatedAt = freshStranger.CreatedAt
	freshStranger.TopicSlugs = []string{"kultur"}
	freshStranger.Vector = zzBenchVector(t, "kultur")

	personal := zzBenchDebate(2)
	personal.CreatedAt = zzBenchNow.Add(-12 * time.Hour)
	personal.UpdatedAt = personal.CreatedAt
	personal.TopicSlugs = []string{"ki-sicherheit"}
	personal.Vector = zzBenchVector(t, "ki-sicherheit")
	personal.CreatorID = &followed

	profile := UserProfile{
		InterestVector:     zzBenchVector(t, "ki-sicherheit"),
		TopicAffinities:    map[string]float64{"ki-sicherheit": 0.9},
		FollowedUserIDs:    []uuid.UUID{followed},
		FollowedTopicSlugs: []string{"ki-sicherheit"},
		Confidence:         0.9,
	}

	forYou := zzBenchRank([]Candidate{freshStranger, personal}, profile, ModeForYou)
	zzBenchAssertAbove(t, forYou, personal.ID, freshStranger.ID,
		"personalized mode: a strongly personal item must lead despite being older")

	chronological := zzBenchRank([]Candidate{freshStranger, personal}, profile, ModeNew)
	zzBenchAssertAbove(t, chronological, freshStranger.ID, personal.ID,
		"mode new: the same input must stay chronological, proving the modes differ")
}

// Property (g) — determinism. WHY: the brief says identical inputs must produce the
// identical order, or the feed jumps when the next page loads.
func TestZZBenchSameInputsSameOrder(t *testing.T) {
	pool := func() []Candidate {
		followed := zzBenchUser(1)
		semantic := zzBenchDebate(1)
		semantic.Vector = zzBenchVector(t, "ki-sicherheit")
		fromFollowed := zzBenchDebate(2)
		fromFollowed.CreatorID = &followed
		live := zzBenchDebate(3)
		live.Type = ItemLiveRoom
		live.LiveIsActive = true
		live.LiveParticipant = 7
		suggestion := zzBenchDebate(4)
		suggestion.Type = ItemSuggestion
		suggestion.TopicSlugs = []string{"ki-sicherheit"}
		stale := zzBenchDebate(5)
		stale.CreatedAt = zzBenchNow.Add(-20 * 24 * time.Hour)
		stale.UpdatedAt = stale.CreatedAt
		plain := zzBenchDebate(6)
		return []Candidate{semantic, fromFollowed, live, suggestion, stale, plain}
	}
	profile := func() UserProfile {
		return UserProfile{
			InterestVector:  zzBenchVector(t, "ki-sicherheit"),
			TopicAffinities: map[string]float64{"ki-sicherheit": 0.7, "kultur": -0.2},
			FollowedUserIDs: []uuid.UUID{zzBenchUser(1)},
			Confidence:      0.8,
		}
	}

	first := zzBenchRank(pool(), profile(), ModeForYou)
	second := zzBenchRank(pool(), profile(), ModeForYou)

	if len(first) != len(second) {
		t.Fatalf("same inputs produced different result lengths: %d vs %d", len(first), len(second))
	}
	for index := range first {
		if first[index].Candidate.ID != second[index].Candidate.ID {
			t.Fatalf("same inputs produced a different order at position %d: %s vs %s",
				index, first[index].Candidate.ID, second[index].Candidate.ID)
		}
	}
}
