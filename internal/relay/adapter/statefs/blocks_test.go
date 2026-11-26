package statefs

import (
	"testing"
	"time"
)

func TestBlocks_TakeLetsTheSamePromptThroughOnceWithinTheWindow(t *testing.T) {
	s := store(t)
	if err := s.NoteBlock("s1", "k", at); err != nil {
		t.Fatal(err)
	}
	if s.TakeBlock("s1", "other", 2*time.Minute, at.Add(time.Second)) {
		t.Error("a different prompt was let through")
	}
	if !s.TakeBlock("s1", "k", 2*time.Minute, at.Add(time.Minute)) {
		t.Error("the same prompt within the window was not let through")
	}
	if s.TakeBlock("s1", "k", 2*time.Minute, at.Add(time.Minute)) {
		t.Error("the block was taken twice")
	}
}

func TestBlocks_ExpireAndAreKeyedBySession(t *testing.T) {
	s := store(t)
	_ = s.NoteBlock("s1", "k", at)
	if s.TakeBlock("s2", "k", 2*time.Minute, at) {
		t.Error("another session took the block")
	}
	if s.TakeBlock("s1", "k", 2*time.Minute, at.Add(3*time.Minute)) {
		t.Error("an expired block was taken")
	}
	if s.TakeBlock("../x", "k", 2*time.Minute, at) {
		t.Error("a bad session id was accepted")
	}
}
