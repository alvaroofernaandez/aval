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
	"fmt"
	"os"
	"path/filepath"

	cfgfile "github.com/alvaroofernaandez/aval/internal/core/config"
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
	return cfgfile.Path(getenv, home, goos)
}

// Load returns the repo value from a config file. A missing file is not an
// error: it only means install has not recorded anything yet. Other keys and
// their validation belong to the config package.
func Load(path string) (string, error) {
	f, _, err := cfgfile.Load(path)
	if err != nil {
		return "", err
	}
	repo, _ := f.Get("repo")
	return repo, nil
}

// Save records repo in the config file, keeping any other lines it holds.
func Save(path, repo string) error {
	return cfgfile.Set(path, "repo", repo)
}
