#!/usr/bin/env bash
# Tests for `aval tmux paste-router`, the router behind cmd+v.
#
# cmd+v cannot carry an image on its own: Ghostty's paste_from_clipboard only
# pushes TEXT down the pty, and image bytes do not fit through it. TUIs that
# accept pasted images (Claude Code) instead watch for ctrl+v — 0x16 — and read
# the system clipboard natively, bypassing the pty.
#
# So one key has to mean two things, and the branch cannot be decided by which
# app is in front: a Claude Code pane reports `2.1.232` as its command, a
# version string that changes on every update. The clipboard CONTENT is the
# stable signal, which is what this router reads.
set -uo pipefail

. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/aval.sh"
SOURCE="$REPO_ROOT/internal/tmux/paste.go"

# Real `osascript -e 'clipboard info'` output, captured on macOS 15.
CLIP_SCREENSHOT='«class PNGf», 703838, «class AVIF», 164185, «class 8BPS», 5245480, GIF picture, 265978, TIFF picture, 23758990'
CLIP_TEXT='«class utxt», 10, «class ut16», 22'
CLIP_RICH_TEXT='«class utxt», 41, «class ut16», 84, «class RTF », 233, TIFF picture, 9210'
CLIP_EMPTY=''

pass=0
fail=0

ok() { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass + 1)); }
ko() { printf '  \033[31mFAIL\033[0m %s\n     expected: %s\n     actual:   %s\n' "$1" "$2" "$3"; fail=$((fail + 1)); }
assert_eq() { [ "$2" = "$3" ] && ok "$1" || ko "$1" "$2" "$3"; }

# ROUTE_ONLY keeps the decision inspectable without a tmux server attached,
# mirroring how launch.sh exposes LAUNCH_RESOLVE_ONLY.
route_for() { CLIPBOARD_INFO="$1" ROUTE_ONLY=1 "$AVAL" tmux paste-router 2>/dev/null; }

echo "paste-router"

assert_eq "the router binary is executable" "yes" \
  "$([ -x "$AVAL" ] && echo yes || echo no)"

# --- the case this exists for -------------------------------------------------
assert_eq "a screenshot routes to the image path" "image" "$(route_for "$CLIP_SCREENSHOT")"

# --- the case that must not regress -------------------------------------------
# 0x16 is quoted-insert in zsh, so sending it for ordinary text would corrupt
# the prompt line. Text has to stay on the plain paste path.
assert_eq "plain text routes to the text path" "text" "$(route_for "$CLIP_TEXT")"

# Copying from a browser or word processor often carries a TIFF rendering
# ALONGSIDE the text. Pasting text is the common intent and the cheaper mistake,
# so the presence of plain text wins over an image flavour.
assert_eq "rich text carrying a TIFF still routes to text" "text" "$(route_for "$CLIP_RICH_TEXT")"

# --- degenerate input ---------------------------------------------------------
assert_eq "an empty clipboard routes to text" "text" "$(route_for "$CLIP_EMPTY")"

# --- the probe must not be hardcoded to a stub --------------------------------
# Without an override the router has to ask the real system clipboard.
assert_eq "falls back to osascript when no override is given" "yes" \
  "$(rg -q 'clipboard info' "$SOURCE" 2>/dev/null && echo yes || echo no)"

# --- the two sides of the branch, by behaviour --------------------------------
# A stub standing in for tmux records the arguments it was handed, so these
# assert what the router DOES rather than what its source happens to contain.
STUB_DIR="$(mktemp -d)"
trap 'rm -rf "$STUB_DIR"' EXIT
STUB="$STUB_DIR/tmux"
cat >"$STUB" <<'STUB_EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$STUB_LOG"
STUB_EOF
chmod +x "$STUB"

# run-shell hands the script launchd's PATH, which has no Homebrew in it — the
# exact trap launch.sh documents. `env -i` reproduces it, and tmux MUST still be
# found or the binding dies with "returned 127".
MINIMAL_PATH=/usr/bin:/bin:/usr/sbin:/sbin
run_router() {
  STUB_LOG="$STUB_DIR/log" env -i \
    PATH="$MINIMAL_PATH" \
    HOME="$HOME" \
    STUB_LOG="$STUB_DIR/log" \
    TMUX_BIN="$STUB" \
    CLIPBOARD_INFO="$1" \
    "$AVAL" tmux paste-router >/dev/null 2>&1
  echo "$?"
}

: >"$STUB_DIR/log"
assert_eq "the image path exits cleanly under a minimal PATH" "0" \
  "$(run_router "$CLIP_SCREENSHOT")"
assert_eq "the image path sends the 0x16 byte" "yes" \
  "$(rg -q 'send-keys.*0x16' "$STUB_DIR/log" 2>/dev/null && echo yes || echo no)"

: >"$STUB_DIR/log"
assert_eq "the text path exits cleanly under a minimal PATH" "0" \
  "$(run_router "$CLIP_TEXT")"
assert_eq "the text path pastes through tmux rather than 0x16" "yes" \
  "$(rg -q 'paste-buffer' "$STUB_DIR/log" 2>/dev/null && echo yes || echo no)"
assert_eq "the text path never sends 0x16" "yes" \
  "$(rg -q '0x16' "$STUB_DIR/log" 2>/dev/null && echo no || echo yes)"

# --- tmux must be located without help from PATH ------------------------------
assert_eq "resolves a real tmux when PATH does not provide one" "yes" \
  "$(bin="$(env -i PATH="$MINIMAL_PATH" HOME="$HOME" RESOLVE_ONLY=1 "$AVAL" tmux paste-router 2>/dev/null)"; \
     [ -n "$bin" ] && [ -x "$bin" ] && echo yes || echo no)"

# --- Linux clipboard backend selection ---------------------------------------
# CLIPBOARD_BACKEND_ONLY=1 prints the backend the router would use. Fake
# binaries on a private PATH decide which tools "exist"; PASTE_OS and
# WAYLAND_DISPLAY are injected so this runs on macOS.
mkbin() { # mkbin <dir> <tool>...
  d="$1"; shift; mkdir -p "$d"
  for t in "$@"; do printf '#!/bin/sh\nexit 0\n' >"$d/$t"; chmod +x "$d/$t"; done
}
backend_for() { # backend_for <os> <wayland|""> <tool>...
  os="$1"; wl="$2"; shift 2
  bd="$(mktemp -d)"; mkbin "$bd" "$@"
  env -i PATH="$bd:/usr/bin:/bin" HOME="$HOME" PASTE_OS="$os" \
    ${wl:+WAYLAND_DISPLAY="$wl"} CLIPBOARD_BACKEND_ONLY=1 "$AVAL" tmux paste-router 2>/dev/null
  rm -rf "$bd"
}
assert_eq "macOS uses pbpaste"                       "pbpaste"  "$(backend_for Darwin "" pbpaste wl-paste xclip)"
assert_eq "linux+wayland prefers wl-paste"           "wl-paste" "$(backend_for Linux wayland-0 wl-paste xclip xsel)"
assert_eq "linux without wayland skips wl-paste"     "xclip"    "$(backend_for Linux "" wl-paste xclip xsel)"
assert_eq "linux falls back to xsel when no xclip"   "xsel"     "$(backend_for Linux "" xsel)"
assert_eq "wayland session without wl-paste uses xclip" "xclip" "$(backend_for Linux wayland-0 xclip)"
assert_eq "linux with no clipboard tool reports none" "none"    "$(backend_for Linux "" )"

# Without a backend the text path must fail quietly: no stderr into the pane.
nb="$(mktemp -d)"
err="$(env -i PATH="$nb:/usr/bin:/bin" HOME="$HOME" PASTE_OS=Linux TMUX_BIN="$STUB" \
  CLIPBOARD_INFO='' "$AVAL" tmux paste-router 2>&1 >/dev/null)"
assert_eq "no clipboard tool: no error output" "" "$err"
rm -rf "$nb"

# Linux image detection comes from the advertised MIME types.
img_route() { # CLIPBOARD_INFO substitutes the probe, as on macOS
  CLIPBOARD_INFO="$1" PASTE_OS=Linux ROUTE_ONLY=1 "$AVAL" tmux paste-router 2>/dev/null; }
assert_eq "linux: image/png alone routes to image"   "image" "$(img_route 'image/png')"
assert_eq "linux: text/plain wins over image/png"    "text"  "$(img_route 'text/plain;charset=utf-8
image/png')"
assert_eq "linux: UTF8_STRING wins over image"       "text"  "$(img_route 'UTF8_STRING
image/png')"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
