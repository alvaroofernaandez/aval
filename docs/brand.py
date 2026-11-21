#!/usr/bin/env python3
"""Generate every image the README shows, in black and white.

    python3 docs/brand.py              # writes into docs/assets/
    python3 docs/brand.py --out DIR    # writes somewhere else (tests use this)
    python3 docs/brand.py --list       # prints the tool labels, one per line

Outputs, each in a light and a dark variant so the README can swap them with
<picture> to follow the reader's GitHub theme:

    hero-{light,dark}.svg    suminagashi rings (docs/hero.py)
    stack-{light,dark}.svg   the logo wall of every tool this repo configures

Logos come from Simple Icons (CC0), vendored under docs/icons/ so the images
rebuild offline and byte for byte. A tool with no published logo gets a
monogram tile instead of an invented one: a logo nobody drew is a lie about
the tool's identity. tests/readme.test.sh fails if the committed images drift
from this output.
"""

import argparse
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import hero  # noqa: E402

ICONS = HERE / "icons"

# (group, [(label, simple-icons slug or None for a monogram, monogram)])
STACK = [
    ("terminal", [
        ("Ghostty", "ghostty", None),
        ("tmux", "tmux", None),
        ("Zsh", "zsh", None),
        ("Neovim", "neovim", None),
        ("yazi", None, "yz"),
        ("Herdr", None, "hd"),
        ("OmniWM", None, "ow"),
    ]),
    ("agentes", [
        ("Claude Code", "claude", None),
        ("OpenCode", "opencode", None),
        ("Engram", None, "en"),
        ("RTK", None, "rt"),
        ("gstack", None, "gs"),
    ]),
    ("toolchain", [
        ("Go", "go", None),
        ("Node.js", "nodedotjs", None),
        ("npm", "npm", None),
        ("Bun", "bun", None),
        ("Python", "python", None),
        ("Bash", "gnubash", None),
        ("Homebrew", "homebrew", None),
    ]),
    ("runtime", [
        ("Docker", "docker", None),
        ("Kiri", None, "ki"),
        ("Lightpanda", None, "lp"),
        ("Git", "git", None),
        ("GitHub CLI", "github", None),
        ("gitleaks", None, "gl"),
    ]),
    ("cli", [
        ("ripgrep", None, "rg"),
        ("fd", None, "fd"),
        ("bat", None, "bt"),
        ("eza", None, "ez"),
        ("fzf", None, "fz"),
        ("zoxide", None, "z"),
        ("atuin", None, "at"),
        ("delta", None, "Δ"),
    ]),
    ("plataformas", [
        ("macOS", "apple", None),
        ("Linux", "linux", None),
        ("Windows", None, "wn"),
    ]),
]

THEMES = {
    "light": {"fg": "#000000", "muted": "#525252", "rule": "#d4d4d4"},
    "dark": {"fg": "#ffffff", "muted": "#a3a3a3", "rule": "#404040"},
}

FONT = "ui-sans-serif,-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif"
MONO = "ui-monospace,SFMono-Regular,Menlo,Consolas,monospace"

# Geometry. Eight tiles per row is the widest group (cli), so every group fits
# on one line and the wall reads as a table, not as a cloud.
W = 1000
LABEL_W = 150
TILE_W = 106
ROW_H = 96
ICON = 30
PAD_Y = 8


def icon_path(slug):
    svg = (ICONS / f"{slug}.svg").read_text()
    match = re.search(r'<path d="([^"]+)"', svg)
    if not match:
        raise SystemExit(f"docs/icons/{slug}.svg has no <path d>")
    return match.group(1)


def esc(text):
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def tile(x, y, label, slug, mono, t):
    cx = x + TILE_W / 2
    ix, iy = cx - ICON / 2, y + 14
    out = []
    if slug:
        scale = ICON / 24
        out.append(
            f'<path transform="translate({ix:.1f} {iy:.1f}) scale({scale:.4f})" '
            f'fill="{t["fg"]}" d="{icon_path(slug)}"/>'
        )
    else:
        # A monogram is drawn as an outline, never filled, so it can't be
        # mistaken for the tool's real mark.
        out.append(
            f'<rect x="{ix + 0.75:.1f}" y="{iy + 0.75:.1f}" width="{ICON - 1.5}" '
            f'height="{ICON - 1.5}" rx="6" fill="none" stroke="{t["fg"]}" '
            f'stroke-width="1.5"/>'
        )
        out.append(
            f'<text x="{cx:.1f}" y="{iy + ICON / 2 + 4.5:.1f}" text-anchor="middle" '
            f'font-family="{MONO}" font-size="13" font-weight="700" '
            f'fill="{t["fg"]}">{esc(mono)}</text>'
        )
    out.append(
        f'<text x="{cx:.1f}" y="{iy + ICON + 24:.1f}" text-anchor="middle" '
        f'font-family="{FONT}" font-size="13" fill="{t["fg"]}">{esc(label)}</text>'
    )
    return out


def stack(theme):
    t = THEMES[theme]
    height = PAD_Y * 2 + ROW_H * len(STACK)
    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {height}" '
        f'width="{W}" height="{height}" role="img" '
        f'aria-label="Herramientas del entorno">',
    ]
    for row, (group, tools) in enumerate(STACK):
        y = PAD_Y + row * ROW_H
        if row:
            parts.append(
                f'<line x1="0" y1="{y}" x2="{W}" y2="{y}" '
                f'stroke="{t["rule"]}" stroke-width="1"/>'
            )
        parts.append(
            f'<text x="0" y="{y + ROW_H / 2 + 4}" font-family="{MONO}" '
            f'font-size="12" letter-spacing="3" fill="{t["muted"]}">'
            f'{esc(group.upper())}</text>'
        )
        for col, (label, slug, mono) in enumerate(tools):
            parts.extend(tile(LABEL_W + col * TILE_W, y, label, slug, mono, t))
    parts.append("</svg>")
    return "\n".join(parts) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(HERE / "assets"))
    ap.add_argument("--list", action="store_true")
    args = ap.parse_args()

    if args.list:
        for _, tools in STACK:
            for label, _, _ in tools:
                print(label)
        return

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    for theme in THEMES:
        (out / f"hero-{theme}.svg").write_text(hero.build(theme) + "\n")
        (out / f"stack-{theme}.svg").write_text(stack(theme))


if __name__ == "__main__":
    main()
