// ytmGO website: language choice, platform detection, live release links,
// copy buttons and the synced-lyrics demo. The page works without it.
(() => {
  "use strict";

  const strings = JSON.parse(document.getElementById("i18n").textContent || "{}");
  const fmt = (key, args = {}) => (strings[key] || "").replace(/\{(\w+)\}/g, (_, k) => args[k] ?? "");
  const store = {
    get: (k) => { try { return localStorage.getItem(k); } catch { return null; } },
    set: (k, v) => { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
  };

  // Language. A visitor who lands on the English page for the first time is
  // sent to their browser's language; an explicit choice always wins.
  const LANG_KEY = "ytmgo-site-lang";
  document.querySelectorAll("a[data-lang]").forEach((a) => {
    a.addEventListener("click", () => store.set(LANG_KEY, a.dataset.lang || "en"));
  });
  const here = document.documentElement.lang;
  if (here === "en" && !store.get(LANG_KEY) && !/bot|crawl|spider/i.test(navigator.userAgent)) {
    const pick = (navigator.languages || [navigator.language]).map((l) => l.toLowerCase()).map((l) => {
      if (/^zh-(tw|hk|mo|hant)/.test(l)) return "zh-hant/";
      if (l.startsWith("zh")) return "zh-hans/";
      if (l.startsWith("ja")) return "ja/";
      if (l.startsWith("en")) return "en";
      return null;
    }).find(Boolean);
    if (pick && pick !== "en") {
      location.replace(pick + location.hash);
      return;
    }
  }
  document.addEventListener("click", (e) => {
    document.querySelectorAll("details.lang[open]").forEach((d) => { if (!d.contains(e.target)) d.open = false; });
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") document.querySelectorAll("details.lang[open]").forEach((d) => { d.open = false; d.querySelector("summary").focus(); });
  });

  const nav = document.querySelector(".nav");
  const onScroll = () => nav.classList.toggle("is-scrolled", scrollY > 8);
  addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  // Platform. Apple silicon can't be told from Intel in a browser, so the
  // macOS card offers both and lists Apple silicon first.
  const ua = navigator.userAgent;
  const platform = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "";
  const os = /iPhone|iPad|Android/i.test(ua) ? null
    : /Mac/i.test(platform) || /Mac OS X/.test(ua) ? "mac"
    : /Win/i.test(platform) || /Windows/.test(ua) ? "windows"
    : /Linux|X11/i.test(platform + ua) ? "linux" : null;
  if (os) {
    const card = document.querySelector(`.platform[data-os="${os}"]`);
    if (card) {
      card.classList.add("is-mine");
      card.parentNode.prepend(card);
    }
    document.querySelectorAll("[data-cta]").forEach((a) => {
      a.href = `#download-${os}`;
      const label = a.querySelector("[data-cta-label]");
      if (label && strings[`cta.${os}`]) label.textContent = fmt(`cta.${os}`);
    });
  }

  // Releases. Not every release carries every platform, so each download
  // links to the newest release that has its file.
  const ASSETS = {
    "mac-arm64": /^ytmgo-([\d.]+)-arm64-notarized\.dmg$/,
    "mac-amd64": /^ytmgo-([\d.]+)-amd64-notarized\.dmg$/,
    "win-setup": /^ytmgo-setup-([\d.]+)-amd64\.exe$/,
    "win-zip": /^ytmgo-([\d.]+)-windows-amd64\.zip$/,
    "linux-deb": /^ytmgo_([\d.]+)_amd64\.deb$/,
    "linux-tar": /^ytmgo-([\d.]+)-linux-amd64\.tar\.gz$/,
  };
  const CACHE_KEY = "ytmgo-site-releases";
  const loadReleases = async () => {
    try {
      const cached = JSON.parse(sessionStorage.getItem(CACHE_KEY) || "null");
      if (cached && Date.now() - cached.at < 10 * 60 * 1000) return cached.releases;
    } catch { /* ignore */ }
    const res = await fetch("https://api.github.com/repos/mreasonyang/ytmgo/releases?per_page=30", { headers: { Accept: "application/vnd.github+json" } });
    if (!res.ok) throw new Error(`GitHub ${res.status}`);
    const releases = (await res.json())
      .filter((r) => !r.draft && !r.prerelease)
      .map((r) => ({ date: r.published_at, assets: r.assets.map((a) => ({ name: a.name, url: a.browser_download_url, size: a.size })) }));
    try { sessionStorage.setItem(CACHE_KEY, JSON.stringify({ at: Date.now(), releases })); } catch { /* ignore */ }
    return releases;
  };
  const dateFmt = new Intl.DateTimeFormat(here, { year: "numeric", month: "short", day: "numeric" });
  const sizeFmt = (bytes) => `${Math.round(bytes / 1048576)} MB`;
  loadReleases().then((releases) => {
    for (const [key, pattern] of Object.entries(ASSETS)) {
      let found = null;
      for (const r of releases) {
        const asset = r.assets.find((a) => pattern.test(a.name));
        if (asset) { found = { ...asset, date: r.date, version: asset.name.match(pattern)[1] }; break; }
      }
      document.querySelectorAll(`[data-asset="${key}"]`).forEach((a) => {
        if (!found) return;
        a.href = found.url;
        a.title = `${found.name} · ${sizeFmt(found.size)}`;
      });
      document.querySelectorAll(`[data-version="${key}"]`).forEach((p) => {
        p.textContent = found ? fmt("version", { version: found.version, date: dateFmt.format(new Date(found.date)) }) : fmt("unavailable");
      });
      if (key === "linux-deb" && found) {
        document.querySelectorAll("[data-apt]").forEach((c) => { c.textContent = `sudo apt install ./${found.name}`; });
      }
    }
  }).catch(() => { /* The static links already point at the releases page. */ });

  // Copy buttons.
  document.querySelectorAll("[data-copy]").forEach((button) => {
    const original = button.textContent;
    button.addEventListener("click", async () => {
      const text = button.parentNode.querySelector("code").textContent;
      try {
        await navigator.clipboard.writeText(text);
        button.textContent = fmt("copied");
        setTimeout(() => { button.textContent = original; }, 1600);
      } catch { /* Clipboard unavailable: the text stays selectable. */ }
    });
  });

  // Synced lyrics demo: one line at a time, centred, as in the app.
  const demo = document.querySelector(".lyricdemo");
  if (demo) {
    const list = demo.querySelector(".lyricdemo__lines");
    const lines = [...list.children];
    const bar = demo.querySelector(".lyricdemo__bar span");
    const still = matchMedia("(prefers-reduced-motion: reduce)");
    let i = 0;
    const show = (n) => {
      lines.forEach((li, j) => {
        li.classList.toggle("is-on", j === n);
        li.classList.toggle("is-past", j < n);
      });
      const li = lines[n];
      list.style.transform = `translateY(${-(li.offsetTop + li.offsetHeight / 2)}px)`;
      bar.style.width = `${((n + 1) / lines.length) * 100}%`;
    };
    show(still.matches ? 6 : 0);
    let timer = null;
    const run = () => {
      if (timer || still.matches) return;
      timer = setInterval(() => { i = (i + 1) % lines.length; show(i); }, 2200);
    };
    const stop = () => { clearInterval(timer); timer = null; };
    new IntersectionObserver(([entry]) => (entry.isIntersecting ? run() : stop())).observe(demo);
    addEventListener("resize", () => show(i));
  }
})();
