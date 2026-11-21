package devmode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Info describes one sandbox on disk.
type Info struct {
	Name, Root string
	Ready      bool
	Modified   time.Time
}

// List returns the sandboxes under base, by name. A directory without the
// sandbox marker is not one and is ignored.
func List(base string) ([]Info, error) {
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s := Sandbox{Name: e.Name(), Root: filepath.Join(base, e.Name())}
		if _, err := os.Stat(s.Marker()); err != nil {
			continue
		}
		info := Info{Name: s.Name, Root: s.Root, Ready: s.Ready()}
		if fi, err := os.Stat(s.Root); err == nil {
			info.Modified = fi.ModTime()
		}
		out = append(out, info)
	}
	return out, nil
}

// Remove deletes one sandbox. It refuses anything that is not a sandbox this
// tool created (no marker), a root CheckRoot rejects, and a missing one.
func Remove(base, name, realHome string) error {
	s, err := Open(base, name, realHome)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(s.Root); err != nil {
		return fmt.Errorf("no sandbox named %q under %s", name, base)
	}
	if _, err := os.Stat(s.Marker()); err != nil {
		return fmt.Errorf("%s is not an aval dev sandbox (no %s marker); refusing to delete it", s.Root, filepath.Base(s.Marker()))
	}
	return os.RemoveAll(s.Root)
}

// Project clones the git repository at src into the sandbox's projects dir so
// hooks and commands can be tried on real code. The source is only read: it is
// a `git clone --local --no-hardlinks` (no object is shared with the original),
// the clone's remotes are removed so no push can reach the original or its
// upstream, and a note records where it came from. Uncommitted changes in the
// source are not carried over.
func Project(s Sandbox, src string, env []string) (string, error) {
	abs, err := filepath.Abs(src)
	if err != nil {
		return "", err
	}
	realSrc, err := resolve(abs)
	if err != nil {
		return "", err
	}
	realRoot, err := resolve(s.Root)
	if err != nil {
		return "", err
	}
	if inside(realRoot, realSrc) {
		return "", fmt.Errorf("%s is already inside the sandbox", src)
	}
	if err := exec.Command("git", "-C", abs, "rev-parse", "--git-dir").Run(); err != nil {
		return "", fmt.Errorf("%s is not a git repository", src)
	}
	dest := filepath.Join(s.Projects(), filepath.Base(realSrc))
	if _, err := os.Lstat(dest); err == nil {
		return "", fmt.Errorf("%s already exists; remove it or reset the sandbox", dest)
	}
	if err := os.MkdirAll(s.Projects(), 0o755); err != nil {
		return "", err
	}
	for _, args := range [][]string{
		{"clone", "--quiet", "--local", "--no-hardlinks", "--", abs, dest},
		{"-C", dest, "remote", "remove", "origin"},
	} {
		c := exec.Command("git", args...)
		c.Env = env
		if out, err := c.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git %s: %v\n%s", args[0], err, out)
		}
	}
	note := fmt.Sprintf("cloned by aval dev from %s\nthe remote was removed on purpose: a push from here must not reach the original\n", abs)
	if err := os.WriteFile(filepath.Join(dest, ".git", "aval-dev-source"), []byte(note), 0o644); err != nil {
		return "", err
	}
	return dest, nil
}
