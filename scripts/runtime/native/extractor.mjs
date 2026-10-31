// Only the MIT player's extraction algorithms and ISC parser are bundled.
// No SDK session, HTTP client, catalog parser, Node shim or filesystem cache.
import { JsAnalyzer } from 'youtubei.js/dist/src/utils/javascript/JsAnalyzer.js';
import { JsExtractor } from 'youtubei.js/dist/src/utils/javascript/JsExtractor.js';
import { nsigMatcher, timestampMatcher } from 'youtubei.js/dist/src/utils/javascript/matchers.js';
// Meriyah 7.4 clones only Identifier and string Literal nodes. QuickJS has no
// host structuredClone API; copy these plain AST records without a DOM shim.
globalThis.structuredClone = node => {
  const copy = value => value === null || typeof value !== 'object' ? value
    : Array.isArray(value) ? value.map(copy)
    : Object.fromEntries(Object.entries(value).map(([key,value])=>[key,copy(value)]));
  return copy(node);
};
globalThis.extractPlayer = source => {
  const analyzer = new JsAnalyzer(source, { extractions: [
    { friendlyName: 'nsigFunction', match: nsigMatcher },
    { friendlyName: 'signatureTimestampVar', match: timestampMatcher, collectDependencies: false },
  ] });
  const result = new JsExtractor(analyzer).buildScript({ disallowSideEffectInitializers: true,
    exportRawValues: true, rawValueOnly: ['signatureTimestampVar'] });
  const timestamp = Number.parseInt(result.exportedRawValues?.signatureTimestampVar);
  if (!Number.isSafeInteger(timestamp) || timestamp<=0) throw new Error('player_timestamp_missing');
  if (!result.exported.includes('nsigFunction')) throw new Error('player_function_missing');
  if (result.output.length>8*1024*1024) throw new Error('player_output_limit');
  return { code: result.output, timestamp };
};
