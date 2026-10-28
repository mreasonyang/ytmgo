// PRIVATE BINARY IPC ONLY. Never attach these pipes to a terminal or log.
import { encodeFrame, FrameReader, REQUEST_LIMIT, REPLY_LIMIT, PROOF_PHASES, PROOF_REASONS, RUNTIME_STAGES } from './engine-ipc.mjs';
import { CATEGORIES } from './control.mjs';
import { validateAccountSession } from './account-session.mjs';
import { guardPremiumModules } from './premium-module-guard.mjs';

const write = process.stdout.write.bind(process.stdout);
for (const name of ['log', 'info', 'warn', 'error', 'debug', 'trace', 'dir']) console[name] = () => {};
process.stdout.write = process.stderr.write = () => true;
let active, engine, stopping = false, failing = false, lastID = 0, expiryTimer;
const input = new FrameReader(REQUEST_LIMIT);

function stop(code = 0) {
  if (stopping) return;
  stopping = true; clearTimeout(expiryTimer); active?.controller.abort(); input.clear(); engine?.close(); process.exit(code);
}
function fail(category = 'runtime_failed', detail = {}) {
  if (stopping || failing) return;
  failing = true; active?.controller.abort();
  if (!CATEGORIES.has(category)) category = 'runtime_failed';
  const reply = { id: active?.id ?? 0, ok: false, category };
  if (PROOF_PHASES.has(detail.safePhase)) reply.phase = detail.safePhase;
  if (PROOF_REASONS.has(detail.safeReason)) reply.reason = detail.safeReason;
  const stage = detail.safeStage ?? engine?.stage;
  if (RUNTIME_STAGES.has(stage)) reply.stage = stage;
  try { write(encodeFrame(reply, REPLY_LIMIT), () => stop(2)); }
  catch { stop(2); }
  setTimeout(() => stop(2), 100).unref();
}
process.on('uncaughtException', () => fail());
process.on('unhandledRejection', () => fail());
process.on('SIGTERM', () => stop()); process.on('SIGINT', () => stop());
process.stdout.on('error', () => stop(2)); process.stdin.on('error', () => stop(2));
process.stdin.on('end', () => stop(input.partial || active ? 2 : 0));

async function accept(request) {
  if (stopping || failing) return;
  if (active || !request || Array.isArray(request) || !Number.isSafeInteger(request.id) || request.id <= lastID) { fail(); return; }
  const fields = request.op === 'warm' ? ['id', 'op', 'entries', 'playerID', 'session'] : request.op === 'resolve' ? ['id', 'op', 'videoId', 'kind'] : null;
  if (!fields || Object.keys(request).some(key => !fields.includes(key)) ||
      (request.op === 'warm' && (!Array.isArray(request.entries) || !request.entries.length || request.entries.length > 4 || typeof request.playerID !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(request.playerID))) ||
      (request.op === 'resolve' && (!/^[A-Za-z0-9_-]{11}$/.test(request.videoId ?? '') || !['audio', 'video'].includes(request.kind)))) { fail(); return; }
  lastID = request.id; active = { id: request.id, controller: new AbortController() };
  const timer = setTimeout(() => fail('request_timeout'), 30_000);
  try {
    if (request.op === 'warm') {
      if (engine) throw Object.assign(new Error('init_failed'), { safeCategory: 'init_failed' });
      request.session = validateAccountSession(request.session);
      if (request.session.musicPremium) guardPremiumModules();
      const { PlaybackEngine } = await import('./engine.mjs');
      engine = new PlaybackEngine({ proofFactory: async () => {
        const { EngineProof } = await import('./engine-proof.mjs');
        return new EngineProof({ onFailure: (category, detail) => fail(category, detail) });
      } });
    } else if (!engine) throw Object.assign(new Error('init_failed'), { safeCategory: 'init_failed' });
    const result = request.op === 'warm'
      ? await engine.warm(request, active.controller.signal)
      : await engine.resolve(request.videoId, request.kind, active.controller.signal);
    if (stopping || failing || active.controller.signal.aborted) return;
    clearTimeout(expiryTimer);
    expiryTimer = setTimeout(() => stop(), result.expiresAt - Date.now() - 30_000); expiryTimer.unref();
    const frame = encodeFrame({ id: request.id, ok: true, ...result }, REPLY_LIMIT);
    await new Promise((resolve, reject) => write(frame, error => error ? reject(error) : resolve()));
    if (!stopping && !failing) active = undefined;
  } catch (error) { fail(error?.safeCategory, error); }
  finally { clearTimeout(timer); }
}
process.stdin.on('data', chunk => {
  if (stopping || failing) return;
  try { input.push(chunk, request => { void accept(request); return !failing; }); }
  catch { fail(); }
});
