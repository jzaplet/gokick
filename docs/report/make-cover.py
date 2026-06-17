#!/usr/bin/env python3
"""
Regenerate mascot_card.png — the baked cover image for the report
(soft drop shadow + white rounded card + mascot, all flattened into one PNG).

Run this ONLY when the source mascot art changes. The shadow is baked into the
pixels on purpose: CSS box-shadow renders inconsistently across PDF viewers
(it often degrades to a hard rectangle), a flattened raster does not.

    pip install Pillow
    python3 make-cover.py

Input:  ../go-vue-cqrs-ddd.png   (transparent-background mascot)
Output: mascot_card.png
"""
from PIL import Image, ImageDraw, ImageFilter

SRC = "../go-vue-cqrs-ddd.png"
OUT = "mascot_card.png"

CW, CH = 1380, 1410          # canvas
cx, cy = 90, 80              # card top-left
cw, ch = 1200, 1200          # card size
rad = 74                     # card corner radius


def rrect_mask(w, h, r):
    m = Image.new("L", (w, h), 0)
    ImageDraw.Draw(m).rounded_rectangle([0, 0, w - 1, h - 1], radius=r, fill=255)
    return m


card_mask = rrect_mask(cw, ch, rad)
canvas = Image.new("RGBA", (CW, CH), (0, 0, 0, 0))

# soft baked shadow
shadow = Image.new("RGBA", (CW, CH), (0, 0, 0, 0))
shape = Image.new("RGBA", (cw, ch), (2, 44, 62, 255))
shape.putalpha(card_mask)
shadow.paste(shape, (cx, cy + 34), shape)          # offset downward
shadow = shadow.filter(ImageFilter.GaussianBlur(46))
a = shadow.split()[3].point(lambda v: int(v * 0.34))  # soften
shadow.putalpha(a)
canvas = Image.alpha_composite(canvas, shadow)

# white card
card = Image.new("RGBA", (cw, ch), (255, 255, 255, 255))
card.putalpha(card_mask)
canvas.alpha_composite(card, (cx, cy))

# mascot, centered with padding
m = Image.open(SRC).convert("RGBA")
pad = 88
box = cw - 2 * pad
s = min(box / m.width, box / m.height)
nw, nh = int(m.width * s), int(m.height * s)
m = m.resize((nw, nh), Image.LANCZOS)
canvas.alpha_composite(m, (cx + (cw - nw) // 2, cy + (ch - nh) // 2))

canvas.save(OUT)
print(f"wrote {OUT} {canvas.size}")
