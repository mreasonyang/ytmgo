// STDOUT IS PRIVATE JSONL IPC. Never attach it to a terminal, tee, or log file.
import { once } from 'node:events';
import { StringDecoder } from 'node:string_decoder';
import { RuntimeController } from './control.mjs';
import { Resolver } from './resolver.mjs';
import { SESSION_LINE_LIMIT } from './account-session.mjs';

const write = process.stdout.write.bind(process.stdout);
for (const name of ['log', 'info', 'warn', 'error', 'debug', 'trace', 'dir']) console[name] = () => {};
process.stdout.write = () => true;
process.stderr.write = () => true;
const shutdownSignal = new AbortController();
let shutdownPromise;
let outputQueue = [];
let writing = false;

function emit(response) {
  if (shutdownSignal.signal.aborted) return;
  const line = JSON.stringify(response) + '\n';
  if (line.length > 4096 || outputQueue.length >= 64) { void shutdown(2); return; }
  outputQueue.push(line);
  void flush();
}

async function flush() {
  if (writing || shutdownSignal.signal.aborted) return;
  writing = true;
  try {
    while (outputQueue.length && !shutdownSignal.signal.aborted) {
      if (!write(outputQueue.shift())) await once(process.stdout, 'drain', { signal: shutdownSignal.signal });
    }
  } catch { void shutdown(2); }
  finally { writing = false; }
}

const controller = new RuntimeController(new Resolver(), emit);
function shutdown(code = 0) {
  if (shutdownPromise) return shutdownPromise;
  shutdownSignal.abort();
  outputQueue = [];
  process.stdin.destroy();
  shutdownPromise = controller.close().catch(() => {}).finally(() => process.exit(code));
  return shutdownPromise;
}
process.on('uncaughtException', () => { void shutdown(2); });
process.on('unhandledRejection', () => { void shutdown(2); });
process.on('SIGTERM', () => { void shutdown(); });
process.on('SIGINT', () => { void shutdown(); });
process.stdout.on('error', () => { void shutdown(2); });

async function readInput() {
  const decoder = new StringDecoder('utf8');
  let pending = '';
  for await (const chunk of process.stdin) {
    pending += decoder.write(chunk);
    let index;
    while ((index = pending.indexOf('\n')) >= 0) {
      const line = pending.slice(0, index);
      pending = pending.slice(index + 1);
      if (Buffer.byteLength(line) > SESSION_LINE_LIMIT) { emit({ id: null, ok: false, category: 'invalid_request' }); continue; }
      let request;
      try { request = JSON.parse(line); } catch { request = null; }
      controller.accept(request);
    }
    if (Buffer.byteLength(pending) > SESSION_LINE_LIMIT) { await shutdown(2); return; }
  }
}
try { await readInput(); } catch { /* Parent closed the private input pipe. */ }
await shutdown();
