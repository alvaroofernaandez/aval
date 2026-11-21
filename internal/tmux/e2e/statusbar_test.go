//go:build !windows

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// `aval tmux statusbar` is tmux's status-right renderer. These run the real
// binary against the real host and encode .agents/DESIGN.md: segment colours,
// contrast pairing, the ASCII constraint and the performance budget. It runs on
// every status refresh, so its cost is paid continuously and is part of the
// contract. Fixture-driven Linux and Darwin readings, the cache's mechanics and
// the exact row are covered by the unit tests in internal/tmux.

const (
	statusInk   = "235"
	statusAlert = "167"
	budget      = 100 * time.Millisecond
)

// statusbar runs the binary with a private cache so tests never share one.
func statusbar(t *testing.T, cache string, extra ...string) result {
	t.Helper()
	env := append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TMPDIR=" + os.TempDir(), "STATUSBAR_CACHE=" + cache}, extra...)
	return runCmd(t, env, 30*time.Second, avalBin, "tmux", "statusbar")
}

func stripStyles(s string) string { return regexp.MustCompile(`#\[[^\]]*\]`).ReplaceAllString(s, "") }

func TestStatusbarLive(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the built binary (skipped under -short)")
	}
	cache := filepath.Join(t.TempDir(), "cache")
	r := statusbar(t, cache)
	out := r.out
	plain := stripStyles(out)

	t.Run("output contract", func(t *testing.T) {
		eq(t, "exits successfully", 0, r.rc)
		eq(t, "produces output", true, out != "")
		eq(t, "stays on a single row", false, strings.Contains(out, "\n"))
		labels := []string{"CPU", "RAM"}
		if runtime.GOOS == "darwin" {
			labels = append(labels, "GPU") // there is no GPU reading elsewhere
		}
		for _, l := range labels {
			wantText(t, "labels the "+l+" segment", l, out)
		}
	})

	// DESIGN.md section 2: one colour per segment, all on ink text. A metric
	// over its threshold renders in the alert colour rather than its role
	// colour, which is the design and not a fault; asserting only on the role
	// colour would depend on how busy the machine happens to be.
	t.Run("one colour per segment, on ink text", func(t *testing.T) {
		roles := []string{"110", "108", "179", "174"} // CPU RAM time date
		if runtime.GOOS == "darwin" {
			roles = append(roles, "140") // GPU
		}
		for _, role := range roles {
			ok := strings.Contains(out, "bg=colour"+role) || strings.Contains(out, "bg=colour"+statusAlert)
			eq(t, "segment background colour"+role+" (or the alert colour) is used", true, ok)
		}
		bg := len(regexp.MustCompile(`bg=colour[0-9]+`).FindAllString(out, -1))
		ink := strings.Count(out, "fg=colour"+statusInk)
		eq(t, "every segment pairs its background with ink text", true, bg > 0 && bg == ink)
		eq(t, "resets styling after each segment", true, strings.Count(out, "#[default]") >= bg)
	})

	// DESIGN.md section 7: plain ASCII only, no powerline glyphs.
	t.Run("plain ASCII", func(t *testing.T) { eq(t, "ASCII", true, isASCII(out)) })

	t.Run("values are sane", func(t *testing.T) {
		pct := func(label string) int {
			m := regexp.MustCompile(label + ` ([0-9]+)`).FindStringSubmatch(out)
			if m == nil {
				return -1
			}
			n, _ := strconv.Atoi(m[1])
			return n
		}
		if c := pct("CPU"); c < 0 || c > 100 {
			t.Errorf("CPU percentage %d outside 0-100", c)
		}
		if runtime.GOOS == "darwin" {
			if g := pct("GPU"); g < 0 || g > 100 {
				t.Errorf("GPU percentage %d outside 0-100", g)
			}
			m := regexp.MustCompile(`RAM ([0-9]+\.[0-9])/([0-9]+\.[0-9])G`).FindStringSubmatch(out)
			eq(t, "RAM reported as used/total", true, m != nil)
			if m != nil {
				used, _ := strconv.ParseFloat(m[1], 64)
				total, _ := strconv.ParseFloat(m[2], 64)
				eq(t, "RAM used never exceeds total", true, used <= total)
			}
		}
	})

	t.Run("clock and date", func(t *testing.T) {
		eq(t, "renders a 24h clock", true, regexp.MustCompile(`\b([01][0-9]|2[0-3]):[0-5][0-9]\b`).MatchString(out))
		eq(t, "renders a day and month", true, regexp.MustCompile(`\b[0-9]{1,2} [A-Z][a-z]{2}\b`).MatchString(out))
	})

	// DESIGN.md section 3: width budget. And section 2: the row ends at the last
	// segment. Its inner padding space is required; what must not survive is the
	// separator after it (two spaces at the end of the plain row).
	t.Run("width and trailing space", func(t *testing.T) {
		if n := len(plain); n > 60 {
			t.Errorf("%d columns, want <= 60", n)
		}
		eq(t, "leaves no separator dangling after the last segment", false, strings.HasSuffix(out, "#[default] "))
		eq(t, "keeps exactly one padding space at the end", false, strings.HasSuffix(plain, "  "))
	})

	// tmux inherits whatever environment launched the server. Under es_ES awk
	// emitted a comma decimal separator and date Spanish month names of varying
	// width. Whole runs of digits collapse to one token: the readings change
	// between the two invocations (a GPU going from 23% to 9% would otherwise
	// read as a formatting difference).
	t.Run("locale independence", func(t *testing.T) {
		digits := regexp.MustCompile(`[0-9]+`)
		es := statusbar(t, filepath.Join(t.TempDir(), "c"), "LANG=es_ES.UTF-8", "LC_ALL=es_ES.UTF-8")
		c := statusbar(t, filepath.Join(t.TempDir(), "c"), "LANG=C", "LC_ALL=C")
		eq(t, "renders identically regardless of locale", digits.ReplaceAllString(c.out, "N"), digits.ReplaceAllString(es.out, "N"))
		if runtime.GOOS == "darwin" { // the RAM reading is macOS-only
			eq(t, "uses a dot as decimal separator", true, regexp.MustCompile(`RAM [0-9]+\.[0-9]`).MatchString(es.out))
		}
	})

	// DESIGN.md section 7: performance budget. Best of three: one sample is at
	// the mercy of whatever else the machine is doing, and a real regression
	// fails all three while only noise is filtered out.
	t.Run("performance budget", func(t *testing.T) {
		statusbar(t, cache) // warm
		best := time.Hour
		for i := 0; i < 3; i++ {
			start := time.Now()
			statusbar(t, cache)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		if best >= budget {
			t.Errorf("runs in %v, want under %v", best, budget)
		}
	})

	// A stale cache must not freeze the clock.
	t.Run("cache is created and an expired one is recomputed", func(t *testing.T) {
		info, err := os.Stat(cache)
		eq(t, "the cache file is created", true, err == nil && info.Size() > 0)

		if err := os.WriteFile(cache, []byte("CACHE ANTIGUO\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-24 * time.Hour)
		if err := os.Chtimes(cache, old, old); err != nil {
			t.Fatal(err)
		}
		fresh := statusbar(t, cache)
		eq(t, "an expired cache is recomputed", false, strings.Contains(fresh.out, "CACHE ANTIGUO"))
	})
}

// The expensive calls must stay gone. Comments are stripped: these names appear
// legitimately in the rationale.
func TestStatusbarAvoidsExpensiveCalls(t *testing.T) {
	src := readText(t, filepath.Join(findRoot(t), "internal", "tmux", "statusbar.go"))
	var code []string
	for _, l := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "//") {
			code = append(code, l)
		}
	}
	body := strings.Join(code, "\n")
	for _, c := range []struct{ name, re string }{
		{"does not shell out to top", `\btop\b`},
		{"does not start a python interpreter", `\bpython3?\b`},
		{"does not call powermetrics (needs sudo)", `\bpowermetrics\b`},
	} {
		eq(t, c.name, false, regexp.MustCompile(c.re).MatchString(body))
	}
}

// findRoot is the repository root, also under -short (when nothing else needs it).
func findRoot(t *testing.T) string {
	t.Helper()
	if repoRoot != "" {
		return repoRoot
	}
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
