// Package ack records that the design pipeline was done for a project.
//
// It writes <cache>/design-gate/<sha1 of project root>, the marker the
// PreToolUse gate (internal/hook) and the
// OpenCode plugin read. The format is a contract with them: three key=value
// lines, surface, epoch and design_sha1.
package ack

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// DefaultTTL is how long an ack stays valid, in seconds.
const DefaultTTL = 14400

// Options configures one ack.
type Options struct {
	Surface string
	Dir     string    // where the command ran; the project root is found from here
	Cache   string    // cache base dir (XDG_CACHE_HOME or ~/.cache)
	Now     time.Time // zero means time.Now()
	TTL     int       // seconds, only for the printed reminder; <=0 means DefaultTTL
}

// TTLFromEnv reads DESIGN_ACK_TTL the way the gate does: anything that is not
// a plain number falls back to the default.
func TTLFromEnv() int { return TTLFrom(os.Getenv) }

// TTLFrom is TTLFromEnv over an injected environment.
func TTLFrom(getenv func(string) string) int {
	n, err := strconv.Atoi(getenv("DESIGN_ACK_TTL"))
	if err != nil || n < 0 {
		return DefaultTTL
	}
	return n
}

// CacheDir is XDG_CACHE_HOME, else ~/.cache.
func CacheDir() (string, error) { return CacheDirFrom(os.Getenv, os.UserHomeDir) }

// CacheDirFrom is CacheDir over an injected environment and home lookup.
func CacheDirFrom(getenv func(string) string, home func() (string, error)) (string, error) {
	if x := getenv("XDG_CACHE_HOME"); x != "" {
		return x, nil
	}
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".cache"), nil
}

func sha1hex(b []byte) string {
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}

// projectRoot is the nearest ancestor holding .git (a directory or a worktree
// file), else dir itself, always as a physical path.
func projectRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	phys, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	for d := phys; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return phys, nil
		}
		d = parent
	}
}

// Ack writes the marker and prints the pipeline reminder to w.
func Ack(w io.Writer, o Options) error {
	if o.Surface == "" {
		return errors.New("usage: aval design ack <surface>   e.g. aval design ack pricing-table")
	}
	root, err := projectRoot(o.Dir)
	if err != nil {
		return err
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	ttl := o.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	dhash, dnote := "", "no .agents/DESIGN.md yet - create it with the first decision"
	if b, err := os.ReadFile(filepath.Join(root, ".agents", "DESIGN.md")); err == nil {
		dhash, dnote = sha1hex(b), ".agents/DESIGN.md (hash locked in)"
	}

	dir := filepath.Join(o.Cache, "design-gate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	marker := fmt.Sprintf("surface=%s\nepoch=%d\ndesign_sha1=%s\n", o.Surface, now.Unix(), dhash)
	if err := os.WriteFile(filepath.Join(dir, sha1hex([]byte(root))), []byte(marker), 0o644); err != nil {
		return err
	}

	fmt.Fprintf(w, "design-ack: %s recorded for %s (valid %d min, until DESIGN.md changes)\n", o.Surface, root, ttl/60)
	fmt.Fprintf(w, `Pipeline, in order:
  1. laws       - which Laws of UX govern this surface and the number each forces (laws-of-ux)
  2. direction  - %s; it outranks default taste
  3. gates      - after the code: `+"`aval design gate`"+` (contrast, hardcodes, overflow) and
                  shot.mjs for 390/768/1440 renders; quote real output, unrun = "not run"
`, dnote)
	return nil
}
