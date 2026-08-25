package aiwork

// Robustheits-Batterie fuer agora-A1-gate — ZUSATZ-Metrik, aendert keine
// PASS/FAIL-Urteile. Wird zur Laufzeit als zz_battery_test.go nach
// agora-backend/internal/aiwork/ kopiert. Der Brief (Bug report): Hintergrund-
// (best-effort) AI-Arbeit wird vor einem frueher eingetroffenen Publish gleicher
// Prioritaet eingelassen; das Gate muss die in-flight-Arbeit begrenzen, Wartende
// innerhalb gleicher Prioritaet in Ankunftsreihenfolge bedienen, required-Arbeit
// vor best-effort durchlassen, Queue-Tiefe und Wartezeit begrenzen, Kontext-
// Abbrueche sauber aufraeumen und niemals Slots leaken. Diese Batterie ergaenzt
// den versteckten Grader (die Paket-eigene Test-Suite, Tamper-Guard auf der
// unveraenderten gate_test.go) um Kanten, die jener nicht abdeckt. Sie benutzt
// ausschliesslich die Baseline-API: NewGate, Gate, Run, Stats, Limits, Priority
// (BestEffort/Required) und die Err-* Werte.
//
// Zwei Stufen, am Testnamen erkennbar:
//
//   TestZZBatReal* — realistische Kanten; der volle Brief-Vertrag muss halten:
//     Ankunftsreihenfolge bei gleicher Prioritaet, required vor best-effort,
//     Grenzwerte (Concurrency/QueueDepth/Wait/Ceiling), Freigabe nach Abschluss,
//     korrektes Verhalten bei Kontext-Abbruch, keine Slot-/Waiter-Leaks.
//
//   TestZZBatPath* — pathologische Eingaben. Bestanden heisst nur: kein Panic,
//     kein Hang, keine Kapazitaets-Leaks, Terminierung. KEINE fachliche Deutung
//     wird erzwungen (z. B. welche Klemmung bei negativen Limits, ob ein bereits
//     abgebrochener Kontext die Arbeit noch startet).
//
// Deterministisch: kein Zufall, keine Zeit-Assertions (nur Ereignis-Waits ueber
// Kanaele/WaitGroups und Stats-Polling mit Deadline — nie eine Aussage ueber
// verstrichene Wanduhrzeit), keine Netz-/Dateisystem-Nebenwirkungen. Jede
// Gate.Run-Messung laeuft mit eigenem Timeout in einer Goroutine, damit ein
// Haenger den Lauf nicht mitreisst. Alle Helfer sind zzag-praefixiert, um
// Kollisionen mit Modell-Testdateien zu vermeiden.

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// ---------- Bausteine ----------

// zzagCall fuehrt Gate.Run mit Panic-Fang und Timeout aus. Ein Timeout oder Panic
// beendet den Test sofort; die (bei Timeout verwaiste) Goroutine kann den
// Binary-Lauf dank recover nicht mehr abschiessen.
func zzagCall(t *testing.T, label string, gate *Gate, ctx context.Context, priority Priority, work func(context.Context) error, timeout time.Duration) error {
	t.Helper()
	type result struct {
		err      error
		panicked any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panicked: r}
			}
		}()
		done <- result{err: gate.Run(ctx, priority, work)}
	}()
	select {
	case res := <-done:
		if res.panicked != nil {
			t.Fatalf("%s: Gate.Run panicked: %v (der Brief verlangt: nicht abstuerzen)", label, res.panicked)
		}
		return res.err
	case <-time.After(timeout):
		t.Fatalf("%s: Gate.Run kehrte nach %v nicht zurueck (Endlosschleife?)", label, timeout)
		return nil
	}
}

// zzagCallAllowPanic ist die nachsichtige Schwester von zzagCall: ein Panic ist
// KEIN Testfehler, sondern wird nur zurueckgemeldet. WHY: Fuer Aufrufer-Fehler,
// die die Go-Konvention ausdruecklich verbietet (nil-Kontext, siehe Doku von
// context: "Do not pass a nil Context"), darf die Batterie kein Nicht-Absturz-
// Verhalten erzwingen — der Brief deckt das nicht. Gemessen wird stattdessen die
// Invariante, die der Brief sehr wohl impliziert: das Gate darf danach weder
// haengen noch einen Slot verloren haben. Ein Timeout bleibt ein Testfehler.
func zzagCallAllowPanic(t *testing.T, label string, gate *Gate, ctx context.Context, priority Priority, work func(context.Context) error, timeout time.Duration) (err error, panicked any) {
	t.Helper()
	type result struct {
		err      error
		panicked any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panicked: r}
			}
		}()
		done <- result{err: gate.Run(ctx, priority, work)}
	}()
	select {
	case res := <-done:
		return res.err, res.panicked
	case <-time.After(timeout):
		t.Fatalf("%s: Gate.Run kehrte nach %v nicht zurueck (Endlosschleife?)", label, timeout)
		return nil, nil
	}
}

// zzagRunBackground startet Gate.Run in eigener Goroutine und liefert einen Kanal,
// der genau einen Wert (den Rueckgabe-Fehler) traegt. WHY: Die meisten Tests
// brauchen nebenlaeufige Aufrufe, deren Ausgang sie per Kanal deterministisch
// abwarten — ohne Sleep. Die Kapazitaet 1 entkoppelt Sender und Empfaenger.
func zzagRunBackground(gate *Gate, ctx context.Context, priority Priority, work func(context.Context) error) chan error {
	out := make(chan error, 1)
	go func() {
		out <- gate.Run(ctx, priority, work)
	}()
	return out
}

// zzagWaitQueued wartet, bis die Warteschlange die gewuenschte Laenge erreicht.
// WHY: Die API liefert kein "ich stehe jetzt in der Queue"-Signal; der Zustand ist
// ueber Stats() beobachtbar. Das ist ein Ereignis-Wait mit Deadline, keine
// Zeit-Assertion: das Testergebnis haengt nur vom Gate-Zustand ab, nie von der
// verstrichenen Wanduhrzeit (gleiches Muster wie die Baseline-Tests, Kanaele/
// WaitGroups statt Timing-Festlegungen).
func zzagWaitQueued(t *testing.T, gate *Gate, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if queued := gate.Stats().Queued; queued == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("queued = %d, want %d (Warteschlange erreichte den Zustand nicht)", gate.Stats().Queued, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// zzagWaitQueuedForActive wartet, bis genau want Slots belegt sind. WHY: Beim
// Aufbau eines Tests muss der "Halter" garantiert drin sein, bevor weitere Aufrufer
// in die Warteschlange gehen — sonst waere nicht deterministisch, wer zuerst dran
// ist. Auch hier: Ereignis-Wait mit Deadline, keine Zeit-Assertion.
func zzagWaitQueuedForActive(t *testing.T, gate *Gate, want int, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if active := gate.Stats().Active; active == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: Active = %d, want %d (Belegung erreichte den Zustand nicht)", what, gate.Stats().Active, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// zzagAssertIdle prueft, dass das Gate vollstaendig geleert ist — kein aktiver
// Slot, keine Wartenden, kein best-effort-Zaehler. WHY: jede Kapazitaet, die nach
// Abschluss aller Aufrufe besetzt bleibt, ist ein Leak, der die Plattform
// dauerhaft schrumpfen laesst (Leitsatz der Paketdoku in gate.go).
func zzagAssertIdle(t *testing.T, label string, gate *Gate) {
	t.Helper()
	st := gate.Stats()
	if st.Active != 0 || st.Queued != 0 || st.ActiveBestEffort != 0 {
		t.Fatalf("%s: Gate nicht geleert: %#v (Slot-/Waiter-Leak)", label, st)
	}
}

// zzagHold belegt einen Slot dauerhaft, bis release geschlossen wird. Liefert den
// Rueckgabe-Kanal des Run-Aufrufs, damit der Test das Ende des Halters abwarten
// kann, bevor er den Leer-Zustand prueft.
func zzagHold(gate *Gate, priority Priority, release chan struct{}, started chan struct{}) chan error {
	return zzagRunBackground(gate, context.Background(), priority, func(context.Context) error {
		if started != nil {
			close(started)
		}
		<-release
		return nil
	})
}

// ---------- Stufe 1: realistische Kanten (TestZZBatReal*) ----------

// WHY: Kern des Bug-Reports: ein spaeter eingetroffener Aufrufer derselben
// Prioritaet darf sich nicht vor einen frueheren schleichen. Vier required-
// Aufrufer druecken hier auf denselben einzigen Slot und werden einzeln
// freigelassen; die Einlass-Reihenfolge muss exakt der Ankunftsreihenfolge
// entsprechen. Der Baseline-Bug (>= statt > in nextAdmissible) laesst bei
// Gleichstand den zuletzt geprueften Wartenden ein, also [3,2,1,0]. Der Grader
// treibt nur zwei Aufrufer mit einem einzigen gemeinsamen Release.
func TestZZBatRealRequiredFIFOStaggeredReleases(t *testing.T) {
	const n = 4
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 8, Wait: 5 * time.Second})

	holderRelease := make(chan struct{})
	holder := zzagHold(gate, Required, holderRelease, nil)
	zzagWaitQueuedForActive(t, gate, 1, "required-FIFO")

	starts := make(chan int, n)
	finish := make([]chan struct{}, n)
	for i := range finish {
		finish[i] = make(chan struct{})
	}
	var callers sync.WaitGroup
	for i := 0; i < n; i++ {
		idx := i
		callers.Add(1)
		go func() {
			defer callers.Done()
			_ = gate.Run(context.Background(), Required, func(context.Context) error {
				starts <- idx
				<-finish[idx]
				return nil
			})
		}()
		// Erst abwarten, dass dieser Aufrufer in der Queue steht, bevor der naechste
		// gestartet wird — damit ist die Ankunftsreihenfolge deterministisch [0..n-1]
		// (Goroutine-Startreihenfolge allein garantiert sie nicht).
		zzagWaitQueued(t, gate, i+1)
	}

	order := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if i == 0 {
			close(holderRelease)
		} else {
			close(finish[order[i-1]])
		}
		select {
		case idx := <-starts:
			order = append(order, idx)
		case <-time.After(5 * time.Second):
			t.Fatalf("required-FIFO: Aufrufer %d wurde nicht eingelassen", i)
		}
	}
	close(finish[order[n-1]])
	callers.Wait()
	if err := <-holder; err != nil {
		t.Fatalf("required-FIFO: Halter lieferte %v", err)
	}

	if want := []int{0, 1, 2, 3}; !reflect.DeepEqual(order, want) {
		t.Fatalf("required-FIFO: Einlass-Reihenfolge %v, erwartet Ankunftsreihenfolge %v (spaeter Ankommender ueberholt frueheren)", order, want)
	}
	zzagAssertIdle(t, "required-FIFO", gate)
}

// WHY: Der Bug laesst fruehere best-effort-Arbeit von spaeterer best-effort-Arbeit
// ueberholen. Hier wartet zusaetzlich ein required-Aufrufer: der eine frei werdende
// Slot muss zuerst an ihn gehen (ein Publish laeuft nicht hinter Hintergrund-Jobs),
// und unter den verbliebenen best-effort-Waitern gilt weiter Ankunftsreihenfolge.
// Erwartet [required, BE0, BE1]; der Baseline-Bug liefert [required, BE1, BE0].
// Der Grader prueft die beiden Faelle (required-vor-best-effort und FIFO) getrennt.
func TestZZBatRealFIFOAcrossPrioritiesRequiredFirst(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 8, Wait: 5 * time.Second})

	holderRelease := make(chan struct{})
	holder := zzagHold(gate, BestEffort, holderRelease, nil)
	zzagWaitQueuedForActive(t, gate, 1, "priority-FIFO")

	type entry struct {
		prio Priority
	}
	waiters := []entry{
		{BestEffort}, // 0: zuerst eingetroffen
		{BestEffort}, // 1: spaeter, gleiche Prioritaet
		{Required},   // 2: spaeter, hoehere Prioritaet
	}
	starts := make(chan int, len(waiters))
	finish := make([]chan struct{}, len(waiters))
	for i := range finish {
		finish[i] = make(chan struct{})
	}
	var wg sync.WaitGroup
	for i, w := range waiters {
		idx, prio := i, w.prio
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = gate.Run(context.Background(), prio, func(context.Context) error {
				starts <- idx
				<-finish[idx]
				return nil
			})
		}()
		// Erst abwarten, dass dieser Aufrufer in der Queue steht, bevor der naechste
		// gestartet wird — damit ist die Ankunftsreihenfolge deterministisch [0..len-1].
		zzagWaitQueued(t, gate, i+1)
	}

	order := make([]int, 0, len(waiters))
	for i := 0; i < len(waiters); i++ {
		if i == 0 {
			close(holderRelease)
		} else {
			close(finish[order[i-1]])
		}
		select {
		case idx := <-starts:
			order = append(order, idx)
		case <-time.After(5 * time.Second):
			t.Fatalf("priority-FIFO: Aufrufer %d wurde nicht eingelassen", i)
		}
	}
	close(finish[order[len(waiters)-1]])
	wg.Wait()
	if err := <-holder; err != nil {
		t.Fatalf("priority-FIFO: Halter lieferte %v", err)
	}

	if want := []int{2, 0, 1}; !reflect.DeepEqual(order, want) {
		t.Fatalf("priority-FIFO: Einlass-Reihenfolge %v, erwartet [2 0 1] (required zuerst, dann best-effort in Ankunftsreihenfolge)", order)
	}
	zzagAssertIdle(t, "priority-FIFO", gate)
}

// WHY: Freigabe nach Abschluss — ein Slot muss zurueckgegeben werden, sobald die
// Arbeit fertig ist, sonst waere die Kapazitaet dauerhaft blockiert. Deterministisch
// ueber Kanaele: waehrend die Arbeit blockiert, ist Active==1; sobald sie endet,
// ist das Gate leer und ein frischer Aufruf kommt sofort durch.
func TestZZBatRealReleaseAfterCompletionFreesSlot(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 5 * time.Second})
	started := make(chan struct{})
	finish := make(chan struct{})
	first := zzagRunBackground(gate, context.Background(), Required, func(context.Context) error {
		close(started)
		<-finish
		return nil
	})
	<-started
	if st := gate.Stats(); st.Active != 1 || st.Queued != 0 {
		t.Fatalf("release-nach-abschluss: waehrend der Arbeit Active=%d Queued=%d, erwartet 1/0", st.Active, st.Queued)
	}
	close(finish)
	if err := <-first; err != nil {
		t.Fatalf("release-nach-abschluss: erster Aufruf lieferte %v", err)
	}
	zzagAssertIdle(t, "release-nach-abschluss", gate)

	admitted := make(chan struct{})
	second := zzagRunBackground(gate, context.Background(), Required, func(context.Context) error {
		close(admitted)
		return nil
	})
	select {
	case <-admitted:
	case <-time.After(5 * time.Second):
		t.Fatalf("release-nach-abschluss: frischer Aufruf nach Abschluss wurde nicht eingelassen (Slot nicht freigegeben?)")
	}
	if err := <-second; err != nil {
		t.Fatalf("release-nach-abschluss: zweiter Aufruf lieferte %v", err)
	}
	zzagAssertIdle(t, "release-nach-abschluss", gate)
}

// WHY: Das Gate ist ausdruecklich keine Spendensteuerung und darf Provider-Fehler
// nicht verschlucken: der Fehler der Arbeit muss 1:1 durchkommen (errors.Is), und
// nach dem Fehler muss der Slot wieder frei sein. Der Grader prueft keine
// Fehlerdurchreichung.
func TestZZBatRealRunReturnsWorkError(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 2, QueueDepth: 4, Wait: 5 * time.Second})
	sentinel := errors.New("zzag provider failure")
	err := zzagCall(t, "work-error", gate, context.Background(), Required, func(context.Context) error {
		return sentinel
	}, 5*time.Second)
	if !errors.Is(err, sentinel) {
		t.Fatalf("work-error: Gate.Run lieferte %v, erwartet den Provider-Fehler %v", err, sentinel)
	}
	zzagAssertIdle(t, "work-error", gate)
}

// WHY: Ein Wartender, der seinen Kontext abgebrochen hat, muss sauber aus der Queue
// genommen werden, ohne einen Slot zu leaken; danach muss das Gate wieder voll
// einsatzfaehig sein. Einfacher, deterministischer Einzel-Abbruch (der Grader
// treibt denselben Abandon-Pfad nur als Rennen ueber viele Versuche, mit 5s-Wait
// und ohne das Ceiling im Spiel).
func TestZZBatRealCancelWhileQueuedCleansUp(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 5 * time.Second})
	holderRelease := make(chan struct{})
	holder := zzagHold(gate, Required, holderRelease, nil)
	zzagWaitQueuedForActive(t, gate, 1, "cancel-queued")

	ctx, cancel := context.WithCancel(context.Background())
	queued := zzagRunBackground(gate, ctx, Required, func(context.Context) error { return nil })
	zzagWaitQueued(t, gate, 1)
	cancel()
	if err := <-queued; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel-queued: abgebrochener Wartender lieferte %v, erwartet context.Canceled", err)
	}
	if st := gate.Stats(); st.Active != 1 || st.Queued != 0 {
		t.Fatalf("cancel-queued: nach Abbruch Active=%d Queued=%d, erwartet 1/0 (Waiter-Leak)", st.Active, st.Queued)
	}
	close(holderRelease)
	if err := <-holder; err != nil {
		t.Fatalf("cancel-queued: Halter lieferte %v", err)
	}
	zzagAssertIdle(t, "cancel-queued", gate)

	admitted := make(chan struct{})
	fresh := zzagRunBackground(gate, context.Background(), Required, func(context.Context) error {
		close(admitted)
		return nil
	})
	select {
	case <-admitted:
	case <-time.After(5 * time.Second):
		t.Fatalf("cancel-queued: frischer Aufruf nach Abbruch wurde nicht eingelassen")
	}
	if err := <-fresh; err != nil {
		t.Fatalf("cancel-queued: frischer Aufruf lieferte %v", err)
	}
}

// WHY: Der Brief verlangt korrektes Verhalten bei Kontext-Abbruch. Das Gate reicht
// seinen Kontext an die Arbeit weiter: wird er abgebrochen, muss die laufende
// Arbeit das sehen und der Run mit context.Canceled enden — ein Gate, das den
// Kontext ignoriert, wuerde die Arbeit ewig weiterlaufen lassen. Der 3s-Guard
// haelt den Test bei einer falschen Implementierung kurz statt zu haengen.
func TestZZBatRealActiveWorkSeesCancellation(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	run := zzagRunBackground(gate, ctx, Required, func(ctx context.Context) error {
		close(started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			return errors.New("zzag: Kontext wurde nicht an die Arbeit weitergegeben")
		}
	})
	<-started
	cancel()
	if err := <-run; !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx-active: Run lieferte %v, erwartet context.Canceled (Arbeit muss den Abbruch sehen)", err)
	}
	zzagAssertIdle(t, "ctx-active", gate)
}

// WHY: Wait:0 bedeutet "nicht warten": Ein Wartender muss sofort aufgeben
// (ErrWaitExpired), ohne einen Provider-Aufruf zu verursachen, und die Abweisung
// muss gezaehlt werden. Deterministisch ueber den Rueckgabe-Kanal, ohne
// Zeit-Assertion (der Grader prüft Wait-Expiry nur mit 30ms und einer
// Elapsed-Zeitaussage).
func TestZZBatRealZeroWaitQueueRefused(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 0})
	holderRelease := make(chan struct{})
	holder := zzagHold(gate, Required, holderRelease, nil)
	zzagWaitQueuedForActive(t, gate, 1, "wait-0")

	calls := 0
	queued := zzagRunBackground(gate, context.Background(), Required, func(context.Context) error {
		calls++
		return nil
	})
	if err := <-queued; !errors.Is(err, ErrOverloaded) || !errors.Is(err, ErrWaitExpired) {
		t.Fatalf("wait-0: Wartender lieferte %v, erwartet ErrWaitExpired", err)
	}
	if calls != 0 {
		t.Fatalf("wait-0: ein aufgegebener Wartender erreichte den Provider (%d Aufrufe)", calls)
	}
	if st := gate.Stats(); st.RefusedWaitExpired != 1 || st.Queued != 0 {
		t.Fatalf("wait-0: Abweisung nicht gezaehlt/aufgeraeumt: %#v", st)
	}
	close(holderRelease)
	if err := <-holder; err != nil {
		t.Fatalf("wait-0: Halter lieferte %v", err)
	}
	zzagAssertIdle(t, "wait-0", gate)
}

// WHY: QueueDepth:0 heisst: es gibt gar keinen Wartebereich. Wer ankommt, waehrend
// der einzige Slot besetzt ist, wird sofort abgewiesen (ErrQueueFull) — auch
// required-Arbeit —, der Provider wird nie gerufen und die Abweisung gezaehlt.
// Deterministisch synchron, weil die Abweisung unmittelbar ist (der Grader nutzt
// QueueDepth 1 mit einem bereits Wartenden).
func TestZZBatRealQueueDepthZeroRefusesImmediately(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 0, Wait: 5 * time.Second})
	holderRelease := make(chan struct{})
	holder := zzagHold(gate, Required, holderRelease, nil)
	zzagWaitQueuedForActive(t, gate, 1, "queue-depth-0")

	calls := 0
	err := zzagCall(t, "queue-depth-0", gate, context.Background(), Required, func(context.Context) error {
		calls++
		return nil
	}, 5*time.Second)
	if !errors.Is(err, ErrOverloaded) || !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue-depth-0: %v, erwartet ErrQueueFull", err)
	}
	if calls != 0 {
		t.Fatalf("queue-depth-0: abgewiesener Aufrufer erreichte den Provider")
	}
	if st := gate.Stats(); st.RefusedQueueFull != 1 || st.Queued != 0 {
		t.Fatalf("queue-depth-0: Abweisung nicht gezaehlt/aufgeraeumt: %#v", st)
	}
	close(holderRelease)
	if err := <-holder; err != nil {
		t.Fatalf("queue-depth-0: Halter lieferte %v", err)
	}
	zzagAssertIdle(t, "queue-depth-0", gate)
}

// WHY: Der reservierte Slot muss unter Saturation halten: Sind alle best-effort-
// Slots am Ceiling belegt und weitere best-effort-Jobs in der Queue, muss eine
// eintreffende required-Arbeit SOFORT starten, und die best-effort-Zaehler duerfen
// das Ceiling nie ueberschreiten. Am Ende laufen alle gewarteten Jobs (kein
// Starvation, keine verfrühte Abweisung) und das Gate leert sich. Der Grader
// deckt nur 2 Halter + 1 wartendes best-effort-Job ab.
func TestZZBatRealBestEffortCeilingUnderSaturation(t *testing.T) {
	const holders = 3
	const queuedBE = 5
	gate := NewGate(Limits{Concurrency: 4, QueueDepth: 16, Wait: 5 * time.Second})

	held := make(chan struct{}, holders)
	holderRelease := make([]chan struct{}, holders)
	holder := make([]chan error, holders)
	for i := 0; i < holders; i++ {
		holderRelease[i] = make(chan struct{})
		idx := i
		holder[i] = zzagRunBackground(gate, context.Background(), BestEffort, func(context.Context) error {
			held <- struct{}{}
			<-holderRelease[idx]
			return nil
		})
	}
	for i := 0; i < holders; i++ {
		select {
		case <-held:
		case <-time.After(5 * time.Second):
			t.Fatalf("ceiling: best-effort-Halter %d wurde nicht eingelassen", i)
		}
	}
	if st := gate.Stats(); st.ActiveBestEffort != holders || st.Active != holders {
		t.Fatalf("ceiling: bei Saturation ActiveBestEffort=%d Active=%d, erwartet %d/%d", st.ActiveBestEffort, st.Active, holders, holders)
	}

	finishAll := make(chan struct{})
	ran := make(chan struct{}, queuedBE)
	var all sync.WaitGroup
	for i := 0; i < queuedBE; i++ {
		all.Add(1)
		go func() {
			defer all.Done()
			_ = gate.Run(context.Background(), BestEffort, func(context.Context) error {
				<-finishAll
				ran <- struct{}{}
				return nil
			})
		}()
	}
	zzagWaitQueued(t, gate, queuedBE)
	if st := gate.Stats(); st.ActiveBestEffort != holders {
		t.Fatalf("ceiling: best-effort-Ceiling %d verletzt: ActiveBestEffort=%d", gate.Stats().BestEffortCeiling, st.ActiveBestEffort)
	}

	requiredStarted := make(chan struct{})
	required := zzagRunBackground(gate, context.Background(), Required, func(context.Context) error {
		close(requiredStarted)
		return nil
	})
	select {
	case <-requiredStarted:
	case <-time.After(5 * time.Second):
		t.Fatalf("ceiling: required-Arbeit musste bei vollem best-effort-Haus sofort starten")
	}
	if st := gate.Stats(); st.ActiveBestEffort > st.BestEffortCeiling {
		t.Fatalf("ceiling: ActiveBestEffort=%d > Ceiling=%d", st.ActiveBestEffort, st.BestEffortCeiling)
	}
	if err := <-required; err != nil {
		t.Fatalf("ceiling: required-Aufruf lieferte %v", err)
	}

	for i := 0; i < holders; i++ {
		close(holderRelease[i])
		if err := <-holder[i]; err != nil {
			t.Fatalf("ceiling: Halter %d lieferte %v", i, err)
		}
	}
	close(finishAll)
	all.Wait()
	for i := 0; i < queuedBE; i++ {
		select {
		case <-ran:
		case <-time.After(5 * time.Second):
			t.Fatalf("ceiling: gewartetes best-effort-Job %d wurde nie eingelassen (Starvation oder verfrühte Abweisung)", i)
		}
	}
	zzagAssertIdle(t, "ceiling", gate)
}

// WHY: Slot-Leaks wuerden die Plattform-Kapazitaet ueber viele Zyklen hinweg
// unmerklich schrumpfen lassen. Viele gemischte Voll-Zyklen (best-effort +
// required, mit Queue und Hand-off) auf EINEM Gate; nach jedem Zyklus muss das
// Gate wieder komplett leer sein. Deterministisch ueber WaitGroups — nach wg.Wait
// sind alle Releases durchgelaufen, weil Done erst nach Run-Rueckkehr laeuft
// (der Grader pruft Leak-Freiheit nur fuer den Abandon-Race-Pfad).
func TestZZBatRealManyCyclesNoLeak(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 2, QueueDepth: 8, Wait: 5 * time.Second})
	const cycles = 50
	for c := 0; c < cycles; c++ {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			prio := Required
			if i%2 == 0 {
				prio = BestEffort
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = gate.Run(context.Background(), prio, func(context.Context) error { return nil })
			}()
		}
		wg.Wait()
		zzagAssertIdle(t, "many-cycles", gate)
	}
}

// ---------- Stufe 2: pathologische Kanten (TestZZBatPath*) ----------
// Bestanden heisst hier nur: kein Panic, kein Hang, keine Kapazitaets-Leaks,
// Terminierung. KEINE fachliche Deutung wird erzwungen.

// WHY: Negative Limits sind eine kaputte Konfiguration. Bestanden heisst nur: kein
// Panic, kein Hang, jeder Aufruf terminiert (Einlass ODER Abweisung) und das Gate
// ist danach leer. Welche Klemmung eine Implementierung waehlt, ist ihre Sache.
func TestZZBatPathNegativeLimits(t *testing.T) {
	gate := NewGate(Limits{Concurrency: -3, QueueDepth: -5, Wait: -time.Second})
	priorities := []Priority{Required, BestEffort, BestEffort}
	for i, prio := range priorities {
		err := zzagCall(t, "neg-limits", gate, context.Background(), prio, func(context.Context) error { return nil }, 5*time.Second)
		if err != nil && !errors.Is(err, ErrOverloaded) {
			t.Fatalf("neg-limits (Aufruf %d): unerwarteter Fehler %v", i, err)
		}
	}
	zzagAssertIdle(t, "neg-limits", gate)
}

// WHY: Ein bereits abgebrochener Kontext ist pathologisch, aber realistisch (der
// Client hat laengst aufgegeben). Bestanden heisst nur: Der Aufruf endet sofort
// (kein Haenger), beschaeftigt keinen Slot dauerhaft und das Gate bleibt danach
// nutzbar. Ob die Arbeit noch laeuft oder nicht, wird bewusst nicht gewertet.
func TestZZBatPathCancelBeforeStart(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = zzagCall(t, "cancel-before-start", gate, ctx, Required, func(context.Context) error { return nil }, 5*time.Second)
	zzagAssertIdle(t, "cancel-before-start", gate)
	if err := zzagCall(t, "cancel-before-start-fresh", gate, context.Background(), Required, func(context.Context) error { return nil }, 5*time.Second); err != nil {
		t.Fatalf("cancel-before-start: Gate nach abgebrochenem Aufruf nicht nutzbar: %v", err)
	}
}

// WHY: Doppel-Freigabe abfangen: Zwei Aufgabesignale sind gleichzeitig aktiv — der
// Kontext wird abgebrochen, waehrend die Wartefrist (Wait:0) sofort ablaufen kann.
// Das Gate darf den Wartenden nur EINMAL aufgeben: Active darf nie unter den
// erwarteten Wert fallen (eine zweite Freigabe wuerde den Slot-Zaehler treiben),
// und am Ende ist alles geleert. Welches der beiden Signale gewinnt, ist egal —
// der Grader treibt das Abbruch/Slot-Rennen nur mit einer langen Wartefrist.
func TestZZBatPathDoubleGiveUpSignals(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 0})
	holderRelease := make(chan struct{})
	holder := zzagHold(gate, Required, holderRelease, nil)
	zzagWaitQueuedForActive(t, gate, 1, "double-giveup")

	ctx, cancel := context.WithCancel(context.Background())
	queued := zzagRunBackground(gate, ctx, Required, func(context.Context) error { return nil })
	cancel()
	err := <-queued
	if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrOverloaded) {
		t.Fatalf("double-giveup: Wartender lieferte %v, erwartet context.Canceled oder ErrOverloaded", err)
	}
	if st := gate.Stats(); st.Active != 1 || st.Queued != 0 {
		t.Fatalf("double-giveup: Active=%d Queued=%d, erwartet 1/0 (Waiter- oder Slot-Leak, doppelte Freigabe?)", st.Active, st.Queued)
	}
	close(holderRelease)
	if err := <-holder; err != nil {
		t.Fatalf("double-giveup: Halter lieferte %v", err)
	}
	zzagAssertIdle(t, "double-giveup", gate)
}

// WHY: Mehrere Wartende teilen sich EINEN Kontext; wird der abgebrochen, muessen
// alle gleichzeitig sauber aufgeraeumt werden — ohne Panic, ohne Waiter- oder
// Slot-Leak. Der Grader treibt den Einzel-Waiter-Abbruch als Rennen; hier: eine
// ganze Gruppe mit einer einzigen Cancel-Welle.
func TestZZBatPathSharedContextManyCancellations(t *testing.T) {
	const n = 4
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 8, Wait: 5 * time.Second})
	holderRelease := make(chan struct{})
	holder := zzagHold(gate, Required, holderRelease, nil)
	zzagWaitQueuedForActive(t, gate, 1, "shared-cancel")

	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- gate.Run(ctx, BestEffort, func(context.Context) error { return nil })
		}()
	}
	zzagWaitQueued(t, gate, n)
	cancel()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrOverloaded) {
			t.Fatalf("shared-cancel: Wartender lieferte %v, erwartet context.Canceled oder ErrOverloaded", err)
		}
	}
	if st := gate.Stats(); st.Active != 1 || st.Queued != 0 {
		t.Fatalf("shared-cancel: Active=%d Queued=%d, erwartet 1/0 (Cancel-Welle raeumte nicht auf)", st.Active, st.Queued)
	}
	close(holderRelease)
	if err := <-holder; err != nil {
		t.Fatalf("shared-cancel: Halter lieferte %v", err)
	}
	zzagAssertIdle(t, "shared-cancel", gate)
}

// WHY: nil-Kontext ist der klassische Aufrufer-Fehler. Die Go-Doku von context
// verbietet ihn ausdruecklich ("Do not pass a nil Context"), und der Brief
// (FIFO-Bugfix) verlangt nirgends, ihn abzufangen — auch die Baseline
// dereferenziert ihn sofort. Ein Panic ist deshalb HIER erlaubt und kein Riss.
// Geprueft wird nur, was der Brief wirklich impliziert: der Aufruf terminiert
// (kein Haenger), das Gate leakt dabei keinen Slot und ist danach weiter nutzbar.
// Vor 2026-08-25 wertete dieser Test das Panic selbst als Riss; das reissen alle
// 13 Abgaben UND die Baseline, mass also keinen Unterschied und war vom Brief
// nicht gedeckt (Befund der Host-Pruefung, siehe pruefbericht.md).
func TestZZBatPathNilContext(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 5 * time.Second})
	_, _ = zzagCallAllowPanic(t, "nil-ctx", gate, nil, Required, func(context.Context) error { return nil }, 5*time.Second)
	zzagAssertIdle(t, "nil-ctx", gate)
	if err := zzagCall(t, "nil-ctx-fresh", gate, context.Background(), Required, func(context.Context) error { return nil }, 5*time.Second); err != nil {
		t.Fatalf("nil-ctx: Gate nach nil-Kontext nicht nutzbar: %v", err)
	}
}

// WHY: Priority ist ein offener Integer-Typ; ein Wert, der weder BestEffort noch
// Required ist, darf weder Panic noch Hang noch Slot-Leak ausloesen. Wie er
// behandelt wird (wie eine Nicht-best-effort-Prioritaet oder als Abweisung) ist
// eine vertretbare Deutung; erzwungen wird nur Terminierung und Leer-Zustand.
func TestZZBatPathOutOfRangePriority(t *testing.T) {
	gate := NewGate(Limits{Concurrency: 1, QueueDepth: 4, Wait: 5 * time.Second})
	priorities := []Priority{42, -7}
	for i, prio := range priorities {
		err := zzagCall(t, "odd-priority", gate, context.Background(), prio, func(context.Context) error { return nil }, 5*time.Second)
		if err != nil && !errors.Is(err, ErrOverloaded) {
			t.Fatalf("odd-priority (Aufruf %d): unerwarteter Fehler %v", i, err)
		}
	}
	zzagAssertIdle(t, "odd-priority", gate)
}
