package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

func needLinks(t *testing.T) {
	t.Helper()
	testenv.NeedSymlinks(t)
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs Developer Mode on Windows")
	}
}

func TestStateString(t *testing.T) {
	for s, want := range map[State]string{StateNew: "new", StateLinked: "linked", StateReplaces: "replaces a file"} {
		if got := s.String(); got != want {
			t.Errorf("State(%d) = %q, want %q", s, got, want)
		}
	}
}

func TestPreviewAbsentIsNew(t *testing.T) {
	_, home, act := fixture(t)
	r := New(Options{Home: home, Symlinks: true})
	if got := r.Preview(act); got != StateNew {
		t.Errorf("Preview = %v, want new", got)
	}
}

func TestPreviewLinkToSourceIsLinked(t *testing.T) {
	needLinks(t)
	_, home, act := fixture(t)
	if err := os.Symlink(act.Source, act.Dest); err != nil {
		t.Fatal(err)
	}
	r := New(Options{Home: home, Symlinks: true})
	if got := r.Preview(act); got != StateLinked {
		t.Errorf("Preview = %v, want linked", got)
	}
}

func TestPreviewForeignLinkReplaces(t *testing.T) {
	needLinks(t)
	repo, home, act := fixture(t)
	other := filepath.Join(repo, "other")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, act.Dest); err != nil {
		t.Fatal(err)
	}
	r := New(Options{Home: home, Symlinks: true})
	if got := r.Preview(act); got != StateReplaces {
		t.Errorf("Preview = %v, want replaces", got)
	}
}

func TestPreviewRealFileReplaces(t *testing.T) {
	_, home, act := fixture(t)
	if err := os.WriteFile(act.Dest, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(Options{Home: home, Symlinks: true})
	if got := r.Preview(act); got != StateReplaces {
		t.Errorf("Preview = %v, want replaces", got)
	}
}

func TestPreviewBrokenLinkReplaces(t *testing.T) {
	needLinks(t)
	_, home, act := fixture(t)
	if err := os.Symlink(filepath.Join(home, "nowhere"), act.Dest); err != nil {
		t.Fatal(err)
	}
	r := New(Options{Home: home, Symlinks: true})
	if got := r.Preview(act); got != StateReplaces {
		t.Errorf("Preview = %v, want replaces", got)
	}
}

func TestPreviewCopyModeIdenticalIsLinked(t *testing.T) {
	_, home, act := fixture(t)
	if err := os.WriteFile(act.Dest, []byte("# from repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(Options{Home: home, Symlinks: false})
	if got := r.Preview(act); got != StateLinked {
		t.Errorf("Preview = %v, want linked", got)
	}
}

func TestPreviewCopyModeDifferentReplaces(t *testing.T) {
	_, home, act := fixture(t)
	if err := os.WriteFile(act.Dest, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(Options{Home: home, Symlinks: false})
	if got := r.Preview(act); got != StateReplaces {
		t.Errorf("Preview = %v, want replaces", got)
	}
}

func TestPreviewWritesNothing(t *testing.T) {
	repo, home, act := fixture(t)
	if err := os.WriteFile(act.Dest, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := act
	missing.Dest = filepath.Join(home, "deep", "er", "x")
	r := New(Options{Home: home, Symlinks: true})
	beforeRepo, beforeHome := treeOf(t, repo), treeOf(t, home)
	r.Preview(act)
	r.Preview(missing)
	if treeOf(t, repo) != beforeRepo || treeOf(t, home) != beforeHome {
		t.Error("Preview changed the filesystem")
	}
}
