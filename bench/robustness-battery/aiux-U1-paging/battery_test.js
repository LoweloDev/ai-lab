/**
 * Robustheits-Batterie fuer aiux-U1-paging — ZUSATZ-Metrik, aendert keine PASS/FAIL-Urteile.
 * Diese Datei wird zur Laufzeit als runtime/web/test/zz_battery.test.js in den Workspace
 * kopiert und nur mit "node --test --test-reporter=tap test/zz_battery.test.js" aufgerufen.
 *
 * Der Brief (bench/tasks/aiux-U1-paging/prompt.txt) nennt genau ein Defektfeld: die
 * Paging-Metadaten von runtime.describe_type — "callers that request a page of type members
 * get a member slice and a hasMore flag that do not agree with totalMembers". Jeder Real-Test
 * kodiert deshalb eine Invariante dieser Uebereinstimmung:
 *   * members ist der Ausschnitt [offset, offset+limit) der Gesamtliste: nie laenger als
 *     limit, bis auf die letzte Seite immer exakt gefuellt (voller Brief-Vertrag),
 *   * hasMore sagt genau aus, ob hinter dieser Seite weitere Members kommen,
 *   * totalMembers ist eine stabile Gesamtzahl, unabhaengig von Seite und Seitengroesse.
 * Die Form der Implementierung wird nie angenommen; benutzt werden nur die Baseline-
 * Schnittstellen createRuntime/createHost und der Tool-Aufruf runtime.describe_type, damit
 * die Batterie auf JEDER korrekten Loesung laeuft. Modell-eigene Tests laufen nicht (es wird
 * nur diese eine Datei aufgerufen).
 *
 * Zwei Stufen, am Testnamen erkennbar (Konvention):
 *   ZZBATReal  ...  realistische Kanten, voller Brief-Vertrag.
 *   ZZBATPath  ...  pathologische Eingaben; bestanden heisst nur: kein Crash/Panic/Hang,
 *                   kein Datenverlust, Terminierung. KEINE fachliche Deutung wird erzwungen
 *                   (eine Loesung darf negative Offsets ablehnen oder klammern, NaN auf einen
 *                   Default mappen usw. — nur terminieren und strukturiert antworten muss sie).
 * Jeder Tool-Aufruf laeuft mit eigenem Timeout, damit ein Haenger den Testlauf nicht mitreisst.
 */

import test from 'node:test';
import assert from 'node:assert/strict';

import { createRuntime } from '../src/index.js';
import { createHost } from './host-fixture.js';

let zzbatSeq = 0;

/** Baut einen frischen Host + Runtime auf Baseline-Art; zusaetzliche Roots koennen gepinnt werden. */
function zzbatSetup(extraRoots = []) {
  const host = createHost();
  const runtime = createRuntime({ graphBudget: 4000, roots: [host, ...extraRoots] });
  return { host, runtime };
}

function zzbatDispatch(runtime, args) {
  return runtime.dispatch({
    version: 1,
    requestId: `zzbat-${++zzbatSeq}`,
    tool: 'runtime.describe_type',
    arguments: args,
  });
}

/** Terminiert garantiert: Race gegen ein Zeitlimit, danach clearTimeout (kein offener Timer). */
function zzbatWithTimeout(promise, ms, label) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`${label}: keine Terminierung nach ${ms} ms (Hang?)`)),
      ms,
    );
    promise.then(
      (v) => { clearTimeout(timer); resolve(v); },
      (e) => { clearTimeout(timer); reject(e); },
    );
  });
}

/** Real-Stufe: realistische Eingabe, der volle Brief-Vertrag muss halten → ok:true. */
async function zzbatCall(runtime, args, label) {
  const response = await zzbatWithTimeout(zzbatDispatch(runtime, args), 5000, label);
  assert.equal(response.ok, true, `${label}: describe_type unerwartet fehlgeschlagen: ${JSON.stringify(response.error)}`);
  return response.result;
}

/** Path-Stufe: bestanden heisst nur eine strukturierte, terminierende Antwort. */
async function zzbatPath(runtime, args, label) {
  const response = await zzbatWithTimeout(zzbatDispatch(runtime, args), 5000, label);
  assert.equal(typeof response, 'object', `${label}: keine Envelope`);
  assert.equal(typeof response.ok, 'boolean', `${label}: Antwort ist keine strukturierte Envelope`);
  assert.equal(response.version, 1, `${label}: Antwort traegt keine Vertrags-Version`);
  return response;
}

// ---------------------------------------------------------------- Stufe 1: Real

// WHY: Der Brief klagt genau die erste Seite an: Der Aufrufer bekommt eine Member-Slice, die
// nicht zu totalMembers passt (die Baseline lieferte bei limit=2 nur 1 Member). Eine korrekte
// Seite muss exakt min(limit, totalMembers-offset) Members tragen und hasMore genau dann true
// sein, wenn dahinter noch Members kommen — nichts verloren, nichts erfunden.
test('ZZBatReal FirstPageIsFull', async () => {
  const { runtime } = zzbatSetup();
  const r = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 2 }, 'first-page');
  assert.ok(r.totalMembers >= 2, `PricingPolicy hat nur ${r.totalMembers} Members`);
  assert.equal(r.members.length, Math.min(2, r.totalMembers));
  assert.equal(r.hasMore, r.members.length < r.totalMembers);
  for (const member of r.members) {
    assert.equal(typeof member.name, 'string');
    assert.ok(member.name.length > 0, 'Member ohne Namen');
  }
});

// WHY: Seitengrenze limit=1 (jeder Aufrufer, der Member einzeln durchgeht). Beim Blaettern
// aller Seiten muss jede Seite genau 1 Member tragen, nur die letzte hasMore=false, und die
// Aneinanderreihung aller Seiten exakt die Gesamtliste ergeben — nichts doppelt, nichts
// verloren, stabile Reihenfolge.
test('ZZBatReal WalkPageSizeOne', async () => {
  const { runtime } = zzbatSetup();
  const full = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 500 }, 'walk-one-full');
  const names = [];
  for (let offset = 0; ; offset++) {
    if (offset > full.totalMembers) assert.fail('Blaettern terminiert nicht');
    const page = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset, limit: 1 }, `walk-one-${offset}`);
    assert.equal(page.totalMembers, full.totalMembers, 'Gesamtzahl darf beim Blaettern nicht driften');
    if (offset >= page.totalMembers) {
      assert.equal(page.members.length, 0);
      assert.equal(page.hasMore, false);
      break;
    }
    assert.equal(page.members.length, 1);
    assert.equal(page.hasMore, offset + 1 < page.totalMembers);
    names.push(page.members[0].name);
  }
  assert.deepEqual(names, full.members.map((member) => member.name));
});

// WHY: Seitengrenze limit=n (genau die Gesamtzahl): eine einzige Seite, die ALLE Members
// traegt, hasMore=false. Ein Aufrufer, der "alles will", muss alles bekommen.
test('ZZBatReal LimitEqualsTotal', async () => {
  const { runtime } = zzbatSetup();
  const n = (await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 1 }, 'limit-eq-n-n')).totalMembers;
  const page = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: n }, 'limit-eq-n');
  assert.equal(page.members.length, n);
  assert.equal(page.hasMore, false);
});

// WHY: Seitengrenze limit=n+1 (ein Member mehr als existiert): die Seite darf nicht groesser
// werden als die Gesamtliste, nur die letzte Seite darf kuerzer sein; hasMore=false.
test('ZZBatReal LimitEqualsTotalPlusOne', async () => {
  const { runtime } = zzbatSetup();
  const n = (await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 1 }, 'limit-n1-n')).totalMembers;
  const page = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: n + 1 }, 'limit-n1');
  assert.equal(page.members.length, n);
  assert.equal(page.hasMore, false);
});

// WHY: Absurd grosse Seitengroesse (Obergrenze des Tool-Vertrags): alles in einer Seite,
// hasMore=false, Gesamtzahl unveraendert. Der Brief-Vertrag schuetzt den Caller, der bewusst
// gross paginiert, vor einer kuerzeren Seite.
test('ZZBatReal HugeLimitOnePage', async () => {
  const { runtime } = zzbatSetup();
  const n = (await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 1 }, 'huge-n')).totalMembers;
  const page = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 500 }, 'huge-limit');
  assert.equal(page.members.length, n);
  assert.equal(page.hasMore, false);
});

// WHY: Off-by-one an der hinteren Seitenkante — die Stelle, an der die Baseline die letzte
// Member verschluckte. offset=n-1 (der letzte Member) muss noch genau 1 Member liefern und
// hasMore=false; offset=n und offset=n+5 sind leere Seiten mit hasMore=false und stabiler
// Gesamtzahl.
test('ZZBatReal LastPageOffByOne', async () => {
  const { runtime } = zzbatSetup();
  const full = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 500 }, 'last-full');
  const n = full.totalMembers;
  assert.ok(n > 0);
  const last = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: n - 1, limit: 1 }, 'last-page');
  assert.equal(last.members.length, 1);
  assert.equal(last.hasMore, false);
  assert.equal(last.members[0].name, full.members[n - 1].name, 'letzte Member muss dieselbe sein wie in der Gesamtliste');
  for (const offset of [n, n + 5]) {
    const page = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset, limit: 1 }, `beyond-${offset}`);
    assert.equal(page.members.length, 0);
    assert.equal(page.hasMore, false);
    assert.equal(page.totalMembers, n);
  }
});

// WHY: Gesamtzahl konsistent: totalMembers darf nicht von der gewaehlten Seite, der
// Seitengroesse oder dem Offset abhaengen. Genau das nennt der Brief als den Defekt
// ("do not agree with totalMembers").
test('ZZBatReal TotalConsistentAcrossPages', async () => {
  const { runtime } = zzbatSetup();
  const seen = new Set();
  for (const [offset, limit] of [[0, 1], [2, 3], [5, 500], [7, 2], [100, 1]]) {
    const page = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset, limit }, `total-${offset}-${limit}`);
    seen.add(page.totalMembers);
  }
  assert.equal(seen.size, 1, 'totalMembers muss ueber alle Seiten identisch sein');
});

// WHY: Stabile Reihenfolge beim Blaettern mit groesserer Seitengroesse: mehrere Seiten mit
// limit=3, aneinandergereiht, muessen exakt die Gesamtliste in derselben Reihenfolge ergeben
// (nichts verloren, nichts doppelt, keine leere Zwischenseite).
test('ZZBatReal WalkLimitThreeStableOrder', async () => {
  const { runtime } = zzbatSetup();
  const full = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 0, limit: 500 }, 'walk3-full');
  const names = [];
  const limit = 3;
  for (let offset = 0; offset < full.totalMembers; offset += limit) {
    const page = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset, limit }, `walk3-${offset}`);
    assert.equal(page.members.length, Math.min(limit, full.totalMembers - offset));
    assert.equal(page.hasMore, offset + page.members.length < full.totalMembers);
    for (const member of page.members) names.push(member.name);
  }
  assert.deepEqual(names, full.members.map((member) => member.name));
});

// WHY: Stabile Reihenfolge ueber Aufrufe hinweg: dieselbe Seite zweimal anfragen muss
// zweimal dieselben Members in derselben Reihenfolge liefern — ein Caller, der nachlae dt,
// darf keinen Sprung sehen.
test('ZZBatReal RepeatedCallsIdentical', async () => {
  const { runtime } = zzbatSetup();
  const a = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 1, limit: 2 }, 'repeat-a');
  const b = await zzbatCall(runtime, { typeName: 'PricingPolicy', offset: 1, limit: 2 }, 'repeat-b');
  assert.deepEqual(a, b);
});

// WHY: Die Paging-Invariante darf nicht an einer einzigen Fixture-Klasse haengen. Auch die
// zweite Klasse der Fixture (Catalogue, mit Instanzfeldern) muss gleichmaessig paginieren —
// das deckt Loesungen auf, die nur den einen bekannten Typ "richtig" machen.
test('ZZBatReal SecondTypeGenerality', async () => {
  const { runtime } = zzbatSetup();
  const n = (await zzbatCall(runtime, { typeName: 'Catalogue', offset: 0, limit: 1 }, 'cat-n')).totalMembers;
  assert.ok(n > 0, 'Catalogue sollte Members haben');
  const walked = [];
  for (let offset = 0; offset < n; offset++) {
    const page = await zzbatCall(runtime, { typeName: 'Catalogue', offset, limit: 1 }, `cat-${offset}`);
    assert.equal(page.members.length, 1);
    assert.equal(page.hasMore, offset + 1 < n);
    walked.push(page.members[0].name);
  }
  const full = await zzbatCall(runtime, { typeName: 'Catalogue', offset: 0, limit: 500 }, 'cat-full');
  assert.deepEqual(walked, full.members.map((member) => member.name));
});

// WHY: Leere Liste: ein Typ ohne eigene Members muss eine leere Members-Liste mit
// totalMembers=0 und hasMore=false liefern — der Caller darf weder eine Phantom-Seite noch
// eine Gesamtzahl != 0 sehen. Die leere Klasse wird ueber die Baseline-Option roots gepinnt.
test('ZZBatReal EmptyTypeEmptyPages', async () => {
  const empty = class Empty {};
  const { runtime } = zzbatSetup([empty]);
  const page = await zzbatCall(runtime, { typeName: 'Empty', offset: 0, limit: 5 }, 'empty-type');
  assert.equal(page.totalMembers, 0);
  assert.deepEqual(page.members, []);
  assert.equal(page.hasMore, false);
});

// ---------------------------------------------------------------- Stufe 2: Path
// Bestanden heisst hier nur: keine Endlosschleife, kein Crash, strukturierte Antwort.

// WHY: Negative und gebrochene Offsets sind kaputte Caller-Argumente. Ob eine Loesung sie
// ablehnt (ok:false, strukturierter Fehler) oder klamment, ist ihre Sache — sie darf nur
// weder haengen noch crashen noch eine unstrukturierte Antwort liefern.
test('ZZBatPath NegativeOffsetTerminates', async () => {
  const { runtime } = zzbatSetup();
  for (const offset of [-1, -1000, -0.5]) {
    const response = await zzbatPath(runtime, { typeName: 'PricingPolicy', offset, limit: 5 }, `neg-${offset}`);
    assert.ok('ok' in response);
  }
});

// WHY: NaN-, undefined- und null-Seiten sind keine gueltigen Seitenzahlen. undefined darf als
// Default interpretiert werden; NaN/null muessen mindestens strukturiert behandelt werden.
// Keine fachliche Deutung wird erzwungen — nur Terminierung und eine Envelope.
test('ZZBatPath NaNandUndefinedPagesTerminate', async () => {
  const { runtime } = zzbatSetup();
  const cases = [
    { offset: NaN, limit: 5 },
    { offset: undefined, limit: 5 },
    { offset: null, limit: 5 },
    { offset: 0, limit: NaN },
    { offset: 0, limit: undefined },
    { offset: 0, limit: null },
  ];
  for (const args of cases) {
    const response = await zzbatPath(runtime, { typeName: 'PricingPolicy', ...args }, `nan-${JSON.stringify(args)}`);
    assert.ok('ok' in response);
  }
});

// WHY: Absurde Seitengroessen (0, negativ, gebrochen, ueber der Vertragsobergrenze) und ein
// riesiger Offset duerfen weder haengen noch crashen. Die Deutung bleibt frei.
test('ZZBatPath AbsurdPageSizesTerminate', async () => {
  const { runtime } = zzbatSetup();
  const cases = [
    { offset: 0, limit: 0 },
    { offset: 0, limit: -5 },
    { offset: 0, limit: 1e9 },
    { offset: 0, limit: 2.5 },
    { offset: 0, limit: Number.MAX_SAFE_INTEGER },
    { offset: 1e7, limit: 5 },
  ];
  for (const args of cases) {
    const response = await zzbatPath(runtime, { typeName: 'PricingPolicy', ...args }, `absurd-${JSON.stringify(args)}`);
    assert.ok('ok' in response);
  }
});

// WHY: Nicht-Arrays bzw. Nicht-Objekte als arguments (String, null) und Nicht-String-Typnamen
// (Zahl, Array, Objekt) sind kaputte Eingaben. Die Dispatch-Maschinerie muss sie strukturiert
// behandeln statt zu crashen; ob sie mit einem Argument-Fehler antwortet, bleibt frei.
test('ZZBatPath NonArrayInputsTerminate', async () => {
  const { runtime } = zzbatSetup();
  const attempts = [
    runtime.dispatch({ version: 1, requestId: `zzbat-${++zzbatSeq}`, tool: 'runtime.describe_type', arguments: 'garbage' }),
    runtime.dispatch({ version: 1, requestId: `zzbat-${++zzbatSeq}`, tool: 'runtime.describe_type', arguments: null }),
    runtime.dispatch({ version: 1, requestId: `zzbat-${++zzbatSeq}`, tool: 'runtime.describe_type', arguments: { typeName: 42, limit: 5 } }),
    runtime.dispatch({ version: 1, requestId: `zzbat-${++zzbatSeq}`, tool: 'runtime.describe_type', arguments: { typeName: ['PricingPolicy'], limit: 5 } }),
    runtime.dispatch({ version: 1, requestId: `zzbat-${++zzbatSeq}`, tool: 'runtime.describe_type', arguments: { typeName: {}, limit: 5 } }),
  ];
  for (const promise of attempts) {
    const response = await zzbatWithTimeout(promise, 5000, 'non-array');
    assert.equal(typeof response, 'object');
    assert.equal(typeof response.ok, 'boolean');
  }
});
