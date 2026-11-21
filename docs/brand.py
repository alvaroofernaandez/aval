#!/usr/bin/env python3
"""Generate every image the README shows.

    python3 docs/brand.py              # writes into docs/assets/
    python3 docs/brand.py --out DIR    # writes somewhere else (tests use this)
    python3 docs/brand.py --list       # prints the tool labels, one per line
    python3 docs/brand.py --no-logo    # prints the tools with no published logo

Outputs, each in a light and a dark variant so the README can swap them with
<picture> to follow the reader's GitHub theme:

    hero-{light,dark}.svg    wordmark + the isotype built from checks (docs/hero.py)
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
import hashlib
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

# Neutral grey cards (r=g=b), one per theme, measured against every logo: no
# single grey works for both — mid grey fails most colour logos, dark grey
# fails the black ones. So each theme gets its own grey and the logos that
# vanish on it are adapted (see DARK_ADAPT and TILES).
THEMES = {
    "light": {"fg": "#1f2328", "muted": "#59636e", "card": "#ededed", "edge": "#d4d4d4", "mono": "#1f2328"},
    "dark": {"fg": "#f0f6fc", "muted": "#9198a1", "card": "#262626", "edge": "#3a3a3a", "mono": "#f0f0f0"},
}

# Marks drawn in black or near-black. On the dark card their dark tones are
# lightened; brand colours (tmux and Bash green, Zsh orange) are kept.
DARK_ADAPT = {"apple.svg", "github.svg", "zsh.svg", "bash.svg", "tmux.svg"}
LIGHT_INK = "#f0f0f0"

# Raster marks too dark for the dark card get a vendored, lightened variant
# (same hue, more lightness) instead of a recolour at build time.
DARK_FILES = {"bat.png": "bat-dark.png"}

# Logos that are app icons with their own square background: clipped to a
# rounded tile so they present as icons, not as hard-edged screenshots.
TILES = {"engram.png", "eza.png", "rtk.png", "opencode.svg", "herdr.svg",
         "omniwm.svg", "gstack.png", "zoxide.svg", "fzf.png"}

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


def _luminance(hex6):
    def c(v):
        v /= 255
        return v / 12.92 if v <= 0.04045 else ((v + 0.055) / 1.055) ** 2.4
    r, g, b = (int(hex6[i:i + 2], 16) for i in (0, 2, 4))
    return 0.2126 * c(r) + 0.7152 * c(g) + 0.0722 * c(b)


def lighten_darks(svg):
    """Turn the dark tones of a mark light, leave its brand colours alone.

    Shapes with no fill default to black, so the root gets a light fill that
    they inherit; explicit dark hex fills are swapped for the same light ink.
    """
    def swap(m):
        h = m.group(1)
        if len(h) == 3:
            h = "".join(ch * 2 for ch in h)
        return LIGHT_INK if _luminance(h) < 0.05 else m.group(0)

    svg = re.sub(r"#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b", swap, svg)
    return re.sub(r"<svg\b", f'<svg fill="{LIGHT_INK}"', svg, count=1)


def aspect(name):
    """Width / height of a vendored logo, from the PNG header or SVG viewBox."""
    path = LOGOS / name
    if path.suffix == ".png":
        head = path.read_bytes()[16:24]
        return int.from_bytes(head[:4], "big") / int.from_bytes(head[4:], "big")
    vb = re.search(r'viewBox="[\d.\-]+[ ,]+[\d.\-]+[ ,]+([\d.]+)[ ,]+([\d.]+)"', path.read_text())
    return float(vb.group(1)) / float(vb.group(2)) if vb else 1.0


def data_uri(name, theme="light"):
    if theme == "dark" and name in DARK_FILES:
        name = DARK_FILES[name]
    path = LOGOS / name
    mime = MIME[path.suffix]
    data = path.read_bytes()
    if theme == "dark" and name in DARK_ADAPT:
        data = lighten_darks(data.decode()).encode()
    return f"data:{mime};base64," + base64.b64encode(data).decode()


def esc(text):
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def tile(x, y, label, logo_file, mono, t, theme):
    cx = x + TILE_W / 2
    top = y + HEAD
    out = [
        f'<rect class="card" x="{cx - CARD / 2:.1f}" y="{top}" width="{CARD}" height="{CARD}" '
        f'rx="20" fill="{t["card"]}" stroke="{t["edge"]}" stroke-width="1"/>'
    ]
    if logo_file:
        off = (CARD - LOGO) / 2
        ix, iy = cx - CARD / 2 + off, top + off
        extra = ""
        if logo_file in TILES:
            cid = "tile-" + logo_file.split(".")[0]
            # Clip to the image's own box inside the square, so a wide mark
            # gets a rounded band rather than an invisible rounded square.
            r = aspect(logo_file)
            cw, ch = (LOGO, LOGO / r) if r >= 1 else (LOGO * r, LOGO)
            cx0, cy0 = ix + (LOGO - cw) / 2, iy + (LOGO - ch) / 2
            out.append(
                f'<clipPath id="{cid}"><rect x="{cx0:.1f}" y="{cy0:.1f}" width="{cw:.1f}" '
                f'height="{ch:.1f}" rx="{min(14, min(cw, ch) * 0.25):.1f}"/></clipPath>'
            )
            extra += f' clip-path="url(#{cid})"'
        if theme == "dark" and (logo_file in DARK_ADAPT or logo_file in DARK_FILES):
            extra += ' data-adapted="dark"'
        out.append(
            f'<image class="logo" width="{LOGO}" height="{LOGO}" '
            f'x="{ix:.1f}" y="{iy:.1f}"{extra} '
            f'preserveAspectRatio="xMidYMid meet" href="{data_uri(logo_file, theme)}"/>'
        )
    else:
        # No published logo: a plain monogram, never a drawn mark, so it can't
        # be mistaken for the tool's identity.
        out.append(
            f'<text x="{cx:.1f}" y="{top + CARD / 2 + 9:.1f}" text-anchor="middle" '
            f'font-family="{MONO}" font-size="26" font-weight="700" '
            f'fill="{t["mono"]}">{esc(mono)}</text>'
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
            parts.extend(tile(col * TILE_W, y, label, logo_file, mono, t, theme))
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
    images = {}
    for theme in THEMES:
        images[f"hero-{theme}"] = hero.build(theme) + "\n"
        images[f"stack-{theme}"] = stack(theme)
        images[f"mark-{theme}"] = logo.mark(theme)
        images[f"wordmark-{theme}"] = logo.wordmark(theme)
    names = write_assets(out, images)

    # Only the committed assets are referenced by the README.
    if out.resolve() == (HERE / "assets").resolve():
        version_readme(HERE.parent / "README.md", names)


def content_name(name, svg):
    """<name>.<sha256[:8]>.svg — the hash is part of the path.

    GitHub serves README images with max-age=300, so an image regenerated
    under an unchanged URL keeps showing the old version. A query string does
    not help: github.com/<repo>/raw/... 302-redirects to raw.githubusercontent
    and drops it. A content hash in the file name survives the redirect.
    """
    return f"{name}.{hashlib.sha256(svg.encode()).hexdigest()[:8]}.svg"


def write_assets(out, images):
    """Write each image under its content name and delete its older versions."""
    names = {}
    for name, svg in images.items():
        for old in list(out.glob(f"{name}.*.svg")) + [out / f"{name}.svg"]:
            if old.exists():
                old.unlink()
        fname = content_name(name, svg)
        (out / fname).write_text(svg)
        names[name] = fname
    return names


def version_readme(readme, names):
    """Point every README image URL at the current content-addressed file."""
    text = readme.read_text()
    pattern = r"docs/assets/(%s)(?:\.[0-9a-f]{8})?\.svg(?:\?v=[0-9a-f]+)?" % "|".join(
        re.escape(n) for n in names
    )
    new = re.sub(pattern, lambda m: f"docs/assets/{names[m.group(1)]}", text)
    if new != text:
        readme.write_text(new)


if __name__ == "__main__":
    main()
