// Package repoloc finds the aval repository on disk.
//
// A binary installed from a release (npm, GitHub) never lives inside the
// checkout, so walking up from the executable is not enough. The lookup order
// is, first match wins:
//
//	--repo  >  $AVAL_REPO  >  config file (repo = ...)  >  working directory  >  executable
//
// The three explicit sources fail loudly when they point at something that is
// not a checkout: falling through to a guess would act on the wrong repository.
// `aval install` records the repository it ran from in the config file, so
// every later command works from any directory.
package repoloc

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Manifest is the file that marks the root of an aval checkout.
const Manifest = "install.manifest"

// EnvVar overrides the configured repository location.
const EnvVar = "AVAL_REPO"

// maxWalk bounds how many parent directories the implicit lookups climb.
const maxWalk = 6

// Sources are the places the repository location can come from.
type Sources struct {
	Flag       string // --repo
	Env        string // $AVAL_REPO
	ConfigFile string // path to the config file that install writes
	Wd         string // working directory
	Exe        string // directory of the running binary
}

// Resolve returns the root of the aval checkout.
func Resolve(s Sources) (string, error) {
	if s.Flag != "" {
		return explicit(s.Flag, "--repo")
	}
	if s.Env != "" {
		return explicit(s.Env, "$"+EnvVar)
	}
	if s.ConfigFile != "" {
		configured, err := Load(s.ConfigFile)
		if err != nil {
			return "", err
		}
		if configured != "" {
			return explicit(configured, "repo in config "+s.ConfigFile)
		}
	}
	for _, start := range []string{s.Wd, s.Exe} {
		if start == "" {
			continue
		}
		if root := walkUp(start); root != "" {
			return root, nil
		}
	}
	return "", fmt.Errorf("cannot find the aval repository (%s): run 'aval install --repo <path>' once from your checkout, or set %s", Manifest, EnvVar)
}

func explicit(path, source string) (string, error) {
	if root := walkUp(path); root != "" {
		return root, nil
	}
	return "", fmt.Errorf("%s points at %s, which is not an aval repository (no %s)", source, path, Manifest)
}

func walkUp(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for range maxWalk {
		if _, err := os.Stat(filepath.Join(dir, Manifest)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ConfigPath is where install records the repository:
// ${XDG_CONFIG_HOME:-~/.config}/aval/config, or %APPDATA%\aval\config on Windows.
func ConfigPath(getenv func(string) string, home, goos string) string {
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "aval", "config")
	}
	if goos == "windows" {
		if a := getenv("APPDATA"); a != "" {
			return filepath.Join(a, "aval", "config")
		}
	}
	return filepath.Join(home, ".config", "aval", "config")
}

// Load returns the repo value from a config file. A missing file is not an
// error: it only means install has not recorded anything yet.
func Load(path string) (string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := keyValue(sc.Text()); ok && k == "repo" {
			return v, nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return "", nil
}

// Save records repo in the config file, keeping any other lines it holds.
func Save(path, repo string) error {
	var kept []string
	if b, err := os.ReadFile(path); err == nil {
		for line := range strings.SplitSeq(strings.TrimRight(string(b), "\n"), "\n") {
			if k, _, ok := keyValue(line); ok && k == "repo" {
				continue
			}
			kept = append(kept, line)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if len(kept) == 0 {
		kept = []string{"# Written by aval install. Override with --repo or $" + EnvVar + "."}
	}
	text := strings.Join(append(kept, "repo = "+repo), "\n") + "\n"

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func keyValue(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}
