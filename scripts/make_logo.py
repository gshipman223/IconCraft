#!/usr/bin/env python3
"""Generate IconCraft's logo: assets/logo.svg (source) + assets/icon.icns.

Design: a full-bleed macOS squircle with an indigo->violet->cyan gradient.
Inside, a 2x2 grid of app-icon tiles; the top-right tile is lifted, rotated
and glowing cyan — an icon being crafted/swapped — with a spark.
Run: python3 scripts/make_logo.py   (requires Pillow)
"""
from pathlib import Path
import math, subprocess, shutil
from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parents[1]
SS = 4                      # supersample
S = 1024 * SS

def lerp(a, b, t): return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))

def gradient(size):
    stops = [(0.0, (124, 58, 237)), (0.52, (79, 70, 229)), (1.0, (6, 182, 212))]
    img = Image.new("RGB", (size, size))
    px = img.load()
    for y in range(size // 8):
        for x in range(size // 8):
            t = min(1.0, (x + y) / (2 * (size // 8)))
            for i in range(len(stops) - 1):
                if stops[i][0] <= t <= stops[i + 1][0]:
                    tt = (t - stops[i][0]) / (stops[i + 1][0] - stops[i][0] or 1)
                    c = lerp(stops[i][1], stops[i + 1][1], tt)
            for dy in range(8):
                for dx in range(8):
                    px[x * 8 + dx, y * 8 + dy] = c
    return img

def rrect(d, box, r, **kw): d.rounded_rectangle(box, radius=r, **kw)

def spark(d, cx, cy, r, fill):
    pts = []
    for i in range(8):
        ang = math.pi / 2 * (i // 2) + (math.pi / 4 if i % 2 else 0)
        # 4-point star: alternate long/short radii every 45deg
        rad = r if i % 2 == 0 else r * 0.28
        a = -math.pi / 2 + i * math.pi / 4
        pts.append((cx + rad * math.cos(a), cy + rad * math.sin(a)))
    d.polygon(pts, fill=fill)

def draw_master():
    img = gradient(S).convert("RGBA")
    d = ImageDraw.Draw(img, "RGBA")
    # soft top light
    glow = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    gd = ImageDraw.Draw(glow)
    gd.ellipse([-S*0.3, -S*0.55, S*0.7, S*0.35], fill=(255, 255, 255, 70))
    img = Image.alpha_composite(img, glow.filter(ImageFilter.GaussianBlur(S // 24)))
    d = ImageDraw.Draw(img, "RGBA")
    # tile grid geometry (in supersampled px, relative to 1024)
    u = S / 1024
    tile, gap = 218 * u, 44 * u
    x0, y0 = (S - (2 * tile + gap)) / 2, (S - (2 * tile + gap)) / 2 + 18 * u
    tiles = [(x0, y0), (x0 + tile + gap, y0), (x0, y0 + tile + gap), (x0 + tile + gap, y0 + tile + gap)]
    overlay = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    od = ImageDraw.Draw(overlay)
    for i, (x, y) in enumerate(tiles):
        if i == 1:  # the crafted tile is drawn lifted, below
            continue
        rrect(od, [x, y, x + tile, y + tile], 52 * u, fill=(255, 255, 255, 46), outline=(255, 255, 255, 110), width=int(3 * u))
    img = Image.alpha_composite(img, overlay)
    d = ImageDraw.Draw(img, "RGBA")
    # lifted tile: rotated cyan tile with glow + white glyph dot-grid
    lift = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    ld = ImageDraw.Draw(lift)
    x, y = tiles[1][0] + 26 * u, tiles[1][1] - 30 * u
    rrect(ld, [x, y, x + tile, y + tile], 52 * u, fill=(34, 211, 238, 255))
    for gy in range(2):
        for gx in range(2):
            cx, cy = x + tile * (0.32 + 0.36 * gx), y + tile * (0.32 + 0.36 * gy)
            ld.ellipse([cx - 26 * u, cy - 26 * u, cx + 26 * u, cy + 26 * u], fill=(255, 255, 255, 235))
    lift = lift.rotate(10, resample=Image.BICUBIC, center=(x + tile / 2, y + tile / 2))
    shadow = lift.split()[3].point(lambda a: int(a * 0.45))
    sh = Image.new("RGBA", (S, S), (10, 6, 40, 0)); sh.putalpha(shadow)
    img.alpha_composite(sh.filter(ImageFilter.GaussianBlur(int(18 * u))), (0, int(16 * u)))
    img.alpha_composite(lift)
    d = ImageDraw.Draw(img, "RGBA")
    spark(d, S * 0.845, S * 0.155, 62 * u, (255, 255, 255, 255))
    spark(d, S * 0.90, S * 0.27, 30 * u, (165, 243, 252, 255))
    # squircle mask
    mask = Image.new("L", (S, S), 0)
    md = ImageDraw.Draw(mask)
    inset = 8 * u
    rrect(md, [inset, inset, S - inset, S - inset], 225 * u, fill=255)
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    out.paste(img, (0, 0), mask)
    return out.resize((1024, 1024), Image.LANCZOS)

def main():
    master = draw_master()
    iconset = ROOT / "build" / "logo.iconset"
    if iconset.exists(): shutil.rmtree(iconset)
    iconset.mkdir(parents=True)
    for name, px in [("icon_16x16.png",16),("icon_16x16@2x.png",32),("icon_32x32.png",32),
                     ("icon_32x32@2x.png",64),("icon_128x128.png",128),("icon_128x128@2x.png",256),
                     ("icon_256x256.png",256),("icon_256x256@2x.png",512),("icon_512x512.png",512),
                     ("icon_512x512@2x.png",1024)]:
        master.resize((px, px), Image.LANCZOS).save(iconset / name)
    master.save(ROOT / "assets" / "logo-1024.png")
    subprocess.run(["iconutil", "-c", "icns", str(iconset), "-o", str(ROOT / "assets" / "icon.icns")], check=True)
    print("wrote assets/icon.icns + assets/logo-1024.png")

if __name__ == "__main__":
    main()
