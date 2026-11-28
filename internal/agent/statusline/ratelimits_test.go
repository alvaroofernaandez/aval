package statusline

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func limitsInput(dir string, five, seven string) string {
	return `{"model":{"display_name":"Opus"},"workspace":{"current_dir":"` + filepath.ToSlash(dir) + `"},` +
		`"rate_limits":{"five_hour":{"used_percentage":` + five + `},"seven_day":{"used_percentage":` + seven + `}}}`
}

func TestRateLimitsArePersistedAtomically(t *testing.T) {
	d := repo(t, "main")
	state := t.TempDir()
	RenderWith(strings.NewReader(limitsInput(d, "42.5", "13")), Options{StateDir: state, Now: now})

	p := filepath.Join(state, RateLimitsFile)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("no state file: %v", err)
	}
	for _, want := range []string{`"five_hour":{"used_percentage":42.5}`, `"seven_day":{"used_percentage":13}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("state file %q lacks %s", b, want)
		}
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
		}
	}
	if left, _ := filepath.Glob(filepath.Join(state, "*.tmp*")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestNoRateLimitsWritesNothing(t *testing.T) {
	d := repo(t, "main")
	state := t.TempDir()
	RenderWith(strings.NewReader(statusInput(d)), Options{StateDir: state, Now: now})
	if _, err := os.Stat(filepath.Join(state, RateLimitsFile)); !os.IsNotExist(err) {
		t.Errorf("state file exists without rate limits: %v", err)
	}
}

func TestAnUnchangedFreshReadingIsNotRewritten(t *testing.T) {
	d := repo(t, "main")
	state := t.TempDir()
	in := limitsInput(d, "10", "20")
	RenderWith(strings.NewReader(in), Options{StateDir: state, Now: now})
	p := filepath.Join(state, RateLimitsFile)
	fresh := now.Add(-10 * time.Second)
	if err := os.Chtimes(p, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	RenderWith(strings.NewReader(in), Options{StateDir: state, Now: now})
	if fi, _ := os.Stat(p); !fi.ModTime().Equal(fresh) {
		t.Errorf("unchanged fresh file was rewritten (mtime %v, want %v)", fi.ModTime(), fresh)
	}

	old := now.Add(-5 * time.Minute)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	RenderWith(strings.NewReader(in), Options{StateDir: state, Now: now})
	if fi, _ := os.Stat(p); fi.ModTime().Equal(old) {
		t.Error("an old reading was not refreshed, so it would read as stale")
	}

	RenderWith(strings.NewReader(limitsInput(d, "11", "20")), Options{StateDir: state, Now: now})
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), `"used_percentage":11`) {
		t.Errorf("a changed reading was not written: %s", b)
	}
}

func TestAnUnwritableStateDirNeverChangesTheLine(t *testing.T) {
	d := repo(t, "main")
	in := limitsInput(d, "42", "13")
	want := RenderWith(strings.NewReader(in), Options{Now: now})
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := RenderWith(strings.NewReader(in), Options{StateDir: notADir, Now: now}); got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

func BenchmarkRenderWithRateLimits(b *testing.B) {
	d := b.TempDir()
	in := limitsInput(d, "42", "13")
	for _, c := range []struct {
		name  string
		state string
	}{{"off", ""}, {"on", b.TempDir()}} {
		b.Run(c.name, func(b *testing.B) {
			o := Options{StateDir: c.state, Now: now}
			for b.Loop() {
				RenderWith(strings.NewReader(in), o)
			}
		})
	}
}
