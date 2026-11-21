#!/usr/bin/env python3
"""The aval mark and wordmark, drawn from geometry.

"Aval" is Spanish for a guarantee: someone vouches for something. So the
guarantee is inside the name — the "v" of aval is drawn as a check mark, and
its long arm rises to the same height as the "l", which gives the word its
rhythm: short, tall, short, tall.

The isotype fuses the "a" and the check: the bowl of the a, with its stem
replaced by the check's long arm. The initial and the guarantee are one shape.

Everything is a stroked primitive — circle, line, polyline — never a font.
A wordmark set in type renders in whatever fallback the viewer has; this one
is identical everywhere. Monoline, round caps, pure black or pure white.
"""

INK = {"light": "#000000", "dark": "#ffffff"}

# Wordmark geometry, in units of the x-height.
H = 100.0          # x-height
S = 22.0           # stroke
ASC = 150.0        # ascender: the "l" and the check's long arm
GAP = 30.0         # space between letters, edge to edge
CHECK_W = 112.0


def _a(x, base):
    """Single-storey geometric a: a ring and a stem on its right tangent."""
    r = (H - S) / 2
    cx, cy = x + S / 2 + r, base - H / 2
    stem = cx + r
    return [
        f'<circle cx="{cx:.1f}" cy="{cy:.1f}" r="{r:.1f}"/>',
        f'<path d="M {stem:.1f} {base - H + S / 2:.1f} V {base - S / 2:.1f}"/>',
    ], H


def _check(x, base, w=CHECK_W, top=ASC):
    """The v: a short arm down, a long arm up to the ascender."""
    # The short arm starts at the x-height, like any v, so the word still reads
    # "aval"; only the long arm departs from the letter and becomes a check.
    p0 = (x + S / 2, base - H + S / 2)
    p1 = (x + w * 0.42, base - S / 2)
    p2 = (x + w - S / 2, base - top + S / 2)
    d = f"M {p0[0]:.1f} {p0[1]:.1f} L {p1[0]:.1f} {p1[1]:.1f} L {p2[0]:.1f} {p2[1]:.1f}"
    return [f'<path d="{d}"/>'], w


def _l(x, base):
    cx = x + S / 2
    return [f'<path d="M {cx:.1f} {base - ASC + S / 2:.1f} V {base - S / 2:.1f}"/>'], S


def word_paths(x=0.0, base=ASC):
    """The four letters as SVG elements, and the total width."""
    parts, cursor = [], x
    # The check leans right over its own box, so the gap after it is tightened
    # to keep the optical spacing even.
    for glyph, gap in ((_a, GAP), (_check, GAP * 0.55), (_a, GAP), (_l, GAP)):
        elems, w = glyph(cursor, base)
        parts += elems
        cursor += w + gap
    return parts, cursor - GAP - x


def word_group(ink, x, y, scale):
    """The wordmark placed inside another SVG (the hero uses this)."""
    elems, _ = word_paths()
    return (
        f'<g transform="translate({x:.1f} {y:.1f}) scale({scale:.4f})" fill="none" '
        f'stroke="{ink}" stroke-width="{S}" stroke-linecap="round" '
        f'stroke-linejoin="round">' + "".join(elems) + "</g>"
    )


def word_size():
    _, w = word_paths()
    return w, ASC


def wordmark(theme):
    w, h = word_size()
    pad = S
    vw, vh = w + pad * 2, h + pad * 2
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {vw:.0f} {vh:.0f}" '
        f'width="{vw:.0f}" height="{vh:.0f}" role="img" aria-label="aval">\n'
        + word_group(INK[theme], pad, pad, 1.0)
        + "\n</svg>\n"
    )


# Isotype geometry on a 512 canvas. The stroke is 1/8 of the canvas so the
# mark keeps 2px lines at a 16px favicon.
M = 512
MS = 64
RING = (184, 314, 112)          # the a's bowl: cx, cy, r
CHECK_TIP = (440, 86)           # the long arm, rising past the bowl


def mark(theme):
    """The a and the check as one gesture.

    The bowl of the "a" is kept, and its stem is replaced by the check: the
    stroke leaves the bowl at its right, dips to the baseline and rises past
    the ascender. Read one way it is the initial; read the other it is the
    guarantee. A check inside a box was tried first and rejected — it is the
    stock "task done" icon of every icon set.
    """
    ink = INK[theme]
    cx, cy, r = RING
    tx, ty = CHECK_TIP
    joint = (cx + r + 40, cy + r)
    style = (
        f'fill="none" stroke="{ink}" stroke-width="{MS}" '
        f'stroke-linecap="round" stroke-linejoin="round"'
    )
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {M} {M}" '
        f'width="{M}" height="{M}" role="img" aria-label="aval">\n'
        f'<circle cx="{cx}" cy="{cy}" r="{r}" {style}/>\n'
        f'<path d="M {cx + r} {cy} L {joint[0]} {joint[1]} L {tx} {ty}" {style}/>\n'
        "</svg>\n"
    )
