package doctor

import (
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/deps"
)

// Options configures a run.
type Options struct {
	Repo       string
	Home       string
	PrivateDir string
	GOOS       string
	// Security enables the extra checks of `aval doctor --security`.
	Security bool
	// Allow lists suppressed check ids ("id" or "id:subject").
	Allow []string
	// LookPath resolves a bare command name; nil means exec.LookPath.
	LookPath func(string) (string, error)
	// Generation is the Claude Code tree of the generation the live settings
	// must match (<generation>/claude-code); empty means the active one, or
	// the repo when none is active.
	Generation string
	// ConfigPath is aval's config file; empty skips the config check.
	ConfigPath string
	// DepsProber checks the external tool inventory; nil skips the check,
	// so a caller that runs the doctor after every sync probes nothing.
	DepsProber deps.Prober
	// DepsTimeout is the budget of one probe; zero means deps.DefaultTimeout.
	DepsTimeout time.Duration
}
