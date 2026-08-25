/**
 * Robustheits-Batterie fuer aiux-U2-denytools — ZUSATZ-Metrik, aendert keine PASS/FAIL-Urteile.
 * Diese Datei wird zur Laufzeit als runtime/web/test/zz_battery.test.js in den Workspace
 * kopiert und nur mit "node --test --test-reporter=tap test/zz_battery.test.js" aufgerufen.
 *
 * Der Brief (bench/tasks/aiux-U2-denytools/prompt.txt) fuehrt eine dritte Layer-2-Option ein:
 *   config.denyTools = [ "tool.name", ... ]  — eine exakte, pro-Tool-Sperre, die am einzigen
 *   Dispatch-Chokepoint greift (Security-Denial statt Tool-Logik), das Tool aus
 *   capabilities().tools verschwinden laesst und unter describe()/integratorDeniedTools
 *   berichtet wird. Zentraler Satz des Briefs: Layer 2 kann nur HINZUFUEGEN, nie lockern —
 *   Layer 1 (Selbstschutz des Runtimes) bleibt unangetastet.
 *
 * Die versteckten Grader decken schon ab: payload.list als Muster-Denial, die Alias-Paare
 * beider Richtungen, einen Vertreter je Tool-Familie, den kompletten Sweep ueber ALLE
 * beworbenen Tools einzeln sowie Layer 1 per Identitaet (write_field auf gepinntem Handle).
 * Die Batterie ergaenzt KANTEN, die dort fehlen (Spec 07): generische Alias-Paare, die nicht
 * search_types/search_classes sind, Gross-/Kleinschreibung, Kombinationen beider Layer,
 * Reihenfolge-Unabhaengigkeit, zur Laufzeit gewaehltes Tool, Layer-1-Sperre per Typname,
 * beides Spellings eines Alias-Paars gleichzeitig, und ein Tool, das an sich nur einen
 * Argument-Fehler wuerfe (ui.snapshot). Verwendet werden nur Baseline-Schnittstellen
 * (createRuntime/createHost + Dispatch-Envelope) und der vom Brief geforderte Config-Schluessel.
 *
 * Zwei Stufen, am Testnamen erkennbar (Konvention):
 *   ZZBATReal  ...  realistische Kanten, voller Brief-Vertrag.
 *   ZZBATPath  ...  pathologische denyTools-Listen; bestanden heisst nur: kein Crash/Panic/Hang,
 *                   strukturierte Envelope, Terminierung. KEINE fachliche Deutung wird erzwungen.
 * Jeder Tool-Aufruf laeuft mit eigenem Timeout, damit ein Haenger den Testlauf nicht mitreisst.
 */

import test from 'node:test';
import assert from 'node:assert/strict';

import { createRuntime } from '../src/index.js';
import { createHost } from './host-fixture.js';

let zzbatSeq = 0;

/** Baut einen frischen Host + Runtime auf Baseline-Art; zusaetzliche Layer-2-Config wird durchgereicht. */
function zzbatSetup(config = {}) {
  const host = createHost();
  const runtime = createRuntime({ graphBudget: 4000, roots: [host], ...config });
  return { host, runtime };
}

function zzbatDispatch(runtime, tool, args = {}) {
  return runtime.dispatch({ version: 1, requestId: `zzbat-${++zzbatSeq}`, tool, arguments: args });
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

/** Real-Stufe: der Aufruf muss mit ok:true zurueckkommen. */
async function zzbatCall(runtime, tool, args, label) {
  const response = await zzbatWithTimeout(zzbatDispatch(runtime, tool, args), 5000, label);
  assert.equal(response.ok, true, `${label}: ${tool} unerwartet fehlgeschlagen: ${JSON.stringify(response.error)}`);
  return response.result;
}

/**
 * Real-Stufe: der Aufruf MUSS als strukturierte Security-Denial zurueckkommen, die das
 * gesperrte Tool nennt — egal mit welcher Schreibweise des Paares (canonical oder Alias).
 */
async function zzbatSecurityDenial(runtime, tool, args, label, namedAs = [tool]) {
  const response = await zzbatWithTimeout(zzbatDispatch(runtime, tool, args), 5000, label);
  assert.equal(response.ok, false, `${label}: ${tool} muss gesperrt sein, kam aber ok:true zurueck`);
  assert.equal(response.error.type, 'RuntimeSecurityError',
    `${label}: ${tool}: erwartet eine Security-Denial, kam ${response.error.type}: ${response.error.message}`);
  assert.ok(namedAs.some((name) => response.error.message.includes(name)),
    `${label}: die Denial muss das Tool nennen (${namedAs.join(' oder ')}), kam: ${response.error.message}`);
  assert.ok(Array.isArray(response.error.stack), `${label}: eine Denial ist eine strukturierte Envelope, kein Throw`);
  return response.error;
}

/** Real-Stufe Kontrolle: ohne Sperre darf derselbe Aufruf nie als Security-Denial zurueckkommen. */
async function zzbatNotSecurityDenial(runtime, tool, args, label) {
  const response = await zzbatWithTimeout(zzbatDispatch(runtime, tool, args), 5000, label);
  if (!response.ok) {
    assert.notEqual(response.error.type, 'RuntimeSecurityError',
      `${label}: ${tool} ohne Sperre darf keine Security-Denial sein: ${response.error.message}`);
  }
  return response;
}

/** Tools, deren Denial der versteckte Grader (grade.test.js / grade.v2.test.js) schon prueft — die Batterie ficht sie nicht erneut an. */
const GRADER_DENIED = new Set([
  'runtime.capabilities', 'runtime.describe_type', 'runtime.find_instances', 'ui.inspect',
  'ui.mutate', 'payload.suspend_persistent', 'payload.delete', 'code.disassemble',
  'payload.list', 'runtime.search_types', 'ui.snapshot',
]);

// ---------------------------------------------------------------- Stufe 1: Real

// WHY: Eine Sperre ist wertlos, wenn sie erst nach der Argument-Validierung greift — dann wuerde
// das Modell noch Argument-Shapes und Fehlerdetails auslesen. payload.reapply und
// runtime.release_handle verlangen Pflicht-Argumente: Wer die Sperre zu spaet prueft, antwortet
// mit "id is required" statt mit einer Security-Denial. Die Sperre muss VOR jeder Tool-Logik am
// Chokepoint entscheiden. (Baseline kennt denyTools nicht → Argument-Fehler → Test reisst dort.)
test('ZZBatReal DeniedToolRefusedBeforeArguments', async () => {
  const { runtime } = zzbatSetup({ denyTools: ['payload.reapply', 'runtime.release_handle'] });
  await zzbatSecurityDenial(runtime, 'payload.reapply', {}, 'reapply-noargs');
  await zzbatSecurityDenial(runtime, 'runtime.release_handle', {}, 'release-noargs');

  const { runtime: open } = zzbatSetup();
  await zzbatNotSecurityDenial(open, 'payload.reapply', {}, 'open-reapply');
  await zzbatNotSecurityDenial(open, 'runtime.release_handle', {}, 'open-release');
});

// WHY: capabilities().tools ist das Modell des Modells von der Oberflaeche; ein gesperrtes Tool,
// das sich dort noch bewirbt, ist ein Vertrags-Luege. Zwei payload.*-Geschwister zu sperren muss
// genau diese zwei entfernen (payload.list/payload.delete/payload.suspend_persistent bleiben) und
// describe() muss exakt die konfigurierte Liste berichten. (Der Grader nutzt payload.list; die
// Batterie nimmt bewusst andere Mitglieder derselben Familie.)
test('ZZBatReal CapabilitiesHideDeniedAndKeepSiblings', async () => {
  const denied = ['payload.disable', 'payload.reapply'];
  const { runtime } = zzbatSetup({ denyTools: denied });
  const caps = await zzbatCall(runtime, 'runtime.capabilities', {}, 'caps-hide');
  for (const tool of denied) {
    assert.equal(caps.tools.includes(tool), false, `${tool} ist gesperrt und darf nicht beworben sein`);
  }
  for (const tool of ['payload.list', 'payload.delete', 'payload.suspend_persistent', 'runtime.search_types', 'ui.inspect']) {
    assert.equal(caps.tools.includes(tool), true, `${tool} ist nicht gesperrt und muss beworben bleiben`);
  }
  assert.deepEqual([...caps.security.integratorDeniedTools].sort(), [...denied].sort(),
    'describe() muss exakt die konfigurierte Liste berichten');
});

// WHY: denyTools ist eine Menge; Umordnen der Liste darf weder die beworbene Oberflaeche noch die
// berichtete Liste noch das Denial-Verhalten aendern. Zwei Runtimes mit identischer Menge in
// verschiedener Reihenfolge muessen sich exakt gleich verhalten — sonst wuerde die Reihenfolge der
// Integrator-Config ins Verhalten durchschlagen.
test('ZZBatReal OrderIndependence', async () => {
  const a = ['payload.reapply', 'payload.disable', 'ui.snapshot'];
  const b = ['ui.snapshot', 'payload.disable', 'payload.reapply'];
  const { runtime: ra } = zzbatSetup({ denyTools: a });
  const { runtime: rb } = zzbatSetup({ denyTools: b });
  const ca = await zzbatCall(ra, 'runtime.capabilities', {}, 'order-a-caps');
  const cb = await zzbatCall(rb, 'runtime.capabilities', {}, 'order-b-caps');
  assert.deepEqual(ca.tools, cb.tools, 'die beworbene Tool-Oberflaeche darf nicht von der Listen-Reihenfolge abhaengen');
  assert.deepEqual([...ca.security.integratorDeniedTools].sort(), [...cb.security.integratorDeniedTools].sort(),
    'die berichtete Sperrliste muss als Menge gleich sein');
  await zzbatSecurityDenial(ra, 'payload.reapply', {}, 'order-a-deny');
  await zzbatSecurityDenial(rb, 'payload.reapply', {}, 'order-b-deny');
});

// WHY: Toolnamen sind in diesem Adapter gross-/kleinschreibungssensitiv (Baseline-ALIASES/TOOLS
// sind exakt). DenyTools sperrt exakt die genannten Namen: 'PAYLOAD.LIST' ist KEIN Tool und darf
// unter einer payload.list-Sperre kein Security-Denial werden (sonst wuerde eine falsche
// Schreibweise eine Denial vortaeuschen), und ein falsch geschriebener Eintrag 'Payload.List'
// darf das echte payload.list nicht sperren. (Spec 07 verlangt Gross-/Kleinschreibung als Kante.)
test('ZZBatReal CaseSensitiveExactMatch', async () => {
  const { runtime } = zzbatSetup({ denyTools: ['payload.list'] });
  await zzbatSecurityDenial(runtime, 'payload.list', {}, 'case-right');
  await zzbatNotSecurityDenial(runtime, 'PAYLOAD.LIST', {}, 'case-unknown');

  const { runtime: bad } = zzbatSetup({ denyTools: ['Payload.List'] });
  const caps = await zzbatCall(bad, 'runtime.capabilities', {}, 'case-bad-caps');
  assert.equal(caps.tools.includes('payload.list'), true,
    'ein falsch geschriebener Eintrag darf das echte Tool nicht sperren');
  const listed = await zzbatCall(bad, 'payload.list', {}, 'case-bad-works');
  assert.ok(Array.isArray(listed.payloads));
});

// WHY: Layer 2 kombiniert Optionen; denyTools muss auch dann einschraenken, wenn der Integrator
// dieselbe Familie gleichzeitig freigibt (allowCodeReplacement:true / allowCodeLoading:true sind
// die Defaults). Die Sperre gewinnt: runtime.replace_method und runtime.load_code werden auch
// bei gesetzten Allow-Flags am Chokepoint verweigert und aus capabilities verschwinden — und
// ohne Sperr-Eintrag ist derselbe Aufruf ein gewoehnlicher Argument-Fehler, nie eine Denial.
test('ZZBatReal DenyWinsOverAllowFlags', async () => {
  const denied = ['runtime.replace_method', 'runtime.load_code'];
  const { runtime } = zzbatSetup({
    denyTools: denied,
    allowCodeReplacement: true,
    allowCodeLoading: true,
  });
  await zzbatSecurityDenial(runtime, 'runtime.replace_method', {}, 'replace-deny-wins');
  await zzbatSecurityDenial(runtime, 'runtime.load_code', {}, 'load-deny-wins');
  const caps = await zzbatCall(runtime, 'runtime.capabilities', {}, 'allowflag-caps');
  assert.equal(caps.tools.includes('runtime.replace_method'), false);
  assert.equal(caps.tools.includes('runtime.load_code'), false);
  assert.deepEqual([...caps.security.integratorDeniedTools].sort(), [...denied].sort());

  const { runtime: open } = zzbatSetup({ allowCodeReplacement: true, allowCodeLoading: true });
  await zzbatNotSecurityDenial(open, 'runtime.replace_method', {}, 'open-replace');
  await zzbatNotSecurityDenial(open, 'runtime.load_code', {}, 'open-load');
});

// WHY: denyTools und denyTypePrefix sind unabhaengige Layer-2-Zaeune; kombiniert muessen beide
// wirksam bleiben (Einschraenkungen komponieren sich). Eine Tool-Sperre (runtime.read_field)
// bleibt aktiv, eine Praefix-Sperre (Secret*) bleibt ueber ein anderes Tool aktiv, und ein
// Aufruf, der in KEINEN Zaun faellt, funktioniert weiter. (Spec 07 verlangt die Kombination
// beider Layer; der Grader kombiniert nie Tool- und Praefix-Sperre.)
test('ZZBatReal ToolFenceAndPrefixFenceCombine', async () => {
  const { runtime } = zzbatSetup({ denyTools: ['runtime.read_field'], denyTypePrefix: ['Secret'] });
  await zzbatSecurityDenial(runtime, 'runtime.read_field', {}, 'toolfence');
  await zzbatSecurityDenial(runtime, 'runtime.find_instances', { typeName: 'SecretVault', limit: 1 }, 'prefixfence');
  const found = await zzbatCall(runtime, 'runtime.find_instances', { typeName: 'PricingPolicy', limit: 1 }, 'both-open');
  assert.ok(Array.isArray(found.instances));
});

// WHY: Der zentrale Brief-Satz — Layer 2 darf Layer 1 NIE lockern. Auch mit denyTools-Sperre und
// beiden Allow-Flags muss die feste Selbstschutz-Grenze des 'AiUx'-Namensraums eine Mutation am
// Chokepoint verweigern. Dieser Test ficht Layer 1 PER TYPNAME an (replace_method/write_static
// auf 'AiUxDispatch'), wo der Grader Layer 1 per Identitaet (write_field auf gepinntem Handle)
// prueft — die beiden Pfade ergaenzen sich. Auf Baseline UND jeder korrekten Abgabe gruen.
test('ZZBatReal Layer1SurvivesLayer2Config', async () => {
  const { runtime } = zzbatSetup({ denyTools: ['payload.list'], allowCodeReplacement: true, allowCodeLoading: true });
  const denial = await zzbatSecurityDenial(runtime, 'runtime.replace_method',
    { typeName: 'AiUxDispatch', method: 'dispatch' }, 'layer1-replace');
  assert.match(denial.message, /layer 1|self-protection|protected namespace/i);
  const denial2 = await zzbatSecurityDenial(runtime, 'runtime.write_static',
    { typeName: 'AiUxDispatch', field: 'anything', value: 1 }, 'layer1-writestatic');
  assert.match(denial2.message, /layer 1|self-protection|protected namespace/i);
});

// WHY: Spec 07 verlangt das "zur Laufzeit gewaehlte Tool". Die beworbene Oberflaeche wird zur
// Laufzeit gelesen, und das ERSTE, ein MITTLERES und das LETZTE Tool (ausser den vom Grader
// abgedeckten) werden je einzeln gesperrt: Was der Adapter selbst verspricht, muss sperrbar
// sein. Je gewaehltes Tool: Denial am Chokepoint, aus capabilities verschwunden, alle anderen
// beworbenen Tools bleiben, describe() meldet exakt [tool].
test('ZZBatReal RuntimeChosenTools', async () => {
  const { runtime: open } = zzbatSetup();
  const advertised = (await zzbatCall(open, 'runtime.capabilities', {}, 'rt-adv')).tools;
  const candidates = advertised.filter((tool) => !GRADER_DENIED.has(tool));
  assert.ok(candidates.length >= 10, 'der Adapter bewirbt genuegend sperrbare Tools jenseits des Graders');
  const picks = [candidates[0], candidates[Math.floor(candidates.length / 2)], candidates[candidates.length - 1]];
  assert.equal(new Set(picks).size, 3, 'drei verschiedene zur Laufzeit gewaehlte Tools');
  for (const tool of picks) {
    const { runtime } = zzbatSetup({ denyTools: [tool] });
    await zzbatSecurityDenial(runtime, tool, {}, `rt-${tool}`);
    const caps = await zzbatCall(runtime, 'runtime.capabilities', {}, `rt-caps-${tool}`);
    assert.equal(caps.tools.includes(tool), false, `${tool} ist gesperrt und darf nicht beworben sein`);
    for (const other of advertised) {
      if (other === tool) continue;
      assert.equal(caps.tools.includes(other), true, `${other} muss beworben bleiben, waehrend ${tool} gesperrt ist`);
    }
    assert.deepEqual([...caps.security.integratorDeniedTools], [tool]);
  }
});

// WHY: Ein Alias-Paar ist EIN Tool mit zwei Namen; beide Schreibweisen gleichzeitig zu sperren
// muss eine einzelne, konsistente Sperre bleiben (kein Doppel-Processing, kein Clash). ui.inspect
// + android.inspect_views in EINER Liste: beide Schreibweisen werden verweigert, der canonicale
// Name verschwindet aus capabilities, Geschwister (ui.mutate, ui.snapshot, payload.list) bleiben,
// und describe() meldet die Sperre unter dem canonicalen Namen ohne Eintraege ausserhalb der zwei
// Schreibweisen. (Dedup/Kanonisierung ist Implementierungsdetail — die Batterie erzwingt sie nicht.)
test('ZZBatReal BothSpellingsOfOneAliasPair', async () => {
  const pair = ['ui.inspect', 'android.inspect_views'];
  const { runtime } = zzbatSetup({ denyTools: pair });
  await zzbatSecurityDenial(runtime, 'ui.inspect', {}, 'pair-canonical', pair);
  await zzbatSecurityDenial(runtime, 'android.inspect_views', {}, 'pair-alias', pair);
  const caps = await zzbatCall(runtime, 'runtime.capabilities', {}, 'pair-caps');
  assert.equal(caps.tools.includes('ui.inspect'), false, 'ein gesperrtes Alias-Paar muss den canonicalen Namen verbergen');
  for (const sibling of ['ui.mutate', 'ui.snapshot', 'payload.list']) {
    assert.equal(caps.tools.includes(sibling), true, `${sibling} muss beworben bleiben`);
  }
  const reported = caps.security.integratorDeniedTools;
  assert.ok(Array.isArray(reported) && reported.includes('ui.inspect'),
    'describe() muss die Sperre unter dem canonicalen Namen melden');
  for (const entry of reported) {
    assert.ok(entry === 'ui.inspect' || entry === 'android.inspect_views',
      `unerwarteter describe()-Eintrag: ${entry}`);
  }
});

// WHY: ui.snapshot wird von diesem Adapter an sich gar nicht bedient — der Aufruf kaeme als
// gewoehnlicher Argument-Fehler zurueck. Eine Sperre muss daraus am Chokepoint eine
// Security-Denial machen (die Sperre laeuft VOR der Tool-Logik, auch fuer Tools, die sonst
// harmlos scheitern) und darf NICHT in andere Tools ueberschwappen: payload.list antwortet noch,
// Reads finden weiterhin Types. (Der Grader sperrt ui.snapshot nirgends — es fehlt in FAMILIES.)
test('ZZBatReal UnsupportedToolFenceable', async () => {
  const { runtime } = zzbatSetup({ denyTools: ['ui.snapshot'] });
  await zzbatSecurityDenial(runtime, 'ui.snapshot', {}, 'snapshot-fenced');
  const listed = await zzbatCall(runtime, 'payload.list', {}, 'snapshot-payload');
  assert.ok(Array.isArray(listed.payloads));
  const types = await zzbatCall(runtime, 'runtime.search_types', { query: 'Pricing', limit: 50 }, 'snapshot-search');
  assert.ok(types.includes('PricingPolicy'));
});

// ---------------------------------------------------------------- Stufe 2: Path
// Bestanden heisst hier nur: kein Crash/Panic/Hang, strukturierte Envelope, Terminierung.
// KEINE fachliche Deutung — auch ein laxe Umgang mit kaputten Eintraegen ist erlaubt.

// WHY: denyTools fehlt ganz, ist [] oder null. Das sind die Faelle, mit denen ein Integrator
// "keine Sperre" ausdrueckt. Der Adapter muss sich normal bauen, capabilities strukturiert
// beantworten (tools bleibt ein Array) und payload.list beantworten — egal wie er die leere
// Sperre intern darstellt.
test('ZZBatPath EmptyAndUndefinedLists', async () => {
  const cases = [
    ['undef', {}],
    ['empty', { denyTools: [] }],
    ['null', { denyTools: null }],
  ];
  for (const [label, cfg] of cases) {
    const { runtime } = zzbatSetup(cfg);
    const caps = await zzbatWithTimeout(zzbatDispatch(runtime, 'runtime.capabilities', {}), 5000, `path-${label}-caps`);
    assert.equal(typeof caps.ok, 'boolean', `${label}: capabilities muss strukturiert antworten`);
    if (caps.ok) assert.ok(Array.isArray(caps.result.tools), `${label}: tools muss ein Array bleiben`);
    const listed = await zzbatWithTimeout(zzbatDispatch(runtime, 'payload.list', {}), 5000, `path-${label}-list`);
    assert.equal(typeof listed.ok, 'boolean', `${label}: payload.list muss strukturiert antworten`);
  }
});

// WHY: Doppelte Eintraege sind kaputte, aber harmlose Eingaben. Die Sperre muss sie strukturiert
// verkraften — weder haengen noch crashen, capabilities weiter beantworten. Wie doppelte Namen
// behandelt werden (Dedup oder nicht), ist frei.
test('ZZBatPath DuplicatesInList', async () => {
  const { runtime } = zzbatSetup({ denyTools: ['payload.reapply', 'payload.reapply', 'ui.mutate', 'payload.reapply'] });
  const denied = await zzbatWithTimeout(zzbatDispatch(runtime, 'payload.reapply', {}), 5000, 'dup-deny');
  assert.equal(typeof denied.ok, 'boolean', 'Duplikate: Antwort muss eine strukturierte Envelope sein');
  if (!denied.ok) assert.equal(typeof denied.error?.type, 'string', 'Duplikate: Fehler muss typisiert sein');
  const caps = await zzbatWithTimeout(zzbatDispatch(runtime, 'runtime.capabilities', {}), 5000, 'dup-caps');
  assert.equal(typeof caps.ok, 'boolean');
  if (caps.ok) assert.ok(Array.isArray(caps.result.tools));
});

// WHY: Nicht-Strings und Leer/Blank-Eintraege (42, null, true, Objekte, Arrays, '', '   ',
// 'payload.list ' mit Leerzeichen) sind kaputte Eintraege. Der Adapter muss sie strukturiert
// verkraften statt zu crashen; wie er sie filtert (oder nicht), ist frei. Der Test erzwingt nur
// Terminierung und strukturierte Envelopes.
test('ZZBatPath NonStringAndBlankEntries', async () => {
  const garbage = [42, null, undefined, true, ['ui.inspect'], { tool: 'payload.list' }, '', '   ', 'payload.list '];
  const { runtime } = zzbatSetup({ denyTools: garbage });
  const caps = await zzbatWithTimeout(zzbatDispatch(runtime, 'runtime.capabilities', {}), 5000, 'garbage-caps');
  assert.equal(typeof caps.ok, 'boolean', 'Muell-Eintraege: capabilities muss strukturiert antworten');
  if (caps.ok) assert.ok(Array.isArray(caps.result.tools));
  const listed = await zzbatWithTimeout(zzbatDispatch(runtime, 'payload.list', {}), 5000, 'garbage-list');
  assert.equal(typeof listed.ok, 'boolean', 'Muell-Eintraege: payload.list muss strukturiert antworten');
  if (!listed.ok) assert.equal(typeof listed.error?.type, 'string', 'Muell-Eintraege: Fehler muss typisiert sein');
});

// WHY: Eine sehr lange Sperrliste (1000 Eintraege) ist der Stress auf Normalisierung und
// Lookup: Sie darf weder den Stack sprengen (rekursive Normalisierung) noch haengen. Bestanden
// heisst nur: capabilities und payload.list terminieren und antworten strukturiert.
test('ZZBatPath VeryLongList', async () => {
  const denyTools = [];
  for (let i = 0; i < 1000; i++) denyTools.push(`tool.fence_${i}`);
  denyTools.push('payload.list');
  const { runtime } = zzbatSetup({ denyTools });
  const caps = await zzbatWithTimeout(zzbatDispatch(runtime, 'runtime.capabilities', {}), 5000, 'long-caps');
  assert.equal(typeof caps.ok, 'boolean', 'lange Liste: capabilities muss strukturiert antworten');
  if (caps.ok) assert.ok(Array.isArray(caps.result.tools));
  const listed = await zzbatWithTimeout(zzbatDispatch(runtime, 'payload.list', {}), 5000, 'long-list');
  assert.equal(typeof listed.ok, 'boolean', 'eine 1000-Eintraege-Liste darf weder haengen noch crashen');
});
