package feed

// Robustheits-Batterie für agora-A5-batcher-scratch — ZUSATZ-Metrik, ändert keine
// PASS/FAIL-Urteile. Diese Datei wird zur Laufzeit als zz_battery_test.go nach
// agora-backend/internal/feed/ kopiert und danach wieder entfernt.
//
// Designregeln (wie bei den versteckten Grade-Properties): Jede Prüfung kodiert
// ausschließlich Verhalten, das der Produkt-Brief impliziert — niemals die Form einer
// bestimmten Implementierung. Zwei Stufen, am Testnamen erkennbar:
//
//   TestZZBatReal* — realistische Kanten. Der volle Brief-Vertrag muss halten:
//     nichts verloren, nichts doppelt, keine leere Seite, Seitengröße respektiert,
//     Handy zeigt einen Beitrag pro Seite.
//
//   TestZZBatPath* — pathologische Eingaben. Bestanden heißt nur: kein Panic,
//     kein Verlust, keine Endlosschleife. KEINE bestimmte fachliche Deutung wird
//     erzwungen (z. B. darf eine Implementierung Duplikat-IDs deduplizieren oder
//     behalten; PageSize 0 darf sie auf einen Default oder eine große Seite abbilden).
//
// Jeder Aufruf von BuildBatches läuft mit eigenem Timeout in einer Goroutine, damit
// eine Endlosschleife den Testlauf nicht mitreißt. Alle Bezeichner sind zzbat-
// präfixiert, um Kollisionen mit eigenen Testdateien der Modelle zu vermeiden.

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------- Bausteine ----------

func zzbatID(seed int) uuid.UUID {
	var id uuid.UUID
	id[0] = byte(seed)
	id[1] = byte(seed >> 8)
	id[2] = 0xB7 // Batterie-Marker, vermeidet zufällige Gleichheit mit Modell-Fixtures
	return id
}

func zzbatItem(seed int, itemType ItemType, score float64, topic string) RankedItem {
	candidate := Candidate{
		ID:         zzbatID(seed),
		Type:       itemType,
		Title:      "battery item",
		TopicSlugs: []string{topic},
	}
	if itemType == ItemLiveRoom {
		candidate.LiveIsActive = true
	}
	return RankedItem{Candidate: candidate, Score: score}
}

func zzbatDesktop(pageSize int) BatchOptions {
	return BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportDesktop,
		UserConfidence: 0.9,
		PageSize:       pageSize,
	}
}

func zzbatMobile(pageSize int) BatchOptions {
	return BatchOptions{
		Mode:           ModeForYou,
		Viewport:       ViewportMobile,
		UserConfidence: 0.9,
		PageSize:       pageSize,
	}
}

// zzbatCall führt BuildBatches mit Panic-Fang und Timeout aus. Ein Timeout oder Panic
// beendet den Test sofort; die (bei Timeout verwaiste) Goroutine kann den Binary-Lauf
// dank recover nicht mehr abschießen.
func zzbatCall(t *testing.T, label string, items []RankedItem, opts BatchOptions, timeout time.Duration) []Batch {
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
			t.Fatalf("%s: BuildBatches panicked: %v (der Brief verlangt: nicht abstürzen)", label, res.panicked)
		}
		return res.batches
	case <-time.After(timeout):
		t.Fatalf("%s: BuildBatches kehrte nach %v nicht zurück (Endlosschleife?)", label, timeout)
		return nil
	}
}

// zzbatCheckExactlyOnce prüft den vollen Brief-Vertrag der Seiten-Zusammenstellung:
// jeder Eingabe-Beitrag genau einmal, nichts erfunden, keine leere Seite, und (falls
// maxPerBatch > 0) keine Seite über der konfigurierten Größe.
func zzbatCheckExactlyOnce(t *testing.T, label string, input []RankedItem, batches []Batch, maxPerBatch int) {
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
			t.Fatalf("%s: Batch %d hat %d Items und überschreitet die konfigurierte Seitengröße %d", label, index, len(batch.Items), maxPerBatch)
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
			t.Fatalf("%s: Eingabe-Item %s erscheint %d-mal über alle Batches, erwartet genau %d", label, id, got[id], n)
		}
	}
	for id := range got {
		if want[id] == 0 {
			t.Fatalf("%s: Batches enthalten Item %s, das nie in der Eingabe war", label, id)
		}
	}
}

// zzbatCheckNoLoss ist der abgeschwächte Vertrag für pathologische Eingaben:
// jede eingegebene ID erscheint mindestens einmal (kein Verlust), keine ID erscheint
// öfter als in der Eingabe (nichts wird vervielfacht), nichts wird erfunden.
// Deduplizieren mehrfach eingegebener IDs ist ausdrücklich erlaubt.
func zzbatCheckNoLoss(t *testing.T, label string, input []RankedItem, batches []Batch) {
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

// zzbatOrderings liefert eine kleine, deterministische Menge von Eingabe-Anordnungen
// (Identität, Umkehrung, Rotation, Hälften-Verschränkung). WHY: Ordnungsunabhängige
// Eigenschaften dürfen nicht an einer einzigen glücklichen Eingabe-Anordnung hängen
// (Präzedenzfall muse-vulkan, 24.08.).
func zzbatOrderings(n int) [][]int {
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

func zzbatArrange(base []RankedItem, order []int) []RankedItem {
	items := make([]RankedItem, len(base))
	for i, idx := range order {
		items[i] = base[idx]
	}
	return items
}

// ---------- Stufe 1: realistische Kanten (TestZZBatReal*) ----------

// WHY: Score-Gleichstände sind im echten Ranking alltäglich. Der Brief verlangt auch
// dann: nichts verloren, nichts doppelt, keine leere Seite, Seitengröße respektiert.
// Keine Annahme über die Reihenfolge innerhalb eines Gleichstands.
func TestZZBatRealScoreTiesDesktopConserved(t *testing.T) {
	base := []RankedItem{
		zzbatItem(1, ItemTextDebate, 90, "ai"),
		zzbatItem(2, ItemTextDebate, 90, "housing"),
		zzbatItem(3, ItemLiveRoom, 90, "science"),
		zzbatItem(4, ItemTextDebate, 80, "culture"),
		zzbatItem(5, ItemTextDebate, 80, "ai"),
		zzbatItem(6, ItemSuggestion, 80, "law"),
		zzbatItem(7, ItemTextDebate, 70, "history"),
		zzbatItem(8, ItemTextDebate, 70, "economics"),
	}
	for _, order := range zzbatOrderings(len(base)) {
		items := zzbatArrange(base, order)
		batches := zzbatCall(t, "ties-desktop", items, zzbatDesktop(3), 10*time.Second)
		zzbatCheckExactlyOnce(t, "ties-desktop", items, batches, 3)
	}
}

// WHY: Auf dem Handy sehen die Leute genau einen Beitrag pro Seite, und ein bewerteter
// Feed führt mit seinem stärksten Beitrag. Bei einem Gleichstand an der Spitze ist
// jeder der punktgleichen Beiträge ein legitimer Anfang — erzwungen wird nur der
// Spitzen-SCORE, nicht ein bestimmtes Item. Gilt für jede Eingabe-Anordnung.
func TestZZBatRealScoreTiesMobileLeadsWithTopScore(t *testing.T) {
	base := []RankedItem{
		zzbatItem(11, ItemTextDebate, 90, "ai"),
		zzbatItem(12, ItemTextDebate, 90, "housing"),
		zzbatItem(13, ItemLiveRoom, 70, "science"),
		zzbatItem(14, ItemTextDebate, 60, "culture"),
	}
	for _, order := range zzbatOrderings(len(base)) {
		items := zzbatArrange(base, order)
		batches := zzbatCall(t, "ties-mobile", items, zzbatMobile(3), 10*time.Second)
		if len(batches) != len(items) {
			t.Fatalf("ties-mobile (Anordnung %v): %d Batches, erwartet %d (ein Beitrag pro Handy-Seite, nichts verloren)", order, len(batches), len(items))
		}
		for index, batch := range batches {
			if len(batch.Items) != 1 {
				t.Fatalf("ties-mobile (Anordnung %v): Batch %d hat %d Items, auf dem Handy ist genau 1 erwartet", order, index, len(batch.Items))
			}
		}
		if batches[0].Items[0].Score != 90 {
			t.Fatalf("ties-mobile (Anordnung %v): erste Handy-Seite trägt Score %v, erwartet den Spitzen-Score 90", order, batches[0].Items[0].Score)
		}
	}
}

// WHY: Der Brief sagt ausdrücklich: Wenn gerade nichts da ist, darf das Ganze nicht
// abstürzen — und ohne Beiträge gibt es keine Seiten. Beide Viewports, nil und leer.
func TestZZBatRealEmptyFeedNoBatches(t *testing.T) {
	cases := []struct {
		label string
		items []RankedItem
		opts  BatchOptions
	}{
		{"nil-desktop", nil, zzbatDesktop(3)},
		{"nil-mobile", nil, zzbatMobile(3)},
		{"empty-desktop", []RankedItem{}, zzbatDesktop(3)},
		{"empty-mobile", []RankedItem{}, zzbatMobile(3)},
	}
	for _, c := range cases {
		if batches := zzbatCall(t, c.label, c.items, c.opts, 10*time.Second); len(batches) != 0 {
			t.Fatalf("%s: leere Eingabe erzeugte %d Batches, erwartet keine", c.label, len(batches))
		}
	}
}

// WHY: Der Brief fordert Abwechslung nur, WENN es Alternativen gibt. Besteht der ganze
// Pool aus einem einzigen Thema, gibt es keine — die Zusammenstellung muss trotzdem
// vollständig, ohne leere Seiten und in endlicher Zeit gelingen (ein naiver
// Abwechslungs-Erzwinger kann hier hängen bleiben oder Items fallen lassen).
func TestZZBatRealSingleTopicPool(t *testing.T) {
	items := []RankedItem{
		zzbatItem(21, ItemTextDebate, 100, "ai"),
		zzbatItem(22, ItemTextDebate, 95, "ai"),
		zzbatItem(23, ItemTextDebate, 90, "ai"),
		zzbatItem(24, ItemTextDebate, 85, "ai"),
		zzbatItem(25, ItemTextDebate, 80, "ai"),
		zzbatItem(26, ItemTextDebate, 75, "ai"),
	}
	batches := zzbatCall(t, "single-topic", items, zzbatDesktop(3), 10*time.Second)
	zzbatCheckExactlyOnce(t, "single-topic", items, batches, 3)
}

// WHY: Der Brief erlaubt Live-Räume oben auf aufeinanderfolgenden Seiten, wenn es
// schlicht nichts anderes gibt. Ein reiner Live-Pool muss also vollständig
// durchlaufen — ein naiver Kadenz-Erzwinger, der auf ein Nicht-Live-Item wartet,
// hängt hier oder verliert Items. Handy zusätzlich: ein Beitrag pro Seite.
func TestZZBatRealAllLivePool(t *testing.T) {
	items := []RankedItem{
		zzbatItem(31, ItemLiveRoom, 100, "ai"),
		zzbatItem(32, ItemLiveRoom, 95, "housing"),
		zzbatItem(33, ItemLiveRoom, 90, "science"),
		zzbatItem(34, ItemLiveRoom, 85, "culture"),
		zzbatItem(35, ItemLiveRoom, 80, "history"),
	}
	desktop := zzbatCall(t, "all-live-desktop", items, zzbatDesktop(3), 10*time.Second)
	zzbatCheckExactlyOnce(t, "all-live-desktop", items, desktop, 3)

	mobile := zzbatCall(t, "all-live-mobile", items, zzbatMobile(3), 10*time.Second)
	zzbatCheckExactlyOnce(t, "all-live-mobile", items, mobile, 1)
	if len(mobile) != len(items) {
		t.Fatalf("all-live-mobile: %d Batches, erwartet %d (ein Beitrag pro Handy-Seite)", len(mobile), len(items))
	}
}

// WHY: Ein realer Feed hat hunderte bewertete Kandidaten. Der Vertrag (vollständig,
// nichts doppelt, keine leere Seite, Seitengröße respektiert) muss auch bei ~200 Items
// mit Themen-Klumpen, Live-Anteil und Score-Gleichständen halten — und zwar in
// endlicher Zeit (quadratisch explodierende oder hängende Heuristiken fallen hier auf).
func TestZZBatRealLargePool(t *testing.T) {
	topics := []string{"ai", "housing", "culture", "science", "history", "law", "economics", "climate", "sports", "music"}
	items := make([]RankedItem, 0, 200)
	for i := 0; i < 200; i++ {
		itemType := ItemTextDebate
		if i%5 == 0 {
			itemType = ItemLiveRoom
		} else if i%9 == 0 {
			itemType = ItemSuggestion
		}
		score := float64(1000 - i/2) // Gleichstände in Zweier-Paaren
		items = append(items, zzbatItem(100+i, itemType, score, topics[i%len(topics)]))
	}
	batches := zzbatCall(t, "large-pool", items, zzbatDesktop(4), 30*time.Second)
	zzbatCheckExactlyOnce(t, "large-pool", items, batches, 4)
}

// WHY: Die Seitengröße kommt vom Aufrufer. An den Rändern des sinnvollen Bereichs
// (1, exakt Poolgröße, weit über Poolgröße) muss der Vertrag unverändert halten;
// mehr als PageSize Items pro Seite darf es nie geben.
func TestZZBatRealPageSizeBounds(t *testing.T) {
	items := []RankedItem{
		zzbatItem(41, ItemTextDebate, 100, "ai"),
		zzbatItem(42, ItemLiveRoom, 95, "ai"),
		zzbatItem(43, ItemTextDebate, 90, "housing"),
		zzbatItem(44, ItemTextDebate, 85, "culture"),
		zzbatItem(45, ItemLiveRoom, 80, "science"),
		zzbatItem(46, ItemTextDebate, 75, "history"),
		zzbatItem(47, ItemSuggestion, 70, "law"),
	}
	for _, pageSize := range []int{1, len(items), 50} {
		batches := zzbatCall(t, "pagesize-bounds", items, zzbatDesktop(pageSize), 10*time.Second)
		zzbatCheckExactlyOnce(t, "pagesize-bounds", items, batches, pageSize)
	}
}

// ---------- Stufe 2: pathologische Kanten (TestZZBatPath*) ----------
// Bestanden heißt hier nur: kein Panic, kein Verlust, keine Endlosschleife.

// WHY: NaN-Scores können aus einer kaputten Bewertung stromaufwärts kommen. NaN bricht
// jede Vergleichsordnung — die Zusammenstellung darf dabei weder abstürzen noch hängen
// noch Beiträge verlieren. Wo NaN-Items landen, ist bewusst nicht vorgeschrieben.
func TestZZBatPathNaNScores(t *testing.T) {
	items := []RankedItem{
		zzbatItem(51, ItemTextDebate, math.NaN(), "ai"),
		zzbatItem(52, ItemTextDebate, 50, "housing"),
		zzbatItem(53, ItemLiveRoom, math.NaN(), "science"),
		zzbatItem(54, ItemTextDebate, 10, "culture"),
	}
	batches := zzbatCall(t, "nan-scores", items, zzbatDesktop(3), 5*time.Second)
	zzbatCheckNoLoss(t, "nan-scores", items, batches)
}

// WHY: Negative und unendliche Scores sind außerhalb jeder Produkt-Erwartung, aber
// numerisch möglich. Kein Panic, kein Verlust, keine Endlosschleife — welche Seite
// ein -Inf-Item bekommt, ist egal.
func TestZZBatPathNegativeAndInfiniteScores(t *testing.T) {
	items := []RankedItem{
		zzbatItem(61, ItemTextDebate, -math.MaxFloat64, "ai"),
		zzbatItem(62, ItemTextDebate, -5, "housing"),
		zzbatItem(63, ItemLiveRoom, math.Inf(1), "science"),
		zzbatItem(64, ItemTextDebate, math.Inf(-1), "culture"),
		zzbatItem(65, ItemTextDebate, 0, "history"),
		zzbatItem(66, ItemSuggestion, 10, "law"),
	}
	batches := zzbatCall(t, "neg-inf-scores", items, zzbatDesktop(3), 5*time.Second)
	zzbatCheckNoLoss(t, "neg-inf-scores", items, batches)
}

// WHY: Dieselbe ID mehrfach in der Rangliste ist eine kaputte Eingabe. Beides ist eine
// vertretbare fachliche Deutung: alle Vorkommen zeigen ODER deduplizieren („nichts
// doppelt"). Erzwungen wird nur: kein Panic, jede ID erscheint mindestens einmal,
// keine ID öfter als eingegeben, keine Endlosschleife.
func TestZZBatPathDuplicateIDs(t *testing.T) {
	dup := zzbatID(71)
	mk := func(score float64, topic string) RankedItem {
		item := zzbatItem(0, ItemTextDebate, score, topic)
		item.Candidate.ID = dup
		return item
	}
	items := []RankedItem{
		mk(90, "ai"),
		mk(80, "housing"),
		mk(70, "science"),
		zzbatItem(72, ItemTextDebate, 60, "culture"),
	}
	batches := zzbatCall(t, "duplicate-ids", items, zzbatDesktop(3), 5*time.Second)
	zzbatCheckNoLoss(t, "duplicate-ids", items, batches)
}

// WHY: PageSize 0 ist der klassische Endlosschleifen-Auslöser (Schrittweite 0 beim
// Zerteilen). Ob die Implementierung auf einen Default zurückfällt oder anders
// aufteilt, ist ihre Sache — sie darf nur nicht hängen, abstürzen oder verlieren.
func TestZZBatPathPageSizeZero(t *testing.T) {
	items := []RankedItem{
		zzbatItem(81, ItemTextDebate, 100, "ai"),
		zzbatItem(82, ItemLiveRoom, 90, "housing"),
		zzbatItem(83, ItemTextDebate, 80, "science"),
		zzbatItem(84, ItemTextDebate, 70, "culture"),
		zzbatItem(85, ItemSuggestion, 60, "law"),
	}
	desktop := zzbatCall(t, "pagesize-zero-desktop", items, zzbatDesktop(0), 5*time.Second)
	zzbatCheckNoLoss(t, "pagesize-zero-desktop", items, desktop)

	mobile := zzbatCall(t, "pagesize-zero-mobile", items, zzbatMobile(0), 5*time.Second)
	zzbatCheckNoLoss(t, "pagesize-zero-mobile", items, mobile)
}

// WHY: Wie PageSize 0, nur als negativer Wert — ein Vorzeichenfehler beim Aufrufer.
// Kein Panic, kein Verlust, keine Endlosschleife; die Deutung bleibt frei.
func TestZZBatPathPageSizeNegative(t *testing.T) {
	items := []RankedItem{
		zzbatItem(91, ItemTextDebate, 100, "ai"),
		zzbatItem(92, ItemLiveRoom, 90, "housing"),
		zzbatItem(93, ItemTextDebate, 80, "science"),
	}
	batches := zzbatCall(t, "pagesize-negative", items, zzbatDesktop(-3), 5*time.Second)
	zzbatCheckNoLoss(t, "pagesize-negative", items, batches)
}
