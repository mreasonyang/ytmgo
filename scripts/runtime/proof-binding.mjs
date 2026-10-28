// The current WEB_REMIX GVS contract is selected by the official page config:
// video-bound experiments use videoId; other requests use dataSyncId for an
// account and visitorData when signed out. These values stay in the child.
// See yt-dlp/extractor/youtube/pot/utils.py and _video.py (2026-10-28).
const fail = (safeReason = 'config_invalid') => Object.assign(new Error('proof_request_rejected'), { safeCategory: 'proof_request_rejected', safePhase: 'context', safeReason });
const validVisitor = value => typeof value === 'string' && value.length > 0 && value.length <= 4096 && /^[A-Za-z0-9_%=+\/-]+$/.test(value);

export function parseProofBinding(html, account) {
  if (typeof html !== 'string' || html.length > 16 * 1024 * 1024) throw fail();
  let config;
  const match = /ytcfg\.set\s*\(\s*({.+?})\s*\)\s*;/s.exec(html);
  if (!match) throw fail('config_missing');
  try {
    config = JSON.parse(match[1]);
  } catch { throw fail(); }
  if (!config || typeof config !== 'object' || Array.isArray(config)) throw fail();
  const visitorData = config.VISITOR_DATA ?? config.INNERTUBE_CONTEXT?.client?.visitorData;
  if (!validVisitor(visitorData)) throw fail('visitor_missing');
  let sessionBinding;
  if (account.loggedIn) {
    const index = config.SESSION_INDEX;
    if (config.LOGGED_IN !== true) throw fail('login_unverified');
    if (![account.accountIndex, String(account.accountIndex)].includes(index)) throw fail('account_mismatch');
    if (typeof config.DATASYNC_ID !== 'string' || !/^[A-Za-z0-9_-]{1,256}\|\|[A-Za-z0-9_-]{0,256}$/.test(config.DATASYNC_ID)) throw fail('binding_missing');
    const [delegated, primary] = config.DATASYNC_ID.split('||');
    if ((account.onBehalfOfUser && (!primary || delegated !== account.onBehalfOfUser)) || (!account.onBehalfOfUser && primary)) throw fail('brand_mismatch');
    sessionBinding = config.DATASYNC_ID;
  } else {
    if (config.LOGGED_IN === true) throw fail('account_mismatch');
    sessionBinding = visitorData;
  }
  const players = config.WEB_PLAYER_CONTEXT_CONFIGS ?? {};
  if (typeof players !== 'object' || Array.isArray(players)) throw fail('experiment_invalid');
  let videoBound = false;
  for (const player of Object.values(players)) {
    if (typeof player?.serializedExperimentFlags !== 'string') continue;
    const flags = new URLSearchParams(player.serializedExperimentFlags).getAll('html5_generate_content_po_token');
    if (flags.some(value => !['true', 'false'].includes(value))) throw fail('experiment_invalid');
    if (flags.at(-1) === 'true') videoBound = true;
  }
  return { visitorData, sessionBinding, videoBound };
}
