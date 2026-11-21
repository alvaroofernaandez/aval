#!/usr/bin/env python3
"""Generate every image the README shows.

    python3 docs/brand.py              # writes into docs/assets/
    python3 docs/brand.py --out DIR    # writes somewhere else (tests use this)
    python3 docs/brand.py --list       # prints the tool labels, one per line
    python3 docs/brand.py --no-logo    # prints the tools with no published logo

Outputs, each in a light and a dark variant so the README can swap them with
<picture> to follow the reader's GitHub theme:

    hero-{light,dark}.svg    aval as a guilloché certificate (docs/hero.py)
    mark-{light,dark}.svg    the aval isotype (docs/logo.py)
    wordmark-{light,dark}.svg  the aval wordmark (docs/logo.py)
    stack-{light,dark}.svg   every tool's real, coloured logo

Logos are vendored under docs/logos/ (provenance in docs/logos/SOURCES.md) and
embedded as data URIs: GitHub renders a README SVG as an <img>, and an <img>
SVG cannot load any file outside itself. A tool that publishes no logo gets a
monogram tile instead of an invented mark. tests/readme.test.sh fails if the
committed images drift from this output.
"""

import argparse
import base64
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import hero  # noqa: E402
import logo  # noqa: E402

LOGOS = HERE / "logos"

# (group, [(label, simple-icons slug or None for a monogram, monogram)])
STACK = [
    ("terminal", [
        ("Ghostty", "ghostty.png", None),
        ("tmux", "tmux.svg", None),
        ("Zsh", "zsh.svg", None),
        ("Neovim", "neovim.svg", None),
        ("yazi", "yazi.png", None),
        ("Herdr", "herdr.svg", None),
        ("OmniWM", "omniwm.svg", None),
    ]),
    ("agentes", [
        ("Claude Code", "claude.svg", None),
        ("OpenCode", "opencode.svg", None),
        ("Engram", "engram.png", None),
        ("RTK", "rtk.png", None),
        ("gstack", "gstack.png", None),
    ]),
    ("toolchain", [
        ("Go", "go.svg", None),
        ("Node.js", "nodejs.svg", None),
        ("npm", "npm.svg", None),
        ("Bun", "bun.svg", None),
        ("Python", "python.svg", None),
        ("Bash", "bash.svg", None),
        ("Homebrew", "homebrew.svg", None),
    ]),
    ("runtime", [
        ("Docker", "docker.svg", None),
        ("Kiri", "kiri.svg", None),
        ("Lightpanda", "lightpanda.png", None),
        ("Git", "git.svg", None),
        ("GitHub CLI", "github.svg", None),
        ("gitleaks", "gitleaks.png", None),
    ]),
    ("cli", [
        ("ripgrep", None, "rg"),
        ("fd", "fd.svg", None),
        ("bat", "bat.png", None),
        ("eza", "eza.png", None),
        ("fzf", "fzf.png", None),
        ("zoxide", "zoxide.svg", None),
        ("atuin", "atuin.png", None),
        ("delta", None, "Δ"),
    ]),
    ("plataformas", [
        ("macOS", "apple.svg", None),
        ("Linux", "linux.svg", None),
        ("Windows", "windows.svg", None),
    ]),
]

# Every logo sits on a light card in both themes. Many marks are drawn for a
# white page (Apple, GitHub, Bash, Zsh are black), and a light card keeps them
# legible on GitHub's dark theme without recolouring anyone's logo.
THEMES = {
    "light": {"fg": "#1f2328", "muted": "#59636e", "card": "#f6f8fa", "edge": "#d1d9e0"},
    "dark": {"fg": "#f0f6fc", "muted": "#9198a1", "card": "#f6f8fa", "edge": "#3d444d"},
}

FONT = "ui-sans-serif,-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif"
MONO = "ui-monospace,SFMono-Regular,Menlo,Consolas,monospace"

# Geometry. Eight tiles is the widest group (cli), so every group fits on one
# line and the wall reads as a table, not a cloud.
COLS = 8
TILE_W = 128
W = COLS * TILE_W
CARD = 88
LOGO = 60
HEAD = 30      # group heading band
ROW_H = HEAD + CARD + 52

MIME = {".svg": "image/svg+xml", ".png": "image/png"}


def data_uri(name):
    path = LOGOS / name
    mime = MIME[path.suffix]
    return f"data:{mime};base64," + base64.b64encode(path.read_bytes()).decode()


def esc(text):
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def tile(x, y, label, logo_file, mono, t):
    cx = x + TILE_W / 2
    top = y + HEAD
    out = [
        f'<rect x="{cx - CARD / 2:.1f}" y="{top}" width="{CARD}" height="{CARD}" '
        f'rx="20" fill="{t["card"]}" stroke="{t["edge"]}" stroke-width="1"/>'
    ]
    if logo_file:
        off = (CARD - LOGO) / 2
        out.append(
            f'<image class="logo" width="{LOGO}" height="{LOGO}" '
            f'x="{cx - CARD / 2 + off:.1f}" y="{top + off:.1f}" '
            f'preserveAspectRatio="xMidYMid meet" href="{data_uri(logo_file)}"/>'
        )
    else:
        # No published logo: a plain monogram, never a drawn mark, so it can't
        # be mistaken for the tool's identity.
        out.append(
            f'<text x="{cx:.1f}" y="{top + CARD / 2 + 9:.1f}" text-anchor="middle" '
            f'font-family="{MONO}" font-size="26" font-weight="700" '
            f'fill="#1f2328">{esc(mono)}</text>'
        )
    out.append(
        f'<text x="{cx:.1f}" y="{top + CARD + 28}" text-anchor="middle" '
        f'font-family="{FONT}" font-size="15" font-weight="500" '
        f'fill="{t["fg"]}">{esc(label)}</text>'
    )
    return out


def stack(theme):
    t = THEMES[theme]
    height = ROW_H * len(STACK)
    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {height}" '
        f'width="{W}" height="{height}" role="img" '
        f'aria-label="Herramientas del entorno">',
    ]
    for row, (group, tools) in enumerate(STACK):
        y = row * ROW_H
        parts.append(
            f'<text x="{TILE_W / 2 - CARD / 2}" y="{y + 16}" font-family="{MONO}" '
            f'font-size="12" letter-spacing="3" fill="{t["muted"]}">'
            f'{esc(group.upper())}</text>'
        )
        for col, (label, logo_file, mono) in enumerate(tools):
            parts.extend(tile(col * TILE_W, y, label, logo_file, mono, t))
    parts.append("</svg>")
    return "\n".join(parts) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(HERE / "assets"))
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--no-logo", action="store_true")
    args = ap.parse_args()

    if args.list or args.no_logo:
        for _, tools in STACK:
            for label, logo_file, _ in tools:
                if args.list or not logo_file:
                    print(label)
        return

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    for theme in THEMES:
        (out / f"hero-{theme}.svg").write_text(hero.build(theme) + "\n")
        (out / f"stack-{theme}.svg").write_text(stack(theme))
        (out / f"mark-{theme}.svg").write_text(logo.mark(theme))
        (out / f"wordmark-{theme}.svg").write_text(logo.wordmark(theme))


if __name__ == "__main__":
    main()
