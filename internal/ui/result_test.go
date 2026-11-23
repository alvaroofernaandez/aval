package ui

import "testing"

func TestCompletedIsFalseWhenQuitOnSelect(t *testing.T) {
	m := send(testModel(t), "q")
	if !m.Quitting {
		t.Fatal("q on the select screen must set Quitting")
	}
	if m.Completed() {
		t.Error("Completed() = true after quitting on the select screen")
	}
}

func TestCompletedIsFalseBeforeTheRunFinishes(t *testing.T) {
	m := testModel(t)
	if m.Completed() {
		t.Error("Completed() = true on a fresh model")
	}
	next, _ := m.Update(key("enter"))
	if next.(Model).Completed() {
		t.Error("Completed() = true while the install screen is still applying")
	}
}

func TestCompletedIsTrueOnceTheRunReachesDone(t *testing.T) {
	m := testModel(t)
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	for i := 0; i < 10 && cmd != nil; i++ {
		msg := cmd()
		next, cmd = m.Update(msg)
		m = next.(Model)
		if m.Screen() == ScreenDone {
			break
		}
	}
	if m.Screen() != ScreenDone {
		t.Fatalf("screen = %v, want done", m.Screen())
	}
	if !m.Completed() {
		t.Error("Completed() = false after every action was applied")
	}
	if !send(m, "q").Completed() {
		t.Error("quitting from the done screen must keep Completed() true")
	}
}
