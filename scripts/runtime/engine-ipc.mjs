import { serialize, DefaultDeserializer } from 'node:v8';

export const REQUEST_LIMIT = 16 * 1024 * 1024;
export const REPLY_LIMIT = 128 * 1024;
export const PROOF_PHASES = new Set(['init', 'context', 'network', 'mint', 'deadline']);
export const PROOF_REASONS = new Set(['policy', 'permission', 'http', 'other', 'origin', 'challenge', 'route', 'authentication', 'youtube_integrity', 'session_bootstrap', 'interpreter_route',
  'config_missing', 'config_invalid', 'visitor_missing', 'login_unverified', 'account_mismatch', 'binding_missing', 'brand_mismatch', 'experiment_invalid']);
export const RUNTIME_STAGES = new Set(['bootstrap', 'sdk_create', 'account_validation', 'proof_config_request', 'proof_config_parse', 'proof_init', 'player_request', 'decipher', 'proof_mint']);
const invalid = () => Object.assign(new Error('runtime_failed'), { safeCategory: 'runtime_failed' });

export function encodeFrame(value, limit) {
  let body;
  try { body = serialize(value); } catch { throw invalid(); }
  if (body.length + 4 > limit) throw invalid();
  const frame = Buffer.allocUnsafe(body.length + 4);
  frame.writeUInt32BE(body.length); body.copy(frame, 4);
  return frame;
}

export function decodeBody(body) {
  try {
    const reader = new DefaultDeserializer(body);
    if (!reader.readHeader()) throw invalid();
    const value = reader.readValue();
    let trailing = false;
    try { reader.readRawBytes(1); trailing = true; } catch {}
    if (trailing) throw invalid();
    return value;
  } catch { throw invalid(); }
}

// A frame is allocated only after its bounded prefix has arrived. No raw
// payload, deserialization error or SDK diagnostic is returned to a caller.
export class FrameReader {
  #limit; #header = Buffer.alloc(4); #headerBytes = 0; #body; #bodyBytes = 0;
  constructor(limit) { this.#limit = limit; }
  get partial() { return this.#headerBytes !== 0 || this.#body !== undefined; }
  clear() { this.#headerBytes = this.#bodyBytes = 0; this.#body = undefined; }
  push(chunk, accept) {
    while (chunk.length) {
      if (!this.#body) {
        const count = Math.min(4 - this.#headerBytes, chunk.length);
        chunk.copy(this.#header, this.#headerBytes, 0, count);
        this.#headerBytes += count; chunk = chunk.subarray(count);
        if (this.#headerBytes !== 4) return;
        const size = this.#header.readUInt32BE(0);
        if (size < 1 || size + 4 > this.#limit) throw invalid();
        this.#body = Buffer.allocUnsafe(size); this.#bodyBytes = 0;
      }
      const count = Math.min(this.#body.length - this.#bodyBytes, chunk.length);
      chunk.copy(this.#body, this.#bodyBytes, 0, count);
      this.#bodyBytes += count; chunk = chunk.subarray(count);
      if (this.#bodyBytes === this.#body.length) {
        const value = decodeBody(this.#body); this.clear();
        if (accept(value) === false) return;
      }
    }
  }
}
