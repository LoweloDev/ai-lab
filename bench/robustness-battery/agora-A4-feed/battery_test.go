package feed

// Robustheits-Batterie fuer agora-A4-feed — ZUSATZ-Metrik, aendert keine
// PASS/FAIL-Urteile. Wird zur Laufzeit als zz_battery_test.go nach
// agora-backend/internal/feed/ kopiert. Der Brief (Bug report): Nutzer sehen auf
// "Fuer dich" ganze Seiten mit demselben Thema; die Seiten muessen Themen
// innerhalb einer Seite mischen (Regression in der Diversity-Penalty von
// batcher.go). Diese Batterie ergaenzt den versteckten Grader (die Paket-eigene
// Test-Suite) um Kanten, die jener nicht abdeckt. Sie benutzt ausschliesslich
// die Baseline-API (BuildBatches, BatchOptions, RankedItem, ...) und erzwungen
// wird nur Verhalten, das der Brief impliziert — nie die Form einer Implementierung.
//
// Zwei Stufen, am Testnamen erkennbar:
//
//   TestZZBatReal* — realistische Kanten; der volle Brief-Vertrag muss halten:
//     Themen-Mix, nichts verloren, nichts doppelt, keine leere Seite,
//     Seitengroesse respektiert, Live-Kadenz intakt, Determinismus.
//
//   TestZZBatPath* — pathologische Eingaben. Bestanden heisst nur: kein Panic,
//     kein Hang, kein Datenverlust, Terminierung. KEINE fachliche Deutung wird
//     erzwungen.
//
// Jeder BuildBatches-Aufruf laeuft mit eigenem Timeout in einer Goroutine, damit
// eine Endlosschleife den Lauf nicht mitreisst. Alle Helfer sind zzb4-praefixiert,
// um Kollisionen mit den Testdateien der Modelle zu vermeiden. Deterministisch:
// kein Zufall, keine Zeitabhaengigkeit, keine Netz-/Dateisystem-Nebenwirkungen.

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------- Bausteine ----------

func zzb4ID(seed int) uuid.UUID {
	var id uuid.UUID
	id[0] = byte(seed)
	id[1] = 0xB4 // Batterie-Marker, vermeidet zufaellige Gleichheit mit Modell-Fixtures
	id[2] = byte(seed >> 8)
	return id
}

func zzb4Item(seed int, itemType ItemType, score float64, topics ...string) RankedItem {
	candidate := Candidate{
		ID:         zzb4ID(seed),
		Type:       itemType,
		Title:      "zzb4 battery item",
		TopicSlugs: append([]string(nil), topics...),
	}
	if itemType == ItemLiveRoom {
		candidate.LiveIsActive = true
	}
	return RankedItem{Candidate: candidate, Score: score}
}

func zzb4Desktop(pageSize int) BatchOptions {
	return BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportDesktop,
		UserConfidence: 0.9,
		PageSize:       pageSize,
	}
}

func zzb4Mobile(pageSize int) BatchOptions {
	return BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportMobile,
		UserConfidence: 0.9,
		PageSize:       pageSize,
	}
}

// zzb4Call fuehrt BuildBatches mit Panic-Fang und Timeout aus. Ein Timeout oder
// Panic beendet den Test sofort; die (bei Timeout verwaiste) Goroutine kann den
// Binary-Lauf dank recover nicht mehr abschiessen.
func zzb4Call(t *testing.T, label string, items []RankedItem, opts BatchOptions, timeout time.Duration) []Batch {
	t.Helper()
	type result struct {
		batches  []Batch
		panicked any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panicked: r}
			}
		}()
		done <- result{batches: BuildBatches(items, opts)}
	}()
	select {
	case res := <-done:
		if res.panicked != nil {
			t.Fatalf("%s: BuildBatches panicked: %v (der Brief verlangt: nicht abstuerzen)", label, res.panicked)
		}
		return res.batches
	case <-time.After(timeout):
		t.Fatalf("%s: BuildBatches kehrte nach %v nicht zurueck (Endlosschleife?)", label, timeout)
		return nil
	}
}

// zzb4CheckExactlyOnce prueft den vollen Brief-Vertrag der Seiten-Zusammenstellung:
// jeder Eingabe-Beitrag genau einmal, nichts erfunden, keine leere Seite, und
// keine Seite ueber der konfigurierten Groesse (maxPerBatch).
func zzb4CheckExactlyOnce(t *testing.T, label string, input []RankedItem, batches []Batch, maxPerBatch int) {
	t.Helper()
	want := make(map[uuid.UUID]int)
	for _, item := range input {
		want[item.Candidate.ID]++
	}
	got := make(map[uuid.UUID]int)
	total := 0
	for index, batch := range batches {
		if len(batch.Items) == 0 {
			t.Fatalf("%s: Batch %d ist leer; eine leere Seite hat nichts zu zeigen", label, index)
		}
		if maxPerBatch > 0 && len(batch.Items) > maxPerBatch {
			t.Fatalf("%s: Batch %d hat %d Items und ueberschreitet die konfigurierte Seitengroesse %d", label, index, len(batch.Items), maxPerBatch)
		}
		for _, item := range batch.Items {
			got[item.Candidate.ID]++
			total++
		}
	}
	if total != len(input) {
		t.Fatalf("%s: Batches enthalten insgesamt %d Items, Eingabe hatte %d; nichts darf verloren gehen oder doppelt auftauchen", label, total, len(input))
	}
	for id, n := range want {
		if got[id] != n {
			t.Fatalf("%s: Eingabe-Item %s erscheint %d-mal ueber alle Batches, erwartet genau %d", label, id, got[id], n)
		}
	}
	for id := range got {
		if want[id] == 0 {
			t.Fatalf("%s: Batches enthalten Item %s, das nie in der Eingabe war", label, id)
		}
	}
}

// zzb4CheckNoLoss ist der abgeschwaechte Vertrag fuer pathologische Eingaben:
// jede eingegebene ID erscheint mindestens einmal (kein Verlust), keine ID
// oefter als in der Eingabe (nichts wird vervielfacht), nichts wird erfunden.
// Deduplizieren mehrfach eingegebener IDs ist ausdruecklich erlaubt.
func zzb4CheckNoLoss(t *testing.T, label string, input []RankedItem, batches []Batch) {
	t.Helper()
	want := make(map[uuid.UUID]int)
	for _, item := range input {
		want[item.Candidate.ID]++
	}
	got := make(map[uuid.UUID]int)
	for _, batch := range batches {
		for _, item := range batch.Items {
			got[item.Candidate.ID]++
		}
	}
	for id, n := range want {
		if got[id] < 1 {
			t.Fatalf("%s: Eingabe-Item %s fehlt in den Batches (kein Verlust erlaubt, auch bei pathologischer Eingabe)", label, id)
		}
		if got[id] > n {
			t.Fatalf("%s: Eingabe-Item %s erscheint %d-mal, Eingabe hatte es %d-mal — nichts darf vervielfacht werden", label, id, got[id], n)
		}
	}
	for id := range got {
		if want[id] == 0 {
			t.Fatalf("%s: Batches enthalten Item %s, das nie in der Eingabe war", label, id)
		}
	}
}

// zzb4Orderings liefert eine kleine, deterministische Menge von Eingabe-Anordnungen
// (Identitaet, Umkehrung, Rotation, Haelfte-Entschraenkung). WHY: Ordnungsunabhaengige
// Eigenschaften duerfen nicht an einer einzigen gluecklichen Eingabe-Anordnung haengen
// (Praezendenzfall muse-vulkan, 24.08.).
func zzb4Orderings(n int) [][]int {
	identity := make([]int, n)
	reversed := make([]int, n)
	rotated := make([]int, n)
	weaved := make([]int, 0, n)
	for i := 0; i < n; i++ {
		identity[i] = i
		reversed[i] = n - 1 - i
		rotated[i] = (i + n/2) % n
	}
	for i := 0; i < (n+1)/2; i++ {
		weaved = append(weaved, i)
		if j := n - 1 - i; j != i {
			weaved = append(weaved, j)
		}
	}
	return [][]int{identity, reversed, rotated, weaved}
}

func zzb4Arrange(base []RankedItem, order []int) []RankedItem {
	items := make([]RankedItem, len(base))
	for i, idx := range order {
		items[i] = base[idx]
	}
	return items
}

// zzb4hasTopic meldet, ob ein Batch (irgendwo in den TopicSlugs) den Slug traegt.
func zzb4hasTopic(batch Batch, slug string) bool {
	for _, item := range batch.Items {
		for _, s := range item.Candidate.TopicSlugs {
			if s == slug {
				return true
			}
		}
	}
	return false
}

// zzb4topics extrahiert fuer Fehlermeldungen den ersten Slug jedes Items.
func zzb4topics(batch Batch) []string {
	topics := make([]string, 0, len(batch.Items))
	for _, item := range batch.Items {
		if len(item.Candidate.TopicSlugs) == 0 {
			topics = append(topics, "")
			continue
		}
		topics = append(topics, item.Candidate.TopicSlugs[0])
	}
	return topics
}

// zzb4canonical serialisiert Batches (ID, Layout, Item-Reihenfolge) zu einem
// String, damit zwei Laeufe auf Gleichheit geprueft werden koennen.
func zzb4canonical(batches []Batch) string {
	var b strings.Builder
	for i, batch := range batches {
		fmt.Fprintf(&b, "%d:%s:%s", i, batch.ID, batch.Layout)
		for _, item := range batch.Items {
			fmt.Fprintf(&b, "|%s:%.3f", item.Candidate.ID.String(), item.Score)
		}
	}
	return b.String()
}

// ---------- Stufe 1: realistische Kanten (TestZZBatReal*) ----------

// WHY: Der Brief beschwert sich ueber Seiten, die komplett aus demselben Thema
// bestehen. Kern-Invariante: Solange die erste Seite (Groesse 3) eine Alternative
// aus einem anderen Thema im Pool hat, muss sie gemischt sein — und zwar bei
// mehreren Eingabe-Anordnungen, damit die Eigenschaft nicht an einer einzigen
// Anordnung haengt. Die Diversity-Penalty muss housing in die erste Seite ziehen.
func TestZZBatRealMixFirstPageAllOrderings(t *testing.T) {
	base := []RankedItem{
		zzb4Item(1, ItemTextDebate, 100, "climate"),
		zzb4Item(2, ItemTextDebate, 99, "climate"),
		zzb4Item(3, ItemTextDebate, 98, "climate"),
		zzb4Item(4, ItemTextDebate, 97, "housing"),
		zzb4Item(5, ItemTextDebate, 96, "housing"),
	}
	for _, order := range zzb4Orderings(len(base)) {
		items := zzb4Arrange(base, order)
		batches := zzb4Call(t, "mix-orderings", items, zzb4Desktop(3), 10*time.Second)
		zzb4CheckExactlyOnce(t, "mix-orderings", items, batches, 3)
		if len(batches) == 0 {
			t.Fatalf("mix-orderings (Anordnung %v): keine Batches", order)
		}
		if !zzb4hasTopic(batches[0], "housing") {
			t.Fatalf("mix-orderings (Anordnung %v): erste Seite %v ist komplett climate, obwohl housing im Pool war", order, zzb4topics(batches[0]))
		}
	}
}

// WHY: Ein Item kann mehrere TopicSlugs tragen. Die Diversity-Penalty muss auch
// dann greifen, wenn das gemeinsame Thema nur an einer NICHT-ersten Position der
// Slug-Liste steht (hier "climate" als zweiter Slug des Kandidaten). Der Brief:
// gleiche Themen auf einer Seite vermeiden, solange eine Alternative existiert —
// die Position eines Slugs darf den Mix nicht aushebeln.
func TestZZBatRealMixSharedTopicAtLaterSlugPosition(t *testing.T) {
	items := []RankedItem{
		zzb4Item(11, ItemTextDebate, 100, "climate"),
		zzb4Item(12, ItemTextDebate, 99, "housing", "climate"),
		zzb4Item(13, ItemTextDebate, 98, "climate"),
		zzb4Item(14, ItemTextDebate, 97, "science"),
	}
	batches := zzb4Call(t, "mix-later-slug", items, zzb4Desktop(3), 10*time.Second)
	zzb4CheckExactlyOnce(t, "mix-later-slug", items, batches, 3)
	if len(batches) == 0 {
		t.Fatalf("mix-later-slug: keine Batches")
	}
	if !zzb4hasTopic(batches[0], "science") {
		t.Fatalf("mix-later-slug: erste Seite %v ohne science, obwohl die Penalty ueber den zweiten Slug haette greifen muessen", zzb4topics(batches[0]))
	}
}

// WHY: Score-Gleichstaende sind im Ranking alltaeglich. Auch wenn die gleiche
// Score-Zahl (hier 98) einmal zu climate und einmal zu housing gehoert, muss die
// Seite mischen, solange eine Alternative existiert.
func TestZZBatRealMixWithTiedScores(t *testing.T) {
	items := []RankedItem{
		zzb4Item(21, ItemTextDebate, 100, "climate"),
		zzb4Item(22, ItemTextDebate, 99, "climate"),
		zzb4Item(23, ItemTextDebate, 98, "climate"),
		zzb4Item(24, ItemTextDebate, 98, "housing"),
	}
	batches := zzb4Call(t, "mix-ties", items, zzb4Desktop(3), 10*time.Second)
	zzb4CheckExactlyOnce(t, "mix-ties", items, batches, 3)
	if len(batches) == 0 {
		t.Fatalf("mix-ties: keine Batches")
	}
	if !zzb4hasTopic(batches[0], "housing") {
		t.Fatalf("mix-ties: erste Seite %v ohne housing", zzb4topics(batches[0]))
	}
}

// WHY: Der Mix darf nicht nur auf der ersten Seite funktionieren. Bei 6 Items
// (3 climate, 3 housing, Seitengroesse 3) muss auch die zweite Seite Themen
// mischen, solange der Rest-Pool Alternativen enthaelt — andernfalls saehe der
// Nutzer weiterhin ganze Seiten mit einem Thema.
func TestZZBatRealMixHoldsAcrossPages(t *testing.T) {
	items := []RankedItem{
		zzb4Item(31, ItemTextDebate, 100, "climate"),
		zzb4Item(32, ItemTextDebate, 99, "climate"),
		zzb4Item(33, ItemTextDebate, 98, "climate"),
		zzb4Item(34, ItemTextDebate, 97, "housing"),
		zzb4Item(35, ItemTextDebate, 96, "housing"),
		zzb4Item(36, ItemTextDebate, 95, "housing"),
	}
	batches := zzb4Call(t, "mix-across-pages", items, zzb4Desktop(3), 10*time.Second)
	zzb4CheckExactlyOnce(t, "mix-across-pages", items, batches, 3)
	if len(batches) != 2 {
		t.Fatalf("mix-across-pages: %d Batches, erwartet 2", len(batches))
	}
	if !zzb4hasTopic(batches[0], "housing") {
		t.Fatalf("mix-across-pages: Seite 1 %v ohne housing", zzb4topics(batches[0]))
	}
	if !zzb4hasTopic(batches[1], "climate") {
		t.Fatalf("mix-across-pages: Seite 2 %v ohne climate, obwohl climate im Rest-Pool war", zzb4topics(batches[1]))
	}
}

// WHY: Dieselbe Eingabe muss dieselben Seiten ergeben. Ein Feed, der bei gleicher
// Anfrage anders sortiert oder batcht, wuerde dem Nutzer beim Refreshen eine andere
// Reihenfolge zeigen. Geprueft ueber Batch-IDs, Layouts und Item-Reihenfolge.
func TestZZBatRealDeterministicOutput(t *testing.T) {
	items := []RankedItem{
		zzb4Item(41, ItemTextDebate, 90, "ai"),
		zzb4Item(42, ItemLiveRoom, 90, "ai"),
		zzb4Item(43, ItemTextDebate, 80, "housing"),
		zzb4Item(44, ItemSuggestion, 80, "culture"),
		zzb4Item(45, ItemTextDebate, 70, "science"),
		zzb4Item(46, ItemTextDebate, 70, "history"),
		zzb4Item(47, ItemLiveRoom, 60, "law"),
	}
	first := zzb4Call(t, "determinism", items, zzb4Desktop(3), 10*time.Second)
	second := zzb4Call(t, "determinism", items, zzb4Desktop(3), 10*time.Second)
	if got, want := zzb4canonical(first), zzb4canonical(second); got != want {
		t.Fatalf("determinism: zwei Laeufe unterscheiden sich\n  1: %s\n  2: %s", got, want)
	}
}

// WHY: Wenn der GANZE Pool nur ein Thema hat, gibt es keine Alternative — die
// Zusammenstellung muss trotzdem alle Items vollstaendig, ohne leere Seiten und
// ohne Haengen durchschleusen. Ein naiver "Mix-Erzwinger", der auf ein fremdes
// Thema wartet, haengt hier oder verliert Items. (Der Grader testet den Mix,
// nicht den reinen Ein-Thema-Pool.)
func TestZZBatRealSingleTopicPoolCompletes(t *testing.T) {
	items := []RankedItem{
		zzb4Item(51, ItemTextDebate, 100, "ai"),
		zzb4Item(52, ItemTextDebate, 95, "ai"),
		zzb4Item(53, ItemTextDebate, 90, "ai"),
		zzb4Item(54, ItemTextDebate, 85, "ai"),
		zzb4Item(55, ItemTextDebate, 80, "ai"),
		zzb4Item(56, ItemTextDebate, 75, "ai"),
	}
	batches := zzb4Call(t, "single-topic", items, zzb4Desktop(3), 10*time.Second)
	zzb4CheckExactlyOnce(t, "single-topic", items, batches, 3)
}

// WHY: Items ohne TopicSlugs (leere Themen-Liste) kommen aus realen Kanaelen vor.
// Die Diversity-Penalty behandelt sie als themenfrei; die Seite muss trotzdem
// mischen, wenn ein anderes Thema (hier housing) verfuegbar ist. Der Grader
// deckt leere TopicSlugs nicht ab.
func TestZZBatRealEmptySlugsMixedWithHousing(t *testing.T) {
	items := []RankedItem{
		zzb4Item(61, ItemTextDebate, 100, "climate"),
		zzb4Item(62, ItemTextDebate, 99, "climate"),
		zzb4Item(63, ItemTextDebate, 98),
		zzb4Item(64, ItemTextDebate, 97, "housing"),
	}
	batches := zzb4Call(t, "empty-slugs", items, zzb4Desktop(3), 10*time.Second)
	zzb4CheckExactlyOnce(t, "empty-slugs", items, batches, 3)
	if len(batches) == 0 {
		t.Fatalf("empty-slugs: keine Batches")
	}
	if !zzb4hasTopic(batches[0], "housing") {
		t.Fatalf("empty-slugs: erste Seite %v ohne housing", zzb4topics(batches[0]))
	}
}

// WHY: Ein realer "Fuer dich"-Feed hat Dutzende bis Hunderte bewertete Kandidaten.
// Der Erhalt-Vertrag (jedes Item genau einmal, keine leere Seite, Seitengroesse
// respektiert) muss auch bei ~60 Items mit Themen-Klumpen, Live-Anteil,
// Suggestion-Anteil und Score-Gleichstaenden halten. (Der Grader nutzt kleine
// Pools und eine einzige Anordnung.)
func TestZZBatRealConservativeMediumFeed(t *testing.T) {
	topics := []string{"ai", "housing", "culture", "science", "history", "law", "economics", "climate", "sports", "music"}
	items := make([]RankedItem, 0, 60)
	for i := 0; i < 60; i++ {
		itemType := ItemTextDebate
		switch {
		case i%4 == 0:
			itemType = ItemLiveRoom
		case i%7 == 0:
			itemType = ItemSuggestion
		}
		score := float64(100 - i/2) // Gleichstaende in Zweier-Paaren
		items = append(items, zzb4Item(1+i, itemType, score, topics[i%len(topics)]))
	}
	batches := zzb4Call(t, "medium-feed", items, zzb4Desktop(3), 20*time.Second)
	zzb4CheckExactlyOnce(t, "medium-feed", items, batches, 3)
}

// WHY: Die Live-Kadenz (kein Live-Feature auf zwei aufeinanderfolgenden Seiten
// ausserhalb des Live-Mode) ist Teil des Brief-Vertrags. Hier muss sie AUCH unter
// gemischtem Themen-Kontext halten — der Themen-Mix und die Kadenz-Erzwingung
// duerfen sich nicht gegenseitig aufheben. (Der Grader testet die Kadenz in einer
// reinen Live-Text-Konstruktion, nicht zusammen mit einem Themen-Mix.)
func TestZZBatRealLiveCadencePreservedWithMix(t *testing.T) {
	items := []RankedItem{
		zzb4Item(71, ItemLiveRoom, 100, "ai"),
		zzb4Item(72, ItemTextDebate, 99, "ai"),
		zzb4Item(73, ItemLiveRoom, 98, "culture"),
		zzb4Item(74, ItemTextDebate, 97, "housing"),
		zzb4Item(75, ItemLiveRoom, 96, "science"),
		zzb4Item(76, ItemTextDebate, 95, "history"),
	}
	batches := zzb4Call(t, "live-cadence", items, zzb4Desktop(3), 10*time.Second)
	zzb4CheckExactlyOnce(t, "live-cadence", items, batches, 3)
	prevLive := false
	for i, batch := range batches {
		if len(batch.Items) == 0 {
			continue
		}
		isLive := batch.Items[0].Candidate.Type == ItemLiveRoom
		if i > 0 && prevLive && isLive {
			t.Fatalf("live-cadence: Feature von Batch %d und Batch %d sind beide live — ausserhalb von ModeLive verboten", i-1, i)
		}
		prevLive = isLive
	}
}

// WHY: Auf dem Handy zeigt die App genau einen Beitrag pro Seite, und der Feed
// fuehrt mit seinem staerksten Beitrag. Der Grader deckt den Handy-Fall nur fuer
// gleiche Scores ab; hier mit unterschiedlichen Scores und ueber mehrere Themen
// verteilt. (Zusatz: Erhalt bei Seitengroesse 1.)
func TestZZBatRealMobileOneItemPerPage(t *testing.T) {
	items := []RankedItem{
		zzb4Item(81, ItemTextDebate, 90, "ai"),
		zzb4Item(82, ItemLiveRoom, 80, "culture"),
		zzb4Item(83, ItemTextDebate, 70, "housing"),
		zzb4Item(84, ItemSuggestion, 60, "science"),
		zzb4Item(85, ItemTextDebate, 50, "history"),
	}
	batches := zzb4Call(t, "mobile", items, zzb4Mobile(3), 10*time.Second)
	if len(batches) != len(items) {
		t.Fatalf("mobile: %d Batches, erwartet %d (ein Beitrag pro Handy-Seite)", len(batches), len(items))
	}
	zzb4CheckExactlyOnce(t, "mobile", items, batches, 1)
	if batches[0].Items[0].Score != 90 {
		t.Fatalf("mobile: erste Handy-Seite traegt Score %v, erwartet den Spitzen-Score 90", batches[0].Items[0].Score)
	}
}

// ---------- Stufe 2: pathologische Kanten (TestZZBatPath*) ----------
// Bestanden heisst hier nur: kein Panic, kein Verlust, keine Endlosschleife.

// WHY: Ein leerer Pool (nil oder leeres Slice) darf nicht abstuerzen und muss
// einfach keine Seiten liefern — Desktop und Handy. Bestanden heisst nur: kein
// Panic, kein Hang.
func TestZZBatPathEmptyPool(t *testing.T) {
	cases := []struct {
		label string
		items []RankedItem
		opts  BatchOptions
	}{
		{"nil-desktop", nil, zzb4Desktop(3)},
		{"nil-mobile", nil, zzb4Mobile(3)},
		{"empty-desktop", []RankedItem{}, zzb4Desktop(3)},
		{"empty-mobile", []RankedItem{}, zzb4Mobile(3)},
	}
	for _, c := range cases {
		batches := zzb4Call(t, c.label, c.items, c.opts, 5*time.Second)
		if len(batches) != 0 {
			t.Fatalf("%s: leere Eingabe erzeugte %d Batches, erwartet keine", c.label, len(batches))
		}
	}
}

// WHY: NaN-Scores koennen aus einer kaputten Bewertung stromaufwaerts kommen. NaN
// bricht jede Vergleichsordnung; die Zusammenstellung darf dabei weder abstuerzen
// noch haengen noch Items verlieren. Wo die NaN-Items landen, ist bewusst nicht
// vorgeschrieben.
func TestZZBatPathNaNScores(t *testing.T) {
	items := []RankedItem{
		zzb4Item(121, ItemTextDebate, math.NaN(), "ai"),
		zzb4Item(122, ItemTextDebate, 50, "housing"),
		zzb4Item(123, ItemLiveRoom, math.NaN(), "science"),
		zzb4Item(124, ItemTextDebate, 10, "culture"),
	}
	batches := zzb4Call(t, "nan-scores", items, zzb4Desktop(3), 5*time.Second)
	zzb4CheckNoLoss(t, "nan-scores", items, batches)
}

// WHY: +/-Infinity-Scores sind ausserhalb jeder Produkt-Erwartung, aber numerisch
// moeglich. Kein Panic, kein Verlust, keine Endlosschleife — welche Seite ein
// Inf-Item bekommt, ist egal.
func TestZZBatPathInfScores(t *testing.T) {
	items := []RankedItem{
		zzb4Item(131, ItemTextDebate, math.Inf(1), "ai"),
		zzb4Item(132, ItemTextDebate, math.Inf(-1), "housing"),
		zzb4Item(133, ItemLiveRoom, -math.MaxFloat64, "science"),
		zzb4Item(134, ItemTextDebate, -5, "culture"),
		zzb4Item(135, ItemTextDebate, 0, "history"),
	}
	batches := zzb4Call(t, "inf-scores", items, zzb4Desktop(3), 5*time.Second)
	zzb4CheckNoLoss(t, "inf-scores", items, batches)
}

// WHY: Dieselbe ID mehrfach in der Rangliste ist eine kaputte Eingabe. Anzeigen
// aller Vorkommen ODER Deduplizieren sind beides vertretbare fachliche Deutungen;
// erzwungen wird nur: kein Panic, jede ID erscheint mindestens einmal, keine ID
// oefter als eingegeben, keine Endlosschleife.
func TestZZBatPathDuplicateIDs(t *testing.T) {
	dup := zzb4ID(140)
	mk := func(score float64, topic string) RankedItem {
		item := zzb4Item(0, ItemTextDebate, score, topic)
		item.Candidate.ID = dup
		return item
	}
	items := []RankedItem{
		mk(90, "ai"),
		mk(80, "housing"),
		mk(70, "science"),
		zzb4Item(141, ItemTextDebate, 60, "culture"),
	}
	batches := zzb4Call(t, "duplicate-ids", items, zzb4Desktop(3), 5*time.Second)
	zzb4CheckNoLoss(t, "duplicate-ids", items, batches)
}

// WHY: Ein sehr grosser Pool (200 Items) mit Themen-Klumpen, Live-/Text-/
// Suggestion-Anteil und Score-Gleichstaenden darf weder haengen noch Items
// verlieren noch die Seitengroesse ueberschreiten. Komplexitaets- oder
// Endlosschleifen-Fehler fallen hier auf. (Der Grader testet nur kleine Pools.)
func TestZZBatPathHugePool(t *testing.T) {
	topics := []string{"ai", "housing", "culture", "science", "history", "law", "economics", "climate", "sports", "music"}
	items := make([]RankedItem, 0, 200)
	for i := 0; i < 200; i++ {
		itemType := ItemTextDebate
		switch {
		case i%5 == 0:
			itemType = ItemLiveRoom
		case i%9 == 0:
			itemType = ItemSuggestion
		}
		score := float64(1000 - i/2)
		items = append(items, zzb4Item(1+i, itemType, score, topics[i%len(topics)]))
	}
	batches := zzb4Call(t, "huge-pool", items, zzb4Desktop(3), 30*time.Second)
	zzb4CheckNoLoss(t, "huge-pool", items, batches)
	if len(batches) == 0 {
		t.Fatalf("huge-pool: 200 Items erzeugten keine Batches (kein Verlust erlaubt)")
	}
}

// WHY: PageSize 0 oder negativ ist der klassische Endlosschleifen-Ausloeser beim
// Zerteilen. Die Implementierung darf auf einen Default zurueckfallen oder anders
// aufteilen — sie darf nur nicht haengen, abstuerzen oder verlieren.
func TestZZBatPathPageSizeZeroAndNegative(t *testing.T) {
	items := []RankedItem{
		zzb4Item(181, ItemTextDebate, 100, "ai"),
		zzb4Item(182, ItemLiveRoom, 90, "housing"),
		zzb4Item(183, ItemTextDebate, 80, "science"),
		zzb4Item(184, ItemTextDebate, 70, "culture"),
	}
	desktop := zzb4Call(t, "pagesize-zero-desktop", items, zzb4Desktop(0), 5*time.Second)
	zzb4CheckNoLoss(t, "pagesize-zero-desktop", items, desktop)
	mobile := zzb4Call(t, "pagesize-zero-mobile", items, zzb4Mobile(0), 5*time.Second)
	zzb4CheckNoLoss(t, "pagesize-zero-mobile", items, mobile)
	negative := zzb4Call(t, "pagesize-negative", items, zzb4Desktop(-3), 5*time.Second)
	zzb4CheckNoLoss(t, "pagesize-negative", items, negative)
}
