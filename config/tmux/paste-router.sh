#!/bin/sh
# Decide what cmd+v should mean, based on what is actually in the clipboard.
#
# cmd+v alone cannot paste an image. Ghostty's paste_from_clipboard is a
# terminal action and the pty carries text only, so image bytes never fit
# through it. TUIs that accept pasted images (Claude Code among them) watch for
# ctrl+v — the 0x16 byte — and then read the system clipboard natively, going
# around the pty entirely.
#
# That makes one key mean two things, and the branch cannot key off the focused
# app: a Claude Code pane reports `2.1.232` as its command, a version string
# that changes with every update. The clipboard's own contents are the stable
# signal, so that is what gets probed here.
#
# Ghostty sends a private sequence for cmd+v which tmux picks up as a user key
# and routes here; 0x16 is never bound, so ctrl+v stays free for Neovim's
# visual-block mode.
#
# CLIPBOARD_INFO substitutes the probe and ROUTE_ONLY=1 prints the decision
# without acting, which is how the tests inspect routing without a clipboard —
# the same escape hatch launch.sh offers through LAUNCH_RESOLVE_ONLY.

# tmux runs this through run-shell, which inherits launchd's PATH
# (/usr/bin:/bin:/usr/sbin:/sbin) rather than the shell's. A bare `tmux` is NOT
# found there and the binding dies with "returned 127" — the same trap launch.sh
# resolves, for the same reason. osascript and pbpaste are in /usr/bin, so only
# tmux needs locating. TMUX_BIN overrides it, as the tests do.
if [ -n "${TMUX_BIN:-}" ] && [ -x "${TMUX_BIN:-}" ]; then
  TMUX_RESOLVED="$TMUX_BIN"
else
  for candidate in \
    /opt/homebrew/bin/tmux \
    /usr/local/bin/tmux \
    "$HOME/.nix-profile/bin/tmux" \
    /usr/bin/tmux
  do
    if [ -x "$candidate" ]; then
      TMUX_RESOLVED="$candidate"
      break
    fi
  done

  if [ -z "${TMUX_RESOLVED:-}" ]; then
    found="$(command -v tmux 2>/dev/null)"
    [ -n "$found" ] && [ -x "$found" ] && TMUX_RESOLVED="$found"
  fi
fi

if [ "${RESOLVE_ONLY:-}" = "1" ]; then
  [ -n "${TMUX_RESOLVED:-}" ] && printf '%s\n' "$TMUX_RESOLVED"
  exit 0
fi

# Platform and clipboard backend. PASTE_OS is a test seam; otherwise uname.
OS="${PASTE_OS:-$(uname -s 2>/dev/null)}"

# Which tool reads the clipboard. macOS has pbpaste in /usr/bin. On Linux the
# session decides: Wayland (WAYLAND_DISPLAY set) wants wl-paste, because xclip
# under XWayland only sees clipboards owned by X clients; X11 uses xclip, then
# xsel. Nothing installed is reported as "none" and handled quietly.
pick_backend() {
  if [ "$OS" = "Darwin" ]; then
    command -v pbpaste >/dev/null 2>&1 && { echo pbpaste; return; }
    echo none; return
  fi
  if [ -n "${WAYLAND_DISPLAY:-}" ] && command -v wl-paste >/dev/null 2>&1; then
    echo wl-paste; return
  fi
  command -v xclip >/dev/null 2>&1 && { echo xclip; return; }
  command -v xsel  >/dev/null 2>&1 && { echo xsel;  return; }
  echo none
}
BACKEND="$(pick_backend)"

if [ "${CLIPBOARD_BACKEND_ONLY:-}" = "1" ]; then
  printf '%s\n' "$BACKEND"
  exit 0
fi

clip_read() {
  case "$BACKEND" in
    pbpaste)  pbpaste ;;
    wl-paste) wl-paste --no-newline ;;
    xclip)    xclip -selection clipboard -o ;;
    xsel)     xsel -b -o ;;
  esac
}

# The flavours the clipboard advertises. macOS: osascript's `clipboard info`.
# Linux: the MIME/target list. xsel cannot list targets, so with xsel only the
# text route is possible, which is also the safe default.
clip_types() {
  case "$BACKEND" in
    wl-paste) wl-paste --list-types ;;
    xclip)    xclip -selection clipboard -t TARGETS -o ;;
    *)        : ;;
  esac
}

if [ -n "${CLIPBOARD_INFO+x}" ]; then
  info="$CLIPBOARD_INFO"
elif [ "$OS" = "Darwin" ]; then
  info="$(osascript -e 'clipboard info' 2>/dev/null)"
else
  info="$(clip_types 2>/dev/null)"
fi

# Plain text wins whenever it is present. Copying from a browser or a word
# processor frequently carries a TIFF rendering next to the text, and pasting
# the text is both the common intent and the cheaper mistake to make: 0x16 is
# quoted-insert in zsh, so guessing "image" for ordinary text corrupts the
# prompt line. Only a clipboard with no text at all — a screenshot — takes the
# image path.
if [ "$OS" = "Darwin" ]; then
  text_re='class utxt'
  image_re='class PNGf|TIFF picture|GIF picture|JPEG picture|class BMP'
else
  text_re='^(text/|UTF8_STRING|STRING|TEXT)'
  image_re='^image/'
fi
if printf '%s' "$info" | grep -qE "$text_re"; then
  route=text
elif printf '%s' "$info" | grep -qE "$image_re"; then
  route=image
else
  route=text
fi

if [ "${ROUTE_ONLY:-}" = "1" ]; then
  printf '%s\n' "$route"
  exit 0
fi

# run-shell exports the pane that invoked the binding; fall back to the active
# one so the router still works when called by hand.
target="${TMUX_PANE:-}"

if [ "$route" = "image" ]; then
  # Hand the byte to the application and let it read the clipboard itself.
  "$TMUX_RESOLVED" send-keys ${target:+-t "$target"} 0x16
else
  # paste-buffer honours bracketed paste, which a raw send-keys would not.
  # No clipboard tool (bare Linux box): do nothing, quietly, rather than paste
  # an empty buffer over the user's tmux buffer or spam the pane with errors.
  [ "$BACKEND" = "none" ] && exit 0
  "$TMUX_RESOLVED" set-buffer -- "$(clip_read 2>/dev/null)" 2>/dev/null
  "$TMUX_RESOLVED" paste-buffer ${target:+-t "$target"} -p
fi
