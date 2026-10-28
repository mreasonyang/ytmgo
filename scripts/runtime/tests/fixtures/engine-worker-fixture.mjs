import { serialize, deserialize } from 'node:v8';
import { readFile, writeFile } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { Worker } from 'node:worker_threads';

let mode = 'normal', generation = 0, count = 0, buffer = Buffer.alloc(0);
const frame = value => { const body = serialize(value); const bytes = Buffer.alloc(body.length + 4); bytes.writeUInt32BE(body.length); body.copy(bytes, 4); return bytes; };
const safe = () => ({ init_ms: 0, proof_init_ms: 0, resolve_ms: 0, decipher_ms: 0, proof_ms: 0, fetch_count: 0, player_requests: 0 });
const reply = request => ({ id: request.id, ok: true, expiresAt: Date.now() + 120_000, safe: safe() });

async function accept(request) {
  if (request.op === 'warm') {
    mode = request.playerID; generation++;
    if (mode === 'hold') return;
    if (mode === 'exit') process.exit(2);
    if (mode === 'malformed') { process.stdout.write(Buffer.from([0,0,0,1,0])); return; }
    if (mode === 'oversize') { const bytes = Buffer.alloc(4); bytes.writeUInt32BE(128 * 1024); process.stdout.write(bytes); return; }
    if (mode === 'permissions') {
      await readFile(new URL(import.meta.url));
      const denied = async work => { try { await work(); return false; } catch (error) { return error.code === 'ERR_ACCESS_DENIED'; } };
      const passed = await denied(() => readFile('/etc/hosts')) &&
        await denied(() => writeFile('/tmp/ytmgo-engine-forbidden-fixture', 'fixture')) &&
        await denied(() => spawn(process.execPath, ['--version'])) &&
        await denied(() => new Worker('0', { eval: true })) && !process.permission.has('addons');
      if (!passed) { process.stdout.write(frame({ id: request.id, ok: false, category: 'runtime_failed' })); return; }
    }
    const value = reply(request);
    if (mode === 'wrong_id') value.id++;
    if (mode === 'extra') value.other = 'synthetic';
    if (mode === 'unknown_error') { value.ok = false; value.category = 'untrusted fixture error'; }
    if (mode === 'unsafe_stage') {
      process.stdout.write(frame({ id: request.id, ok: false, category: 'init_failed', stage: 'synthetic-private-stage' })); return;
    }
    if (mode === 'expired') value.expiresAt = Date.now() + 10;
    if (mode === 'far_future') value.expiresAt = Date.now() + 25 * 60 * 60_000;
    if (mode === 'missing_safe') delete value.safe;
    if (mode === 'unsafe_metric') value.safe.other = 'synthetic';
    if (mode === 'stderr') process.stderr.write('private synthetic diagnostic');
    let bytes = frame(value);
    if (mode === 'double') bytes = Buffer.concat([bytes, bytes]);
    if (mode === 'trailing') { const body = Buffer.concat([serialize(value), Buffer.from([0])]); bytes = Buffer.alloc(body.length + 4); bytes.writeUInt32BE(body.length); body.copy(bytes, 4); }
    if (mode === 'fragmented') { for (let i = 0; i < bytes.length; i += 7) { process.stdout.write(bytes.subarray(i, i + 7)); await new Promise(resolve => setImmediate(resolve)); } }
    else process.stdout.write(bytes);
  } else {
    if (request.videoId === 'holdresolve') return;
    if (mode === 'late') { setTimeout(() => process.stdout.write(frame(reply(request))), 200); return; }
    if (mode === 'resolve_error') {
      process.stdout.write(frame({ id: request.id, ok: false, category: 'proof_request_rejected', phase: 'mint', reason: 'http', stage: 'proof_mint' }));
      setTimeout(() => process.stdout.write(frame(reply(request))), 50); return;
    }
    const value = reply(request);
    value.media = { url: `https://fixture.googlevideo.com/media?fixture=${process.pid}-${generation}-${++count}`, duration: 120, length: 65536, contentType: request.kind === 'video' ? 'video/mp4' : 'audio/webm' };
    if (request.kind === 'video') Object.assign(value.media, { width: 640, height: 360 });
    Object.assign(value.safe, { kind: request.kind, codec: request.kind === 'video' ? 'avc1' : 'opus', bitrate: 128000, format_count: 1 });
    if (mode === 'foreign_media') value.media.url = 'https://example.com/synthetic';
    if (mode === 'bad_media') value.media.length = -1;
    if (mode === 'sentinel') value.media.url += '&n=enhanced_except_synthetic';
    process.stdout.write(frame(value));
  }
}
process.stdin.on('data', chunk => {
  buffer = Buffer.concat([buffer, chunk]);
  while (buffer.length >= 4 && buffer.length >= buffer.readUInt32BE(0) + 4) {
    const length = buffer.readUInt32BE(0); const request = deserialize(buffer.subarray(4, length + 4)); buffer = buffer.subarray(length + 4);
    void accept(request);
  }
});
process.stdin.on('end', () => process.exit(0));
