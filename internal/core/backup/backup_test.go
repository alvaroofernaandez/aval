package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need Developer Mode on Windows")
	}
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, path string) {
	t.Helper()
	testenv.NeedSymlinks(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// tree describes a directory byte for byte, ignoring the backup store itself.
func tree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if rel == ".aval-backup" {
			return filepath.SkipDir
		}
		fi, _ := os.Lstat(p)
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			tg, _ := os.Readlink(p)
			lines = append(lines, fmt.Sprintf("L %s -> %s", rel, tg))
		case fi.IsDir():
			lines = append(lines, fmt.Sprintf("D %s %o", rel, fi.Mode().Perm()))
		default:
			b, _ := os.ReadFile(p)
			h := sha256.Sum256(b)
			lines = append(lines, fmt.Sprintf("F %s %o %s", rel, fi.Mode().Perm(), hex.EncodeToString(h[:])))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// clock hands out one new second per call so every run gets its own ID.
func clock() func() time.Time {
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	n := 0
	return func() time.Time { n++; return t0.Add(time.Duration(n) * time.Second) }
}

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	home := t.TempDir()
	s := NewStore(home)
	s.Now = clock()
	return s, home
}

func TestRestoreRoundTripIsByteIdentical(t *testing.T) {
	skipWindows(t)
	s, home := newStore(t)
	repo := t.TempDir()
	put(t, filepath.Join(home, ".zshrc"), "mine\n")
	put(t, filepath.Join(home, ".config/tmux/tmux.conf"), "set -g x\n")
	link(t, "/somewhere/else", filepath.Join(home, ".gitconfig"))
	before := tree(t, home)

	rec := s.Begin()
	for _, name := range []string{".zshrc", ".config/tmux", ".gitconfig", ".config/new"} {
		p := filepath.Join(home, name)
		if _, err := rec.Displace(p); err != nil {
			t.Fatalf("Displace %s: %v", name, err)
		}
		link(t, filepath.Join(repo, "src"), p)
		if err := rec.Wrote(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := rec.Commit(); err != nil {
		t.Fatal(err)
	}

	run, err := s.Resolve("latest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Restore(run, RestoreOptions{Repo: repo}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if after := tree(t, home); after != before {
		t.Errorf("tree differs after restore\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestRestoreRemovesOnlyWhatAvalWrote(t *testing.T) {
	testenv.NeedSymlinks(t)
	skipWindows(t)
	s, home := newStore(t)
	repo := t.TempDir()
	rec := s.Begin()
	paths := map[string]string{}
	for _, n := range []string{"repolink", "wrotefile", "editedfile", "foreignlink"} {
		paths[n] = filepath.Join(home, n)
		if _, err := rec.Displace(paths[n]); err != nil {
			t.Fatal(err)
		}
	}
	link(t, filepath.Join(repo, "x"), paths["repolink"])
	put(t, paths["wrotefile"], "aval wrote this")
	put(t, paths["editedfile"], "aval wrote this")
	link(t, filepath.Join(repo, "y"), paths["foreignlink"])
	for _, p := range paths {
		if err := rec.Wrote(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := rec.Commit(); err != nil {
		t.Fatal(err)
	}
	// After the run: the user edits one file and re-points one link.
	put(t, paths["editedfile"], "user edit")
	os.Remove(paths["foreignlink"])
	link(t, "/user/target", paths["foreignlink"])

	run, _ := s.Resolve("latest")
	if _, err := s.Restore(run, RestoreOptions{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"repolink", "wrotefile"} {
		if _, err := os.Lstat(paths[gone]); err == nil {
			t.Errorf("%s should have been removed", gone)
		}
	}
	if b, _ := os.ReadFile(paths["editedfile"]); string(b) != "user edit" {
		t.Errorf("edited file was touched: %q", b)
	}
	if tg, _ := os.Readlink(paths["foreignlink"]); tg != "/user/target" {
		t.Errorf("foreign link was touched: %q", tg)
	}
}

func TestRestoreKeepsLaterEditsInDisplaced(t *testing.T) {
	s, home := newStore(t)
	p := filepath.Join(home, "settings.json")
	put(t, p, "original")
	rec := s.Begin()
	if _, err := rec.Snapshot(p); err != nil {
		t.Fatal(err)
	}
	put(t, p, "aval wrote")
	if err := rec.Wrote(p); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rec.Commit(); err != nil {
		t.Fatal(err)
	}
	put(t, p, "edited afterwards")

	run, _ := s.Resolve("latest")
	if _, err := s.Restore(run, RestoreOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "original" {
		t.Errorf("restored content = %q, want original", b)
	}
	var kept bool
	filepath.WalkDir(run.Dir, func(q string, d fs.DirEntry, _ error) error {
		if d != nil && !d.IsDir() && strings.Contains(q, ".displaced") {
			if b, _ := os.ReadFile(q); string(b) == "edited afterwards" {
				kept = true
			}
		}
		return nil
	})
	if !kept {
		t.Error("the later edit was not preserved under .displaced")
	}
}

func TestIdenticalRunIsSkipped(t *testing.T) {
	s, home := newStore(t)
	p := filepath.Join(home, "f")
	apply := func() (Run, bool) {
		put(t, p, "same")
		rec := s.Begin()
		if _, err := rec.Snapshot(p); err != nil {
			t.Fatal(err)
		}
		put(t, p, "new")
		rec.Wrote(p)
		run, skipped, err := rec.Commit()
		if err != nil {
			t.Fatal(err)
		}
		return run, skipped
	}
	first, skipped := apply()
	if skipped {
		t.Fatal("first run must not be skipped")
	}
	second, skipped := apply()
	if !skipped || second.ID != first.ID {
		t.Errorf("second run: skipped=%v id=%q, want skipped reusing %q", skipped, second.ID, first.ID)
	}
	runs, _ := s.List()
	if len(runs) != 1 {
		t.Errorf("%d runs on disk, want 1", len(runs))
	}
}

func makeRuns(t *testing.T, s *Store, home string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		p := filepath.Join(home, "f")
		put(t, p, fmt.Sprintf("v%d-%d", i, time.Now().UnixNano()))
		rec := s.Begin()
		if _, err := rec.Snapshot(p); err != nil {
			t.Fatal(err)
		}
		if _, _, err := rec.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetentionKeepsNewestFive(t *testing.T) {
	s, home := newStore(t)
	makeRuns(t, s, home, 8)
	runs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 5 {
		t.Fatalf("%d runs kept, want 5", len(runs))
	}
	if !runs[0].Time.After(runs[4].Time) {
		t.Errorf("runs are not newest first: %v .. %v", runs[0].Time, runs[4].Time)
	}
}

func TestPinnedRunSurvivesRetention(t *testing.T) {
	s, home := newStore(t)
	makeRuns(t, s, home, 1)
	first, _ := s.Resolve("latest")
	if err := s.Pin(first.ID, true); err != nil {
		t.Fatal(err)
	}
	makeRuns(t, s, home, 9)
	runs, _ := s.List()
	if len(runs) != 6 {
		t.Fatalf("%d runs, want 5 unpinned + 1 pinned", len(runs))
	}
	var pinned bool
	for _, r := range runs {
		if r.ID == first.ID && r.Pinned {
			pinned = true
		}
	}
	if !pinned {
		t.Error("pinned run was pruned")
	}
}

func TestListIncludesLegacyDirsAndNeverPrunesThem(t *testing.T) {
	s, home := newStore(t)
	legacy := filepath.Join(home, ".aval-backup", "20250101120000")
	put(t, filepath.Join(legacy, ".zshrc"), "old")
	makeRuns(t, s, home, 7)
	runs, _ := s.List()
	var found *Run
	for i := range runs {
		if runs[i].ID == "20250101120000" {
			found = &runs[i]
		}
	}
	if found == nil || !found.Legacy {
		t.Fatalf("legacy dir not listed as legacy: %+v", runs)
	}
	if _, err := s.Restore(*found, RestoreOptions{}); err == nil {
		t.Error("restoring a legacy dir without manifest must error")
	}
}

func TestResolve(t *testing.T) {
	s, home := newStore(t)
	makeRuns(t, s, home, 2)
	runs, _ := s.List()
	for _, ref := range []string{"latest", runs[0].ID} {
		got, err := s.Resolve(ref)
		if err != nil || got.ID != runs[0].ID {
			t.Errorf("Resolve(%q) = %q, %v; want %q", ref, got.ID, err, runs[0].ID)
		}
	}
	if _, err := s.Resolve(runs[0].ID[:8]); err == nil {
		t.Error("an ambiguous prefix must error")
	}
	if _, err := s.Resolve("nope"); err == nil {
		t.Error("unknown id must error")
	}
}

func TestPendingMarker(t *testing.T) {
	s, home := newStore(t)
	p := filepath.Join(home, "f")
	put(t, p, "x")
	rec := s.Begin()
	if err := rec.Open(); err != nil {
		t.Fatal(err)
	}
	if err := rec.SetPending(); err != nil {
		t.Fatal(err)
	}
	rec.Snapshot(p)
	pend, _ := s.Pending()
	if len(pend) != 1 {
		t.Fatalf("pending = %d, want 1", len(pend))
	}
	if _, _, err := rec.Commit(); err != nil {
		t.Fatal(err)
	}
	if pend, _ := s.Pending(); len(pend) != 0 {
		t.Error("commit must clear the pending marker")
	}
}

func TestCreatedDirsAreRemovedOnlyWhenEmpty(t *testing.T) {
	s, home := newStore(t)
	rec := s.Begin()
	deep := filepath.Join(home, "a", "b", "c")
	if err := rec.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(deep, "f")
	rec.Displace(file)
	put(t, file, "aval")
	rec.Wrote(file)
	if _, _, err := rec.Commit(); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(home, "a", "keep"), "user file")
	run, _ := s.Resolve("latest")
	if _, err := s.Restore(run, RestoreOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "a", "b")); err == nil {
		t.Error("empty created dirs a/b should be gone")
	}
	if _, err := os.Stat(filepath.Join(home, "a", "keep")); err != nil {
		t.Error("a non-empty dir and its user file must survive")
	}
}
