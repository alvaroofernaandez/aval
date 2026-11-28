package sync

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite internal/core/sync/testdata/parity/golden")

// TestSyncGolden pins what aval sync writes for a frozen copy of the repo's
// Claude settings, a live file with personal keys and a private overlay, on
// every OS flavour. The goldens were captured before sync moved onto the
// agentcfg Claude adapter (#9); the refactor must leave them byte for byte.
func TestSyncGolden(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			home := t.TempDir()
			copyTree(t, filepath.Join("testdata", "parity", "home"), home)
			opt := Options{
				Repo:       filepath.Join("testdata", "parity", "repo"),
				Home:       home,
				PrivateDir: filepath.Join("testdata", "parity", "private"),
				GOOS:       goos,
				AvalBin:    `C:\Users\me\.local\bin\aval.exe`,
				Now:        fixedNow,
			}
			rep, err := Run(opt)
			if err != nil {
				t.Fatal(err)
			}
			report := fmt.Sprintf("changed: %s\ncreated: %v\nwritten: %v\nwarnings:\n%s\n",
				strings.Join(rep.Settings.Changed, ", "), rep.Settings.Created, rep.Settings.Written,
				strings.Join(rep.Settings.Warnings, "\n"))
			for _, r := range rep.Rules {
				rel, _ := filepath.Rel(home, r.Path)
				report += fmt.Sprintf("rules %s changed=%v skipped=%v\n", filepath.ToSlash(rel), r.Changed, r.Skipped)
			}
			golden(t, goos+".report.txt", report)
			golden(t, goos+".settings.json", read(t, filepath.Join(home, ".claude", "settings.json")))
			golden(t, goos+".codex-AGENTS.md", read(t, filepath.Join(home, ".codex", "AGENTS.md")))
		})
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "parity", "golden", name)
	if *updateGolden {
		write(t, path, got)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/core/sync -run TestSyncGolden -update)", err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("%s differs from the golden captured before the refactor:\n got: %q\nwant: %q", name, got, want)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		write(t, filepath.Join(dst, rel), string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// BenchmarkSync is the #9 budget: a full sync (load, render through the Claude
// adapter, merge, write) of the parity fixture under 1 s.
func BenchmarkSync(b *testing.B) {
	for b.Loop() {
		home := b.TempDir()
		opt := Options{Repo: filepath.Join("testdata", "parity", "repo"), Home: home,
			PrivateDir: filepath.Join("testdata", "parity", "private"), GOOS: "linux", Now: fixedNow}
		if _, err := Run(opt); err != nil {
			b.Fatal(err)
		}
	}
}
