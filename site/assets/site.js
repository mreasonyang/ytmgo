// ytmGO website: the language switcher, platform detection, live release links,
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

  // Language. The switcher saves the choice; boot.js restores it on the next
  // visit to the root.
  document.querySelectorAll("a[data-lang]").forEach((a) => {
    a.addEventListener("click", () => store.set("ytmgo-site-lang", a.dataset.lang || "en"));
  });
  const here = document.documentElement.lang;
  document.addEventListener("click", (e) => {
    document.querySelectorAll("details.lang[open]").forEach((d) => { if (!d.contains(e.target)) d.open = false; });
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") document.querySelectorAll("details.lang[open]").forEach((d) => { d.open = false; d.querySelector("summary").focus(); });
  });

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
  }
  // Phones and tablets can't run ytmGO: say so where the requirements go.
  if (!os) {
    const req = document.querySelector('[data-pick="req"]');
    if (req) req.textContent = fmt("desktop");
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
  // Both labels share one cell, so the button keeps the longer one's width
  // and "Copy" crosses into "Copied" in place rather than swapping.
  document.querySelectorAll("[data-copy]").forEach((button) => {
    const idle = document.createElement("span");
    const done = document.createElement("span");
    idle.textContent = button.textContent;
    done.textContent = fmt("copied");
    done.setAttribute("aria-hidden", "true");
    button.replaceChildren(idle, done);
    button.addEventListener("click", async () => {
      const text = button.parentNode.querySelector("code").textContent;
      try {
        await navigator.clipboard.writeText(text);
        button.classList.add("is-done");
        button.setAttribute("aria-label", done.textContent);
        setTimeout(() => { button.classList.remove("is-done"); button.removeAttribute("aria-label"); }, 1600);
      } catch { /* Clipboard unavailable: the text stays selectable. */ }
    });
  });

  // Motion: the player's play button pauses every animation on the page, and
  // reduced motion starts paused.
  let moving = !still;
  const onMotion = [];
  const setMoving = (on) => {
    moving = on;
    if (!on) document.querySelectorAll(".rv:not(.is-in)").forEach((el) => el.classList.add("is-in"));
    document.documentElement.classList?.toggle("is-paused", !on);
    onMotion.forEach((f) => f());
  };

  // The headline is sung: words light up one after another, like a lyric.
  if (!still) {
    const segmenter = typeof Intl.Segmenter === "function" ? new Intl.Segmenter(here, { granularity: "word" }) : null;
    document.querySelectorAll("[data-sing]").forEach((line) => {
      let n = 0;
      // Only text is split into words; the <wbr> phrase breaks stay put.
      [...line.childNodes].filter((node) => node.nodeType === 3).forEach((node) => {
        const text = node.textContent;
        const parts = segmenter ? [...segmenter.segment(text)].map((s) => ({ text: s.segment, word: s.isWordLike }))
          : text.split(/(\s+)/).map((t) => ({ text: t, word: /\S/.test(t) }));
        const words = document.createDocumentFragment();
        let last = null;
        for (const part of parts) {
          if (!part.text) continue;
          if (/^\s+$/.test(part.text)) { words.append(part.text); last = null; continue; }
          if (!part.word && last) { last.textContent += part.text; continue; }
          const w = document.createElement("span");
          w.className = "w";
          w.textContent = part.text;
          w.style.setProperty("--at", `${0.3 + n++ * 0.28}s`);
          words.append(w);
          last = w;
        }
        node.replaceWith(words);
      });
      requestAnimationFrame(() => line.classList.add("is-sung"));
    });
  }

  // Synced lyrics demo, as in the app: the lyrics start at the top, and the
  // sung line rests a third of the way down once it gets there. Click a line
  // to jump to it.
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
      const view = list.parentElement;
      const y = Math.max(0, li.offsetTop + li.offsetHeight / 2 - view.clientHeight / 3);
      list.style.transform = `translateY(${-y}px)`;
      view.classList.toggle("is-scrolled", y > 0.5);
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

  // The page player. Each track's length is in data-time; scrolling through a
  // track plays through its length, and the seek bar scrolls back. A track
  // starts playing once its top passes 40% of the window, so the clock runs
  // from 0:00 at the top of the page to the full length at the bottom.
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
    // Scroll positions where each track starts and ends.
    const spans = () => {
      const max = Math.max(1, document.documentElement.scrollHeight - innerHeight);
      const starts = tops().map((top, i) => (i ? Math.min(max, Math.max(0, top - innerHeight * 0.4)) : 0));
      return starts.map((a, i) => [a, Math.max(a + 1, i + 1 < starts.length ? starts[i + 1] : max)]);
    };
    let current = -1;
    let seeking = false;
    player.querySelector("[data-total]").textContent = clock(total);
    player.hidden = false;

    follow = () => {
      const s = spans();
      let k = 0;
      s.forEach(([a], i) => { if (a <= scrollY) k = i; });
      const f = Math.min(1, Math.max(0, (scrollY - s[k][0]) / (s[k][1] - s[k][0])));
      const now = starts[k] + f * lengths[k];
      elapsed.textContent = clock(now);
      if (!seeking) seek.value = String(Math.round((now / total) * 1000));
      seek.setAttribute("aria-valuetext", `${clock(now)} / ${clock(total)}`);
      seek.style.setProperty("--p", `${(now / total) * 100}%`);
      if (k === current) return;
      const changed = current !== -1;
      current = k;
      tracks.forEach((track, i) => track.classList.toggle("is-playing", i === k));
      links.forEach((a, i) => a.classList.toggle("is-playing", i === k));
      player.querySelector("[data-now-title]").textContent = tracks[k].querySelector(".track__t").textContent;
      player.querySelector("[data-now-n]").textContent = tracks[k].querySelector(".track__n").textContent;
      player.style.setProperty("--tint", getComputedStyle(tracks[k]).getPropertyValue("--tint"));
      if (changed && moving && player.animate) {
        player.querySelectorAll("[data-now-title], [data-now-n]").forEach((el) => el.animate(
          [{ opacity: 0, transform: "translateY(8px)" }, { opacity: 1, transform: "none" }],
          { duration: 320, easing: "cubic-bezier(.2, .7, .2, 1)" }));
      }
    };
    const go = (k) => {
      const behavior = still ? "instant" : "smooth";
      if (k <= 0) scrollTo({ top: 0, behavior });
      else tracks[Math.min(k, tracks.length - 1)].scrollIntoView({ behavior });
    };
    player.querySelector("[data-prev]").addEventListener("click", () => {
      // Like a player: back to the start of this track, or the previous one
      // if you're already there.
      go(scrollY > tops()[current] + 80 && current > 0 ? current : current - 1);
    });
    player.querySelector("[data-next]").addEventListener("click", () => go(current + 1));
    seek.addEventListener("input", () => {
      seeking = true;
      const want = (Number(seek.value) / 1000) * total;
      let k = 0;
      starts.forEach((s, i) => { if (s <= want) k = i; });
      const [a, b] = spans()[k];
      scrollTo({ top: a + ((want - starts[k]) / lengths[k]) * (b - a), behavior: "instant" });
    });
    seek.addEventListener("change", () => { seeking = false; });
    const label = () => toggle.setAttribute("aria-label", fmt(moving ? "pause" : "play"));
    toggle.addEventListener("click", () => { setMoving(!moving); label(); });
    setMoving(moving);
    label();
  }

  // Reveals: content eases up as it scrolls into view, groups in a short
  // stagger. Only with motion; otherwise nothing is ever hidden.
  if (!still && typeof IntersectionObserver === "function") {
    // Blocks, not rows: each section's head, its title and text, its media
    // and its points fade in once.
    const groups = [
      [".track--hero .track__head", 0, 0],
      [".hero__below > *", 380, 110],
      [".shot--hero", 620, 0],
      [".track:not(.track--hero) .track__head, .album__head, .album__actions, .tracklist, .faq h2", 0, 0],
      ["div.lede > *, h2.lede, .split__copy > *", 60, 90],
      [".lyrics, .shot:not(.shot--hero), .points--row, .points--grid, .meters, .table, .footnote", 160, 0],
      [".get > *", 0, 80],
      [".others, .get__all, .faq__grid > div", 120, 0],
    ];
    const reveal = new IntersectionObserver((entries) => entries.forEach((entry) => {
      if (!entry.isIntersecting) return;
      entry.target.classList.add("is-in");
      reveal.unobserve(entry.target);
    }), { rootMargin: "0px 0px -6% 0px" });
    for (const [selector, delay, step] of groups) {
      const seen = new Map();
      document.querySelectorAll(selector).forEach((el) => {
        const i = seen.get(el.parentNode) || 0;
        seen.set(el.parentNode, i + 1);
        el.classList.add("rv");
        el.style.setProperty("--rv-delay", `${delay + Math.min(i, 8) * step}ms`);
      });
    }
    // Commit the hidden state before observing, so what's already on screen
    // (the hero) eases in rather than appearing in place.
    void document.body.offsetHeight;
    document.querySelectorAll(".rv").forEach((el) => reveal.observe(el));
  }

  // Disclosures animate only after the first paint, so closing the download
  // list for a detected platform doesn't play on load.
  if (typeof requestAnimationFrame === "function") {
    requestAnimationFrame(() => requestAnimationFrame(() => document.documentElement.classList?.add("is-ready")));
  }

  let queued = false;
  const frame = () => {
    queued = false;
    follow();
  };
  const schedule = () => { if (!queued) { queued = true; requestAnimationFrame(frame); } };
  if (player) {
    addEventListener("scroll", schedule, { passive: true });
    addEventListener("resize", schedule);
    frame();
  }
})();
