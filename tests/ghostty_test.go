package tests

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Ghostty's split navigation keybindings, and the launch command in its config.
//
// The 8 panels in a Ghostty window are Ghostty SPLITS, each running its own
// tmux session, so moving between them is Ghostty's job: tmux never sees those
// keys. Ghostty has no "go to split N" (goto_split takes directions and
// previous/next only), so navigation is directional plus cycling, and the
// bindings must not collide with macOS shortcuts or vim-tmux-navigator.

func ghosttyConf(t *testing.T) string { return read(t, "config/ghostty/config") }

func TestGhosttyHomeRowKeysMoveBetweenSplits(t *testing.T) {
	conf := ghosttyConf(t)
	for _, dir := range []string{"left", "down", "up", "right"} {
		mustHave(t, `^keybind = .*[hjkl]=goto_split:`+dir, conf, "a home-row key moves "+dir)
	}
}

func TestGhosttyKeysCycleThroughSplits(t *testing.T) {
	conf := ghosttyConf(t)
	for _, way := range []string{"previous", "next"} {
		mustHave(t, `^keybind = .*=goto_split:`+way, conf, "a key cycles to the "+way+" split")
	}
}

// cmd+h hides the app, cmd+m minimises, cmd+q quits: binding them inside a
// terminal takes them away everywhere else in the window.
func TestGhosttyDoesNotShadowMacOSSystemShortcuts(t *testing.T) {
	conf := ghosttyConf(t)
	for _, k := range []string{`cmd\+h=`, `cmd\+m=`, `cmd\+q=`} {
		mustNotHave(t, `^keybind = `+k, conf, "binds bare "+strings.TrimSuffix(k, "="))
	}
}

// Ghostty receives keys BEFORE tmux, so binding ctrl+h/j/k/l here would break
// pane navigation inside every tmux session.
func TestGhosttyLeavesCtrlHjklToTmuxAndVim(t *testing.T) {
	mustNotHave(t, `^keybind = ctrl\+[hjkl]=`, ghosttyConf(t), "ctrl+hjkl is bound")
}

// On the "Spanish - ISO" layout Option composes the whole programming
// character set (@ | # [ ] { } \ ~); turning it into Alt makes them untypable.
func TestGhosttyOptionStaysACompositionModifier(t *testing.T) {
	mustNotHave(t, `^macos-option-as-alt = (true|left|right)`, ghosttyConf(t), "Option is turned into Alt")
}

func TestGhosttyNoKeyIsBoundTwice(t *testing.T) {
	seen := map[string]int{}
	for _, m := range regexp.MustCompile(`(?m)^keybind = [^=]+`).FindAllString(ghosttyConf(t), -1) {
		seen[m]++
	}
	var dupes []string
	for k, n := range seen {
		if n > 1 {
			dupes = append(dupes, k)
		}
	}
	sort.Strings(dupes)
	eq(t, "keys bound more than once", "", strings.Join(dupes, "; "))
}

func TestGhosttyAcceptsTheConfig(t *testing.T) {
	bin := "/Applications/Ghostty.app/Contents/MacOS/ghostty"
	if _, err := os.Stat(bin); err != nil {
		var lerr error
		if bin, lerr = exec.LookPath("ghostty"); lerr != nil {
			t.Skip("ghostty binary not found")
		}
	}
	r := run(repo, nil, "", bin, "+validate-config", "--config-file="+p("config", "ghostty", "config"))
	eq(t, "ghostty +validate-config output", "", strings.TrimSpace(r.Out()))
}

// Ghostty starts its command through `/usr/bin/login -flp <user> /bin/bash`,
// which inherits launchd's minimal PATH: a bare `tmux` is not found and the
// window dies with "exec: tmux: not found". The config must not reintroduce one.
func TestGhosttyLaunchCommandIsAnchoredAndNotPathDependent(t *testing.T) {
	cmds := regexp.MustCompile(`(?m)^\s*command\s*=.*$`).FindAllString(ghosttyConf(t), -1)
	isTrue(t, "ghostty config sets a command", len(cmds) > 0)
	for _, c := range cmds {
		mustHave(t, `=\s*(shell:)?(/|\$HOME/)`, c, "ghostty command uses an absolute or $HOME-anchored path")
		mustNotHave(t, `=\s*tmux\b`, c, "ghostty command relies on PATH for tmux")
	}
}
