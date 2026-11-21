package tmux

import (
	"fmt"
	"regexp"
	"strings"
)

// Route patterns. Plain text wins whenever it is present: copying from a
// browser or a word processor frequently carries a TIFF rendering next to the
// text, and pasting the text is both the common intent and the cheaper mistake
// — 0x16 is quoted-insert in zsh, so guessing "image" for ordinary text
// corrupts the prompt line. Only a clipboard with no text at all (a
// screenshot) takes the image path.
var (
	darwinText  = regexp.MustCompile(`class utxt`)
	darwinImage = regexp.MustCompile(`class PNGf|TIFF picture|GIF picture|JPEG picture|class BMP`)
	linuxText   = regexp.MustCompile(`(?m)^(text/|UTF8_STRING|STRING|TEXT)`)
	linuxImage  = regexp.MustCompile(`(?m)^image/`)
)

func route(info string, darwin bool) string {
	textRe, imageRe := linuxText, linuxImage
	if darwin {
		textRe, imageRe = darwinText, darwinImage
	}
	switch {
	case textRe.MatchString(info):
		return "text"
	case imageRe.MatchString(info):
		return "image"
	}
	return "text"
}

// pasteRouter decides what cmd+v should mean, based on what is actually in the
// clipboard.
//
// cmd+v alone cannot paste an image. Ghostty's paste_from_clipboard is a
// terminal action and the pty carries text only, so image bytes never fit
// through it. TUIs that accept pasted images (Claude Code among them) watch
// for ctrl+v — the 0x16 byte — and then read the system clipboard natively.
//
// The branch cannot key off the focused app: a Claude Code pane reports its
// version string as its command, which changes with every update. The
// clipboard's own contents are the stable signal.
//
// Ghostty sends a private sequence for cmd+v which tmux picks up as a user key
// and routes here; 0x16 is never bound, so ctrl+v stays free for terminal editors.
//
// CLIPBOARD_INFO substitutes the probe and ROUTE_ONLY=1 prints the decision
// without acting, which is how the tests inspect routing without a clipboard.
func pasteRouter(env Env) int {
	// tmux runs this through run-shell, which inherits launchd's PATH. A bare
	// `tmux` is NOT found there, so it is located by absolute path. TMUX_BIN
	// overrides it, as the tests do.
	tmuxPath := ""
	if v := env.Getenv("TMUX_BIN"); v != "" && env.IsExec(v) {
		tmuxPath = v
	} else {
		tmuxPath = findTmux(env)
	}

	if env.Getenv("RESOLVE_ONLY") == "1" {
		if tmuxPath != "" {
			fmt.Fprintln(env.Stdout, tmuxPath)
		}
		return 0
	}

	// PASTE_OS is a test seam; otherwise the platform.
	osName := env.Getenv("PASTE_OS")
	if osName == "" {
		osName = map[string]string{"darwin": "Darwin", "linux": "Linux"}[env.GOOS]
		if osName == "" {
			osName = env.GOOS
		}
	}
	darwin := osName == "Darwin"

	backend := pickBackend(env, darwin)
	if env.Getenv("CLIPBOARD_BACKEND_ONLY") == "1" {
		fmt.Fprintln(env.Stdout, backend)
		return 0
	}

	var info string
	if v, ok := env.LookupEnv("CLIPBOARD_INFO"); ok {
		info = v
	} else if darwin {
		osa, err := env.LookPath("osascript")
		if err != nil || osa == "" {
			osa = "/usr/bin/osascript"
		}
		info, _ = env.Output(osa, "-e", "clipboard info")
	} else {
		info = clipTypes(env, backend)
	}

	r := route(info, darwin)
	if env.Getenv("ROUTE_ONLY") == "1" {
		fmt.Fprintln(env.Stdout, r)
		return 0
	}

	if tmuxPath == "" {
		fmt.Fprintln(env.Stderr, "paste-router: tmux not found")
		return 127
	}
	tm := env.NewRunner(tmuxPath, "")

	// run-shell exports the pane that invoked the binding; fall back to the
	// active one so the router still works when called by hand.
	var target []string
	if pane := env.Getenv("TMUX_PANE"); pane != "" {
		target = []string{"-t", pane}
	}

	if r == "image" {
		// Hand the byte to the application and let it read the clipboard itself.
		tm.Run(append(append([]string{"send-keys"}, target...), "0x16")...)
		return 0
	}

	// paste-buffer honours bracketed paste, which a raw send-keys would not. No
	// clipboard tool (bare Linux box): do nothing, quietly, rather than paste an
	// empty buffer over the user's tmux buffer or spam the pane with errors.
	if backend == "none" {
		return 0
	}
	text := clipRead(env, backend)
	tm.Query("set-buffer", "--", trim(text))
	tm.Run(append(append([]string{"paste-buffer"}, target...), "-p")...)
	return 0
}

// pickBackend names the tool that reads the clipboard. macOS has pbpaste in
// /usr/bin. On Linux the session decides: Wayland wants wl-paste, because
// xclip under XWayland only sees clipboards owned by X clients; X11 uses
// xclip, then xsel. Nothing installed is "none".
func pickBackend(env Env, darwin bool) string {
	has := func(n string) bool { p, err := env.LookPath(n); return err == nil && p != "" }
	if darwin {
		if has("pbpaste") {
			return "pbpaste"
		}
		return "none"
	}
	if env.Getenv("WAYLAND_DISPLAY") != "" && has("wl-paste") {
		return "wl-paste"
	}
	for _, n := range []string{"xclip", "xsel"} {
		if has(n) {
			return n
		}
	}
	return "none"
}

func clipRead(env Env, backend string) string {
	p, _ := env.LookPath(backend)
	var args []string
	switch backend {
	case "wl-paste":
		args = []string{"--no-newline"}
	case "xclip":
		args = []string{"-selection", "clipboard", "-o"}
	case "xsel":
		args = []string{"-b", "-o"}
	}
	out, _ := env.Output(p, args...)
	return out
}

// clipTypes lists the flavours the clipboard advertises on Linux. xsel cannot
// list targets, so with xsel only the text route is possible, which is also
// the safe default.
func clipTypes(env Env, backend string) string {
	p, _ := env.LookPath(backend)
	var args []string
	switch backend {
	case "wl-paste":
		args = []string{"--list-types"}
	case "xclip":
		args = []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}
	default:
		return ""
	}
	out, _ := env.Output(p, args...)
	return strings.TrimSpace(out)
}
