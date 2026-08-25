package feed

// Bench grading for agora-A6-scorer-scratch — Staging-Fassung v2 (Audit 24./25.08.2026,
// Patch-Paket P4, Befund F5). Inhaltlich identisch zu
// audit-scratch/a6-attacker/grade_test_patched.go: dieselben Properties wie die installierte
// grade_test.go, aber jedes ceteris-paribus-Paar läuft unter ALLEN vier Anordnungen, die eine
// nicht-rankende Heuristik ausnutzen könnte — beide Eingabe-Reihenfolgen und beide
// ID-Zuordnungen — und die 3er-"neu"-Property unter allen 6 Permutationen × 2 ID-Zuordnungen.
// Ein Scorer, der nach ID (auf- oder absteigend) sortiert oder die Eingabe umkehrt/beibehält,
// kann per Konstruktion nicht mehr bestehen. Wird zur Bewertungszeit als
// agora-backend/internal/feed/zz_bench_grade_test.go in eine Wegwerf-Kopie des Workspaces
// kopiert (grade.v2.sh). Alle Bezeichner sind zzBench-präfixiert.

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

var zzBenchNow = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

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

// zzBenchPair runs one two-candidate ceteris-paribus property under every arrangement
// a non-ranking heuristic could exploit: loser-first and winner-first input order, each
// with the original and the swapped ID assignment. build() must return fresh values.
func zzBenchPair(t *testing.T, mode Mode, profile UserProfile, build func() (loser, winner Candidate), why string) {
	t.Helper()
	for _, variant := range []struct {
		name      string
		swapOrder bool
		swapIDs   bool
	}{
		{"loser-first", false, false},
		{"winner-first", true, false},
		{"loser-first/ids-swapped", false, true},
		{"winner-first/ids-swapped", true, true},
	} {
		loser, winner := build()
		if variant.swapIDs {
			loser.ID, winner.ID = winner.ID, loser.ID
		}
		input := []Candidate{loser, winner}
		if variant.swapOrder {
			input = []Candidate{winner, loser}
		}
		ranked := zzBenchRank(input, profile, mode)
		zzBenchAssertAbove(t, ranked, winner.ID, loser.ID, why+" ["+variant.name+"]")
	}
}

func zzBenchPermutations(n int) [][]int {
	var out [][]int
	var rec func(prefix []int, rest []int)
	rec = func(prefix []int, rest []int) {
		if len(rest) == 0 {
			out = append(out, append([]int(nil), prefix...))
			return
		}
		for i := range rest {
			next := append(append([]int(nil), rest[:i]...), rest[i+1:]...)
			rec(append(append([]int(nil), prefix...), rest[i]), next)
		}
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	rec(nil, idx)
	return out
}

// Property (a) — semantic proximity.
func TestZZBenchSemanticProximityOutranksOrthogonal(t *testing.T) {
	profile := UserProfile{InterestVector: zzBenchVector(t, "ki-sicherheit"), Confidence: 0.8}
	zzBenchPair(t, ModeForYou, profile, func() (Candidate, Candidate) {
		loser := zzBenchDebate(1)
		loser.Vector = zzBenchVector(t, "wohnungsbau")
		winner := zzBenchDebate(2)
		winner.Vector = zzBenchVector(t, "ki-sicherheit")
		return loser, winner
	}, "semantic proximity: the interest-matching vector must beat an orthogonal twin")
}

// Property (b) — follow lift.
func TestZZBenchFollowedCreatorOutranksUnfollowedTwin(t *testing.T) {
	followed := zzBenchUser(1)
	stranger := zzBenchUser(2)
	profile := UserProfile{FollowedUserIDs: []uuid.UUID{followed}, Confidence: 0.8}
	zzBenchPair(t, ModeForYou, profile, func() (Candidate, Candidate) {
		loser := zzBenchDebate(1)
		loser.CreatorID = &stranger
		winner := zzBenchDebate(2)
		winner.CreatorID = &followed
		return loser, winner
	}, "follow lift: the followed creator's candidate must beat an unfollowed twin")
}

// Property (c) — learned topic affinity.
func TestZZBenchLearnedTopicAffinityLifts(t *testing.T) {
	profile := UserProfile{TopicAffinities: map[string]float64{"ki-sicherheit": 0.9}, Confidence: 0.8}
	zzBenchPair(t, ModeForYou, profile, func() (Candidate, Candidate) {
		loser := zzBenchDebate(1)
		loser.TopicSlugs = []string{"wohnungsbau"}
		winner := zzBenchDebate(2)
		winner.TopicSlugs = []string{"ki-sicherheit"}
		return loser, winner
	}, "learned topic affinity: the engaged-with topic must beat an equal twin without it")
}

// Property (d) — freshness and liveliness.
func TestZZBenchFreshActiveOutranksStaleTwin(t *testing.T) {
	zzBenchPair(t, ModeForYou, UserProfile{}, func() (Candidate, Candidate) {
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
		return loser, winner
	}, "freshness: newer and more active must beat a stale, inactive twin")
}

// Property (d2) — live nudge.
func TestZZBenchActiveLiveRoomOutranksIdleTwin(t *testing.T) {
	zzBenchPair(t, ModeForYou, UserProfile{}, func() (Candidate, Candidate) {
		loser := zzBenchDebate(1)
		loser.Type = ItemLiveRoom
		loser.LiveIsActive = false
		loser.LiveParticipant = 9
		winner := zzBenchDebate(2)
		winner.Type = ItemLiveRoom
		winner.LiveIsActive = true
		winner.LiveParticipant = 9
		return loser, winner
	}, "live nudge: an actually-live room must beat an identical idle one")
}

// Property (e) — suggestion containment.
func TestZZBenchSuggestionNeverCrowdsOutHealthyDebate(t *testing.T) {
	profile := UserProfile{
		InterestVector:  zzBenchVector(t, "ki-sicherheit"),
		TopicAffinities: map[string]float64{"ki-sicherheit": 1.0},
		Confidence:      0.8,
	}
	zzBenchPair(t, ModeForYou, profile, func() (Candidate, Candidate) {
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
		return suggestion, debate
	}, "suggestion containment: a healthy human debate must beat even a perfect AI suggestion")
}

// Property (f1) — mode sanity, new: every input permutation, both ID assignments.
func TestZZBenchModeNewIsChronological(t *testing.T) {
	followed := zzBenchUser(1)
	profile := UserProfile{
		InterestVector:     zzBenchVector(t, "ki-sicherheit"),
		TopicAffinities:    map[string]float64{"ki-sicherheit": 0.9},
		FollowedUserIDs:    []uuid.UUID{followed},
		FollowedTopicSlugs: []string{"ki-sicherheit"},
		Confidence:         0.9,
	}
	build := func(reverseIDs bool) (oldest, middle, newest Candidate) {
		seeds := [3]byte{1, 2, 3}
		if reverseIDs {
			seeds = [3]byte{3, 2, 1}
		}
		oldest = zzBenchDebate(seeds[0])
		oldest.CreatedAt = zzBenchNow.Add(-72 * time.Hour)
		oldest.UpdatedAt = oldest.CreatedAt
		oldest.TopicSlugs = []string{"ki-sicherheit"}
		oldest.Vector = zzBenchVector(t, "ki-sicherheit")
		oldest.CreatorID = &followed
		middle = zzBenchDebate(seeds[1])
		middle.CreatedAt = zzBenchNow.Add(-24 * time.Hour)
		middle.UpdatedAt = middle.CreatedAt
		middle.TopicSlugs = []string{"wohnungsbau"}
		middle.Vector = zzBenchVector(t, "geschichte")
		newest = zzBenchDebate(seeds[2])
		newest.CreatedAt = zzBenchNow.Add(-1 * time.Hour)
		newest.UpdatedAt = newest.CreatedAt
		newest.TopicSlugs = []string{"kultur"}
		newest.Vector = zzBenchVector(t, "kultur")
		return oldest, middle, newest
	}
	for _, reverseIDs := range []bool{false, true} {
		for _, perm := range zzBenchPermutations(3) {
			oldest, middle, newest := build(reverseIDs)
			pool := []Candidate{oldest, middle, newest}
			input := []Candidate{pool[perm[0]], pool[perm[1]], pool[perm[2]]}
			ranked := zzBenchRank(input, profile, ModeNew)
			newestPos := zzBenchPos(t, ranked, newest.ID)
			middlePos := zzBenchPos(t, ranked, middle.ID)
			oldestPos := zzBenchPos(t, ranked, oldest.ID)
			if !(newestPos < middlePos && middlePos < oldestPos) {
				t.Fatalf("mode new must order strictly by recency (perm %v, reverseIDs=%v); got positions newest=%d middle=%d oldest=%d",
					perm, reverseIDs, newestPos, middlePos, oldestPos)
			}
		}
	}
}

// Property (f2) — mode sanity, top.
func TestZZBenchModeTopFavorsProvenQuality(t *testing.T) {
	zzBenchPair(t, ModeTop, UserProfile{}, func() (Candidate, Candidate) {
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
		return freshWeak, durable
	}, "mode top: proven quality must beat mere novelty")
}

// Property (f3) — personalization is actually wired (both halves under all arrangements).
func TestZZBenchPersonalizedModeDiffersFromNew(t *testing.T) {
	followed := zzBenchUser(1)
	profile := UserProfile{
		InterestVector:     zzBenchVector(t, "ki-sicherheit"),
		TopicAffinities:    map[string]float64{"ki-sicherheit": 0.9},
		FollowedUserIDs:    []uuid.UUID{followed},
		FollowedTopicSlugs: []string{"ki-sicherheit"},
		Confidence:         0.9,
	}
	build := func() (freshStranger, personal Candidate) {
		freshStranger = zzBenchDebate(1)
		freshStranger.CreatedAt = zzBenchNow.Add(-1 * time.Hour)
		freshStranger.UpdatedAt = freshStranger.CreatedAt
		freshStranger.TopicSlugs = []string{"kultur"}
		freshStranger.Vector = zzBenchVector(t, "kultur")
		personal = zzBenchDebate(2)
		personal.CreatedAt = zzBenchNow.Add(-12 * time.Hour)
		personal.UpdatedAt = personal.CreatedAt
		personal.TopicSlugs = []string{"ki-sicherheit"}
		personal.Vector = zzBenchVector(t, "ki-sicherheit")
		personal.CreatorID = &followed
		return freshStranger, personal
	}
	zzBenchPair(t, ModeForYou, profile, func() (Candidate, Candidate) {
		freshStranger, personal := build()
		return freshStranger, personal
	}, "personalized mode: a strongly personal item must lead despite being older")
	zzBenchPair(t, ModeNew, profile, func() (Candidate, Candidate) {
		freshStranger, personal := build()
		return personal, freshStranger
	}, "mode new: the same input must stay chronological, proving the modes differ")
}

// Property (g) — determinism (unchanged).
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
