import { registerHooks } from 'node:module';

// Installed before loading the engine's application module graph. A Premium
// process cannot accidentally retain a provider or DOM through a future import.
export function guardPremiumModules() {
  return registerHooks({ resolve(specifier, context, nextResolve) {
    if (/(?:^|[/\\])(?:jsdom|happy-dom|linkedom|bgutils-js|proxy-agent)(?:[/\\]|$)|(?:engine-proof|proof-policy|proof-binding|proof-session)\.mjs|vendor[/\\]bgutil/.test(specifier)) {
      throw Object.assign(new Error('unexpected_authentication'), { safeCategory: 'unexpected_authentication' });
    }
    const result = nextResolve(specifier, context);
    if (/(?:^|[/\\])(?:jsdom|happy-dom|linkedom|bgutils-js|proxy-agent)(?:[/\\]|$)|(?:engine-proof|proof-policy|proof-binding|proof-session)\.mjs|vendor[/\\]bgutil/.test(result.url)) {
      throw Object.assign(new Error('unexpected_authentication'), { safeCategory: 'unexpected_authentication' });
    }
    return result;
  } });
}
