#!/usr/bin/env python3
"""The README hero: the aval mark, built out of passed checks.

    python3 docs/brand.py          # writes hero-light.svg and hero-dark.svg

A brand lockup, not a document: the wordmark set large and flush left with
the claim "Nada sin pruebas." beneath it, and on the right the isotype drawn
as a matrix of tiny check marks: the cells that fall on
the mark's stroke are inked, every other cell is a faint check. The mark is
literally made of passed checks — "nada sin pruebas" as a picture.

Pure geometry, no randomness: the matrix is a distance test against the same
circle and polyline that docs/logo.py draws, so the hero can never drift from
the logo. Black and white only.
"""

import math

import logo

W, H = 1200, 520
PAD = 56

THEMES = {
    "light": {"bg": "#ffffff", "ink": "#000000", "muted": "#525252", "faint": "#d4d4d4"},
    "dark": {"bg": "#000000", "ink": "#ffffff", "muted": "#a3a3a3", "faint": "#2e2e2e"},
}

MONO = "ui-monospace,SFMono-Regular,Menlo,Consolas,monospace"

# The matrix: a square of CELLS x CELLS checks on the right of the grid.
CELLS = 20
CELL = 17
MX = W - PAD - CELLS * CELL      # left edge of the matrix


def _seg_dist(px, py, ax, ay, bx, by):
    dx, dy = bx - ax, by - ay
    t = max(0.0, min(1.0, ((px - ax) * dx + (py - ay) * dy) / (dx * dx + dy * dy)))
    return math.hypot(px - (ax + t * dx), py - (ay + t * dy))


def on_mark(u, v):
    """Is the point (u, v), in the logo's 512 canvas, on the mark's stroke?"""
    rx, ry, r = logo.RING
    tx, ty = logo.CHECK_TIP
    jx, jy = rx + r + 40, ry + r
    half = logo.MS / 2
    if abs(math.hypot(u - rx, v - ry) - r) <= half:
        return True
    return (
        _seg_dist(u, v, rx + r, ry, jx, jy) <= half
        or _seg_dist(u, v, jx, jy, tx, ty) <= half
    )


def mini_check(x, y, on, t):
    """One cell: a small check, inked when it falls on the mark."""
    p = [(0.24, 0.54), (0.43, 0.72), (0.78, 0.30)]
    d = "M " + " L ".join(f"{x + a * CELL:.1f} {y + b * CELL:.1f}" for a, b in p)
    if on:
        return f'<path class="ck on" d="{d}" stroke="{t["ink"]}" stroke-width="2.2"/>'
    return f'<path class="ck" d="{d}" stroke="{t["faint"]}" stroke-width="1.3"/>'


def matrix(t, top):
    scale = 512 / (CELLS * CELL)
    out = [
        f'<g data-part="matrix" fill="none" stroke-linecap="round" '
        f'stroke-linejoin="round">'
    ]
    for row in range(CELLS):
        for col in range(CELLS):
            # Sample the logo at the cell's centre.
            u = (col + 0.5) * CELL * scale
            v = (row + 0.5) * CELL * scale
            out.append(mini_check(MX + col * CELL, top + row * CELL, on_mark(u, v), t))
    out.append("</g>")
    return out


SANS = "ui-sans-serif,-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif"


def build(theme="light"):
    t = THEMES[theme]
    scale = 1.45
    ww, wh = logo.word_size()

    # The lockup: wordmark, claim, descriptor — one block, centred vertically.
    claim_gap, claim_size, desc_gap = 74, 36, 42
    block = wh * scale + claim_gap + desc_gap
    top = (H - block) / 2
    claim_y = top + wh * scale + claim_gap
    desc_y = claim_y + desc_gap

    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" '
        f'width="{W}" height="{H}" role="img" '
        f'aria-label="aval. Nada sin pruebas. Harness personal para agentes de IA.">',
        f'<rect width="{W}" height="{H}" fill="{t["bg"]}"/>',
        logo.word_group(t["ink"], PAD - 4, top, scale),
        f'<text x="{PAD}" y="{claim_y:.0f}" font-family="{SANS}" font-size="{claim_size}" '
        f'font-weight="600" letter-spacing="-0.8" fill="{t["ink"]}">Nada sin pruebas.</text>',
        f'<text x="{PAD + 2}" y="{desc_y:.0f}" font-family="{MONO}" font-size="13" '
        f'letter-spacing="3" fill="{t["muted"]}">HARNESS PERSONAL PARA AGENTES DE IA</text>',
    ]
    # The brand visual: the isotype built from checks, as tall as the lockup.
    parts += matrix(t, (H - CELLS * CELL) / 2)
    parts.append("</svg>")
    return "\n".join(parts)


if __name__ == "__main__":
    import sys

    print(build(sys.argv[1] if len(sys.argv) > 1 else "light"))
