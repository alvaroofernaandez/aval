package statefs

import (
	"testing"
	"time"
)

func TestHandoff_TakenOnceByCwdWithinTheAge(t *testing.T) {
	s := store(t)
	if err := s.PutHandoff("/work/app", "", "capsule text", at); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.TakeHandoff("/work/other", "", at, 10*time.Minute); ok {
		t.Error("another cwd took the handoff")
	}
	got, ok := s.TakeHandoff("/work/app/", "", at.Add(5*time.Minute), 10*time.Minute)
	if !ok || got != "capsule text" {
		t.Fatalf("got %q %v, want the text", got, ok)
	}
	if _, ok := s.TakeHandoff("/work/app", "", at.Add(5*time.Minute), 10*time.Minute); ok {
		t.Error("the handoff was taken twice")
	}
}

func TestHandoff_StaleIsIgnoredAndDeleted(t *testing.T) {
	s := store(t)
	_ = s.PutHandoff("/work/app", "", "old", at)
	if _, ok := s.TakeHandoff("/work/app", "", at.Add(11*time.Minute), 10*time.Minute); ok {
		t.Error("a stale handoff was returned")
	}
	if _, ok := s.TakeHandoff("/work/app", "", at.Add(time.Minute), 10*time.Minute); ok {
		t.Error("a stale handoff was kept")
	}
}

func TestHandoff_KeyIncludesTheOwner(t *testing.T) {
	s := store(t)
	_ = s.PutHandoff("/work/app", "111", "from A", at)
	_ = s.PutHandoff("/work/app", "222", "from B", at)
	if got, ok := s.TakeHandoff("/work/app", "222", at, time.Hour); !ok || got != "from B" {
		t.Fatalf("owner 222 got %q %v", got, ok)
	}
	if got, ok := s.TakeHandoff("/work/app", "111", at, time.Hour); !ok || got != "from A" {
		t.Fatalf("owner 111 got %q %v", got, ok)
	}
}

func TestHandoff_OwnerReadsTheLegacyCwdOnlyKey(t *testing.T) {
	s := store(t)
	_ = s.PutHandoff("/work/app", "", "legacy", at)
	if got, ok := s.TakeHandoff("/work/app", "333", at, time.Hour); !ok || got != "legacy" {
		t.Fatalf("got %q %v, want the legacy handoff", got, ok)
	}
	_ = s.PutHandoff("/work/app", "444", "keyed", at)
	if _, ok := s.TakeHandoff("/work/app", "", at, time.Hour); ok {
		t.Error("an unowned read took a keyed handoff")
	}
}
