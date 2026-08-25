/**
 * Robustheits-Batterie fuer agora-A2-jsonld — ZUSATZ-Metrik, aendert keine PASS/FAIL-Urteile.
 * Diese Datei wird zur Laufzeit als agora-web/tests/zz_battery.test.mjs in eine Wegwerf-Kopie
 * des Workspace kopiert und nur mit "node --test --test-reporter=tap tests/zz_battery.test.mjs"
 * aufgerufen. Modell-eigene Tests laufen nicht.
 *
 * Der Brief (bench/tasks/agora-A2-jsonld/prompt.txt) fordert einen Serialisierungs-Helper
 * jsonLdScriptBody (src/lib/jsonLdScript.ts), der den Body des inline
 * <script type="application/ld+json">-Blocks der Debatten-Seite so erzeugt, dass user-authored
 * Text (Titel, These) den Block nicht verlassen kann: Die HTML-Spec beendet einen
 * script-Element-Body beim LITERALEN `</script`-Text (und kennt in der Escaped State die
 * Kommentar-Sequenzen <!-- ... -->), sie parst dort kein JSON. Die Spezifikation des Tasks
 * (tests/jsonLdScript.test.mjs) verlangt deshalb:
 *   * keine `</script`-Ausbruchs-Sequenz, kein unescaptes `<`/`>`,
 *   * `&` als \u0026 (ueberlebt andere HTML-Kontexte),
 *   * die Ausgabe bleibt gueltiges JSON und JSON.parse liefert deep-equal die Eingabe
 *     (HTML-Entity-Escaping wie &lt; waere falsch).
 * Diese Batterie benutzt exakt diese Schnittstelle — der einzige Brief-Vertrag des Tasks —
 * und kodiert ausschliesslich Verhalten, das der Brief impliziert, nie die Form einer
 * Implementierung. Sie wiederholt NICHT die Faelle des versteckten Graders (grade.sh: nur die
 * 4 Spec-Tests + Verdrahtungs-Grep in page.tsx), sondern ergaenzt Kanten der Serialisierung.
 * Die Verdrahtung in page.tsx prueft der Grader selbst; die Batterie testet sie bewusst nicht
 * (keine Dateisystem-Leseabhängigkeit, kein Grading-Duplikat).
 *
 * Zwei Stufen, am Testnamen erkennbar (Konvention):
 *   ZZBATReal ... realistische Kanten, voller Brief-Vertrag (kein Ausbruch, kein unescaptes
 *                 <>/&, valid JSON, semantische Gleichheit nach JSON.parse, Idempotenz).
 *   ZZBATPath ... pathologische Eingaben; bestanden heisst nur: kein Crash/Hang, Terminierung,
 *                 (kein Datenverlust, wo eine Rundreise sinnvoll ist). KEINE fachliche
 *                 Deutung wird erzwungen — ein Helper darf bei Zyklen/BigInt werfen oder
 *                 einen Wert liefern, undefined/NaN/Infinity in null/Default ueberfuehren.
 *
 * WICHTIG zum Baseline-Selbsttest: In der Baseline existiert src/lib/jsonLdScript.ts nicht
 * (das ist genau das Problem des Tasks). Der Helper wird deshalb nicht statisch importiert
 * (das wuerde den ganzen Lauf als buildable:false kippen), sondern einmalig dynamisch und
 * fehlerabgefangen: Ist die Datei nicht da, reissen die Real-Tests einzeln mit klarer Meldung,
 * und die Path-Tests gelten als unerheblich (skip) — Terminierung ist ohnehin garantiert.
 *
 * Deterministisch: kein Zufall, keine Zeit-/Netz-/Dateisystem-Nebenwirkungen. Jeder Test
 * traegt ein eigenes node:test-Timeout (5 s); ein echter synchroner Haenger kann node nicht
 * preempten — den faengt der Runner-Timeout (150 s) und verbucht buildable:false (bekanntes
 * Node-Charakteristikum, siehe done-05.txt). Alle Bezeichner sind zzbat-praefixiert.
 */

import test from 'node:test';
import assert from 'node:assert/strict';

// ---- Helper-Laden (fehlerabgefangen, s.o.) ------------------------------------------------

let zzbatHelper = null;
let zzbatHelperError = null;
try {
  const mod = await import('../src/lib/jsonLdScript.ts');
  zzbatHelper = mod.jsonLdScriptBody;
} catch (err) {
  zzbatHelperError = String(err && err.message ? err.message : err);
}

/** Real-Stufe: ruft den Helper; fehlt er, failt der Test sichtbar (Baseline = reisst). */
function zzbatBody(value, label) {
  if (zzbatHelperError) assert.fail(`${label}: jsonLdScriptBody nicht verfuegbar — ${zzbatHelperError}`);
  if (typeof zzbatHelper !== 'function') assert.fail(`${label}: jsonLdScriptBody ist keine Funktion`);
  return zzbatHelper(value);
}

/** Path-Stufe: Kapselt den Helper-Aufruf; skip, wenn es nichts zu testen gibt. */
function zzbatTryBody(value) {
  if (zzbatHelperError || typeof zzbatHelper !== 'function') return { skipped: true };
  try {
    return { ok: true, out: zzbatHelper(value) };
  } catch {
    return { threw: true };
  }
}

/** Path-Pass-Kriterium: Terminierung; falls ein String zurueckkam, ist es einer. */
function zzbatAssertPathTerminates(r, label) {
  if (r.skipped) return;
  if (r.ok) assert.equal(typeof r.out, 'string', `${label}: Helper liefert keine Zeichenkette`);
}

// ---------------------------------------------------------------- Stufe 1: Real

// WHY: Der Kernbefund des Briefs — `</script>` in user-authored Text schliesst den
// ld+json-Block und macht den Rest zu lebendigem Markup. Der volle Brief-Vertrag: kein
// `</script`, kein unescaptes `<`/`>` (die HTML-Spec scannt den Body nach literalem
// `</script`, bevor irgendetwas als JSON gelesen wird), und die Ausgabe bleibt semantisch
// die Eingabe. Alle Formvarianten (Gross/Klein, mit Attributen, ohne `>`, mittig im Text).
function zzbatCheckBreakoutFree(body, label) {
  assert.equal(typeof body, 'string', `${label}: Helper liefert keine Zeichenkette`);
  assert.ok(!body.includes('</script'), `${label}: Ausgabe enthaelt noch </script`);
  assert.ok(!/[<>]/.test(body), `${label}: unescaptes < oder > ueberlebt`);
}

test('ZZBatReal ClosingScriptBreakoutNeutralised', { timeout: 5000 }, () => {
  const payloads = [
    '</script>',
    'a</script>b',
    '</script><img src=x onerror=alert(1)>',
    '<script>evil()</script>',
    '</ScRiPt>',
    '</script ',
    '... them.</script>',
    '<svg/onload=alert(1)>',
  ];
  for (const payload of payloads) {
    const value = { headline: `t ${payload}`, articleBody: payload, url: 'https://x.example/d' };
    const body = zzbatBody(value, `breakout ${JSON.stringify(payload)}`);
    zzbatCheckBreakoutFree(body, `breakout ${JSON.stringify(payload)}`);
    assert.deepEqual(JSON.parse(body), value, `breakout ${JSON.stringify(payload)}: Rundreise kaputt`);
  }
});

// WHY: In der HTML-Script-Spec sind <!-- ... --> (Script-data-escaped state) zweite
// Ausbruchspforten: ein `<!--` kann den Rest des Bodys bis `-->` verschlucken. Der Brief
// verlangt, dass user-authored Text den Block nie verlassen kann; alle drei Zeichen <, >, -
// sind im geparsten JSON unkritisch, also muss die Serialisierung sie ertragen.
test('ZZBatReal HtmlCommentVectorsNeutralised', { timeout: 5000 }, () => {
  const payloads = ['<!--', '-->', '<!-- comment -->', '<!--<script>-->', '<!-- </script> -->', 'x<!--y-->z'];
  for (const payload of payloads) {
    const value = { note: payload, tags: [payload] };
    const body = zzbatBody(value, `comment ${JSON.stringify(payload)}`);
    assert.ok(!body.includes('<!--'), `comment ${JSON.stringify(payload)}: <!-- ueberlebt`);
    assert.ok(!body.includes('-->'), `comment ${JSON.stringify(payload)}: --> ueberlebt`);
    assert.ok(!/[<>]/.test(body), `comment ${JSON.stringify(payload)}: unescaptes < oder > ueberlebt`);
    assert.deepEqual(JSON.parse(body), value, `comment ${JSON.stringify(payload)}: Rundreise kaputt`);
  }
});

// WHY: Die Spec verlangt ausdruecklich, dass & und > als \u0026 / \u003e fliehen: & ist eine
// Entity-Grenze, die in andere HTML-Kontexte hineinwirkt, > beendet Tags. Nach dem Parsen
// muessen beide wieder exakt da sein (sonst waere das Escape falsch).
test('ZZBatReal AmpAndGtEscapedAsUnicode', { timeout: 5000 }, () => {
  const value = { about: 'Climate & Environment', note: 'a > b', extra: 'x < y & z' };
  const body = zzbatBody(value, 'amp-gt');
  assert.ok(body.includes('\\u0026'), '& muss als \\u0026 erscheinen');
  assert.ok(body.includes('\\u003e'), '> muss als \\u003e erscheinen');
  assert.ok(body.includes('\\u003c'), '< muss als \\u003c erscheinen');
  assert.ok(!/[<>&]/.test(body), 'kein unescaptes <, > oder & darf ueberleben');
  assert.deepEqual(JSON.parse(body), value, 'Rundreise kaputt');
});

// WHY: U+2028/U+2029 sind im JSON-String legal (und fuer JSON.parse unschaedlich), beenden
// aber JS-String-Literale, falls der Body je in einem JS-Kontext landet. Ob ein Helper sie
// zusaetzlich escaped oder nicht, ist laut Brief eine freie Design-Entscheidung (2 PASS-
// Abgaben escapen sie nicht); erzwungen wird nur: semantische Gleichheit nach dem Parsen.
test('ZZBatReal LineSeparatorsSurviveRoundTrip', { timeout: 5000 }, () => {
  const value = { a: 'line\u2028sep', b: 'para\u2029sep', c: 'mix\u2028\u2029', d: 'x\u2028y\u2029z' };
  const body = zzbatBody(value, 'line-sep');
  assert.equal(typeof body, 'string', 'Helper liefert keine Zeichenkette');
  assert.deepEqual(JSON.parse(body), value, 'U+2028/U+2029 durften beim Parsen nicht verloren gehen');
});

// WHY: Unicode-Escapes als BUCHSTABEN im Eingabetext (z. B. ein Thesis-String, der den Text
// "\u003c" enthaelt) und echte Unicode-Zeichen duerfen durch das Escape nach dem
// JSON.stringify nicht doppelt-escapt oder verfaelscht werden: JSON.stringify escapet den
// Backslash, und die anschliessende Ersetzung muss den Rest unangetastet lassen.
test('ZZBatReal UnicodeEscapeTextRoundTrips', { timeout: 5000 }, () => {
  const value = {
    literalEscape: '\\u003c\\u2028\\u0026',
    unicode: 'üñíçødé ☃ 中文 🚀',
    mixed: '\\u003c' + '<' + '>',
    combining: 'e\u0301\u0301',
    rtl: '\u05e9\u05dc\u05d5\u05dd',
  };
  const body = zzbatBody(value, 'unicode-escape');
  assert.deepEqual(JSON.parse(body), value, 'Unicode-Escape-Text muss die Rundreise unbeschadet ueberstehen');
  assert.ok(!/[<>]/.test(body), 'kein unescaptes < oder >');
});

// WHY: Strings mit Anfuehrungszeichen und Backslashes sind die haertesten Faelle fuer jede
// JSON-Nachbearbeitung: ein Ersetzen, das nicht auf dem Stringifizierten operiert, wuerde
// hier Zeichen verschlucken oder verdoppeln. Einzeiler-/Zweizeiler-/Tab-Brueche und
// woeertliche Backslash-Sequenzen muessen alle bytegenau zurueckkommen.
test('ZZBatReal QuotesAndBackslashesRoundTrip', { timeout: 5000 }, () => {
  const value = {
    double: 'say "hello"',
    single: "it's",
    bs: 'a\\b',
    dbl: 'a\\\\b',
    newline: 'a\nb',
    backslashN: 'a\\nb',
    tab: 'a\tb',
    mixed: '\\"\'\\',
    endings: 'ends with \\',
  };
  const body = zzbatBody(value, 'quotes-bs');
  assert.deepEqual(JSON.parse(body), value, 'Anfuehrungszeichen/Backslashes muessen die Rundreise ueberstehen');
  assert.ok(!/[<>]/.test(body));
});

// WHY: Die Debatten-Metadaten sind verschachtelte Objekte/Arrays, und die Schluessel selbst
// sind user-/datengetrieben (Themen-Slugs, Tags). Das Escape muss AUCH in Objekt-Schluesseln
// greifen (JSON.stringify escaped Schluessel nicht), und die Rundreise muss Schluessel und
// Werte exakt rekonstruieren — auch leere Schluessel und numerisch aussehende Schluessel.
test('ZZBatReal NestedStructuresRoundTrip', { timeout: 5000 }, () => {
  const value = {
    '': 'empty key',
    'a<b': 1,
    'c>d': '&',
    'x&y': [1, 2, 3],
    'ünï': { 'nested<key>': [{ deep: null, flag: true }] },
    arr: [[], [[]], [1, [2, [3]]]],
    mixed: { 0: 'zero', '0.5': 'point', '-1': 'neg' },
  };
  const body = zzbatBody(value, 'nested');
  assert.deepEqual(JSON.parse(body), value, 'verschachtelte Strukturen/Keys muessen die Rundreise ueberstehen');
  assert.ok(!/[<>]/.test(body), 'kein unescaptes < oder > (auch in Keys)');
});

// WHY: "Idempotenz (zweimal anwenden = einmal)": Wird der Round-Trip-Wert (JSON.parse der
// ersten Ausgabe) erneut serialisiert, muss bytegenau dieselbe Zeichenkette herauskommen —
// ein Nachladen/erneutes Rendern darf den eingebetteten Body nicht aendern. Genau die
// Staerke der \uXXXX-Escapes: Sie ueberleben den Parse als Originalzeichen und werden beim
// zweiten Lauf identisch wieder erzeugt.
test('ZZBatReal IdempotentApplyTwice', { timeout: 5000 }, () => {
  const value = {
    title: 'Debate </script> & "quotes"',
    body: 'a < b > c <!-- --> \u2028\u2029 \\u003c',
    nested: { t: ['x&y', 'p<q'] },
  };
  const once = zzbatBody(value, 'idem');
  const twice = zzbatBody(JSON.parse(once), 'idem-twice');
  assert.equal(twice, once, 'zweite Anwendung muss byteidentisch zur ersten sein');
  assert.deepEqual(JSON.parse(once), value, 'Rundreise kaputt');
});

// WHY: "Ausgabe bleibt gueltiges JSON und entspricht semantisch der Eingabe" — der volle
// Vertrag ueber einen breiten, alle Vektoren mischenden Payload (Strings mit allen
// Sonderzeichen, Zahlen aller Formen, Booleans, null, leere Container, tiefe Schachtelung).
// JSON.parse darf nicht werfen und muss deep-equal zur Eingabe fuehren.
test('ZZBatReal ValidJsonSemanticEquality', { timeout: 5000 }, () => {
  const value = {
    str: 'quote " backslash \\ angle < > amp & line\u2028 para\u2029 comment <!-- -->',
    nums: [0, 1, -1, 1.5, -0.5, 1e-7, 1e21, 0.1, Number.MAX_SAFE_INTEGER, 42],
    bools: [true, false, null],
    empty: { emptyObj: {}, emptyArr: [] },
    deep: { a: { b: { c: { d: ['</script>', 'x>y'] } } } },
    keys: { 'k<1': 'v', 'k&2': null, 'k>3': [true] },
  };
  const body = zzbatBody(value, 'valid-json');
  zzbatCheckBreakoutFree(body, 'valid-json');
  const parsed = JSON.parse(body);
  assert.deepEqual(parsed, value, 'Ausgabe muss semantisch der Eingabe entsprechen');
});

// WHY: Der reale Einsatz — der JSON-LD-Block, den die Seite fuer eine oeffentliche Debatte
// erzeugt (DiscussionForumPosting, Titel/These user-authored, verschachtelte
// interactionStatistic und publisher). Genau dieser Block muss ohne Ausbruch auskommen und
// sich rundreisen lassen; die Verdrahtung selbst prueft der Grader.
test('ZZBatReal RealisticDebatePayload', { timeout: 5000 }, () => {
  const value = {
    '@context': 'https://schema.org',
    '@type': 'DiscussionForumPosting',
    headline: 'Is a hot dog a sandwich? </script>',
    articleBody: 'Reactors retired early raise emissions.</script><img/src=x onerror=alert(1)>',
    url: 'https://urgrund.example/debate/123',
    about: ['Climate & Environment', 'money & finance'],
    interactionStatistic: {
      '@type': 'InteractionCounter',
      interactionType: 'https://schema.org/CommentAction',
      userInteractionCount: 42,
    },
    publisher: { '@type': 'Organization', name: 'Urgrund' },
  };
  const body = zzbatBody(value, 'realistic');
  zzbatCheckBreakoutFree(body, 'realistic');
  assert.ok(body.includes('\\u0026'), '& muss escaped sein');
  assert.deepEqual(JSON.parse(body), value, 'Rundreise kaputt');
});

// ---------------------------------------------------------------- Stufe 2: Path
// Bestanden heisst hier nur: kein Crash/Hang, Terminierung (ggf. kein Datenverlust, wo eine
// Rundreise sinnvoll ist). KEINE fachliche Deutung wird erzwungen.

// WHY: Zyklische Referenzen sind kein JSON und JSON.stringify wirft nativ einen TypeError.
// Ob ein Helper wirft (vom Aufrufer faengbar) oder eine Zeichenkette liefert, bleibt ihm
// ueberlassen — er darf nur weder haengen noch den Prozess mitreissen.
test('ZZBatPath CyclicReferencesTerminate', { timeout: 5000 }, () => {
  const self = {};
  self.self = self;
  const arr = [];
  arr.push(arr);
  const mutual = {};
  mutual.o = {};
  mutual.o.back = mutual;
  for (const [name, value] of [['self', self], ['array', arr], ['mutual', mutual]]) {
    const r = zzbatTryBody(value);
    zzbatAssertPathTerminates(r, `cycle-${name}`);
  }
});

// WHY: BigInt ist in JSON.stringify ein TypeError, Symbol-Werte werden still geschluckt
// (Eigenschaften verschwinden, Array-Plaetze werden null). Beides sind vertretbare, native
// Deutungen — erzwungen wird nur Terminierung ohne Prozess-Crash.
test('ZZBatPath BigIntAndSymbolTerminate', { timeout: 5000 }, () => {
  const cases = [
    ['top-bigint', 1n],
    ['obj-bigint', { a: 1n, b: 2 }],
    ['arr-bigint', [1n, 2]],
    ['top-symbol', Symbol('s')],
    ['obj-symbol', { s: Symbol('x'), b: 1 }],
    ['arr-symbol', [Symbol('x'), 2]],
  ];
  for (const [name, value] of cases) {
    const r = zzbatTryBody(value);
    zzbatAssertPathTerminates(r, `bigint-${name}`);
  }
});

// WHY: Date-Objekte werden zu ISO-Strings, Funktionen verschwinden bzw. werden null. Das ist
// die native, verlustfreie-zu-String-Deutung von JSON.stringify; liefert der Helper eine
// Zeichenkette, muss sie zumindest gueltiges JSON sein.
test('ZZBatPath DateAndFunctionTerminate', { timeout: 5000 }, () => {
  const fn = () => 42;
  const cases = [
    ['top-date', new Date('2026-01-01T00:00:00.000Z')],
    ['obj-date', { d: new Date('2026-01-01T00:00:00.000Z'), n: 1 }],
    ['obj-fn', { f: fn, g: 'x' }],
    ['arr-fn', [fn, 1, 'y']],
  ];
  for (const [name, value] of cases) {
    const r = zzbatTryBody(value);
    zzbatAssertPathTerminates(r, `datefn-${name}`);
    if (r.ok) {
      assert.doesNotThrow(() => JSON.parse(r.out), `datefn-${name}: Ausgabe ist kein gueltiges JSON`);
    }
  }
});

// WHY: undefined, NaN, Infinity und -0 sind im JSON-Vertrag nicht darstellbar (Eigenschaften
// verschwinden, Werte werden null bzw. 0). Welche dieser nativen Deutungen ein Helper
// uebernimmt (auch Top-Level-undefined als "" oder "null"), bleibt frei — er darf nur
// terminieren, ohne zu crashen. Bewusst KEIN JSON-Gueltigkeitszwang: "" ist fuer
// Top-Level-undefined eine legitime Antwort.
test('ZZBatPath UndefinedNaNInfinityTerminate', { timeout: 5000 }, () => {
  const cases = [
    ['top-undefined', undefined],
    ['obj-undefined', { a: undefined, b: 1 }],
    ['arr-undefined', [undefined, 1]],
    ['top-nan', NaN],
    ['obj-nan', { n: NaN }],
    ['top-inf', Infinity],
    ['top-neg-inf', -Infinity],
    ['negzero', -0],
    ['deep-garbage', { a: { b: [NaN, Infinity, undefined, null] } }],
  ];
  for (const [name, value] of cases) {
    const r = zzbatTryBody(value);
    zzbatAssertPathTerminates(r, `garbage-${name}`);
  }
});

// WHY: Riesen-Payloads (200k-Item-Array, ~1.4 MB-String, 1000 Ebenen tiefe Schachtelung)
// muessen im 5-s-Fenster terminieren; wo eine Rundreise moeglich ist (Array-Laenge,
// String-Inhalt, kein Ausbruch), darf nichts verloren gehen. Ein quadratisches oder
// re-entrantes Escaping wuerde hier ausbremsen oder haengen.
test('ZZBatPath HugePayloadsTerminate', { timeout: 5000 }, () => {
  const bigArray = Array.from({ length: 200000 }, (_, i) => ({ id: i, tag: 'x<>&', s: 'str-' + i }));
  const bigString = 'a<>&\u2028b'.repeat(200000);
  const deep = (() => {
    const root = {};
    let cur = root;
    for (let i = 0; i < 1000; i++) {
      cur.next = {};
      cur = cur.next;
    }
    cur.val = '</script>';
    return root;
  })();
  const cases = [
    ['array', bigArray, (parsed) => assert.equal(parsed.length, bigArray.length, 'Riesige Arrays muessen vollstaendig bleiben')],
    ['string', bigString, (parsed) => assert.equal(parsed, bigString, 'Riesiger String muss bytegenau zurueckkommen')],
    ['deep', deep, (parsed, raw) => assert.ok(!raw.includes('</script'), 'tief geschachtelter Ausbruch darf nicht ueberleben')],
  ];
  for (const [name, value, check] of cases) {
    const r = zzbatTryBody(value);
    zzbatAssertPathTerminates(r, `huge-${name}`);
    if (r.ok) {
      const parsed = JSON.parse(r.out);
      check(parsed, r.out);
    }
  }
});
