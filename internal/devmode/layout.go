// Package devmode is the sandbox behind `aval dev`: a throwaway HOME, XDG
// tree, tool config dirs and PATH, so installing, syncing and doctoring aval
// can be tried without touching the real system, the real config or anything
// in the user's projects.
//
// Nothing here changes the process it runs in. It computes environments and
// paths, and the few functions that write only ever write under a Sandbox root
// that CheckRoot has approved.
package devmode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Sandbox is one named, self-contained machine under Root.
//
//	Root/home      HOME, XDG_*, ~/.claude, ~/.codex, ~/.engram ...
//	Root/repo      a copy of the aval checkout (never the checkout itself)
//	Root/repo/bin  the built aval; first on PATH. It sits inside the copy so
//	               aval's own repo lookup (walk up from the executable) finds it
//	Root/projects  clones made by `aval dev project`
//	Root/tmp       TMPDIR and the tmux socket dir
//	Root/rc        shell rc wrappers that add the prompt marker
type Sandbox struct{ Name, Root string }

func (s Sandbox) Home() string      { return filepath.Join(s.Root, "home") }
func (s Sandbox) Repo() string      { return filepath.Join(s.Root, "repo") }
func (s Sandbox) Bin() string       { return filepath.Join(s.Root, "repo", "bin") }
func (s Sandbox) Tmp() string       { return filepath.Join(s.Root, "tmp") }
func (s Sandbox) Projects() string  { return filepath.Join(s.Root, "projects") }
func (s Sandbox) RC() string        { return filepath.Join(s.Root, "rc") }
func (s Sandbox) GitConfig() string { return filepath.Join(s.Root, "gitconfig") }
func (s Sandbox) Marker() string    { return filepath.Join(s.Root, ".aval-dev") }

// AvalBin is the built binary's path for goos.
func (s Sandbox) AvalBin(goos string) string {
	name := "aval"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(s.Bin(), name)
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

// ValidName accepts short, path-safe names: no separators, no leading dot.
func ValidName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid sandbox name %q: use 1-40 letters, digits, '.', '_' or '-', starting with a letter or digit", name)
	}
	return nil
}

// BaseDir is where sandboxes live: ${XDG_CACHE_HOME:-~/.cache}/aval/dev.
func BaseDir(getenv func(string) string, realHome string) string {
	cache := getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(realHome, ".cache")
	}
	return filepath.Join(cache, "aval", "dev")
}

// Open validates name and the resulting root, and returns the Sandbox. It
// creates nothing.
func Open(base, name, realHome string) (Sandbox, error) {
	if err := ValidName(name); err != nil {
		return Sandbox{}, err
	}
	s := Sandbox{Name: name, Root: filepath.Join(base, name)}
	if err := CheckRoot(s.Root, realHome); err != nil {
		return Sandbox{}, err
	}
	return s, nil
}

// sensitive are the places under the real HOME that a sandbox must never
// overlap: its root may not sit inside one (the sandbox would write there) nor
// contain one (removing the sandbox would delete it).
var sensitive = []string{
	".claude", ".ssh", ".gnupg", ".codex", ".gemini", ".engram", ".aval-backup",
	filepath.Join(".config", "aval", "private"),
}

// CheckRoot refuses a sandbox root that is, contains or lies inside the real
// HOME or one of its sensitive directories. Symlinks are resolved first, so a
// root that merely points into ~/.ssh is refused too.
func CheckRoot(root, realHome string) error {
	r, err := resolve(root)
	if err != nil {
		return err
	}
	h, err := resolve(realHome)
	if err != nil {
		return err
	}
	if r == h {
		return fmt.Errorf("refusing sandbox root %s: it is the real home directory", root)
	}
	for _, rel := range sensitive {
		s, err := resolve(filepath.Join(realHome, rel))
		if err != nil {
			return err
		}
		if inside(s, r) || inside(r, s) {
			return fmt.Errorf("refusing sandbox root %s: it overlaps %s", root, s)
		}
	}
	// the root may not be an ancestor of the real home either (rm would take it)
	if inside(r, h) {
		return fmt.Errorf("refusing sandbox root %s: it contains the real home directory", root)
	}
	return nil
}

// inside reports whether p is dir or lies under it.
func inside(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve makes p absolute and resolves symlinks in its deepest existing
// ancestor, so a path that does not exist yet still gets a truthful answer.
func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rest := ""
	cur := abs
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			return filepath.Join(real, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// GuardNested refuses `aval dev` inside a dev shell unless nested is set.
func GuardNested(getenv func(string) string, nested bool) error {
	if getenv("AVAL_DEV") == "1" && !nested {
		return errors.New("already inside an aval dev shell (AVAL_DEV=1); exit it first, or pass --nested to stack another sandbox")
	}
	return nil
}
