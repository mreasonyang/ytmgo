// ytmGO website: language choice, platform detection, live release links,
// copy buttons, the sung headline, the lyrics demo and the page player.
// The page works without it.
(() => {
  "use strict";

  const strings = JSON.parse(document.getElementById("i18n").textContent || "{}");
  const fmt = (key, args = {}) => (strings[key] || "").replace(/\{(\w+)\}/g, (_, k) => args[k] ?? "");
  const store = {
    get: (k) => { try { return localStorage.getItem(k); } catch { return null; } },
    set: (k, v) => { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
  };

  // Language. The root restores a saved choice, otherwise it follows the
  // browser's language. Explicit localized URLs keep their requested language.
  const LANG_KEY = "ytmgo-site-lang";
  document.querySelectorAll("a[data-lang]").forEach((a) => {
    a.addEventListener("click", () => store.set(LANG_KEY, a.dataset.lang || "en"));
  });
  const here = document.documentElement.lang;
  if (here === "en" && !/bot|crawl|spider/i.test(navigator.userAgent)) {
    const saved = store.get(LANG_KEY);
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
  document.addEventListener("click", (e) => {
    document.querySelectorAll("details.lang[open]").forEach((d) => { if (!d.contains(e.target)) d.open = false; });
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") document.querySelectorAll("details.lang[open]").forEach((d) => { d.open = false; d.querySelector("summary").focus(); });
  });

  document.documentElement.classList?.add("js");
  const still = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

  const nav = document.querySelector(".nav");
  const onScroll = () => nav.classList.toggle("is-scrolled", scrollY > 8);
  addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  // Platform. Apple silicon can't be told from Intel in a browser, so macOS
  // offers both and lists Apple silicon first.
  const ua = navigator.userAgent;
  const platform = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "";
  const os = /iPhone|iPad|Android/i.test(ua) ? null
    : /Mac/i.test(platform) || /Mac OS X/.test(ua) ? "mac"
    : /Win/i.test(platform) || /Windows/.test(ua) ? "windows"
    : /Linux|X11/i.test(platform + ua) ? "linux" : null;
  const PICKS = { mac: ["mac-arm64", "mac-amd64"], windows: ["win-setup", "win-zip"], linux: ["linux-deb", "linux-tar"] };
  const card = os && document.querySelector(`.platform[data-os="${os}"]`);
  if (card) {
    card.classList.add("is-mine");
    card.parentNode.prepend(card);
    const others = document.querySelector("[data-others]");
    if (others) others.open = false;
    document.querySelectorAll("[data-cta-label]").forEach((label) => { label.textContent = fmt(`cta.${os}`); });
  }
  // The download track's buttons mirror this computer's platform card. Only
  // macOS keeps the card's labels, since its two buttons differ by chip.
  const mirror = () => {
    if (!card) return;
    PICKS[os].forEach((key, i) => {
      const from = card.querySelector(`[data-asset="${key}"]`);
      const to = document.querySelector(`[data-pick="${i ? "secondary" : "primary"}"]`);
      if (!from || !to) return;
      to.href = from.href;
      to.title = from.title;
      to.hidden = false;
      to.querySelector("[data-pick-label]").textContent = i || os === "mac" ? from.textContent : fmt(`cta.${os}`);
    });
    const req = document.querySelector('[data-pick="req"]');
    const version = card.querySelector(".platform__version");
    if (req) req.textContent = [card.querySelector(".platform__req").textContent, version && version.textContent].filter(Boolean).join(" · ");
  };
  mirror();

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
    mirror();
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

  // Motion: the player's play button pauses every animation on the page, and
  // reduced motion starts paused.
  let moving = !still;
  const onMotion = [];
  const setMoving = (on) => {
    moving = on;
    document.documentElement.classList?.toggle("is-paused", !on);
    onMotion.forEach((f) => f());
  };

  // The headline is sung: words light up one after another, like a lyric.
  if (!still) {
    const segmenter = typeof Intl.Segmenter === "function" ? new Intl.Segmenter(here, { granularity: "word" }) : null;
    document.querySelectorAll("[data-sing]").forEach((line) => {
      let n = 0;
      line.querySelectorAll("span").forEach((span) => {
        const text = span.textContent;
        const parts = segmenter ? [...segmenter.segment(text)].map((s) => ({ text: s.segment, word: s.isWordLike }))
          : text.split(/(\s+)/).map((t) => ({ text: t, word: /\S/.test(t) }));
        span.textContent = "";
        let last = null;
        for (const part of parts) {
          if (!part.text) continue;
          if (/^\s+$/.test(part.text)) { span.append(part.text); last = null; continue; }
          if (!part.word && last) { last.textContent += part.text; continue; }
          const w = document.createElement("span");
          w.className = "w";
          w.textContent = part.text;
          w.style.setProperty("--at", `${0.3 + n++ * 0.28}s`);
          span.append(w);
          last = w;
        }
      });
      requestAnimationFrame(() => line.classList.add("is-sung"));
    });
  }

  // Synced lyrics demo: one line at a time, centred, as in the app. Click a
  // line to jump to it.
  const lyrics = document.querySelector(".lyrics");
  if (lyrics) {
    const list = lyrics.querySelector(".lyrics__lines");
    const lines = [...list.children];
    const bar = lyrics.querySelector(".lyrics__bar span");
    let at = still ? 6 : 0;
    let timer = null;
    let visible = false;
    const show = () => {
      lines.forEach((li, j) => {
        li.classList.toggle("is-on", j === at);
        li.classList.toggle("is-past", j < at);
      });
      const li = lines[at];
      list.style.transform = `translateY(${-(li.offsetTop + li.offsetHeight / 2)}px)`;
      bar.style.width = `${((at + 1) / lines.length) * 100}%`;
    };
    const run = () => {
      clearInterval(timer);
      timer = visible && moving ? setInterval(() => { at = (at + 1) % lines.length; show(); }, 2400) : null;
    };
    lines.forEach((li, j) => li.addEventListener("click", () => { at = j; show(); run(); }));
    show();
    onMotion.push(run);
    new IntersectionObserver(([entry]) => { visible = entry.isIntersecting; run(); }).observe(lyrics);
    addEventListener("resize", show);
  }

  // Lines sung as you scroll past them.
  const sung = [...document.querySelectorAll("[data-lines] li")];
  const singLines = () => {
    const y = innerHeight * 0.62;
    let k = still ? sung.length - 1 : -1;
    if (!still) sung.forEach((li, i) => { if (li.getBoundingClientRect().top < y) k = i; });
    sung.forEach((li, i) => {
      li.classList.toggle("is-on", i === k || (still && i <= k));
      li.classList.toggle("is-past", !still && i < k);
    });
  };

  // The page player. Each track's length is in data-time; scrolling through a
  // track plays through its length, and the seek bar scrolls back.
  const player = document.querySelector("[data-player]");
  const tracks = [...document.querySelectorAll("section.track[data-time]")];
  let follow = () => {};
  if (player && tracks.length) {
    const lengths = tracks.map((t) => t.dataset.time.split(":").reduce((m, s) => m * 60 + Number(s), 0));
    const starts = lengths.map((_, i) => lengths.slice(0, i).reduce((a, b) => a + b, 0));
    const total = lengths.reduce((a, b) => a + b, 0);
    const clock = (s) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, "0")}`;
    const links = [...document.querySelectorAll("[data-track-link]")];
    const seek = player.querySelector("[data-seek]");
    const elapsed = player.querySelector("[data-elapsed]");
    const toggle = player.querySelector("[data-toggle]");
    const tops = () => tracks.map((t) => t.getBoundingClientRect().top + scrollY);
    const ends = (t, i) => (i + 1 < t.length ? t[i + 1] : document.documentElement.scrollHeight);
    const mark = () => innerHeight * 0.4;
    let current = -1;
    let seeking = false;
    player.querySelector("[data-total]").textContent = clock(total);
    player.hidden = false;

    follow = () => {
      const t = tops();
      const y = scrollY + mark();
      let k = 0;
      t.forEach((top, i) => { if (top <= y) k = i; });
      const f = Math.min(1, Math.max(0, (y - t[k]) / (ends(t, k) - t[k])));
      const atEnd = scrollY + innerHeight >= document.documentElement.scrollHeight - 2;
      const now = atEnd ? total : starts[k] + f * lengths[k];
      elapsed.textContent = clock(now);
      if (!seeking) seek.value = String(Math.round((now / total) * 1000));
      seek.style.setProperty("--p", `${(now / total) * 100}%`);
      if (k === current) return;
      current = k;
      tracks.forEach((track, i) => track.classList.toggle("is-playing", i === k));
      links.forEach((a, i) => a.classList.toggle("is-playing", i === k));
      player.querySelector("[data-now-title]").textContent = tracks[k].querySelector(".track__t").textContent;
      player.querySelector("[data-now-n]").textContent = tracks[k].querySelector(".track__n").textContent;
      player.style.setProperty("--tint", getComputedStyle(tracks[k]).getPropertyValue("--tint"));
    };
    const go = (k) => {
      const behavior = still ? "auto" : "smooth";
      if (k <= 0) scrollTo({ top: 0, behavior });
      else tracks[Math.min(k, tracks.length - 1)].scrollIntoView({ behavior });
    };
    player.querySelector("[data-prev]").addEventListener("click", () => {
      const t = tops();
      // Like a player: back to the start of this track, or the previous one
      // if you're already there.
      go(scrollY > t[current] + 80 && current > 0 ? current : current - 1);
    });
    player.querySelector("[data-next]").addEventListener("click", () => go(current + 1));
    seek.addEventListener("input", () => {
      seeking = true;
      const want = (Number(seek.value) / 1000) * total;
      let k = 0;
      starts.forEach((s, i) => { if (s <= want) k = i; });
      const t = tops();
      const f = (want - starts[k]) / lengths[k];
      scrollTo({ top: t[k] + f * (ends(t, k) - t[k]) - mark(), behavior: "auto" });
    });
    seek.addEventListener("change", () => { seeking = false; });
    const label = () => toggle.setAttribute("aria-label", fmt(moving ? "pause" : "play"));
    toggle.addEventListener("click", () => { setMoving(!moving); label(); });
    setMoving(moving);
    label();
  }

  let queued = false;
  const frame = () => {
    queued = false;
    follow();
    singLines();
  };
  const schedule = () => { if (!queued) { queued = true; requestAnimationFrame(frame); } };
  if (player || sung.length) {
    addEventListener("scroll", schedule, { passive: true });
    addEventListener("resize", schedule);
    frame();
  }
})();
