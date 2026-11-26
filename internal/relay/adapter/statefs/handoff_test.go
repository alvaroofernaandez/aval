package statefs

import (
	"testing"
	"time"
)

func TestHandoff_TakenOnceByCwdWithinTheAge(t *testing.T) {
	s := store(t)
	if err := s.PutHandoff("/work/app", "capsule text", at); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.TakeHandoff("/work/other", at, 10*time.Minute); ok {
		t.Error("another cwd took the handoff")
	}
	got, ok := s.TakeHandoff("/work/app/", at.Add(5*time.Minute), 10*time.Minute)
	if !ok || got != "capsule text" {
		t.Fatalf("got %q %v, want the text", got, ok)
	}
	if _, ok := s.TakeHandoff("/work/app", at.Add(5*time.Minute), 10*time.Minute); ok {
		t.Error("the handoff was taken twice")
	}
}

func TestHandoff_StaleIsIgnoredAndDeleted(t *testing.T) {
	s := store(t)
	_ = s.PutHandoff("/work/app", "old", at)
	if _, ok := s.TakeHandoff("/work/app", at.Add(11*time.Minute), 10*time.Minute); ok {
		t.Error("a stale handoff was returned")
	}
	if _, ok := s.TakeHandoff("/work/app", at.Add(time.Minute), 10*time.Minute); ok {
		t.Error("a stale handoff was kept")
	}
}
