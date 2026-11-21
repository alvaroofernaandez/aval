#!/usr/bin/env python3
"""The README hero: aval as a certificate.

    python3 docs/brand.py          # writes hero-light.svg and hero-dark.svg

Guilloché is the engraved lattice of interlaced curves on banknotes, share
certificates and diplomas. It exists to vouch for a document: it is too
precise to forge by hand, so its presence is the guarantee. That is what
"aval" means, so the hero is built as one: a guilloché rosette sealed with
the aval isotype, an engraved rope border, a circle of microtext, and beside
it the word with its definition.

Everything is computed — sine waves in polar coordinates — and seeded by
nothing, so the image is identical on every run. Pure black and white; depth
comes from line density and opacity, never from hue.
"""

import math

import logo

W, H = 1200, 440

THEMES = {
    "light": {"bg": "#ffffff", "ink": "#000000", "muted": "#525252"},
    "dark": {"bg": "#000000", "ink": "#ffffff", "muted": "#a3a3a3"},
}

SERIF = "ui-serif,'New York',Georgia,'Times New Roman',serif"
MONO = "ui-monospace,SFMono-Regular,Menlo,Consolas,monospace"

# The rosette sits left of centre; the text block takes the right.
CX, CY = 290, H / 2
SEAL_R = 58          # clear disc that holds the isotype

# Interlaced bands, outermost first: (base radius, amplitude, lobes, strands).
# Each strand is the same wave shifted in phase, which is what produces the
# woven moiré of real guilloché rather than a stack of separate rings.
BANDS = [
    (136, 17, 24, 18),
    (100, 13, 18, 14),
    (74, 7, 12, 10),
]


def wave_ring(cx, cy, base, amp, lobes, phase, points=720):
    """One closed polar sine wave: r(θ) = base + amp·sin(lobes·θ + phase)."""
    pts = []
    for i in range(points):
        t = i / points * math.tau
        r = base + amp * math.sin(lobes * t + phase)
        pts.append(f"{cx + r * math.cos(t):.2f} {cy + r * math.sin(t):.2f}")
    return "M " + " L ".join(pts) + " Z"


def rosette(t):
    out = [f'<g data-part="guilloche" fill="none" stroke="{t["ink"]}" stroke-width="0.5">']
    for base, amp, lobes, strands in BANDS:
        for j in range(strands):
            phase = j / strands * math.tau / 2
            out.append(
                f'<path stroke-opacity="0.45" d="{wave_ring(CX, CY, base, amp, lobes, phase)}"/>'
            )
    # Hairline rings that bound each band, as an engraver would rule them.
    for r in (157, 118, 86, SEAL_R + 6):
        out.append(f'<circle cx="{CX}" cy="{CY}" r="{r}" stroke-opacity="0.9"/>')
    out.append("</g>")
    return out


def microtext(t):
    """Banknote microprint: the thesis set around the rosette.

    Each glyph is placed and rotated on its own instead of using <textPath>,
    which librsvg (and so most non-browser viewers) does not render.
    """
    r = 168
    phrase = "AVAL · NADA SIN PRUEBAS · " * 4
    out = [
        f'<g font-family="{MONO}" font-size="9" fill="{t["muted"]}" '
        f'text-anchor="middle">'
    ]
    for i, ch in enumerate(phrase):
        if ch == " ":
            continue
        a = i / len(phrase) * math.tau - math.pi / 2
        x, y = CX + r * math.cos(a), CY + r * math.sin(a)
        out.append(
            f'<text x="{x:.1f}" y="{y:.1f}" '
            f'transform="rotate({math.degrees(a) + 90:.1f} {x:.1f} {y:.1f})">{ch}</text>'
        )
    out.append("</g>")
    return out


def seal(t):
    """The isotype in a clear disc at the rosette's centre, like a stamp."""
    return [
        f'<g data-part="seal">',
        f'<circle cx="{CX}" cy="{CY}" r="{SEAL_R}" fill="{t["bg"]}" '
        f'stroke="{t["ink"]}" stroke-width="2"/>',
        logo.mark_group(t["ink"], CX + 3, CY, 76),
        "</g>",
    ]


def rope(t, y0, y1):
    """An engraved rope border: interlaced sine strands across the width."""
    mid, amp = (y0 + y1) / 2, (y1 - y0) / 2 - 1
    out = [f'<g fill="none" stroke="{t["ink"]}" stroke-width="0.6" stroke-opacity="0.6">']
    for j in range(6):
        phase = j / 6 * math.tau / 2
        pts = []
        for i in range(0, W - 80 + 1, 3):
            x = 40 + i
            y = mid + amp * math.sin(i / 9.0 + phase)
            pts.append(f"{x:.1f} {y:.2f}")
        out.append(f'<path d="M {" L ".join(pts)}"/>')
    out.append("</g>")
    return out


def frame(t):
    """Double rule, the border of a certificate."""
    return [
        f'<rect x="16" y="16" width="{W - 32}" height="{H - 32}" fill="none" '
        f'stroke="{t["ink"]}" stroke-width="1.5"/>',
        f'<rect x="24" y="24" width="{W - 48}" height="{H - 48}" fill="none" '
        f'stroke="{t["ink"]}" stroke-width="0.6"/>',
    ]


def entry(t):
    """The dictionary entry: wordmark, pronunciation, definition, thesis."""
    x = 600
    ww, _ = logo.word_size()
    scale = 0.95
    return [
        logo.word_group(t["ink"], x - 6, 78, scale),
        f'<text x="{x}" y="270" font-family="{MONO}" font-size="16" '
        f'fill="{t["muted"]}">/aˈβal/ · nombre masculino</text>',
        f'<text x="{x}" y="312" font-family="{SERIF}" font-size="26" '
        f'fill="{t["ink"]}"><tspan font-style="italic">Garantía:</tspan> '
        f"alguien responde por algo.</text>",
        f'<line x1="{x}" y1="340" x2="{x + 360}" y2="340" stroke="{t["ink"]}" '
        f'stroke-width="0.8"/>',
        f'<text x="{x}" y="370" font-family="{MONO}" font-size="14" '
        f'letter-spacing="4" fill="{t["muted"]}">HARNESS PERSONAL · NADA SIN PRUEBAS</text>',
    ]


def build(theme="light"):
    t = THEMES[theme]
    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" '
        f'width="{W}" height="{H}" role="img" '
        f'aria-label="aval — Garantía: alguien responde por algo. Harness personal.">',
        f'<rect width="{W}" height="{H}" fill="{t["bg"]}"/>',
    ]
    parts += frame(t)
    parts += rope(t, 32, 48)
    parts += rope(t, H - 48, H - 32)
    parts += rosette(t)
    parts += microtext(t)
    parts += seal(t)
    parts += entry(t)
    parts.append("</svg>")
    return "\n".join(parts)


if __name__ == "__main__":
    import sys

    print(build(sys.argv[1] if len(sys.argv) > 1 else "light"))
