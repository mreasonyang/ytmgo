// ytmGO website: runs in <head>, before the page paints. A visit to the root
// goes straight to the visitor's language, so they never see the English page
// flash first; then the page is marked as scripted for site.js's styles.
(() => {
  "use strict";
  const root = document.documentElement;

  // The root restores a choice saved by the language switcher, otherwise it
  // follows the browser's language. Explicit localized URLs keep their
  // requested language, and crawlers stay on English.
  if (root.lang === "en" && !/bot|crawl|spider/i.test(navigator.userAgent)) {
    let saved = null;
    try { saved = localStorage.getItem("ytmgo-site-lang"); } catch { /* private mode */ }
    const pick = ["en", "zh-hans/", "zh-hant/", "ja/"].includes(saved) ? saved
      : (navigator.languages || [navigator.language]).map((l) => l.toLowerCase()).map((l) => {
        if (/^zh-(tw|hk|mo|hant)/.test(l)) return "zh-hant/";
        if (l.startsWith("zh")) return "zh-hans/";
        if (l.startsWith("ja")) return "ja/";
        if (l.startsWith("en")) return "en";
        return null;
      }).find(Boolean);
    if (pick && pick !== "en") {
      location.replace(pick + location.search + location.hash);
      return;
    }
  }
  root.classList.add("js");
})();
