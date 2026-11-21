//go:build !windows

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// confServer boots a real tmux server on the repository's home/tmux.conf and
// waits for it to finish loading. The assertions below are about the EFFECTIVE
// values after the whole config has run, TPM plugins included: grepping the text
// of tmux.conf would miss the actual failure mode, since the kanagawa theme
// assigns during plugin load and overwrites anything set earlier in the file.
func confServer(t *testing.T) *srv {
	t.Helper()
	s := newSrv(t)
	if _, err := os.Stat(filepath.Join(s.home, ".tmux.conf")); err != nil {
		t.Fatalf("home/tmux.conf not found under %s", configRoot)
	}
	s.start("", "-f", filepath.Join(s.home, ".tmux.conf"))
	// The last thing the file sets is the button's click binding; once it is
	// there the file has been read to the end.
	if _, ok := waitUntil(settle, "MouseDown1Status", func() string { return s.q("list-keys", "-T", "root") }); !ok {
		t.Fatal("tmux.conf did not finish loading")
	}
	return s
}

func TestTmuxConf(t *testing.T) {
	s := confServer(t)
	root := s.q("list-keys", "-T", "root")
	prefix := s.q("list-keys", "-T", "prefix")
	has := func(re, text string) bool { return regexp.MustCompile(re).MatchString(text) }

	// Context that drove these: on a "Spanish - ISO" macOS layout Option+E is
	// the DEAD KEY for the acute accent. The OS consumes it to compose a/e/i, so
	// M-e never reaches tmux. No binding may collide with a dead key, and at
	// least one route must work without Option at all.
	t.Run("keybindings", func(t *testing.T) {
		// a route that never goes through Option
		eq(t, "prefix+e toggles the sidebar", true, has(`bind-key\s+-T prefix\s+e\b.*sidebar-toggle`, prefix))

		// a one-press route that avoids dead keys
		alt := regexp.MustCompile(`M-[a-z]\s+run-shell.*sidebar-toggle`).FindString(root)
		altKey := strings.TrimPrefix(regexp.MustCompile(`M-[a-z]`).FindString(alt), "M-")
		eq(t, "a single-press Alt binding exists", true, altKey != "")
		// Option+<key> dead keys on Spanish ISO: acute, circumflex, diaeresis, tilde.
		eq(t, "the Alt binding is not a Spanish-ISO dead key", false, strings.Contains("eiun", altKey) && altKey != "")

		// the retired binding must be gone
		eq(t, "M-e is no longer bound to the sidebar", false, has(`M-e\s+run-shell.*sidebar-toggle`, root))
		eq(t, "does not clash with the M-g scratch popup", true, "M-"+altKey != "M-g")

		// Three routes by design: M-t, prefix+e, and the clickable status button.
		eq(t, "all three routes run the same toggle", 3, strings.Count(root+"\n"+prefix, "aval tmux sidebar-toggle"))
		eq(t, "the mouse route exists as a keyboard-independent fallback", true, has(`MouseDown1Status.*sidebar-toggle`, root))

		// closing an opened file
		eq(t, "a single-press key closes an opened file", true, has(`M-w\s+run-shell.*close-file`, root))
		eq(t, "prefix+k closes an opened file without Option", true, has(`bind-key\s+-T prefix\s+k\b.*close-file`, prefix))
		eq(t, "the close key is not a Spanish-ISO dead key", false, has(`M-[eiun]\s+run-shell.*close-file`, root))

		// `mouse on` means tmux owns the drag, so Ghostty has no selection of its
		// own and cmd+c comes back empty unless tmux exports the copy itself. The
		// default `external` only relays OSC 52 from other programs, never tmux's
		// own copies, which left mouse selections stranded in the paste buffer.
		eq(t, "tmux exports its own copies via OSC 52", "on", s.q("show", "-gv", "set-clipboard"))
	})

	// yazi sends DA1/DSR probes at startup to detect terminal features. With
	// allow-passthrough off, tmux blocks them, yazi waits for the timeout and
	// prints "Terminal response timeout" over the panel for several seconds.
	t.Run("passthrough for yazi", func(t *testing.T) {
		eq(t, "allow-passthrough is enabled for yazi's probes", "on", s.q("show", "-gv", "allow-passthrough"))
	})

	// The clickable button exists because the keyboard route is fragile here:
	// on a Spanish ISO layout Option+E is a dead key, and Option handling
	// depends on macos-option-as-alt. A mouse target depends on none of that.
	t.Run("sidebar button", func(t *testing.T) {
		left := s.q("show", "-gv", "status-left")
		wantText(t, "status-left declares a clickable range", "range=user|sidebar", left)
		wantText(t, "the range is closed with norange", "norange", left)
		wantText(t, "the button carries a readable label", "FILES", left)

		wantText(t, "a status click is bound", "MouseDown1Status", root)
		eq(t, "the click handler checks which range was hit", true, has(`MouseDown1Status.*mouse_status_range`, root))
		eq(t, "clicking the button runs the toggle", true, has(`MouseDown1Status.*sidebar-toggle`, root))
		eq(t, "mouse support is enabled", "on", s.q("show", "-gv", "mouse"))

		// The default MouseDown1Status action selects a window. Overriding it
		// blindly would break clicking on the window list.
		eq(t, "clicking outside the button still selects a window", true, has(`MouseDown1Status.*select-window`, root))

		// DESIGN.md: a control must not look like a metric. Metric segments are
		// solid blocks (bg=colourNNN). The button is not.
		button := regexp.MustCompile(`range=user\|sidebar.*?norange`).FindString(left)
		eq(t, "the button is found in status-left", true, button != "")
		eq(t, "the button is not painted as a solid metric block", false, has(`bg=colour[0-9]`, button))
		eq(t, "the button renders in plain ASCII", true, isASCII(button))
	})

	// DESIGN.md section 2: the bar inherits the terminal background.
	t.Run("status style", func(t *testing.T) {
		// TPM loads plugins asynchronously; styles settle after it finishes, so
		// the value is read once it has stopped changing.
		style := stable(settle, 600*time.Millisecond, func() string { return s.q("show", "-gv", "status-style") })

		wantText(t, "status background is default (inherits the terminal)", "bg=default", style)
		eq(t, "status background is not tmux's green default", false, strings.Contains(style, "bg=green"))
		eq(t, "status foreground is set explicitly", true, strings.Contains(style, "fg="))

		for _, opt := range []string{"status-left-style", "status-right-style"} {
			eq(t, opt+" does not paint a green band", false, strings.Contains(s.q("show", "-gv", opt), "bg=green"))
		}

		// window list stays readable on a transparent bar
		cur := s.q("show", "-gv", "window-status-current-style")
		eq(t, "current window is distinguishable", true, cur != "" && cur != "default")

		// the segments still render
		wantText(t, "status-right still runs the segment bar", "tmux statusbar", s.q("show", "-gv", "status-right"))
	})
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// The sidebar-specific yazi config: static files, plus yazi itself when present.
//
// The sidebar runs yazi with its own YAZI_CONFIG_HOME so it can drop the parent
// and preview columns, which do not fit in ~60 cols. yazi does NOT merge that
// directory with ~/.config/yazi, so the opener and show_hidden must be repeated
// there, and are therefore asserted to stay identical to the main config.
// ---------------------------------------------------------------------------

func yaziPaths() (mainCfg, sidebarHome, sidebarCfg string) {
	sidebarHome = filepath.Join(configRoot, "config", "yazi-sidebar")
	return filepath.Join(configRoot, "config", "yazi", "yazi.toml"), sidebarHome, filepath.Join(sidebarHome, "yazi.toml")
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tomlValue is the value of a simple `key = value` line.
func tomlValue(text, key string) string {
	m := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*(.*?)\s*$`).FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// openerLines are the lines wiring yazi's opener to the helper.
func openerLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "aval tmux open-file") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func TestYaziSidebarConfig(t *testing.T) {
	mainCfg, sidebarHome, sidebarCfg := yaziPaths()

	t.Run("sidebar config exists", func(t *testing.T) {
		if _, err := os.Stat(sidebarCfg); err != nil {
			t.Fatalf("sidebar config missing: %v", err)
		}
	})
	text := readText(t, sidebarCfg)

	// yazi moved this introspection between releases: newer builds answer with
	// `ya env` and reduce `yazi --debug` to a deprecation notice, older ones only
	// have `--debug`. Both are tried, newest first, because which yazi is
	// installed is the machine's business and not this repo's. A build that
	// answers neither is a skip, not a failure: asserting against an empty string
	// would claim the config is broken when only how yazi reports paths changed.
	t.Run("yazi accepts the config", func(t *testing.T) {
		if testing.Short() {
			t.Skip("runs yazi (skipped under -short)")
		}
		skipUnlessYazi(t)
		env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "YAZI_CONFIG_HOME=" + sidebarHome}
		// Bounded and without a controlling terminal: `yazi --debug` probes
		// /dev/tty and, in a headless pty, would otherwise block indefinitely.
		debug := probe(t, env, "ya", "env")
		if !strings.Contains(strings.ToLower(debug), "yazi") {
			debug = probe(t, env, "yazi", "--debug")
		}
		if debug == "" || strings.Contains(strings.ToLower(debug), "deprecated") {
			t.Skip("this yazi reports no config paths")
		}
		wantText(t, "yazi loads the sidebar config", sidebarHome, debug)
		eq(t, "yazi reports no config error", false, regexp.MustCompile(`(?i)invalid|failed to parse|error.*toml`).MatchString(debug))
	})

	// layout: tree only
	t.Run("layout shows the tree only", func(t *testing.T) {
		ratio := tomlValue(text, "ratio")
		wantText(t, "ratio hides the parent column", "0,", ratio)
		eq(t, "ratio hides the preview column", true, regexp.MustCompile(`0\s*\]$`).MatchString(ratio))
		cols := strings.Fields(regexp.MustCompile(`[^0-9]`).ReplaceAllString(ratio, " "))
		n := 0
		if len(cols) > 1 {
			n, _ = strconv.Atoi(cols[1])
		}
		eq(t, "ratio keeps the current column visible", true, n > 0)
	})

	// no divergence from the main config
	t.Run("opener and show_hidden match the main config", func(t *testing.T) {
		main := readText(t, mainCfg)
		// The bash version compared lines matching a stale name ("open-in-work-pane")
		// that neither file contains, so it compared two empty results. This
		// compares the real opener, and insists it is present.
		want := openerLines(main)
		eq(t, "the main config wires the opener", true, len(want) > 0)
		eq(t, "opener matches the main config", strings.Join(want, "\n"), strings.Join(openerLines(text), "\n"))
		eq(t, "show_hidden is set in the main config", true, tomlValue(main, "show_hidden") != "")
		eq(t, "show_hidden matches the main config", tomlValue(main, "show_hidden"), tomlValue(text, "show_hidden"))
	})
}
