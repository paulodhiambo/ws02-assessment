const test = require('node:test');
const assert = require('node:assert');
const { createServer } = require('./app');

async function withServer(fn) {
  const server = createServer({ slowDelayMs: 50, log: () => {} }).listen(0);
  await new Promise((r) => server.once('listening', r));
  const base = `http://127.0.0.1:${server.address().port}`;
  try { await fn(base); } finally { server.close(); }
}

test('returns a known user', () => withServer(async (base) => {
  const res = await fetch(`${base}/users/1`);
  assert.strictEqual(res.status, 200);
  const body = await res.json();
  assert.strictEqual(body.id, 1);
  assert.ok(body.email);
}));

test('returns 404 and empty object for unknown user', () => withServer(async (base) => {
  const res = await fetch(`${base}/users/42`);
  assert.strictEqual(res.status, 404);
  assert.deepStrictEqual(await res.json(), {});
}));

test('simulates a backend error', () => withServer(async (base) => {
  const res = await fetch(`${base}/users/500`);
  assert.strictEqual(res.status, 500);
}));
