import { parseScript as parse } from 'meriyah/dist/meriyah.mjs';
// JsExtractor accepts start/end offsets. Avoid allocating a redundant two-item
// range array on every AST node while retaining identical source slices.
export function parseScript(source, options) {
  return parse(source, {...options, ranges:{start:true,end:true}});
}
