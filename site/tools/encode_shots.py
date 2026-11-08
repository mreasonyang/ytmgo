#!/usr/bin/env python3
"""Encode the rendered UI screenshots and icons into the website's assets.

    YTMGO_SITE_SHOTS_DIR=/tmp/shots go test -run TestSiteScreenshots .
    python3 site/tools/encode_shots.py /tmp/shots

Each screenshot is written at 1x and 2x as WebP; the icon becomes the favicon
and touch icon, and the English home screen becomes the social card.
Re-render only the card from the shipped home screen with:

    python3 site/tools/encode_shots.py --social
"""
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "assets"
SHOTS = ["home", "album", "immersive", "light", "compact"]


def encode(src: Path, dst: Path) -> None:
    im = Image.open(src).convert("RGB")
    dst.parent.mkdir(parents=True, exist_ok=True)
    for scale in (1, 2):
        size = (im.width * scale // 2, im.height * scale // 2)
        im.resize(size, Image.LANCZOS).save(dst.with_name(f"{dst.name}@{scale}x.webp"), "WEBP", quality=84, method=6)


def font(name: str, size: int, axes: list[int]) -> ImageFont.FreeTypeFont:
    f = ImageFont.truetype(str(ASSETS / "fonts" / name), size)
    f.set_variation_by_axes(axes)
    return f


def social(home: Path, icon: Image.Image) -> None:
    """The link-preview card, in the site's own look: the intro track's rust
    wash, the wordmark and headline, and the real home screen."""
    w, h = 1200, 630
    tint = (176, 71, 43)
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
    draw.rounded_rectangle((559, 149, 560 + shot.width, 150 + shot.height), 16, outline=(70, 40, 34), width=1)
    badge = icon.resize((76, 76), Image.LANCZOS)
    card.paste(badge, (64, 64), badge)
    brand = font("bricolage-grotesque.woff2", 44, [96, 720, 88])
    draw.text((156, 102), "ytm", font=brand, fill=(255, 255, 255), anchor="lm")
    draw.text((156 + draw.textlength("ytm", font=brand), 102), "GO", font=brand, fill=(255, 90, 79), anchor="lm")
    lines = ("Your music", "deserves more than", "a browser tab")
    size = 76
    while max(draw.textlength(t, font=font("bricolage-grotesque.woff2", size, [96, 720, 88])) for t in lines) > 466:
        size -= 2
    title = font("bricolage-grotesque.woff2", size, [96, 720, 88])
    y = 236
    for line in lines:
        draw.text((62, y), line, font=title, fill=(255, 255, 255))
        y += round(size * 0.98)
    mono = font("jetbrains-mono.woff2", 19, [500])
    draw.text((64, 548), "Free · macOS · Windows · Linux", font=mono, fill=(179, 179, 179))
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
