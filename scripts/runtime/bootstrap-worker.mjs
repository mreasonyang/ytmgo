// STDOUT IS PRIVATE BINARY IPC. Cache contents must never enter logs or files.
import { serialize } from 'node:v8';
import { createAnonymousSession, MemoryCache } from './sdk-session.mjs';
import { createBootstrapFetch } from './bootstrap-fetch.mjs';
import { bootstrapFailureDetails } from './bootstrap-diagnostics.mjs';

const write = process.stdout.write.bind(process.stdout);
for (const name of ['log', 'info', 'warn', 'error', 'debug', 'trace', 'dir']) console[name] = () => {};
process.stdout.write = process.stderr.write = () => true;
const IPC_LIMIT = 16 * 1024 * 1024;
const failure = category => Object.assign(new Error(category), { safeCategory: category });
const controller = new AbortController();
let stopping = false;
const transport = createBootstrapFetch(controller.signal);

function stop(code = 0) {
  if (stopping) return;
  stopping = true;
  controller.abort();
  clearTimeout(timer);
  process.exit(code);
}

function respond(reply) {
  if (stopping) return;
  const payload = serialize(reply);
  if (payload.byteLength + 4 > IPC_LIMIT) { stop(2); return; }
  const header = Buffer.alloc(4);
  header.writeUInt32BE(payload.byteLength);
  write(header);
  write(payload, () => stop());
}

const timer = setTimeout(() => stop(2), 30_000);
process.on('SIGTERM', () => stop());
process.on('SIGINT', () => stop());
process.on('uncaughtException', () => stop(2));
process.on('unhandledRejection', () => stop(2));
process.stdout.on('error', () => stop(2));
process.stdin.on('error', () => stop(2));
process.stdin.once('end', () => stop());
process.stdin.resume();

try {
  const cache = new MemoryCache();
  const yt = await createAnonymousSession({ cache, fetch: transport.fetch });
  if (controller.signal.aborted) throw failure('request_cancelled');
  const playerID = yt.session.player.player_id;
  if (typeof playerID !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(playerID) || !cache.size) throw failure('init_failed');
  respond({ ok: true, entries: cache.snapshot(), playerID, fetchCount: transport.count });
} catch (error) {
  const safe = transport.failure(error);
  respond({ ok: false, category: safe.safeCategory, ...bootstrapFailureDetails(safe) });
}
