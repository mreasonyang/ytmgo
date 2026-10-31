import { BotGuardClient } from 'bgutils-js/botguard';
import { WebPoMinter } from 'bgutils-js/webpo';
import { getHeaders } from 'bgutils-js/utils';
// No media or network is provided by this page. Go fetches the bounded,
// allowlisted interpreter and exchanges the integrity response over private IPC.
for (const name of ['log','error','warn','info','debug','trace']) console[name]=()=>{};
// A proof realm is never a player, including during validation. Retain offline
// audio analysis if required by the interpreter, but make audible APIs absent.
for (const name of ['AudioContext','webkitAudioContext','Audio','speechSynthesis','RTCPeerConnection','webkitRTCPeerConnection']) {
  Object.defineProperty(globalThis,name,{value:undefined,writable:false,configurable:false});
}
Object.defineProperty(HTMLMediaElement.prototype,'play',{value:()=>Promise.reject(new Error('media_disabled')),writable:false,configurable:false});
let client, signals, minter;
globalThis.ytmgoProof = Object.freeze({
  headers() { return getHeaders(); },
  async snapshot(input) {
    if (client) throw new Error('already_initialized');
    globalThis.yt = { config_: input.config };
    if (input.globalName in globalThis) throw new Error('proof_global_conflict');
    try { (0,eval)(`(function(){\n${input.script}\n}).call(globalThis);`); }
    catch { throw new Error('proof_interpreter_failed'); }
    if (!globalThis[input.globalName]?.a) throw new Error('proof_interpreter_missing');
    try { client = await BotGuardClient.create({program: input.program, globalName: input.globalName, globalObject: globalThis}); }
    catch { throw new Error('proof_program_failed'); }
    signals=[];
    let snapshot;
    try { snapshot = await client.snapshot({ webPoSignalOutput: signals }); }
    catch { throw new Error('proof_snapshot_failed'); }
    if (typeof snapshot!=='string' || !snapshot || snapshot.length>128*1024) throw new Error('proof_snapshot_invalid');
    if (!signals.length) throw new Error('proof_signals_missing');
    return snapshot;
  },
  async integrity(input) {
    if (!client || !signals || minter) throw new Error('invalid_state');
    minter=await WebPoMinter.create(input,signals);
    return true;
  },
  async mint(binding) {
    if (!minter || typeof binding!=='string' || !binding || binding.length>4096) throw new Error('invalid_state');
    const token=await minter.mintAsWebsafeString(binding);
    if (!token || token.length>24*1024) throw new Error('mint_failed');
    return token;
  },
});
