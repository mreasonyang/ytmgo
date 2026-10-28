// Offline fixture only. The test appends one fixed run(mode) call in a temp dir.
import { serialize } from 'node:v8';
import { readFile, writeFile } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { Worker } from 'node:worker_threads';
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

function frame(value, trailing = false) {
  let payload = serialize(value);
  if (trailing) payload = Buffer.concat([payload, Buffer.from([1])]);
  const header = Buffer.alloc(4);
  header.writeUInt32BE(payload.length);
  return Buffer.concat([header, payload]);
}

export async function run(mode) {
  process.stdin.resume();
  process.stdin.on('end', () => process.exit(0));
  const bytes = new ArrayBuffer(4);
  new DataView(bytes).setUint32(0, process.pid);
  const value = { ok: true, entries: [['fixture', bytes]], playerID: 'fixture_player', fetchCount: 3 };
  if (mode === 'hold') { setInterval(() => {}, 1000); return; }
  if (mode === 'empty') { process.exit(0); return; }
  if (mode === 'permissions') {
    let readDenied = false, writeDenied = false, spawnDenied = false, workerDenied = false;
    await readFile(new URL(import.meta.url));
    try { await readFile('/etc/hosts'); } catch (error) { readDenied = error.code === 'ERR_ACCESS_DENIED'; }
    try { await writeFile(new URL('./forbidden-write', import.meta.url), 'fixture'); } catch (error) { writeDenied = error.code === 'ERR_ACCESS_DENIED'; }
    try { const child = spawn(process.execPath, ['--version']); child.kill(); } catch (error) { spawnDenied = error.code === 'ERR_ACCESS_DENIED'; }
    try { const worker = new Worker('', { eval: true }); await worker.terminate(); } catch (error) { workerDenied = error.code === 'ERR_ACCESS_DENIED'; }
    if (!readDenied || !writeDenied || !spawnDenied || !workerDenied || process.permission.has('addons')) {
      process.stdout.write(frame({ ok: false, category: 'init_failed' }), () => process.exit(0)); return;
    }
  }
  if (mode === 'stderr') process.stderr.write('offline fixture diagnostic\n');
  if (mode === 'unknown_error') { value.ok = false; delete value.entries; delete value.playerID; delete value.fetchCount; value.category = 'untrusted fixture diagnostic'; }
  if (mode === 'reported_error') { value.ok = false; delete value.entries; delete value.playerID; delete value.fetchCount; value.category = 'cache_limit'; }
  if (mode === 'bad_schema') value.extra = 'untrusted fixture diagnostic';
  if (mode === 'bad_entries') value.entries = Array.from({ length: 5 }, (_, i) => [String(i), bytes]);
  if (mode === 'duplicate_entries') value.entries = [['fixture', bytes], ['fixture', bytes]];
  if (mode === 'large_cache') value.entries = [['fixture', new ArrayBuffer(8 * 1024 * 1024 + 1)]];
  if (mode === 'bad_player') value.playerID = 'invalid player id';
  if (mode === 'bad_count') value.fetchCount = -1;
  if (mode === 'oversize_header') {
    const header = Buffer.alloc(4); header.writeUInt32BE(16 * 1024 * 1024);
    process.stdout.write(header); setInterval(() => {}, 1000); return;
  }
  if (mode === 'bad_serialization') {
    const bad = Buffer.from([0, 0, 0, 3, 0, 0, 0]);
    process.stdout.write(bad, () => process.exit(0)); return;
  }
  let output = frame(value, mode === 'trailing');
  if (mode === 'truncated') output = output.subarray(0, output.length - 2);
  if (mode === 'valid_then_failure') output = Buffer.concat([output, frame({ ok: false, category: 'init_failed' })]);
  if (mode === 'fragmented') {
    for (let i = 0; i < output.length; i += 3) { process.stdout.write(output.subarray(i, i + 3)); await delay(1); }
    process.exit(0); return;
  }
  if (mode === 'late_exit') {
    process.stdout.write(output);
    await delay(250);
    process.exit(0); return;
  }
  if (mode === 'reply_hold') { process.stdout.write(output); setInterval(() => {}, 1000); return; }
  process.stdout.write(output, () => process.exit(mode === 'nonzero_after_reply' ? 2 : 0));
}
