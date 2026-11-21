package tests

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// `aval dev` promises one thing above all: nothing it runs reaches the real
// machine. These tests hold it to that with a fake "real" HOME that is hashed
// (content and mtime) before and after, so a single stray write fails them.

// devMachine is a fake real HOME with the kind of files dev mode must never
// touch, and the environment a developer's shell would have.
type devMachine struct {
	home string
	env  []string
}

func newDevMachine(t *testing.T) devMachine {
	t.Helper()
	needTool(t, "go", "git")
	home := t.TempDir()
	write(t, filepath.Join(home, ".claude", "settings.json"), `{"personal":true}`)
	write(t, filepath.Join(home, ".claude", ".credentials.json"), "secret")
	write(t, filepath.Join(home, ".ssh", "id_ed25519"), "private key")
	write(t, filepath.Join(home, ".config", "aval", "private", "settings.json"), `{"env":{"X":"1"}}`)
	write(t, filepath.Join(home, ".zshrc"), "# real zshrc")
	write(t, filepath.Join(home, ".gitconfig"), "[user]\n\tname = Real Dev\n\temail = real@example.com\n[credential]\n\thelper = store\n")

	gocache := run("", nil, "", "go", "env", "GOCACHE").Stdout
	gomod := run("", nil, "", "go", "env", "GOMODCACHE").Stdout
	gopath := run("", nil, "", "go", "env", "GOPATH").Stdout
	goenv := run("", nil, "", "go", "env", "GOENV").Stdout
	env := isoEnv(home,
		withHookTools(t),
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"),
		"GOCACHE="+strings.TrimSpace(gocache), "GOMODCACHE="+strings.TrimSpace(gomod), "GOPATH="+strings.TrimSpace(gopath),
		"GOENV="+strings.TrimSpace(goenv),
		"SHELL=/bin/sh",
	)
	// the sandbox base exists up front so only what is inside it may change
	if err := os.MkdirAll(filepath.Join(home, ".cache", "aval", "dev"), 0o755); err != nil {
		t.Fatal(err)
	}
	return devMachine{home: home, env: env}
}

func (m devMachine) base() string { return filepath.Join(m.home, ".cache", "aval", "dev") }

func (m devMachine) aval(t *testing.T, kv []string, args ...string) result {
	t.Helper()
	env := m.env
	if len(kv) > 0 {
		env = withEnv(m.env, kv...)
	}
	return run(repo, env, "", avalBin(t), args...)
}

// digest hashes every file's path, content and mtime, every symlink target and
// every directory under root, skipping the given absolute subtrees.
func digest(t *testing.T, root string, skip ...string) string {
	t.Helper()
	var lines []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		for _, s := range skip {
			if path == s {
				return filepath.SkipDir
			}
		}
		rel, _ := filepath.Rel(root, path)
		info, _ := d.Info()
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(path)
			lines = append(lines, "L "+rel+" -> "+l)
		case d.IsDir():
			lines = append(lines, "D "+rel)
		default:
			b, _ := os.ReadFile(path)
			sum := sha256.Sum256(b)
			lines = append(lines, "F "+rel+" "+hex.EncodeToString(sum[:])+" "+info.ModTime().String())
		}
		return nil
	})
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func repoHooksPath(t *testing.T) string {
	t.Helper()
	return run(repo, nil, "", "git", "config", "--local", "--get-all", "core.hooksPath").Stdout
}

func TestDevRunKeepsTheRealMachineByteIdentical(t *testing.T) {
	slow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probes")
	}
	needGit(t)
	m := newDevMachine(t)
	sandboxes := m.base()
	before := digest(t, m.home, sandboxes)
	hooksBefore := repoHooksPath(t)

	r := m.aval(t, nil, "dev", "run", "--", "sh", "-c",
		`env; echo wrote > "$HOME/probe"; echo wrote > "$TMPDIR/probe"; aval doctor >/dev/null 2>&1; exit 3`)
	if r.Code != 3 {
		t.Fatalf("the command's exit code must pass through: got %d\n%s", r.Code, r.Out())
	}

	root := filepath.Join(sandboxes, "default")
	vars := map[string]string{}
	for _, l := range strings.Split(r.Stdout, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			vars[k] = v
		}
	}
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
		"CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_CLI_HOME", "AVAL_PRIVATE_DIR", "GIT_CONFIG_GLOBAL",
		"ENGRAM_DATA_DIR", "TMPDIR", "TMUX_TMPDIR", "ZDOTDIR"} {
		v, ok := vars[k]
		if !ok || !strings.HasPrefix(v, root+string(filepath.Separator)) {
			t.Errorf("%s = %q, want a path under %s", k, v, root)
		}
	}
	isTrue(t, "AVAL_DEV=1 is set", vars["AVAL_DEV"] == "1")
	isTrue(t, "the sandbox bin leads PATH", strings.HasPrefix(vars["PATH"], filepath.Join(root, "repo", "bin")+":"))

	isTrue(t, "the install ran in the sandbox ($HOME/.claude/settings.json exists)", exists(filepath.Join(root, "home", ".claude", "settings.json")))
	isTrue(t, "the shell wrote its probe under the sandbox", exists(filepath.Join(root, "home", "probe")) && exists(filepath.Join(root, "tmp", "probe")))
	isTrue(t, "the sandbox has no .git (nothing can configure the real repo)", !exists(filepath.Join(root, "repo", ".git")))
	idCfg := readAbs(t, filepath.Join(root, "gitconfig"))
	isTrue(t, "only the git identity was copied", strings.Contains(idCfg, "Real Dev") && !strings.Contains(idCfg, "credential"))

	if after := digest(t, m.home, sandboxes); after != before {
		t.Error("the real HOME changed while a command ran in the sandbox")
	}
	if got := repoHooksPath(t); got != hooksBefore {
		t.Errorf("the real checkout's core.hooksPath changed: %q -> %q", hooksBefore, got)
	}
}

func TestDevRefusesToNestWithoutFlag(t *testing.T) {
	slow(t)
	m := newDevMachine(t)
	before := digest(t, m.home)
	r := m.aval(t, []string{"AVAL_DEV=1"}, "dev", "run", "--", "true")
	if r.Code == 0 || !strings.Contains(r.Out(), "--nested") {
		t.Fatalf("inside a dev shell `aval dev` must be refused and mention --nested: exit %d\n%s", r.Code, r.Out())
	}
	if digest(t, m.home) != before {
		t.Error("a refused run still wrote to the machine")
	}
}

func TestDevRefusesASandboxInsideSensitiveDirs(t *testing.T) {
	slow(t)
	m := newDevMachine(t)
	for _, bad := range []string{
		filepath.Join(m.home, ".claude", "cache"),
		filepath.Join(m.home, ".ssh"),
		filepath.Join(m.home, ".config", "aval", "private", "x"),
	} {
		before := digest(t, m.home)
		r := m.aval(t, []string{"XDG_CACHE_HOME=" + bad}, "dev", "run", "--", "true")
		if r.Code == 0 || !strings.Contains(r.Out(), "refusing") {
			t.Errorf("a sandbox under %s must be refused: exit %d\n%s", bad, r.Code, r.Out())
		}
		if digest(t, m.home) != before {
			t.Errorf("a refused sandbox under %s still wrote to the machine", bad)
		}
	}
}

func TestDevLifecycleLsResetRm(t *testing.T) {
	slow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probes")
	}
	m := newDevMachine(t)

	if r := m.aval(t, nil, "dev", "ls"); r.Code != 0 || strings.Contains(r.Stdout, "default") {
		t.Fatalf("no sandboxes yet:\n%s", r.Out())
	}
	if r := m.aval(t, nil, "dev", "run", "--name", "alpha", "--", "sh", "-c", `echo mine > "$HOME/mine.txt"`); r.Code != 0 {
		t.Fatalf("run: %s", r.Out())
	}
	root := filepath.Join(m.base(), "alpha")
	isTrue(t, "the file was written in the sandbox", exists(filepath.Join(root, "home", "mine.txt")))

	if r := m.aval(t, nil, "dev", "ls"); !strings.Contains(r.Stdout, "alpha") {
		t.Errorf("ls must list alpha:\n%s", r.Out())
	}
	if r := m.aval(t, nil, "dev", "reset", "--name", "alpha"); r.Code != 0 {
		t.Fatalf("reset: %s", r.Out())
	}
	isTrue(t, "reset wipes what the sandbox held", !exists(filepath.Join(root, "home", "mine.txt")))
	isTrue(t, "reset leaves a freshly installed sandbox", exists(filepath.Join(root, "home", ".claude", "settings.json")))

	if r := m.aval(t, nil, "dev", "rm", "--name", "alpha"); r.Code != 0 {
		t.Fatalf("rm: %s", r.Out())
	}
	isTrue(t, "rm deletes the sandbox", !exists(root))
	if r := m.aval(t, nil, "dev", "rm", "--name", "alpha"); r.Code == 0 {
		t.Error("removing a missing sandbox must fail")
	}
}

func TestDevDiffIsReadOnly(t *testing.T) {
	slow(t)
	m := newDevMachine(t)
	before := digest(t, m.home)
	hooksBefore := repoHooksPath(t)

	r := m.aval(t, nil, "dev", "diff")
	if r.Code != 0 {
		t.Fatalf("dev diff: exit %d\n%s", r.Code, r.Out())
	}
	for _, want := range []string{"aval --yes", "aval sync", "read-only"} {
		if !strings.Contains(r.Out(), want) {
			t.Errorf("diff output is missing %q:\n%s", want, r.Out())
		}
	}
	if digest(t, m.home) != before {
		t.Error("`aval dev diff` wrote to the real HOME (mtimes or content changed, or files appeared)")
	}
	if got := repoHooksPath(t); got != hooksBefore {
		t.Errorf("diff touched the checkout's git config: %q -> %q", hooksBefore, got)
	}
}

func TestDevProjectClonesWithoutTouchingTheSource(t *testing.T) {
	slow(t)
	m := newDevMachine(t)
	src := filepath.Join(t.TempDir(), "client-app")
	write(t, filepath.Join(src, "main.go"), "package main")
	git(t, src, "init", "-q", "-b", "main")
	git(t, src, "add", ".")
	git(t, src, "commit", "-q", "-m", "x")
	git(t, src, "remote", "add", "origin", "https://example.invalid/real.git")
	srcBefore := digest(t, src)
	homeBefore := digest(t, m.home, m.base())

	r := m.aval(t, nil, "dev", "project", src)
	if r.Code != 0 {
		t.Fatalf("dev project: exit %d\n%s", r.Code, r.Out())
	}
	clone := filepath.Join(m.base(), "default", "projects", "client-app")
	isTrue(t, "the clone is in the sandbox", exists(filepath.Join(clone, "main.go")))
	isTrue(t, "the output says where it is and that the remote was dropped",
		strings.Contains(r.Out(), clone) && strings.Contains(r.Out(), "remote"))
	isTrue(t, "no remote is left", git(t, clone, "remote") == "")
	if digest(t, src) != srcBefore {
		t.Error("the source repository changed")
	}
	if digest(t, m.home, m.base()) != homeBefore {
		t.Error("the real HOME changed")
	}
}
