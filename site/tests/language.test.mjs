import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const boot = readFileSync(new URL("../assets/boot.js", import.meta.url), "utf8");
const site = readFileSync(new URL("../assets/site.js", import.meta.url), "utf8");
const key = "ytmgo-site-lang";

// Run the shipped scripts as a page does: boot.js in <head>, then site.js
// unless boot.js navigated away, with just the browser interfaces they use.
// No network requests or real browser preferences are changed by these checks.
function visit({ saved = null, languages = ["en-US"], here = "en", search = "", hash = "", blocked = false, userAgent = "Browser" } = {}) {
  const storage = new Map(saved === null ? [] : [[key, saved]]);
  const clicks = new Map();
  const links = ["", "zh-hans/", "zh-hant/", "ja/"].map((slug) => ({
    dataset: { lang: slug },
    addEventListener: (event, handler) => { if (event === "click") clicks.set(slug, handler); },
  }));
  let redirect = null;
  const context = vm.createContext({
    document: {
      documentElement: { lang: here, classList: { add() {}, toggle() {} } },
      getElementById: () => ({ textContent: "{}" }),
      querySelectorAll: (selector) => selector === "a[data-lang]" ? links : [],
      querySelector: (selector) => selector === ".nav" ? { classList: { toggle() {} } } : null,
      addEventListener() {},
    },
    localStorage: {
      getItem: (name) => { if (blocked) throw new Error("Storage unavailable"); return storage.get(name) ?? null; },
      setItem: (name, value) => { if (blocked) throw new Error("Storage unavailable"); storage.set(name, value); },
    },
    sessionStorage: { getItem: () => null, setItem() {} },
    navigator: { languages, language: languages[0], userAgent, platform: "" },
    location: { search, hash, replace: (url) => { redirect = url; } },
    fetch: async () => ({ ok: true, json: async () => [] }),
    addEventListener() {},
    scrollY: 0,
  });
  vm.runInContext(boot, context, { timeout: 1000 });
  if (redirect === null) vm.runInContext(site, context, { timeout: 1000 });
  return { redirect, choose: (slug) => clicks.get(slug)(), saved: () => storage.get(key) };
}

test("a manual language choice is restored on the next root visit", () => {
  for (const slug of ["", "zh-hans/", "zh-hant/", "ja/"]) {
    const page = visit({ here: "zh-Hans" });
    page.choose(slug);
    const next = visit({ saved: page.saved(), languages: ["zh-CN"] });
    assert.equal(next.redirect, slug || null, slug || "English");
  }
});

test("an English preference wins over a Chinese browser preference", () => {
  assert.equal(visit({ saved: "en", languages: ["zh-CN", "en-US"] }).redirect, null);
});

test("first visits follow the first supported browser language", () => {
  for (const [languages, expected] of [
    [["en-US", "zh-CN"], null],
    [["zh-CN", "en-US"], "zh-hans/"],
    [["zh-TW", "en-US"], "zh-hant/"],
    [["zh-Hant-HK"], "zh-hant/"],
    [["ja-JP"], "ja/"],
    [["fr-FR", "ja-JP"], "ja/"],
    [["fr-FR"], null],
  ]) assert.equal(visit({ languages }).redirect, expected, languages.join(", "));
});

test("explicit localized URLs remain in their requested language", () => {
  for (const here of ["zh-Hans", "zh-Hant", "ja"]) {
    assert.equal(visit({ here, saved: "en" }).redirect, null, here);
  }
});

test("unknown saved preferences fall back to browser language", () => {
  for (const saved of ["invalid", "https://example.com/", "//example.com/"]) {
    assert.equal(visit({ saved, languages: ["ja-JP"] }).redirect, "ja/", saved);
  }
});

test("language redirects preserve the query and fragment", () => {
  const suffix = "?ref=kyun.si#download";
  for (const saved of [null, "zh-hans/"]) {
    const page = visit({ saved, languages: ["zh-CN"], search: "?ref=kyun.si", hash: "#download" });
    assert.equal(page.redirect, "zh-hans/" + suffix);
  }
});

test("blocked storage still permits browser language detection and switching", () => {
  assert.equal(visit({ blocked: true, languages: ["ja-JP"] }).redirect, "ja/");
  assert.doesNotThrow(() => visit({ blocked: true, here: "ja" }).choose(""));
});

test("crawlers stay on the English root", () => {
  assert.equal(visit({ userAgent: "ExampleBot", saved: "ja/", languages: ["ja-JP"] }).redirect, null);
});
