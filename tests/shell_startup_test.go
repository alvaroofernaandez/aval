package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// Interactive zsh startup stays fast without losing behaviour. Measured
// 2026-11-21 (M4): 113.7 ms per `zsh -i -c exit` against 2.7 ms for a bare zsh;
// the cost was oh-my-zsh and five `eval "$(tool init)"` calls on every start.

const startupBudget = 80 * time.Millisecond

func TestShellStartupStructure(t *testing.T) {
	testenv.SkipWindows(t, "drives zsh and the zsh startup files")
	zshrc := read(t, "home/zshrc")
	eq(t, "oh-my-zsh is not loaded", 0, count(`oh-my-zsh\.sh`, zshrc))
	mustNotHave(t, `eval "\$\(|source <\(`, zshrc, "no tool init is evaluated from a subprocess at startup")
	mustNotHave(t, `\$\(uname\)|\$\(go env`, zshrc, "no subprocess for facts zsh already knows")
	for _, l := range strings.Split(zshrc, "\n") {
		if has(`/Users/[a-z]|/home/[a-z]`, l) && !strings.Contains(l, "linuxbrew") {
			t.Errorf("hard-coded home directory: %s", l)
		}
	}
	mustHave(t, `^export FZF_ALT_C_COMMAND=.*--exclude`, zshrc, "fzf's directory widget command uses the right variable")
	eq(t, "no hook calls a node manager that is not installed (nvm use)", 0, count(`nvm use`, zshrc))
}

// zshProbe runs a script in an interactive zsh and returns its stdout.
func zshProbe(t *testing.T, env []string, script string) string {
	t.Helper()
	r := run(t.TempDir(), env, "", "zsh", "-i", "-c", script)
	return r.Stdout
}

func TestShellStartupBehaviourSurvives(t *testing.T) {
	testenv.SkipWindows(t, "drives zsh and the zsh startup files")
	needTool(t, "zsh")
	env := zshEnv(t)
	probe := zshProbe(t, env, `
  local w k
  w=$(whence -w z); print "z=${w##* }"
  k=$(bindkey "^R"); print "atuin=${k##* }"
  k=$(bindkey "^T"); print "fzf=${k##* }"
  print "hist=$HISTSIZE share=$([[ -o sharehistory ]] && print on || print off)"
  print "prompt=$(( ${+functions[prompt_powerlevel10k_setup]} || ${+functions[_p9k_precmd]} ))"
`)
	line := func(prefix string) string {
		for _, l := range strings.Split(probe, "\n") {
			if strings.HasPrefix(l, prefix) {
				return l
			}
		}
		return ""
	}
	t.Run("zoxide z is defined", func(t *testing.T) {
		needTool(t, "zoxide")
		eq(t, "z", "z=function", line("z="))
	})
	t.Run("atuin owns Ctrl-R", func(t *testing.T) {
		needTool(t, "atuin")
		isTrue(t, "atuin owns Ctrl-R: "+line("atuin="), strings.HasPrefix(line("atuin="), "atuin=atuin"))
	})
	t.Run("fzf owns Ctrl-T", func(t *testing.T) {
		needTool(t, "fzf")
		isTrue(t, "fzf owns Ctrl-T: "+line("fzf="), strings.HasPrefix(line("fzf="), "fzf=fzf"))
	})
	t.Run("history is large and shared", func(t *testing.T) {
		mustHave(t, `^hist=(5[0-9]{4}|[6-9][0-9]{4}|[0-9]{6,}) share=on`, probe, "history is large and shared")
	})
	t.Run("powerlevel10k prompt is loaded", func(t *testing.T) {
		if !exists("/opt/homebrew/share/powerlevel10k") && !exists("/usr/local/share/powerlevel10k") &&
			!exists("/home/linuxbrew/.linuxbrew/share/powerlevel10k") {
			t.Skip("powerlevel10k not installed")
		}
		eq(t, "prompt", "prompt=1", line("prompt="))
	})
}

// A cache older than its binary (what `brew upgrade` produces) is regenerated,
// so an upgrade is never silently masked by a stale init script.
func TestShellStartupStaleInitCacheIsRegenerated(t *testing.T) {
	testenv.SkipWindows(t, "drives zsh and the zsh startup files")
	needTool(t, "zsh", "zoxide")
	env := zshEnv(t)
	cacheHome := ""
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "XDG_CACHE_HOME="); ok {
			cacheHome = v
		}
	}
	cache := filepath.Join(cacheHome, "zsh-init", "zoxide.zsh")
	run(t.TempDir(), env, "", "zsh", "-i", "-c", "exit") // warm: writes the cache
	if !exists(cache) {
		t.Fatalf("first start did not write %s", cache)
	}
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cache, old, old); err != nil {
		t.Fatal(err)
	}
	run(t.TempDir(), env, "", "zsh", "-i", "-c", "exit")
	bin, _ := exec.LookPath("zoxide")
	bst, _ := os.Stat(bin) // follow symlinks like `-nt`
	cst, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	isTrue(t, "a stale init cache is regenerated after an upgrade", cst.ModTime().After(bst.ModTime()))
}

func TestShellStartupBudget(t *testing.T) {
	testenv.SkipWindows(t, "drives zsh and the zsh startup files")
	needTool(t, "zsh")
	slow(t)
	env := zshEnv(t)
	for i := 0; i < 3; i++ { // warmup, and fills the init cache
		run(t.TempDir(), env, "", "zsh", "-i", "-c", "exit")
	}
	var d []time.Duration
	for i := 0; i < 20; i++ {
		start := time.Now()
		run(t.TempDir(), env, "", "zsh", "-i", "-c", "exit")
		d = append(d, time.Since(start))
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	// The gate is the fastest start: load only adds time, a regression slows
	// every start (#125). The median is printed to show how loaded the run was.
	if fastest, median := d[0], d[len(d)/2]; fastest >= startupBudget {
		t.Errorf("interactive startup fastest %v (median %v), budget %v", fastest, median, startupBudget)
	}
}
