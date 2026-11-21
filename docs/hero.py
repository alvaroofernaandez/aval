#!/usr/bin/env python3
"""The README hero: the aval mark, built out of passed checks.

    python3 docs/brand.py          # writes hero-light.svg and hero-dark.svg

An editorial, Swiss-grid composition. The wordmark is set large and flush
left; mono metadata sits above and below on hairline rules. On the right,
the isotype is drawn as a matrix of tiny check marks: the cells that fall on
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
CELL = 15
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


def meta(t, x, y, text, anchor="start", colour=None):
    return (
        f'<text x="{x}" y="{y}" text-anchor="{anchor}" font-family="{MONO}" '
        f'font-size="13" letter-spacing="2.5" fill="{colour or t["muted"]}">{text}</text>'
    )


def build(theme="light"):
    t = THEMES[theme]
    top, bottom = PAD + 8, H - PAD
    rule_top, rule_bottom = top + 20, bottom - 52
    scale = 1.6
    ww, wh = logo.word_size()

    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" '
        f'width="{W}" height="{H}" role="img" '
        f'aria-label="aval — Garantía: alguien responde por algo. Harness personal.">',
        f'<rect width="{W}" height="{H}" fill="{t["bg"]}"/>',
        # Header: name of the thing on the left, what kind of word it is on the right.
        meta(t, PAD, top, "AVAL — HARNESS PERSONAL", colour=t["ink"]),
        meta(t, W - PAD, top, "/aˈβal/ · NOMBRE MASCULINO", anchor="end"),
        f'<line x1="{PAD}" y1="{rule_top}" x2="{W - PAD}" y2="{rule_top}" '
        f'stroke="{t["ink"]}" stroke-width="1"/>',
        # The wordmark, flush left, vertically centred between the rules.
        logo.word_group(
            t["ink"], PAD - 4, (rule_top + rule_bottom) / 2 - wh * scale / 2, scale
        ),
    ]
    # The matrix is centred between the rules, so it never touches either.
    parts += matrix(t, (rule_top + rule_bottom) / 2 - CELLS * CELL / 2)
    # Footer: three columns, the definition first.
    parts += [
        f'<line x1="{PAD}" y1="{rule_bottom}" x2="{W - PAD}" y2="{rule_bottom}" '
        f'stroke="{t["ink"]}" stroke-width="1"/>',
        meta(t, PAD, rule_bottom + 30, "DEFINICIÓN"),
        f'<text x="{PAD}" y="{bottom}" font-family="{MONO}" font-size="16" '
        f'fill="{t["ink"]}">Garantía: alguien responde por algo.</text>',
        meta(t, 560, rule_bottom + 30, "TESIS"),
        f'<text x="560" y="{bottom}" font-family="{MONO}" font-size="16" '
        f'fill="{t["ink"]}">Nada se da por bueno sin pruebas.</text>',
        meta(t, W - PAD, rule_bottom + 30, "PARA", anchor="end"),
        f'<text x="{W - PAD}" y="{bottom}" text-anchor="end" font-family="{MONO}" '
        f'font-size="16" fill="{t["ink"]}">Claude Code · OpenCode</text>',
        "</svg>",
    ]
    return "\n".join(parts)


if __name__ == "__main__":
    import sys

    print(build(sys.argv[1] if len(sys.argv) > 1 else "light"))
