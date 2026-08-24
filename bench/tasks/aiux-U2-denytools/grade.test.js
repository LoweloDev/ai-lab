/**
 * Grading suite for the `denyTools` Layer-2 integrator option. Drives the runtime the way the
 * conformance suite does — one JSON tool call at a time through the single chokepoint.
 */
import test from 'node:test';
import assert from 'node:assert/strict';

import { createRuntime } from '../src/index.js';
import { RuntimeProtection } from '../src/protection.js';
import { createHost } from './host-fixture.js';

let sequence = 0;

function setup(config = {}) {
  const host = createHost();
  const runtime = createRuntime({ graphBudget: 4_000, roots: [host], ...config });
  return { host, runtime };
}

async function call(runtime, tool, args = {}) {
  return runtime.dispatch({ version: 1, requestId: `grade-${++sequence}`, tool, arguments: args });
}

async function succeed(runtime, tool, args = {}) {
  const response = await call(runtime, tool, args);
  assert.equal(response.ok, true, `${tool} failed: ${JSON.stringify(response.error)}`);
  return response.result;
}

async function deny(runtime, tool, args = {}) {
  const response = await call(runtime, tool, args);
  assert.equal(response.ok, false, `${tool} unexpectedly succeeded`);
  return response.error;
}

test('denyTools: a denied tool is refused at dispatch as a structured RuntimeSecurityError naming it', async () => {
  const { runtime } = setup({ denyTools: ['payload.list'] });
  const error = await deny(runtime, 'payload.list');
  assert.equal(error.type, 'RuntimeSecurityError',
    `expected a security denial, got ${error.type}: ${error.message}`);
  assert.match(error.message, /payload\.list/, 'the refusal must name the blocked tool');
  assert.ok(Array.isArray(error.stack), 'a denial is a structured failure envelope, not a throw');
});

test('denyTools: a denied tool is absent from the capabilities tools array', async () => {
  const { runtime } = setup({ denyTools: ['payload.list'] });
  const caps = await succeed(runtime, 'runtime.capabilities');
  assert.equal(caps.tools.includes('payload.list'), false,
    'a denied tool must not be advertised by runtime.capabilities');
  assert.equal(caps.tools.includes('runtime.search_types'), true,
    'tools that are not denied stay advertised');
  assert.equal(caps.tools.includes('payload.delete'), true,
    'denial is per exact tool name, not per prefix');
});

test('denyTools: denying the canonical name also blocks calls made under its alias', async () => {
  const { runtime } = setup({ denyTools: ['runtime.search_types'] });
  const direct = await deny(runtime, 'runtime.search_types', { query: 'Pricing' });
  assert.equal(direct.type, 'RuntimeSecurityError');
  const viaAlias = await deny(runtime, 'runtime.search_classes', { query: 'Pricing' });
  assert.equal(viaAlias.type, 'RuntimeSecurityError',
    'runtime.search_classes is the same tool as runtime.search_types and must be blocked too');
  assert.match(viaAlias.message, /runtime\.search_(types|classes)/);
});

test('denyTools: denying the alias spelling blocks the canonical tool as well', async () => {
  const { runtime } = setup({ denyTools: ['runtime.search_classes'] });
  const viaAlias = await deny(runtime, 'runtime.search_classes', { query: 'Pricing' });
  assert.equal(viaAlias.type, 'RuntimeSecurityError');
  const direct = await deny(runtime, 'runtime.search_types', { query: 'Pricing' });
  assert.equal(direct.type, 'RuntimeSecurityError',
    'denying runtime.search_classes must also block runtime.search_types');
  assert.match(direct.message, /runtime\.search_(types|classes)/);
});

test('denyTools: without the option, nothing changes', async () => {
  const { runtime } = setup();
  const types = await succeed(runtime, 'runtime.search_types', { query: 'Pricing', limit: 50 });
  assert.ok(types.includes('PricingPolicy'));
  const viaAlias = await succeed(runtime, 'runtime.search_classes', { query: 'Pricing', limit: 50 });
  assert.ok(viaAlias.includes('PricingPolicy'), 'the Android alias keeps working');
  const listed = await succeed(runtime, 'payload.list');
  assert.ok(Array.isArray(listed.payloads));
  const caps = await succeed(runtime, 'runtime.capabilities');
  for (const tool of ['payload.list', 'runtime.search_types', 'ui.inspect', 'runtime.capabilities']) {
    assert.ok(caps.tools.includes(tool), `${tool} must stay advertised by default`);
  }
  assert.deepEqual(caps.security.integratorDeniedTools, [],
    'describe() reports an empty denied-tools list by default');
});

test('denyTools: describe() reports the configured list under integratorDeniedTools', async () => {
  const denied = ['ui.inspect', 'runtime.search_types'];
  const direct = new RuntimeProtection({ denyTools: [...denied] }).describe();
  assert.ok(Array.isArray(direct.integratorDeniedTools),
    'RuntimeProtection.describe() must carry integratorDeniedTools');
  assert.deepEqual([...direct.integratorDeniedTools].sort(), [...denied].sort());

  const { runtime } = setup({ denyTools: [...denied] });
  const caps = await succeed(runtime, 'runtime.capabilities');
  assert.deepEqual([...caps.security.integratorDeniedTools].sort(), [...denied].sort(),
    'the list must surface through capabilities.security as well');
});
