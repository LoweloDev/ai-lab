/**
 * Grading suite v2 for the `denyTools` Layer-2 integrator option (audit F13 / package P6).
 *
 * Complements grade.test.js, which stays in force unchanged. That file drives the option with exactly
 * two tools (payload.list, runtime.search_types + its alias), so an implementation that enforces the
 * fence for just those names passes it (audit-scratch/cheat). This file closes the gap along three
 * axes: one representative from every tool family (runtime.*, ui.*, payload.*, code.*), the alias
 * pairs of every family in both directions, and — read at run time from what runtime.capabilities()
 * advertises rather than from a list written down here — every single tool the adapter itself says
 * it has. Everything goes through the one JSON chokepoint, the way the conformance suite does.
 */
import test from 'node:test';
import assert from 'node:assert/strict';

import { createRuntime } from '../src/index.js';
import { createHost } from './host-fixture.js';

let sequence = 0;

function setup(config = {}) {
  const host = createHost();
  const runtime = createRuntime({ graphBudget: 4_000, roots: [host], ...config });
  return { host, runtime };
}

async function call(runtime, tool, args = {}) {
  return runtime.dispatch({ version: 1, requestId: `grade-v2-${++sequence}`, tool, arguments: args });
}

async function succeed(runtime, tool, args = {}) {
  const response = await call(runtime, tool, args);
  assert.equal(response.ok, true, `${tool} failed: ${JSON.stringify(response.error)}`);
  return response.result;
}

/** A denied call must come back as a structured RuntimeSecurityError that names the blocked tool. */
async function expectDenied(runtime, tool, args = {}, namedAs = [tool]) {
  const response = await call(runtime, tool, args);
  assert.equal(response.ok, false, `${tool} must be refused while it is denied`);
  assert.equal(response.error.type, 'RuntimeSecurityError',
    `${tool}: expected a security denial, got ${response.error.type}: ${response.error.message}`);
  assert.ok(namedAs.some((name) => response.error.message.includes(name)),
    `${tool}: the refusal must name the blocked tool (${namedAs.join(' or ')}), got: ${response.error.message}`);
  assert.ok(Array.isArray(response.error.stack), 'a denial is a structured failure envelope, not a throw');
  return response.error;
}

/** Without a denial the same call may succeed or fail on its arguments — but never as a security denial. */
async function expectNotSecurityDenied(runtime, tool, args = {}) {
  const response = await call(runtime, tool, args);
  if (!response.ok) {
    assert.notEqual(response.error.type, 'RuntimeSecurityError',
      `${tool} without denyTools must not be a security denial: ${response.error.message}`);
  }
  return response;
}

/**
 * One tool per family. The arguments are chosen so that the undenied call resolves to a
 * non-security outcome (success or an argument error), which proves the denial happens at the
 * chokepoint before any tool logic runs — ui.* has no DOM under node and would otherwise answer
 * with a RuntimeArgumentError, never with a security error.
 */
const FAMILIES = Object.freeze([
  { family: 'runtime', tool: 'runtime.describe_type', args: { typeName: 'PricingPolicy' } },
  { family: 'runtime', tool: 'runtime.find_instances', args: { typeName: 'PricingPolicy', limit: 1 } },
  { family: 'ui', tool: 'ui.inspect', args: {} },
  { family: 'ui', tool: 'ui.mutate', args: { handle: 'no-such-handle' } },
  { family: 'payload', tool: 'payload.suspend_persistent', args: { suspended: false } },
  { family: 'payload', tool: 'payload.delete', args: { id: 'no-such-payload' } },
  { family: 'code', tool: 'code.disassemble', args: { typeName: 'PricingPolicy', method: 'priceFor' } },
]);

/** Canonical name, Android alias, arguments — one pair per family that has an alias. */
const ALIAS_PAIRS = Object.freeze([
  { canonical: 'runtime.describe_type', alias: 'runtime.describe_class', args: { typeName: 'PricingPolicy' } },
  { canonical: 'runtime.replace_method', alias: 'runtime.redefine_class_file', args: {} },
  { canonical: 'runtime.load_code', alias: 'runtime.execute_dex_file', args: {} },
  { canonical: 'ui.inspect', alias: 'android.inspect_views', args: {} },
  { canonical: 'ui.mutate', alias: 'android.mutate_view', args: { handle: 'no-such-handle' } },
]);

test('denyTools v2: one tool from every family is refused at dispatch with a RuntimeSecurityError naming it', async () => {
  for (const { family, tool, args } of FAMILIES) {
    const { runtime: open } = setup();
    await expectNotSecurityDenied(open, tool, args);

    const { runtime: fenced } = setup({ denyTools: [tool] });
    await expectDenied(fenced, tool, args);
    assert.equal(family, tool.split('.')[0], 'table sanity: the row belongs to the family it claims');
  }

  // All families fenced at once; a tool that is not on the list keeps working.
  const { runtime } = setup({ denyTools: FAMILIES.map((row) => row.tool) });
  for (const { tool, args } of FAMILIES) await expectDenied(runtime, tool, args);
  const listed = await succeed(runtime, 'payload.list');
  assert.ok(Array.isArray(listed.payloads), 'payload.list is not denied and must still answer');
});

test('denyTools v2: every denied family tool disappears from capabilities.tools and the rest stay advertised', async () => {
  const denied = FAMILIES.map((row) => row.tool);
  const { runtime } = setup({ denyTools: denied });
  const caps = await succeed(runtime, 'runtime.capabilities');
  assert.ok(Array.isArray(caps.tools), 'capabilities.tools must be an array');
  for (const tool of denied) {
    assert.equal(caps.tools.includes(tool), false, `${tool} is denied and must not be advertised`);
  }
  for (const tool of ['runtime.capabilities', 'runtime.attach', 'runtime.search_types', 'payload.list', 'ui.snapshot']) {
    assert.equal(caps.tools.includes(tool), true, `${tool} is not denied and must stay advertised`);
  }
  assert.deepEqual([...caps.security.integratorDeniedTools].sort(), [...denied].sort(),
    'describe() must report exactly the configured list');

  // Denial is per exact tool name: fencing one member of a family leaves its siblings alone.
  const { runtime: single } = setup({ denyTools: ['code.disassemble'] });
  const tools = (await succeed(single, 'runtime.capabilities')).tools;
  assert.equal(tools.includes('code.disassemble'), false);
  for (const tool of ['runtime.describe_type', 'ui.inspect', 'payload.delete', 'ui.mutate']) {
    assert.equal(tools.includes(tool), true, `${tool} must stay advertised when only code.disassemble is denied`);
  }
});

test('denyTools v2: alias pairs of every family are fenced in both directions', async () => {
  for (const { canonical, alias, args } of ALIAS_PAIRS) {
    const names = [canonical, alias];

    const { runtime: open } = setup();
    await expectNotSecurityDenied(open, canonical, args);
    await expectNotSecurityDenied(open, alias, args);

    const { runtime: byCanonical } = setup({ denyTools: [canonical] });
    await expectDenied(byCanonical, canonical, args, names);
    await expectDenied(byCanonical, alias, args, names);

    const { runtime: byAlias } = setup({ denyTools: [alias] });
    await expectDenied(byAlias, alias, args, names);
    await expectDenied(byAlias, canonical, args, names);
  }
});

test('denyTools v2: denying either spelling hides the canonical tool from capabilities.tools', async () => {
  for (const { canonical, alias } of ALIAS_PAIRS) {
    const { runtime: byCanonical } = setup({ denyTools: [canonical] });
    const viaCanonical = (await succeed(byCanonical, 'runtime.capabilities')).tools;
    assert.equal(viaCanonical.includes(canonical), false,
      `${canonical} is denied and must not be advertised`);

    const { runtime: byAlias } = setup({ denyTools: [alias] });
    const viaAlias = (await succeed(byAlias, 'runtime.capabilities')).tools;
    assert.equal(viaAlias.includes(canonical), false,
      `${alias} denies the same tool as ${canonical}, so ${canonical} must not be advertised`);
    assert.equal(viaAlias.includes('runtime.search_types'), true, 'unrelated tools stay advertised');
  }
});

test('denyTools v2: every tool the adapter advertises at run time can be denied on its own', async () => {
  // Read the list from the running adapter, not from a table: whatever it claims to offer must be
  // fenceable, including tools no grading table names.
  const { runtime: open } = setup();
  const advertised = (await succeed(open, 'runtime.capabilities')).tools;
  assert.ok(Array.isArray(advertised) && advertised.length >= 20,
    `expected the full tool surface, got ${JSON.stringify(advertised)}`);
  assert.ok(advertised.every((tool) => typeof tool === 'string' && tool.includes('.')));

  for (const tool of advertised) {
    const { runtime } = setup({ denyTools: [tool] });
    await expectDenied(runtime, tool, {});
    if (tool === 'runtime.capabilities') continue; // denied itself: nothing left to enumerate through
    const caps = await succeed(runtime, 'runtime.capabilities');
    assert.equal(caps.tools.includes(tool), false, `${tool} is denied and must not be advertised`);
    for (const other of advertised) {
      if (other === tool) continue;
      assert.equal(caps.tools.includes(other), true,
        `${other} is not denied and must stay advertised while ${tool} is`);
    }
    assert.deepEqual([...caps.security.integratorDeniedTools], [tool]);
  }
});

test('denyTools v2: layer 2 only adds restrictions — a denied list leaves layer 1 self-protection intact', async () => {
  const { runtime } = setup({ denyTools: ['payload.list', 'ui.inspect'] });
  const handle = runtime.pin(runtime);
  // Reading the guard stays allowed, rewriting it stays a layer-1 denial, with or without a fence.
  const described = await succeed(runtime, 'runtime.describe_object', { handle });
  assert.ok(described.fields.length > 0, 'inspecting the runtime is deliberately permitted');
  const denial = await expectDenied(runtime, 'runtime.write_field',
    { handle, field: 'dispatch', value: null }, ['runtime.write_field']);
  assert.match(denial.message, /layer 1 self-protection/);
  // And a fence on other tools never widens what the host may do.
  const types = await succeed(runtime, 'runtime.search_types', { query: 'Pricing', limit: 50 });
  assert.ok(types.includes('PricingPolicy'));
});
