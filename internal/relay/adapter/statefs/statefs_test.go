package statefs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

func store(t *testing.T) Store {
	return Store{Dir: t.TempDir(), PID: 100, Start: 1, Alive: func(int, int64) bool { return true }}
}

var at = time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)

func TestJournal_PreparedThenCommitted(t *testing.T) {
	s := store(t)
	if err := s.Prepare("s1", Entry{Trigger: "auto", TokensBefore: 212_000, At: at}); err != nil {
		t.Fatal(err)
	}
	if e, err := s.Read("s1"); err != nil || e.State != Prepared || e.TokensBefore != 212_000 {
		t.Fatalf("after prepare: %+v, %v", e, err)
	}
	if err := s.Commit("s1", 61_000); err != nil {
		t.Fatal(err)
	}
	if e, err := s.Read("s1"); err != nil || e.State != Committed || e.TokensAfter != 61_000 {
		t.Fatalf("after commit: %+v, %v", e, err)
	}
}

func TestJournal_VoidOnResume(t *testing.T) {
	s := store(t)
	if err := s.Prepare("s1", Entry{Trigger: "auto", At: at}); err != nil {
		t.Fatal(err)
	}
	if err := s.VoidPrepared("s1"); err != nil {
		t.Fatal(err)
	}
	if e, err := s.Read("s1"); err != nil || e.State != Void {
		t.Fatalf("got %+v, %v; want void", e, err)
	}
}

func TestJournal_ReadMissing(t *testing.T) {
	if _, err := store(t).Read("none"); !errors.Is(err, ErrNoEntry) {
		t.Fatalf("got %v, want ErrNoEntry", err)
	}
}

func TestLock_SecondRelaySkips(t *testing.T) {
	s := store(t)
	release, err := s.Lock("s1")
	if err != nil {
		t.Fatal(err)
	}
	other := s
	other.PID = 200
	if _, err := other.Lock("s1"); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock: got %v, want ErrLocked", err)
	}
	release()
	if _, err := other.Lock("s1"); err != nil {
		t.Fatalf("lock after release: %v", err)
	}
}

func TestLock_StaleReclaimed(t *testing.T) {
	s := store(t)
	if _, err := s.Lock("s1"); err != nil {
		t.Fatal(err)
	}
	other := s
	other.PID = 200
	other.Alive = func(pid int, start int64) bool { return pid != 100 } // the holder died
	if _, err := other.Lock("s1"); err != nil {
		t.Fatalf("stale lock not reclaimed: %v", err)
	}
}

func TestSessionIDMustBeAFileName(t *testing.T) {
	s := store(t)
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`} {
		if err := s.Prepare(id, Entry{}); !errors.Is(err, ErrBadSessionID) {
			t.Errorf("Prepare(%q): got %v, want ErrBadSessionID", id, err)
		}
	}
}

func TestSummary_RoundTripsOneLine(t *testing.T) {
	s := store(t)
	want := Summary{Relays: 3, Before: 212_000, After: 61_000, Floor: 58_000, Trigger: "auto"}
	if err := s.WriteSummary("s1", want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Dir, "sessions", "s1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "relays=3 before=212000 after=61000 floor=58000 trigger=auto\n" {
		t.Fatalf("summary line: %q", raw)
	}
	if got, err := s.ReadSummary("s1"); err != nil || got != want {
		t.Fatalf("read back %+v, %v; want %+v", got, err, want)
	}
}

func TestAtomicWrite_NoPartialFile(t *testing.T) {
	s := store(t)
	for i := range 3 {
		if err := s.WriteSummary("s1", Summary{Relays: i}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(s.Dir, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "s1" {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestStateFilesAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX file modes")
	}
	s := store(t)
	if err := s.Prepare("s1", Entry{At: at}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteSummary("s1", Summary{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join("journal", "s1.json"), filepath.Join("sessions", "s1")} {
		fi, err := os.Stat(filepath.Join(s.Dir, p))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, want 0600", p, fi.Mode().Perm())
		}
	}
}

func TestChain_AppendAndRead(t *testing.T) {
	s := store(t)
	if c, err := s.ReadChain("s1"); err != nil || c.Epoch() != 0 {
		t.Fatalf("fresh chain: epoch %d, %v", c.Epoch(), err)
	}
	for i, fp := range []string{"a", "b"} {
		if err := s.AppendLink("s1", relay.Link{SessionID: "s1", Fingerprint: fp, Turn: 10 * (i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	c, err := s.ReadChain("s1")
	if err != nil || c.Epoch() != 2 || c.Links[1].Fingerprint != "b" {
		t.Fatalf("got %+v, %v", c, err)
	}
}
