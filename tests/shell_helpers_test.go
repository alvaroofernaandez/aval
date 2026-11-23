package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// Shell helpers born from the habits audit (14,391 atuin commands, 2026-11-21):
// `clear` typos, `docker-compose up --build`, `sleep 600` to wait on a job, and
// so on. home/zsh/helpers.zsh stays zsh by design; these tests exec it.

func helpers(t *testing.T) string {
	t.Helper()
	needTool(t, "zsh")
	h := p("home", "zsh", "helpers.zsh")
	isTrue(t, "helpers.zsh exists", exists(h))
	return h
}

func TestShellHelpersAreSourcedByZshrc(t *testing.T) {
	testenv.SkipWindows(t, "tests the zsh helpers")
	mustHave(t, `zsh/helpers\.zsh`, read(t, "home/zshrc"), "zshrc sources the helpers")
}

type zshBox struct {
	t       *testing.T
	root    string
	bin     string
	helpers string
	calls   string
}

// newZshBox gives each test stub binaries that record how they were called
// instead of starting containers.
func newZshBox(t *testing.T) *zshBox {
	t.Helper()
	b := &zshBox{t: t, root: t.TempDir(), helpers: helpers(t)}
	b.bin = filepath.Join(b.root, "bin")
	b.calls = filepath.Join(b.root, "calls")
	for _, c := range []string{"make", "docker", "npm", "git", "npx"} {
		write(t, filepath.Join(b.bin, c), "#!/bin/sh\necho \""+c+" $*\" >> \""+b.calls+"\"\n")
		if err := os.Chmod(filepath.Join(b.bin, c), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

// z runs a script in dir with the stubs first on PATH.
func (b *zshBox) z(dir, script string, stdin string) result {
	env := isoEnv(filepath.Join(b.root, "home"), "PATH="+b.bin+":/usr/bin:/bin")
	return run(dir, env, stdin, "zsh", "-f", "-c", "source '"+b.helpers+"'; "+script)
}

func (b *zshBox) lastCall() string {
	lines := strings.Split(strings.TrimRight(b.read(), "\n"), "\n")
	return lines[len(lines)-1]
}

func (b *zshBox) read() string {
	c, _ := os.ReadFile(b.calls)
	return string(c)
}

func (b *zshBox) reset() { _ = os.WriteFile(b.calls, nil, 0o644) }

func TestShellHelpersStackUpAndDown(t *testing.T) {
	testenv.SkipWindows(t, "tests the zsh helpers")
	b := newZshBox(t)
	mk, dc, np := filepath.Join(b.root, "mk"), filepath.Join(b.root, "dc"), filepath.Join(b.root, "np")
	write(t, mk+"/Makefile", "up-build:\n\techo\n")
	write(t, dc+"/compose.yaml", "services: {}\n")
	write(t, np+"/package.json", `{"scripts":{"dev:stack:up":"x","dev:stack:down":"y"}}`+"\n")

	cases := []struct {
		name, dir, script, stdin, want string
	}{
		{"up runs make up-build when the Makefile has it", mk, "up", "", "make up-build"},
		{"up runs docker compose up --build -d for a compose project", dc, "up", "", "docker compose up --build -d"},
		{"up runs the npm stack script otherwise", np, "up", "", "npm run dev:stack:up"},
		{"down stops compose without deleting volumes", dc, "down", "", "docker compose down"},
		{"down -v asks first and does nothing without a yes", dc, "down -v", "", ""},
		{"down -v deletes volumes after a yes", dc, "down -v", "y\n", "docker compose down -v"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b.reset()
			b.z(c.dir, c.script, c.stdin)
			got := ""
			if s := strings.TrimRight(b.read(), "\n"); s != "" {
				got = b.lastCall()
			}
			eq(t, c.name, c.want, got)
		})
	}
}

func TestShellHelpersWaitforReturnsOnceTheConditionHolds(t *testing.T) {
	testenv.SkipWindows(t, "tests the zsh helpers")
	t.Parallel()
	b := newZshBox(t)
	flag := filepath.Join(b.root, "flag")
	polled := filepath.Join(b.root, "polled")
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The condition records that it ran, so the test knows waitfor is
		// really waiting before it lets the flag appear.
		b.z(b.root, "WAITFOR_INTERVAL=1 WAITFOR_QUIET=1 waitfor 'touch "+polled+"; [[ -f "+flag+" ]]'", "")
	}()
	isTrue(t, "waitfor evaluated its condition", eventually(30*time.Second, func() bool { _, err := os.Stat(polled); return err == nil }))
	select {
	case <-done:
		t.Fatal("waitfor returned before the condition held")
	default:
	}
	write(t, flag, "")
	select {
	case <-done:
	case <-time.After(30 * time.Second): // hard stop, not an assertion on speed
		t.Fatal("waitfor did not return after the condition held")
	}
}

func TestShellHelpersGitAndReactDoctorShortcuts(t *testing.T) {
	testenv.SkipWindows(t, "tests the zsh helpers")
	b := newZshBox(t)
	b.reset()
	b.z(b.root, "gp", "")
	eq(t, "gp pulls with submodules, then shows a short status",
		"git pull --recurse-submodules|git status -sb", strings.Join(strings.Split(strings.TrimRight(b.read(), "\n"), "\n"), "|"))
	b.reset()
	b.z(b.root, "rd", "")
	eq(t, "rd runs react-doctor on the diff", "npx -y react-doctor@latest . --verbose --diff", b.lastCall())
}

func TestShellHelpersTypoAliasesExist(t *testing.T) {
	testenv.SkipWindows(t, "tests the zsh helpers")
	b := newZshBox(t)
	for _, a := range []string{"cñear", "ckear", "clea", "clar", "exir", "exirt"} {
		r := b.z(b.root, "alias -- "+a+" >/dev/null && echo yes || echo no", "")
		eq(t, "typo alias "+a+" exists", "yes", strings.TrimSpace(r.Stdout))
	}
}

// wt: one worktree per task, with the repo's ignored .env files and its own port.
func TestShellHelpersWorktreePerTask(t *testing.T) {
	testenv.SkipWindows(t, "tests the zsh helpers")
	needTool(t, "git")
	b := newZshBox(t)
	R := filepath.Join(b.root, "repo")
	if err := os.MkdirAll(R, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, R, "init", "-q", "-b", "main")
	write(t, R+"/.gitignore", ".env*\n.worktrees/\n")
	write(t, R+"/.env", "SECRET=1\n")
	git(t, R, "add", ".gitignore")
	git(t, R, "commit", "-qm", "init")

	// Real git here, not the stubs: only the PATH differs from z().
	wt := func(script string) string {
		env := isoEnv(filepath.Join(b.root, "home"),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		return run(R, env, "", "zsh", "-f", "-c", "source '"+b.helpers+"'; "+script).Out()
	}
	wt("wt feat-a")
	wtDir := R + "/.worktrees/feat-a"
	eq(t, "wt creates .worktrees/<name> on its own branch", "feat-a", git(t, wtDir, "branch", "--show-current"))
	eq(t, "wt copies the ignored .env files", "SECRET=1", strings.TrimSpace(readAbs(t, wtDir+"/.env")))
	portRe := `(?m)^PORT=(\d+)$`
	port := func() string {
		m := regexpFind(portRe, readAbs(t, wtDir+"/.env.local"))
		return m
	}
	p1 := port()
	isTrue(t, "wt gives the worktree its own port (not 3000): "+p1, p1 != "" && p1 != "3000")
	wt("wt feat-a")
	eq(t, "the port is stable for the same name", p1, port())
	isTrue(t, "wt ls lists it", strings.Contains(wt("wt ls"), "feat-a"))
	wt("wt rm feat-a")
	isTrue(t, "wt rm removes the worktree", !isDir(wtDir))
}

// forrepos: one command across every repo.
func TestShellHelpersForrepos(t *testing.T) {
	testenv.SkipWindows(t, "tests the zsh helpers")
	needTool(t, "git")
	b := newZshBox(t)
	F := filepath.Join(b.root, "projects")
	for _, d := range []string{"a", "b", "group/c", "not-a-repo"} {
		if err := os.MkdirAll(filepath.Join(F, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []string{"a", "b", "group/c"} {
		git(t, filepath.Join(F, r), "init", "-q")
	}
	forrepos := func(cmd string) result {
		env := isoEnv(filepath.Join(b.root, "home"), "FORREPOS_ROOT="+F)
		return run(b.root, env, "", "zsh", "-f", "-c", "source '"+b.helpers+"'; forrepos '"+cmd+"'")
	}
	out := forrepos("pwd").Out()
	eq(t, "forrepos runs in every repo, nested ones included", 3, count(`/(a|b|group/c)$`, out))
	mustHave(t, `^group/c +`, out, "forrepos prefixes output with the repo name")

	r := forrepos(`[ $(basename $PWD) != a ]`)
	eq(t, "a failing repo does not stop the others, and the exit is non-zero", 1, r.Code)
	mustHave(t, `failed.*\ba\b`, r.Out(), "...with a summary naming it")
}
