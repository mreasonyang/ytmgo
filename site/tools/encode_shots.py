#!/usr/bin/env python3
"""Encode the rendered UI screenshots and icons into the website's assets.

    YTMGO_SITE_SHOTS_DIR=/tmp/shots go test -run TestSiteScreenshots .
    python3 site/tools/encode_shots.py /tmp/shots

Each screenshot is written at 1x and 2x as WebP; the icon becomes the favicon
and touch icon, and the English home screen becomes the social card.
"""
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "assets"
SHOTS = ["home", "album", "immersive", "light", "compact"]


def encode(src: Path, dst: Path) -> None:
    im = Image.open(src).convert("RGB")
    dst.parent.mkdir(parents=True, exist_ok=True)
    for scale in (1, 2):
        size = (im.width * scale // 2, im.height * scale // 2)
        im.resize(size, Image.LANCZOS).save(dst.with_name(f"{dst.name}@{scale}x.webp"), "WEBP", quality=84, method=6)


def social(home: Path, icon: Image.Image) -> None:
    w, h = 1200, 630
    card = Image.new("RGB", (w, h), (10, 10, 11))
    glow = Image.new("RGB", (w, h), (0, 0, 0))
    ImageDraw.Draw(glow).ellipse((620, 40, 1300, 720), fill=(255, 90, 79))
    card = Image.blend(card, glow.filter(ImageFilter.GaussianBlur(160)), 0.45)
    shot = Image.open(home).convert("RGB")
    shot = shot.resize((900, 900 * shot.height // shot.width), Image.LANCZOS)
    mask = Image.new("L", shot.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, *shot.size), 18, fill=255)
    card.paste(shot, (360, 110), mask)
    badge = icon.resize((220, 220), Image.LANCZOS)
    card.paste(badge, (90, 205), badge)
    card.save(ASSETS / "social.png", optimize=True)


def main() -> None:
    shots = Path(sys.argv[1])
    for locale in sorted(p.name for p in shots.iterdir() if p.is_dir()):
        for name in SHOTS:
            encode(shots / locale / f"{name}.png", ASSETS / "shots" / locale / name)
    icon = Image.open(ROOT.parent / "resources" / "icon.png").convert("RGBA")
    for size in (32, 180, 512):
        icon.resize((size, size), Image.LANCZOS).save(ASSETS / f"icon-{size}.png", optimize=True)
    social(shots / "en" / "home.png", icon)


if __name__ == "__main__":
    main()
