#!/usr/bin/env python3
"""Encode the rendered UI screenshots and icons into the website's assets.

    YTMGO_SITE_SHOTS_DIR=/tmp/shots go test -run TestSiteScreenshots .
    python3 site/tools/encode_shots.py /tmp/shots

Each screenshot is written at 1x and 2x as WebP; the icon becomes the favicon
and touch icon, and the English home screen becomes the social card.
Re-render only the card from the shipped home screen with:

    python3 site/tools/encode_shots.py --social
"""
import json
import subprocess
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "assets"
TOKENS = json.loads((ROOT.parent / "design" / "tokens.json").read_text())


def token(hex_value: str) -> tuple[int, int, int]:
    """An opaque colour from design/tokens.json as an RGB tuple."""
    return tuple(int(hex_value[i:i + 2], 16) for i in (1, 3, 5))


DARK = TOKENS["color"]["dark"]
SHOTS = ["home", "album", "immersive", "light", "compact"]


def encode(src: Path, dst: Path) -> None:
    im = Image.open(src).convert("RGB")
    dst.parent.mkdir(parents=True, exist_ok=True)
    for scale in (1, 2):
        size = (im.width * scale // 2, im.height * scale // 2)
        im.resize(size, Image.LANCZOS).save(dst.with_name(f"{dst.name}@{scale}x.webp"), "WEBP", quality=84, method=6)


def font(family: str, size: int, weight: int) -> ImageFont.FreeTypeFont:
    """The system's font for a fontconfig family ("sans-serif", "monospace"):
    ytmGO ships no typefaces, so the card uses the system's, as the site does."""
    style = ":bold" if weight >= 600 else ""
    path = subprocess.run(["fc-match", "-f", "%{file}", family + style],
                          capture_output=True, text=True, check=True).stdout
    f = ImageFont.truetype(path, size)
    try:
        axes = f.get_variation_axes()
    except OSError:
        return f
    f.set_variation_by_axes([weight if a["name"] in (b"Weight", "Weight") else a["default"] for a in axes])
    return f


def social(home: Path, icon: Image.Image) -> None:
    """The link-preview card, in the site's own look: the intro track's rust
    wash, the wordmark and headline, and the real home screen."""
    w, h = 1200, 630
    tint = token(TOKENS["tint"]["rust"])
    card = Image.new("RGB", (w, h), (0, 0, 0))
    wash = ImageDraw.Draw(card)
    for y in range(h):
        k = max(0.0, 0.78 - 0.78 * y / (h * 0.9))
        wash.line([(0, y), (w, y)], fill=tuple(round(c * k) for c in tint))
    shot = Image.open(home).convert("RGB")
    shot = shot.resize((760, 760 * shot.height // shot.width), Image.LANCZOS)
    mask = Image.new("L", shot.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, *shot.size), 16, fill=255)
    card.paste(shot, (560, 150), mask)
    draw = ImageDraw.Draw(card)
    draw.rounded_rectangle((559, 149, 560 + shot.width, 150 + shot.height), 16, outline=tuple(round(c * .4) for c in tint), width=1)
    badge = icon.resize((76, 76), Image.LANCZOS)
    card.paste(badge, (64, 64), badge)
    brand = font("sans-serif", 44, 800)
    draw.text((156, 102), "ytm", font=brand, fill=token(DARK["text"]), anchor="lm")
    draw.text((156 + draw.textlength("ytm", font=brand), 102), "GO", font=brand, fill=token(DARK["accent"]), anchor="lm")
    lines = ("Your music", "deserves more than", "a browser tab")
    size = 76
    while max(draw.textlength(t, font=font("sans-serif", size, 800)) for t in lines) > 466:
        size -= 2
    title = font("sans-serif", size, 800)
    y = 236
    for line in lines:
        draw.text((62, y), line, font=title, fill=token(DARK["text"]))
        y += round(size * 0.98)
    mono = font("monospace", 19, 500)
    draw.text((64, 548), "Free · macOS · Windows · Linux", font=mono, fill=token(DARK["secondary"]))
    card.save(ASSETS / "social.png", optimize=True)


def main() -> None:
    icon = Image.open(ROOT.parent / "resources" / "icon.png").convert("RGBA")
    if sys.argv[1] == "--social":
        social(ASSETS / "shots" / "en" / "home@2x.webp", icon)
        return
    shots = Path(sys.argv[1])
    for locale in sorted(p.name for p in shots.iterdir() if p.is_dir()):
        for name in SHOTS:
            encode(shots / locale / f"{name}.png", ASSETS / "shots" / locale / name)
    for size in (32, 180, 512):
        icon.resize((size, size), Image.LANCZOS).save(ASSETS / f"icon-{size}.png", optimize=True)
    social(shots / "en" / "home.png", icon)


if __name__ == "__main__":
    main()
