package tests

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// Hygiene checks on the versioned shell and git configuration: the rot that
// accumulates silently (installers appending the same PATH line on every run,
// paths from someone else's machine, tools installed but never wired up).

var rePathExport = regexp.MustCompile(`export PATH="[^"]*"`)

func TestShellHygieneNoPathExportAppearsTwice(t *testing.T) {
	seen := map[string]int{}
	for _, l := range rePathExport.FindAllString(read(t, "home/zshrc"), -1) {
		seen[l]++
	}
	worst := 0
	for l, n := range seen {
		if n > 1 {
			t.Errorf("PATH export repeated %d times: %s", n, l)
		}
		if n > worst {
			worst = n
		}
	}
	eq(t, "the most repeated PATH line appears exactly once", 1, worst)
}

func TestShellHygieneNoDirectoryRepeatedWithinOnePathExport(t *testing.T) {
	for _, l := range rePathExport.FindAllString(read(t, "home/zshrc"), -1) {
		body := strings.TrimSuffix(strings.TrimPrefix(l, `export PATH="`), `"`)
		seen := map[string]bool{}
		for _, d := range strings.Split(body, ":") {
			if d == "" || d == "$PATH" {
				continue
			}
			if seen[d] {
				t.Errorf("%q listed twice in %s", d, l)
			}
			seen[d] = true
		}
	}
}

// The original bash check used a look-around that ripgrep does not support, so
// it failed to run and the assertion passed vacuously. Done by hand here.
func TestShellHygieneNoPathBelongingToAnotherUser(t *testing.T) {
	zshrc := read(t, "home/zshrc")
	reHome := regexp.MustCompile(`/(?:home|Users)/([a-z][a-z0-9_-]+)`)
	for _, m := range reHome.FindAllStringSubmatch(zshrc, -1) {
		if m[1] != "alvaroofernaandez" && m[1] != "linuxbrew" { // linuxbrew is a prefix, not a user
			t.Errorf("home directory of another user: %s", m[0])
		}
	}
	for _, line := range strings.Split(zshrc, "\n") {
		if strings.Contains(line, "PROJECT_PATHS=") && reHome.MatchString(line) {
			t.Errorf("orphan variable pointing at a foreign path: %s", line)
		}
	}
}

// delta only affects git if git is told to use it.
func TestShellHygieneDeltaIsWiredIntoGit(t *testing.T) {
	needTool(t, "delta")
	cfg := read(t, "home/gitconfig")
	mustHave(t, `pager\s*=\s*delta`, cfg, "git uses delta as its pager")
	mustHave(t, `diffFilter\s*=\s*delta`, cfg, "interactive diffs go through delta")
}

func TestShellHygieneZshrcStillParsesAsZsh(t *testing.T) {
	testenv.SkipWindows(t, "drives zsh")
	needTool(t, "zsh")
	r := run(repo, nil, "", "zsh", "-n", p("home", "zshrc"))
	eq(t, "zsh -n home/zshrc\n"+r.Out(), 0, r.Code)
}

func TestShellHygieneGitconfigStillParses(t *testing.T) {
	needTool(t, "git")
	r := run(repo, nil, "", "git", "config", "--file", p("home", "gitconfig"), "--list")
	eq(t, "git config --list on home/gitconfig\n"+r.Out(), 0, r.Code)
}

// The file declaring each entry once is not enough: nested shells prepend to an
// inherited PATH that already contains them.
func TestShellHygieneZshKeepsPathUnique(t *testing.T) {
	testenv.SkipWindows(t, "drives zsh")
	mustHave(t, `typeset -U path`, read(t, "home/zshrc"), "zsh is told to keep PATH unique")
}

func TestShellHygieneInteractiveShellHasNoRepeatedPathEntry(t *testing.T) {
	testenv.SkipWindows(t, "drives zsh")
	needTool(t, "zsh")
	// Start with a PATH that deliberately repeats an entry: typeset -U must fix it.
	env := withEnv(zshEnv(t), "PATH=/usr/bin:/bin:/usr/bin:"+os.Getenv("PATH"))
	r := run(t.TempDir(), env, "", "zsh", "-ic", "echo $PATH")
	seen := map[string]bool{}
	var dupes []string
	for _, d := range strings.Split(strings.TrimSpace(r.Stdout), ":") {
		if d != "" && seen[d] {
			dupes = append(dupes, d)
		}
		seen[d] = true
	}
	sort.Strings(dupes)
	eq(t, "repeated PATH entries in an interactive shell", "", strings.Join(dupes, " "))
}

// zshEnv is the environment of an interactive zsh that loads the repo's zshrc
// the way the installer would, but inside a throwaway $HOME.
func zshEnv(t testing.TB) []string {
	t.Helper()
	home := t.TempDir()
	for src, dst := range map[string]string{"zshrc": ".zshrc", "p10k.zsh": ".p10k.zsh", "zprofile": ".zprofile"} {
		if err := os.Symlink(p("home", src), home+"/"+dst); err != nil {
			t.Fatal(err)
		}
	}
	return isoEnv(home, "ZDOTDIR="+home)
}
