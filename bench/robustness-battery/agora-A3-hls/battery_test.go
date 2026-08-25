package livehls

// Robustheits-Batterie fuer agora-A3-hls — ZUSATZ-Metrik, aendert keine
// PASS/FAIL-Urteile. Wird zur Laufzeit als zz_battery_test.go nach
// agora-backend/internal/livehls/ kopiert. Der Brief (Feature request): die
// HLS-Auslieferung kennt die Segment-Erweiterung `.webm` noch nicht; `.webm`
// muss als immutables Medien-Segment mit Content-Type `video/webm` klassifiziert
// werden — und die Proxy-Konfiguration vor dem Origin-Bucket muss dieselbe
// Extension-Liste tragen wie die Cache-Policy (Kopplung, die ein Test erzwingt).
// Diese Batterie ergaenzt den versteckten Grader (grade_test.v2.go: genau ein
// WebM-Key, Klasse + Content-Type) um die Kanten der Klassifikation. Sie benutzt
// ausschliesslich die Baseline-API: ClassifyKey, CacheControlForKey,
// ContentTypeForKey, SegmentExtensions, Class/ClassSegment/ClassPlaylist,
// SegmentCacheControl, PlaylistCacheControl, PlaylistContentType.
//
// Zwei Stufen, am Testnamen erkennbar:
//
//   TestZZBatReal* — realistische Kanten; der volle Brief-Vertrag muss halten:
//     `.webm` ist ein immutables Segment mit Content-Type video/webm, in allen
//     Segment-/Playlist-/Init-Mustern, Gross-/Kleinschreibung egal, auch in
//     Unterverzeichnissen; die uebrigen Segment- und Manifest-Muster bleiben
//     unveraendert; Policy<->Klasse sind konsistent; die Edge-Config kennt .webm.
//
//   TestZZBatPath* — pathologische Eingaben. Bestanden heisst nur: kein Panic,
//     kein Hang, Terminierung. KEINE fachliche Deutung wird erzwungen.
//
// Deterministisch: kein Zufall, keine Zeitabhaengigkeit, keine Netz-/
// Dateisystem-Nebenwirkungen (ausser einem read-only Blick auf die im Workspace
// gepinnte Edge-Config deploy/hls-cache-headers.yml, wie die Baseline es auch
// tut). Alle Helfer sind zzb3-praefixiert, um Kollisionen mit den Testdateien
// der Modelle zu vermeiden.

import (
	"os"
	"strings"
	"testing"
)

// ---------- Bausteine ----------

// zzb3CheckSegment verlangt den vollen Brief-Vertrag fuer einen Segment-Key:
// Klasse ClassSegment, Cache-Control = SegmentCacheControl und der erwartete
// Content-Type. WHY: das ist die Invariante, um die der ganze Task kreist —
// ein Medien-Segment wird einmal geschrieben, darf nie mutabel ausgeliefert
// werden und muss mit einem dekodierbaren Media-Type rausgehen.
func zzb3CheckSegment(t *testing.T, key, wantCT string) {
	t.Helper()
	if got := ClassifyKey(key); got != ClassSegment {
		t.Fatalf("ClassifyKey(%q) = %q, erwartet ClassSegment", key, got)
	}
	if got := CacheControlForKey(key); got != SegmentCacheControl {
		t.Fatalf("CacheControlForKey(%q) = %q, erwartet %q", key, got, SegmentCacheControl)
	}
	if got := ContentTypeForKey(key); got != wantCT {
		t.Fatalf("ContentTypeForKey(%q) = %q, erwartet %q", key, got, wantCT)
	}
}

// zzb3NoPanic ruft alle drei Policy-Funktionen mit Panic-Fang auf. WHY: Die
// Path-Tests wollen nur beweisen, dass kaputte Keys die Klassifikation nicht
// abstuerzen lassen — jede Panic bricht den Test sofort ab.
func zzb3NoPanic(t *testing.T, label, key string) {
	t.Helper()
	calls := []struct {
		name string
		fn   func() any
	}{
		{"ClassifyKey", func() any { return ClassifyKey(key) }},
		{"CacheControlForKey", func() any { return CacheControlForKey(key) }},
		{"ContentTypeForKey", func() any { return ContentTypeForKey(key) }},
	}
	for _, c := range calls {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s: %s(%q) panicked: %v", label, c.name, key, r)
				}
			}()
			c.fn()
		}()
	}
}

// ---------- Stufe 1: realistische Kanten (TestZZBatReal*) ----------

// WHY: Kern des Briefs. LiveKit-Egress kann WebM-Segmente schreiben; die
// Klassifikation muss `.webm` als immutables Segment erkennen, es mit dem
// Segment-Cache-Control versehen und als video/webm ausliefern. Geprueft an den
// realistischen Key-Formen: rolling live segment, materialisiertes Clip-Segment,
// Init-Muster.
func TestZZBatRealWebmIsImmutableSegment(t *testing.T) {
	keys := []string{
		"live/room-1/seg_1.webm",
		"live/room-1/segment_05598.webm",
		"clips/room-1/8f0b/segment-00003.webm",
		"live/room-1/init.webm",
	}
	for _, key := range keys {
		zzb3CheckSegment(t, key, "video/webm")
	}
}

// WHY: Die Klassifikation ist seit jeher gross-/kleinschreibungs-egal (Baseline
// senkt die Extension). Die neue webm-Klasse darf daran nichts aendern — ein
// vom Egress anders grossgeschriebenes `.WEBM` muss genauso als Segment rausgehen.
func TestZZBatRealWebmCaseInsensitive(t *testing.T) {
	for _, key := range []string{
		"live/room-1/SEG_1.WEBM",
		"live/room-1/Seg_1.WebM",
		"live/room-1/seg_1.webm",
	} {
		zzb3CheckSegment(t, key, "video/webm")
	}
}

// WHY: Segmente liegen nicht nur eine Ebene tief (Aufnahme-Verzeichnisse,
// Renditions-Pfade unter clips/, Zeitstempel-Pfade). Die Extension-Klassifikation
// darf sich nicht am Verzeichnistief aufhaengen — auch tief verschachtelt bleibt
// .webm ein Segment. (Der Grader testet nur den flachen live/<room>/seg_1.webm.)
func TestZZBatRealWebmDeepNesting(t *testing.T) {
	keys := []string{
		"live/room-1/2026-08-25/seg_00001.webm",
		"live/room-1/rec/audio/webm/seg_00001.webm",
		"clips/room-1/8f0b/renditions/720p/segment-00003.webm",
	}
	for _, key := range keys {
		if got := ClassifyKey(key); got != ClassSegment {
			t.Fatalf("ClassifyKey(%q) = %q: auch in Unterverzeichnissen bleibt .webm ein Segment", key, got)
		}
	}
}

// WHY: SegmentExtensions() ist die Liste, an der die Edge-Config gespiegelt wird
// (TestEdgeConfigMirrorsPolicy). Wenn .webm dort fehlt, kann der Proxy WebM-
// Segmente nie cacheable ausliefern — der Brief verlangt ausdruecklich die
// Kopplung beider Schichten. Manifest-Extensions duerfen nie in der Liste stehen.
func TestZZBatRealSegmentExtensionsAdvertiseWebm(t *testing.T) {
	exts := SegmentExtensions()
	seen := make(map[string]bool, len(exts))
	for _, e := range exts {
		if !strings.HasPrefix(e, ".") || e != strings.ToLower(e) {
			t.Fatalf("SegmentExtensions() enthaelt %q, erwartet einen kleinen, punktierten Extension", e)
		}
		seen[e] = true
	}
	if !seen[".webm"] {
		t.Fatalf("SegmentExtensions() = %v: .webm fehlt — ohne die Erweiterung in der Liste kann die Edge-Config .webm nicht als Segment behandeln", exts)
	}
	if seen[".m3u8"] || seen[".m3u"] {
		t.Fatalf("SegmentExtensions() enthaelt ein Manifest (%v); ein Manifest ist nie immutable", exts)
	}
}

// WHY: Die webm-Ergaenzung darf die bestehenden Segment-Extensions nicht
// verschieben — ein Regressionsschutz quer durch alle heute bekannten
// Segment-Formate (Transport-Stream, fMP4-Segment/Init, M4A/AAC-Audio) inklusive
// ihrer Content-Types.
func TestZZBatRealExistingSegmentsUnchanged(t *testing.T) {
	contentTypes := map[string]string{
		".ts":  "video/mp2t",
		".m4s": "video/iso.segment",
		".mp4": "video/mp4",
		".m4v": "video/mp4",
		".m4a": "audio/mp4",
		".aac": "audio/aac",
	}
	for ext, wantCT := range contentTypes {
		key := "live/room-1/seg_00001" + ext
		zzb3CheckSegment(t, key, wantCT)
	}
}

// WHY: Manifeste werden unter festem Key neu geschrieben; eine stale Kopie am
// Edge ist ein eingefrorener Stream fuer jeden Zuschauer dahinter. Das gilt auch,
// wenn der Player-URL ein Query-String angehaengt ist (Signatur/CDN-Parameter) —
// ein Manifest mit Query darf weder als Segment noch als immutable ausgeliefert
// werden. Fuer saubere Playlist-Keys gilt der volle Vertrag inklusive Content-Type.
func TestZZBatRealPlaylistsStayMutableWithQueryStrings(t *testing.T) {
	for _, key := range []string{
		"live/room-1/live.m3u8",
		"live/room-1/index.m3u8",
		"clips/room-1/8f0b/clip.m3u8",
		"live/room-1/session.m3u",
	} {
		if got := ClassifyKey(key); got != ClassPlaylist {
			t.Fatalf("ClassifyKey(%q) = %q, erwartet ClassPlaylist", key, got)
		}
		if got := CacheControlForKey(key); got != PlaylistCacheControl {
			t.Fatalf("CacheControlForKey(%q) = %q, erwartet %q", key, got, PlaylistCacheControl)
		}
		if got := ContentTypeForKey(key); got != PlaylistContentType {
			t.Fatalf("ContentTypeForKey(%q) = %q, erwartet %q", key, got, PlaylistContentType)
		}
	}
	for _, key := range []string{
		"live/room-1/live.m3u8?token=abc123",
		"live/room-1/index.m3u8?hdnts=exp=1",
		"clips/room-1/8f0b/clip.m3u8?v=2",
	} {
		if got := ClassifyKey(key); got == ClassSegment {
			t.Fatalf("ClassifyKey(%q) = %q: ein Manifest mit Query-String darf nie als Segment eingestuft werden", key, got)
		}
		if cc := CacheControlForKey(key); strings.Contains(cc, "immutable") {
			t.Fatalf("CacheControlForKey(%q) = %q: ein Manifest darf mit Query-String nie immutable ausgeliefert werden", key, cc)
		}
	}
}

// WHY: Alles, was nicht als Segment erkannt wird, ist mutabel (das Paket ist
// konservativ: falsch-mutabel kostet eine Revalidierung, falsch-immutable friert
// einen Stream ein). Unbekannte Objekte — mit oder ohne Query-String — duerfen
// also nie immutable rausgehen.
func TestZZBatRealUnknownKeysNeverImmutable(t *testing.T) {
	for _, key := range []string{
		"live/room-1/whatever",
		"live/room-1/thumb.jpg",
		"live/room-1/captions.vtt",
		"live/room-1/playlist.m3u8.json",
		"live/room-1/unknown?x=1",
	} {
		if got := ClassifyKey(key); got == ClassSegment {
			t.Fatalf("ClassifyKey(%q) = %q: ein unbekannter Key ist kein immutables Segment", key, got)
		}
		if cc := CacheControlForKey(key); strings.Contains(cc, "immutable") {
			t.Fatalf("CacheControlForKey(%q) = %q: unbekannte Objekte duerfen nie immutable ausgeliefert werden", key, cc)
		}
	}
}

// WHY: Konsistenz Policy<->Klasse: Klassifikation und Cache-Control muessen
// zusammenhaengen (Klasse segment => immutable Segment-Header, Klasse playlist =>
// no-cache), und ein Segment darf nie mit einem generischen/leeren Content-Type
// ausgeliefert werden (Player wuerden es nicht dekodieren). Ueber eine gemischte
// Key-Menge inklusive webm.
func TestZZBatRealPolicyClassConsistency(t *testing.T) {
	segmentKeys := []string{
		"live/room-1/seg_00001.ts",
		"live/room-1/seg_00001.webm",
		"clips/room-1/8f0b/init.mp4",
		"live/room-1/seg_00001.m4s",
		"live/room-1/audio_00001.aac",
	}
	playlistKeys := []string{
		"live/room-1/live.m3u8",
		"live/room-1/index.m3u8",
		"clips/room-1/8f0b/clip.m3u8",
		"live/room-1/unknown-object",
	}
	for _, key := range segmentKeys {
		if got := ClassifyKey(key); got != ClassSegment {
			t.Fatalf("ClassifyKey(%q) = %q, erwartet ClassSegment", key, got)
		}
		if got := CacheControlForKey(key); got != SegmentCacheControl {
			t.Fatalf("CacheControlForKey(%q) = %q, erwartet %q", key, got, SegmentCacheControl)
		}
		if ct := ContentTypeForKey(key); ct == "" || ct == "application/octet-stream" {
			t.Fatalf("ContentTypeForKey(%q) = %q: ein Segment darf nicht generisch/leer ausgeliefert werden", key, ct)
		}
	}
	for _, key := range playlistKeys {
		if got := ClassifyKey(key); got != ClassPlaylist {
			t.Fatalf("ClassifyKey(%q) = %q, erwartet ClassPlaylist", key, got)
		}
		if got := CacheControlForKey(key); got != PlaylistCacheControl {
			t.Fatalf("CacheControlForKey(%q) = %q, erwartet %q", key, got, PlaylistCacheControl)
		}
	}
}

// WHY: Der Brief sagt es explizit: die Proxy-Konfiguration vor dem Origin-Bucket
// muss mit der Backend-Policy uebereinstimmen (der Segment-Router waehlt per
// Extension-Regexp die cachebaren Objekte aus). Unter -run ZZBat laeuft der
// Baseline-Kopplungstest nicht mit — deshalb prueft die Batterie direkt, dass der
// PathRegexp-Router der Edge-Config .webm in seiner Alternation fuehrt. Read-only
// auf die im Workspace gepinnte Datei, deterministisch.
func TestZZBatRealEdgeConfigAgreesOnWebm(t *testing.T) {
	const edgeConfig = "../../../deploy/hls-cache-headers.yml"
	raw, err := os.ReadFile(edgeConfig)
	if err != nil {
		t.Fatalf("Edge-Config %s nicht lesbar: %v — die Proxy-Konfiguration muss die Segment-Auslieferung mitfuehren", edgeConfig, err)
	}
	ruleLine := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "PathRegexp(") {
			ruleLine = line
			break
		}
	}
	if ruleLine == "" {
		t.Fatalf("kein PathRegexp-Segment-Router in %s — der Brief verlangt die Kopplung Policy<->Proxy", edgeConfig)
	}
	const opening = `\.(`
	start := strings.Index(ruleLine, opening)
	if start < 0 {
		t.Fatalf("Segment-Router %q waehlt nicht per Extension-Alternation aus", ruleLine)
	}
	rest := ruleLine[start+len(opening):]
	end := strings.Index(rest, `)$`)
	if end < 0 {
		t.Fatalf("Segment-Router %q hat keine abgeschlossene Extension-Alternation", ruleLine)
	}
	hasWebm := false
	for _, ext := range strings.Split(rest[:end], "|") {
		if strings.TrimSpace(ext) == "webm" {
			hasWebm = true
		}
	}
	if !hasWebm {
		t.Fatalf("Segment-Router %q kennt .webm nicht — Backend und Proxy muessen dieselbe Segment-Extension-Liste tragen", ruleLine)
	}
}

// ---------- Stufe 2: pathologische Kanten (TestZZBatPath*) ----------
// Bestanden heisst hier nur: kein Panic, kein Hang, Terminierung. KEINE
// fachliche Deutung wird erzwungen.

// WHY: Leere und nur-Whitespace-Keys sind kaputte Aufrufe, die stromaufwaerts
// entstehen koennen. Die Klassifikation darf daran nicht abstuerzen.
func TestZZBatPathEmptyKeys(t *testing.T) {
	for _, key := range []string{"", " ", "\t", "\n", "."} {
		zzb3NoPanic(t, "empty-keys", key)
	}
}

// WHY: Sehr lange Keys (tiefe Pfade, lange Segmentnamen, generierte IDs) sind
// bei Skalierung normal. Die Klassifikation ist reine String-Arbeit und muss in
// endlicher Zeit ohne Panic durchlaufen.
func TestZZBatPathVeryLongKeys(t *testing.T) {
	longName := strings.Repeat("segment-0123456789", 5000)
	deep := strings.Repeat("room-0123456789/", 500)
	zzb3NoPanic(t, "long-keys", "live/"+deep+longName+".webm")
	zzb3NoPanic(t, "long-keys-nosep", strings.Repeat("x", 65536))
}

// WHY: Sonderzeichen und Unicode in Raum-/Dateinamen (z. B. Umlaute, Emojis,
// reservierte URL-Zeichen) kommen aus echten Benutzereingaben. Auch ein
// Segment-Key mit Query-String ist pathologisch (gehoert nicht zur Bucket-Key-
// Schreibweise, erscheint aber in URLs) — bestanden heisst nur: kein Panic, kein
// Hang; die Deutung bleibt offen.
func TestZZBatPathSpecialCharsAndUnicode(t *testing.T) {
	for _, key := range []string{
		"live/ürgrund-räume/seg_1.webm",
		"live/room-🎙/seg_1.webm",
		"live/room with spaces/seg_1.webm",
		"live/room-1/seg_1.webm?token=abc&sig=%2F&v=1",
		"live/room-1/100%25+seg#1.webm",
	} {
		zzb3NoPanic(t, "special-chars", key)
	}
}

// WHY: Traversal-Strings in Keys waeren bei Dateisystem-Zugriff eine Gefahr —
// hier sind sie nur Strings. Bestanden heisst: keine Panik, keine Endlosschleife;
// die Klassifikation darf durch ".."/"../" nichts kaputt machen.
func TestZZBatPathTraversalStrings(t *testing.T) {
	for _, key := range []string{
		"../../../etc/passwd",
		"../live/../room/seg.webm",
		"..\\..\\live\\seg.webm",
		"/etc/passwd",
		"live/room/../seg.webm",
	} {
		zzb3NoPanic(t, "traversal", key)
	}
}

// WHY: Doppelte Slashes und abschliessende Slashes sind kaputte Pfad-Schreibweisen
// aus zusammengesetzten Strings. Keine Panik, keine Endlosschleife; die Deutung
// ist der Implementierung ueberlassen.
func TestZZBatPathDoubleAndTrailingSlashes(t *testing.T) {
	for _, key := range []string{
		"live//room//seg.webm",
		"live/room/seg.webm/",
		"//live/room/seg.webm",
		"live/room//seg_1.ts",
	} {
		zzb3NoPanic(t, "slashes", key)
	}
}
