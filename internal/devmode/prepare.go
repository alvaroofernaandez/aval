package devmode

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Create makes the sandbox directories and its marker. It is idempotent. The
// marker is what lets Remove and List tell a sandbox from any other directory.
func Create(base, name, realHome string) (Sandbox, error) {
	s, err := Open(base, name, realHome)
	if err != nil {
		return s, err
	}
	for _, d := range []string{s.Home(), s.Tmp(), filepath.Join(s.Tmp(), "tmux"), s.Projects(), s.RC()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return s, err
		}
	}
	// tmux refuses a socket dir that others can enter
	_ = os.Chmod(filepath.Join(s.Tmp(), "tmux"), 0o700)
	if _, err := os.Stat(s.Marker()); err != nil {
		if err := os.WriteFile(s.Marker(), []byte("aval dev sandbox "+name+"\n"), 0o644); err != nil {
			return s, err
		}
	}
	return s, nil
}

func (s Sandbox) readyFile() string { return filepath.Join(s.Root, ".aval-dev-ready") }

// Ready reports whether the first-entry install and sync finished.
func (s Sandbox) Ready() bool {
	_, err := os.Stat(s.readyFile())
	return err == nil
}

// skipDirs are never copied when the source is not a git checkout.
var skipDirs = map[string]bool{".git": true, "node_modules": true, "bin": true, "dist": true, "dist-npm": true}

// CopyRepo copies the checkout at src into dst: tracked and untracked files,
// not ignored ones and never .git. The sandbox installs from this copy, so
// links it creates point into the sandbox and `git config` runs by the install
// cannot reach the real repository. It is safe to repeat: changed files are
// overwritten, nothing is deleted. src is only read.
func CopyRepo(src, dst string) error {
	files, err := listFiles(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, rel := range files {
		from := filepath.Join(src, rel)
		to := filepath.Join(dst, rel)
		info, err := os.Lstat(from)
		if err != nil {
			continue // tracked but deleted in the working tree
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(from)
			if err != nil || filepath.IsAbs(target) {
				continue
			}
			if !inside(".", filepath.Join(filepath.Dir(rel), target)) {
				continue // would point outside the copy
			}
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return err
			}
			_ = os.Remove(to)
			if err := os.Symlink(target, to); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := copyFile(from, to, info); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(from, to string, info fs.FileInfo) error {
	if old, err := os.Lstat(to); err == nil && old.Mode().IsRegular() &&
		old.Size() == info.Size() && old.ModTime().Equal(info.ModTime()) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.Remove(to) // a read-only or linked leftover
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

// listFiles is the file list of a checkout, relative to src.
func listFiles(src string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(src, ".git")); err == nil {
		c := exec.Command("git", "-C", src, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
		c.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		if out, err := c.Output(); err == nil {
			var files []string
			for _, f := range strings.Split(string(out), "\x00") {
				if f != "" {
					files = append(files, filepath.FromSlash(f))
				}
			}
			return files, nil
		}
	}
	var files []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if rel != "." && skipDirs[d.Name()] && (d.Name() != "bin" || filepath.Dir(rel) == ".") {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

// WriteGitIdentity gives the sandbox a minimal global git config holding only
// user.name and user.email read from the real one (via realEnv). Nothing else
// is copied: no credential helper, no signing key, no URL rewrites.
func WriteGitIdentity(s Sandbox, realEnv []string) error {
	if err := os.MkdirAll(filepath.Dir(s.GitConfig()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.GitConfig(), nil, 0o644); err != nil {
		return err
	}
	for _, key := range []string{"user.name", "user.email"} {
		c := exec.Command("git", "config", "--global", "--get", key)
		c.Env = realEnv
		out, err := c.Output()
		val := strings.TrimSpace(string(out))
		if err != nil || val == "" {
			continue
		}
		w := exec.Command("git", "config", "--file", s.GitConfig(), key, val)
		w.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + s.Home(), "GIT_CONFIG_NOSYSTEM=1"}
		if b, err := w.CombinedOutput(); err != nil {
			return fmt.Errorf("writing %s: %v: %s", key, err, b)
		}
	}
	return nil
}

// withGoPaths adds GOCACHE, GOMODCACHE and GOPATH to base when it does not set
// them, taking the values the real environment would use. Without this the
// sandbox HOME would make every Go command inside it start from a cold cache.
func withGoPaths(base []string) []string {
	have := map[string]bool{}
	for _, e := range base {
		k, _, _ := strings.Cut(e, "=")
		have[k] = true
	}
	if have["GOCACHE"] && have["GOMODCACHE"] && have["GOPATH"] {
		return base
	}
	c := exec.Command("go", "env", "GOCACHE", "GOMODCACHE", "GOPATH")
	c.Env = base
	out, err := c.Output()
	if err != nil {
		return base
	}
	vals := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	out2 := append([]string{}, base...)
	for i, k := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		if i < len(vals) && vals[i] != "" && !have[k] {
			out2 = append(out2, k+"="+strings.TrimRight(vals[i], "\r"))
		}
	}
	return out2
}

// Build compiles the checkout at srcRepo into the sandbox. It writes only the
// binary, under the sandbox bin dir; the source tree is read.
func Build(s Sandbox, srcRepo string, env []string, goos string) error {
	if _, err := os.Stat(filepath.Join(srcRepo, "go.mod")); err != nil {
		return fmt.Errorf("%s is not a Go source checkout (no go.mod): dev mode builds aval from source", srcRepo)
	}
	if err := os.MkdirAll(s.Bin(), 0o755); err != nil {
		return err
	}
	c := exec.Command("go", "build", "-o", s.AvalBin(goos), "./cmd/aval")
	c.Dir = srcRepo
	c.Env = env
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("go build ./cmd/aval failed: %v\n%s", err, out)
	}
	return nil
}

// PrepareOptions describe one entry into a sandbox.
type PrepareOptions struct {
	Base, Name, RealHome, SrcRepo string
	Environ                       []string // the caller's (real) environment
	Out, ErrOut                   io.Writer
}

// Prepare makes the sandbox ready to use and returns it with the environment
// to run commands in: directories, a fresh copy of the repo, the git identity
// (first time only), a fresh build, the environment self-check, and on first
// entry `aval --yes --copy` plus `aval sync` so it mirrors a new machine.
func Prepare(o PrepareOptions) (Sandbox, []string, error) {
	goos := runtime.GOOS
	s, err := Create(o.Base, o.Name, o.RealHome)
	if err != nil {
		return s, nil, err
	}
	realEnv := withGoPaths(o.Environ)
	env := Env(s, realEnv, goos, o.RealHome)
	if err := VerifyEnv(env, s.Root, goos); err != nil {
		return s, nil, fmt.Errorf("refusing to start: %w", err)
	}

	if err := CopyRepo(o.SrcRepo, s.Repo()); err != nil {
		return s, nil, fmt.Errorf("copying the repo: %w", err)
	}
	if _, err := os.Stat(s.GitConfig()); err != nil {
		if err := WriteGitIdentity(s, o.Environ); err != nil {
			return s, nil, err
		}
	}
	if err := Build(s, o.SrcRepo, realEnv, goos); err != nil {
		return s, nil, err
	}

	if !s.Ready() {
		fmt.Fprintf(o.Out, "aval dev: first entry, installing into the sandbox (%s)\n", s.Root)
		for _, args := range [][]string{{"--yes", "--copy"}, {"sync"}} {
			c := exec.Command(s.AvalBin(goos), args...)
			c.Dir = s.Repo()
			c.Env = env
			c.Stdout, c.Stderr = o.Out, o.ErrOut
			if err := c.Run(); err != nil {
				return s, env, fmt.Errorf("aval %s inside the sandbox: %w", strings.Join(args, " "), err)
			}
		}
		if err := os.WriteFile(s.readyFile(), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
			return s, env, err
		}
	}
	return s, env, nil
}

// Run executes argv inside the sandbox with env and returns its exit code. The
// error is non-nil only when the command could not be started.
func Run(s Sandbox, env []string, argv []string, in io.Reader, out, errOut io.Writer) (int, error) {
	if len(argv) == 0 {
		return 2, errors.New("no command given")
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = s.Home()
	c.Env = env
	c.Stdin, c.Stdout, c.Stderr = in, out, errOut
	// LookPath must use the sandbox PATH, not ours: `aval` has to resolve to the
	// sandbox build.
	if !strings.ContainsAny(argv[0], `/\`) {
		for _, e := range env {
			if k, v, ok := strings.Cut(e, "="); ok && strings.EqualFold(k, "PATH") {
				if p, err := lookIn(argv[0], v); err == nil {
					c.Path = p
				}
			}
		}
	}
	err := c.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 127, err
	}
	return 0, nil
}

func lookIn(name, pathList string) (string, error) {
	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = []string{".exe", ".cmd", ".bat", ""}
	}
	for _, dir := range filepath.SplitList(pathList) {
		for _, ext := range exts {
			p := filepath.Join(dir, name+ext)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() && (runtime.GOOS == "windows" || fi.Mode()&0o111 != 0) {
				return p, nil
			}
		}
	}
	return "", exec.ErrNotFound
}
