package devmode

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func sandboxAt(t *testing.T) (Sandbox, string) {
	t.Helper()
	realHome := t.TempDir()
	base := filepath.Join(realHome, ".cache", "aval", "dev")
	s, err := Open(base, "default", realHome)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, realHome
}

func TestValidName(t *testing.T) {
	for _, ok := range []string{"default", "a", "feat-1", "my_box.2", "A9"} {
		if err := ValidName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, ".hidden", "-x", "has space", strings.Repeat("a", 60)} {
		if err := ValidName(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestBaseDir(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	home := filepath.Join(string(filepath.Separator), "h")

	got := BaseDir(get(map[string]string{"XDG_CACHE_HOME": "/x/cache"}), home)
	if want := filepath.Join("/x/cache", "aval", "dev"); got != want {
		t.Errorf("XDG_CACHE_HOME: got %q, want %q", got, want)
	}
	got = BaseDir(get(nil), home)
	if want := filepath.Join(home, ".cache", "aval", "dev"); got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
}

func TestCheckRootRefusesSensitiveDirs(t *testing.T) {
	realHome := t.TempDir()
	for _, d := range []string{".claude", ".ssh", filepath.Join(".config", "aval", "private")} {
		if err := os.MkdirAll(filepath.Join(realHome, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	refuse := []string{
		filepath.Join(realHome, ".claude"),
		filepath.Join(realHome, ".claude", "cache", "dev"),
		filepath.Join(realHome, ".ssh", "x"),
		filepath.Join(realHome, ".config", "aval", "private", "box"),
		realHome,                           // would delete the real home on rm
		filepath.Join(realHome, ".config"), // an ancestor of the private overlay
		filepath.Join(realHome, ".claude", "does", "not", "exist", "yet"),
	}
	for _, root := range refuse {
		if err := CheckRoot(root, realHome); err == nil {
			t.Errorf("CheckRoot(%s) must refuse", root)
		}
	}
	if err := CheckRoot(filepath.Join(realHome, ".cache", "aval", "dev", "default"), realHome); err != nil {
		t.Errorf("the default location must be accepted: %v", err)
	}
}

func TestCheckRootFollowsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	realHome := t.TempDir()
	ssh := filepath.Join(realHome, ".ssh")
	if err := os.MkdirAll(ssh, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "innocent")
	if err := os.Symlink(ssh, link); err != nil {
		t.Fatal(err)
	}
	if err := CheckRoot(filepath.Join(link, "box"), realHome); err == nil {
		t.Fatal("a root that resolves into ~/.ssh through a symlink must be refused")
	}
}

func TestOpenRejectsBadNameAndUnsafeBase(t *testing.T) {
	realHome := t.TempDir()
	base := filepath.Join(realHome, ".cache", "aval", "dev")
	if _, err := Open(base, "../escape", realHome); err == nil {
		t.Error("a name with .. must be rejected")
	}
	if _, err := Open(filepath.Join(realHome, ".claude", "dev"), "default", realHome); err == nil {
		t.Error("a base inside ~/.claude must be rejected")
	}
	s, err := Open(base, "default", realHome)
	if err != nil || s.Root != filepath.Join(base, "default") {
		t.Errorf("Open = %+v, %v", s, err)
	}
}

func TestEnvConfinesEveryIsolatedVariable(t *testing.T) {
	s, realHome := sandboxAt(t)
	base := []string{
		"HOME=" + realHome, "XDG_CONFIG_HOME=/real/cfg", "XDG_DATA_HOME=/real/data",
		"XDG_STATE_HOME=/real/state", "XDG_CACHE_HOME=/real/cache", "XDG_RUNTIME_DIR=/run/user/1",
		"CLAUDE_CONFIG_DIR=/real/claude", "CODEX_HOME=/real/codex", "GEMINI_CLI_HOME=/real/gemini",
		"AVAL_PRIVATE_DIR=/real/private", "AVAL_BIN=/real/aval", "AVAL_LESSONS_DIR=/real/lessons",
		"GIT_CONFIG_GLOBAL=/real/gitconfig", "GIT_DIR=/real/.git", "GIT_WORK_TREE=/real",
		"ENGRAM_DATA_DIR=/real/engram", "ENGRAM_CLOUD_TOKEN=secret", "ENGRAM_PORT=7437",
		"TMUX=/tmp/tmux-1/default,1,0", "TMUX_PANE=%1", "ZDOTDIR=/real/zsh", "TMPDIR=/real/tmp",
		"GOCACHE=/real/gocache", "GOMODCACHE=/real/gomod", "LANG=es_ES.UTF-8", "PATH=/usr/bin:/bin",
	}
	env := Env(s, base, "linux", realHome)
	m := envMap(env)

	confined := map[string]string{
		"HOME": s.Home(), "XDG_CONFIG_HOME": filepath.Join(s.Home(), ".config"),
		"XDG_DATA_HOME":     filepath.Join(s.Home(), ".local", "share"),
		"XDG_STATE_HOME":    filepath.Join(s.Home(), ".local", "state"),
		"XDG_CACHE_HOME":    filepath.Join(s.Home(), ".cache"),
		"CLAUDE_CONFIG_DIR": filepath.Join(s.Home(), ".claude"), "CODEX_HOME": filepath.Join(s.Home(), ".codex"),
		"GEMINI_CLI_HOME":   s.Home(),
		"AVAL_PRIVATE_DIR":  filepath.Join(s.Home(), ".config", "aval", "private"),
		"GIT_CONFIG_GLOBAL": s.GitConfig(), "ENGRAM_DATA_DIR": filepath.Join(s.Home(), ".engram"),
		"TMPDIR": s.Tmp(), "TMUX_TMPDIR": filepath.Join(s.Tmp(), "tmux"), "ZDOTDIR": s.RC(),
	}
	for k, want := range confined {
		if m[k] != want {
			t.Errorf("%s = %q, want %q", k, m[k], want)
		}
		if !within(s.Root, m[k]) {
			t.Errorf("%s = %q escapes the sandbox root", k, m[k])
		}
	}

	if m["AVAL_DEV"] != "1" || m["AVAL_DEV_NAME"] != "default" || m["AVAL_DEV_ROOT"] != s.Root || m["AVAL_DEV_REAL_HOME"] != realHome {
		t.Errorf("dev markers wrong: %v", m)
	}
	if m["GIT_CONFIG_NOSYSTEM"] != "1" || m["AVAL_NO_UPDATE_CHECK"] != "1" {
		t.Error("system git config and the update check must be off")
	}
	if !strings.HasPrefix(m["PATH"], s.Bin()+string(os.PathListSeparator)) || !strings.Contains(m["PATH"], "/usr/bin") {
		t.Errorf("PATH must start with the sandbox bin and keep the rest: %q", m["PATH"])
	}
	for _, gone := range []string{"XDG_RUNTIME_DIR", "TMUX", "TMUX_PANE", "AVAL_BIN", "AVAL_LESSONS_DIR", "GIT_DIR", "GIT_WORK_TREE", "ENGRAM_CLOUD_TOKEN"} {
		if _, ok := m[gone]; ok {
			t.Errorf("%s leaks from the real environment", gone)
		}
	}
	if m["GOCACHE"] != "/real/gocache" || m["GOMODCACHE"] != "/real/gomod" || m["LANG"] != "es_ES.UTF-8" {
		t.Error("Go caches (content addressed) and locale must be preserved")
	}
	if p := m["ENGRAM_PORT"]; p == "" || p == "7437" {
		t.Errorf("ENGRAM_PORT must differ from the real daemon's: %q", p)
	}
	if err := VerifyEnv(env, s.Root, "linux"); err != nil {
		t.Errorf("VerifyEnv on a good env: %v", err)
	}
	// no duplicate keys
	seen := map[string]bool{}
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if seen[k] {
			t.Errorf("duplicate key %s", k)
		}
		seen[k] = true
	}
}

func TestEnvWindowsIsolatesProfile(t *testing.T) {
	s, realHome := sandboxAt(t)
	env := Env(s, []string{"USERPROFILE=" + realHome, "APPDATA=C:\\real", "Path=C:\\Windows"}, "windows", realHome)
	m := envMap(env)
	for _, k := range []string{"USERPROFILE", "HOME", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP"} {
		if !within(s.Root, m[k]) {
			t.Errorf("%s = %q escapes the sandbox root", k, m[k])
		}
	}
}

func TestVerifyEnvCatchesAnEscape(t *testing.T) {
	s, realHome := sandboxAt(t)
	env := Env(s, nil, "linux", realHome)
	bad := append(env, "CODEX_HOME="+filepath.Join(realHome, ".codex"))
	if err := VerifyEnv(bad, s.Root, "linux"); err == nil || !strings.Contains(err.Error(), "CODEX_HOME") {
		t.Fatalf("VerifyEnv must name the escaping variable, got %v", err)
	}
	missing := envWithout(env, "CLAUDE_CONFIG_DIR")
	if err := VerifyEnv(missing, s.Root, "linux"); err == nil {
		t.Fatal("VerifyEnv must fail when an isolated variable is unset")
	}
}

func envWithout(env []string, key string) []string {
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return out
}

func TestShellSelection(t *testing.T) {
	s, _ := sandboxAt(t)
	look := func(name string) (string, error) {
		if name == "nope" {
			return "", errors.New("not found")
		}
		return "/bin/" + name, nil
	}

	z := Shell(s, "darwin", "/bin/zsh", look)
	if z.Path != "/bin/zsh" || len(z.Args) != 1 || z.Args[0] != "-i" {
		t.Errorf("zsh spec = %+v", z)
	}
	if !strings.Contains(z.Files[filepath.Join(s.RC(), ".zshrc")], "[aval dev:default]") {
		t.Errorf("zsh rc must carry the prompt marker: %v", z.Files)
	}

	b := Shell(s, "linux", "/usr/bin/bash", look)
	if len(b.Args) < 3 || b.Args[0] != "--rcfile" || b.Args[1] != filepath.Join(s.RC(), "bashrc") {
		t.Errorf("bash spec = %+v", b)
	}
	if !strings.Contains(b.Files[filepath.Join(s.RC(), "bashrc")], "[aval dev:default]") {
		t.Error("bash rc must carry the marker")
	}

	o := Shell(s, "linux", "/usr/bin/tcsh", look)
	if !strings.Contains(strings.Join(o.Env, " "), "[aval dev:default]") {
		t.Errorf("an unknown shell still shows the marker via PS1: %+v", o)
	}

	f := Shell(s, "linux", "", look)
	if filepath.Base(f.Path) != "sh" {
		t.Errorf("with no $SHELL the fallback is sh, got %q", f.Path)
	}

	p := Shell(s, "windows", `C:\Program Files\Git\bin\bash.exe`, look)
	if filepath.Base(p.Path) != "pwsh" && filepath.Base(p.Path) != "powershell" {
		t.Errorf("Windows ignores $SHELL and prefers PowerShell, got %q", p.Path)
	}
	if !strings.Contains(strings.Join(p.Args, " "), "[aval dev:default]") {
		t.Errorf("powershell must show the marker: %+v", p)
	}

	noPS := func(name string) (string, error) { return "", errors.New("missing") }
	c := Shell(s, "windows", "", noPS)
	if !strings.EqualFold(filepath.Base(c.Path), "cmd.exe") && c.Path != "cmd" {
		t.Errorf("Windows falls back to cmd, got %q", c.Path)
	}
	if !strings.Contains(strings.Join(c.Args, " "), "[aval dev:default]") {
		t.Errorf("cmd must show the marker: %+v", c)
	}
}

func TestGuardNested(t *testing.T) {
	in := func(k string) string {
		if k == "AVAL_DEV" {
			return "1"
		}
		return ""
	}
	out := func(string) string { return "" }
	if err := GuardNested(in, false); err == nil || !strings.Contains(err.Error(), "--nested") {
		t.Errorf("inside a dev shell without --nested: %v", err)
	}
	if err := GuardNested(in, true); err != nil {
		t.Errorf("--nested must allow it: %v", err)
	}
	if err := GuardNested(out, false); err != nil {
		t.Errorf("outside a dev shell: %v", err)
	}
}

func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = env
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitEnv(extra ...string) []string {
	return append(append([]string{}, os.Environ()...), append([]string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	}, extra...)...)
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func treeHash(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		info, _ := d.Info()
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			lines = append(lines, "L "+rel+" -> "+l)
		case d.IsDir():
			lines = append(lines, "D "+rel)
		default:
			b, _ := os.ReadFile(p)
			sum := sha256.Sum256(b)
			lines = append(lines, "F "+rel+" "+hex.EncodeToString(sum[:])+" "+info.ModTime().String())
		}
		return nil
	})
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func TestCopyRepoCopiesTrackedAndUntrackedButNotGitOrIgnored(t *testing.T) {
	needGit(t)
	src := t.TempDir()
	run(t, src, gitEnv(), "git", "init", "-q")
	put(t, filepath.Join(src, "install.manifest"), "m")
	put(t, filepath.Join(src, "config", "a.md"), "tracked")
	put(t, filepath.Join(src, ".gitignore"), "ignored.txt\n/bin/\n")
	run(t, src, gitEnv(), "git", "add", ".")
	run(t, src, gitEnv(), "git", "commit", "-q", "-m", "x")
	put(t, filepath.Join(src, "config", "new.md"), "untracked")
	put(t, filepath.Join(src, "ignored.txt"), "ignored")
	put(t, filepath.Join(src, "bin", "aval"), "stale binary")
	if runtime.GOOS != "windows" {
		if err := os.Symlink("/etc/passwd", filepath.Join(src, "abs-link")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("config/a.md", filepath.Join(src, "rel-link")); err != nil {
			t.Fatal(err)
		}
	}
	before := treeHash(t, src)

	dst := filepath.Join(t.TempDir(), "repo")
	if err := CopyRepo(src, dst); err != nil {
		t.Fatalf("CopyRepo: %v", err)
	}
	for _, want := range []string{"install.manifest", "config/a.md", "config/new.md"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(want))); err != nil {
			t.Errorf("%s was not copied: %v", want, err)
		}
	}
	for _, not := range []string{".git", "ignored.txt", "bin", "abs-link"} {
		if _, err := os.Lstat(filepath.Join(dst, not)); err == nil {
			t.Errorf("%s must not be copied", not)
		}
	}
	if runtime.GOOS != "windows" {
		if l, err := os.Readlink(filepath.Join(dst, "rel-link")); err != nil || l != "config/a.md" {
			t.Errorf("a relative in-repo link is kept: %q %v", l, err)
		}
	}
	if treeHash(t, src) != before {
		t.Error("CopyRepo modified the source checkout")
	}

	// A second copy picks up changes and removes nothing it did not own.
	put(t, filepath.Join(src, "config", "a.md"), "changed")
	if err := CopyRepo(src, dst); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dst, "config", "a.md"))
	if string(b) != "changed" {
		t.Errorf("refresh did not update the copy: %q", b)
	}
}

func TestCopyRepoWithoutGitWalksTheTree(t *testing.T) {
	src := t.TempDir()
	put(t, filepath.Join(src, "install.manifest"), "m")
	put(t, filepath.Join(src, "node_modules", "x", "y.js"), "skip")
	put(t, filepath.Join(src, ".git", "config"), "skip")
	dst := filepath.Join(t.TempDir(), "repo")
	if err := CopyRepo(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "install.manifest")); err != nil {
		t.Error("manifest missing")
	}
	for _, not := range []string{"node_modules", ".git"} {
		if _, err := os.Lstat(filepath.Join(dst, not)); err == nil {
			t.Errorf("%s must be skipped", not)
		}
	}
}

func TestWriteGitIdentityCopiesOnlyNameAndEmail(t *testing.T) {
	needGit(t)
	s, realHome := sandboxAt(t)
	realCfg := filepath.Join(realHome, "real.gitconfig")
	put(t, realCfg, "[user]\n\tname = Ada\n\temail = ada@example.com\n\tsigningkey = SECRETKEY\n[credential]\n\thelper = store\n[url \"git@github.com:\"]\n\tinsteadOf = https://github.com/\n")
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteGitIdentity(s, []string{"GIT_CONFIG_GLOBAL=" + realCfg, "HOME=" + realHome, "PATH=" + os.Getenv("PATH")}); err != nil {
		t.Fatalf("WriteGitIdentity: %v", err)
	}
	b, err := os.ReadFile(s.GitConfig())
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"Ada", "ada@example.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("identity is missing %q:\n%s", want, got)
		}
	}
	for _, leak := range []string{"SECRETKEY", "credential", "insteadOf", "signingkey"} {
		if strings.Contains(got, leak) {
			t.Errorf("%q leaked into the sandbox gitconfig:\n%s", leak, got)
		}
	}
}

func TestProjectClonesWithoutTouchingTheSource(t *testing.T) {
	needGit(t)
	s, realHome := sandboxAt(t)
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "myproj")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, src, gitEnv(), "git", "init", "-q", "-b", "main")
	put(t, filepath.Join(src, "main.go"), "package main")
	run(t, src, gitEnv(), "git", "add", ".")
	run(t, src, gitEnv(), "git", "commit", "-q", "-m", "x")
	run(t, src, gitEnv(), "git", "remote", "add", "origin", "https://example.invalid/real.git")
	before := treeHash(t, src)

	env := Env(s, os.Environ(), runtime.GOOS, realHome)
	dst, err := Project(s, src, env)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !within(s.Projects(), dst) || filepath.Base(dst) != "myproj" {
		t.Errorf("clone must live under the sandbox projects dir: %s", dst)
	}
	if _, err := os.Stat(filepath.Join(dst, "main.go")); err != nil {
		t.Errorf("working tree missing: %v", err)
	}
	if remotes := run(t, dst, gitEnv(), "git", "remote"); remotes != "" {
		t.Errorf("the clone must have no remote (a push would reach the original): %q", remotes)
	}
	note, err := os.ReadFile(filepath.Join(dst, ".git", "aval-dev-source"))
	if err != nil || !strings.Contains(string(note), src) {
		t.Errorf("the clone records where it came from: %q %v", note, err)
	}
	if treeHash(t, src) != before {
		t.Error("Project modified the source repository")
	}
	if _, err := Project(s, src, env); err == nil {
		t.Error("a second clone over the first must be refused, not overwritten")
	}
	if _, err := Project(s, filepath.Join(t.TempDir(), "missing"), env); err == nil {
		t.Error("a source that is not a git repo must be refused")
	}
	if _, err := Project(s, s.Projects(), env); err == nil {
		t.Error("a source inside the sandbox must be refused")
	}
}

func TestListAndRemove(t *testing.T) {
	realHome := t.TempDir()
	base := filepath.Join(realHome, ".cache", "aval", "dev")

	if got, err := List(base); err != nil || len(got) != 0 {
		t.Fatalf("an empty base lists nothing: %v %v", got, err)
	}
	a, err := Create(base, "a", realHome)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := Create(base, "b", realHome); err != nil {
		t.Fatal(err)
	}
	// a directory that is not a sandbox must be ignored and never removed
	stranger := filepath.Join(base, "stranger")
	put(t, filepath.Join(stranger, "keep.txt"), "mine")

	got, err := List(base)
	if err != nil || len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("List = %+v, %v", got, err)
	}
	if err := Remove(base, "stranger", realHome); err == nil {
		t.Error("Remove must refuse a directory without the sandbox marker")
	}
	if _, err := os.Stat(filepath.Join(stranger, "keep.txt")); err != nil {
		t.Error("the stranger directory was touched")
	}
	if err := Remove(base, "a", realHome); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(a.Root); err == nil {
		t.Error("sandbox a still exists")
	}
	if err := Remove(base, "a", realHome); err == nil {
		t.Error("removing a missing sandbox is an error")
	}
	if err := Remove(base, "../b", realHome); err == nil {
		t.Error("a traversal name must be refused")
	}
}
