import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { once } from 'node:events';

test('worker validates control-only IPC without SDK/auth/network initialization and exits on EOF', { timeout: 5000 }, async () => {
  const worker = fileURLToPath(new URL('../worker.mjs', import.meta.url));
  const child = spawn(process.execPath, [worker], { env: { PATH: '/usr/bin:/bin' }, stdio: ['pipe', 'pipe', 'pipe'] });
  let stderrBytes = 0, pending = '';
  const responses = [];
  child.stderr.on('data', chunk => { stderrBytes += chunk.length; });
  let complete;
  const received = new Promise(resolve => { complete = resolve; });
  child.stdout.on('data', chunk => {
    pending += chunk.toString('utf8');
    let index;
    while ((index = pending.indexOf('\n')) >= 0) {
      const line = pending.slice(0, index); pending = pending.slice(index + 1);
      responses.push(JSON.parse(line));
      if (responses.length === 3) complete();
    }
  });
  child.stdin.write(JSON.stringify({ id: 1, op: '__proto__' }) + '\n');
  child.stdin.write(JSON.stringify({ id: 2, op: 'cancel', target: 999 }) + '\n');
  child.stdin.write(JSON.stringify({ id: 3, op: 'release', url: 'invalid' }) + '\n');
  try {
    await received;
    assert.equal(responses.find(x => x.id === 1)?.category, 'invalid_request');
    assert.equal(responses.find(x => x.id === 2)?.ok, true);
    assert.equal(responses.find(x => x.id === 3)?.category, 'invalid_capability');
    const exited = once(child, 'exit');
    child.stdin.end();
    const [code] = await exited;
    assert.equal(code, 0);
    assert.equal(stderrBytes, 0);
  } finally { child.kill(); }
});

test('private configure accepts bounded large sessions and emits no cookie or unknown input', { timeout: 5000 }, async () => {
  const worker = fileURLToPath(new URL('../worker.mjs', import.meta.url));
  const child = spawn(process.execPath, [worker], { env: { PATH: '/usr/bin:/bin' }, stdio: ['pipe', 'pipe', 'pipe'] });
  let stderrBytes = 0, pending = '', output = '';
  const replies = [];
  child.stderr.on('data', chunk => { stderrBytes += chunk.length; });
  let complete;
  const received = new Promise(resolve => { complete = resolve; });
  child.stdout.on('data', chunk => {
    output += chunk.toString(); pending += chunk.toString();
    let index;
    while ((index = pending.indexOf('\n')) >= 0) {
      replies.push(JSON.parse(pending.slice(0, index))); pending = pending.slice(index + 1);
      if (replies.length === 3) complete();
    }
  });
  const sentinel = 'synthetic-private-only';
  const value = sentinel + 'x'.repeat(12000);
  const session = { loggedIn: true, musicPremium: true, cookie: `SAPISID=${value}; SID=${value}; HSID=${value}; SSID=${value}`, accountIndex: 2, onBehalfOfUser: '', revision: 2 };
  child.stdin.write(JSON.stringify({ id: 1, op: 'configure', session }) + '\n');
  child.stdin.write(JSON.stringify({ id: 2, op: 'configure', session: { ...session, revision: 1 } }) + '\n');
  child.stdin.write(JSON.stringify({ id: 3, op: 'configure', session: { ...session, cookie: `UNKNOWN=${sentinel}` } }) + '\n');
  try {
    await received;
    assert.equal(replies.find(reply => reply.id === 1)?.ok, true);
    assert.equal(replies.find(reply => reply.id === 2)?.category, 'invalid_request');
    assert.equal(replies.find(reply => reply.id === 3)?.category, 'invalid_request');
    assert.equal(output.includes(sentinel), false); assert.equal(output.includes('cookie'), false); assert.equal(stderrBytes, 0);
    const exited = once(child, 'exit'); child.stdin.end(); assert.equal((await exited)[0], 0);
  } finally { child.kill(); }
});
