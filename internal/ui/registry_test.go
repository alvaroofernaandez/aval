package ui

import "testing"

// withViews swaps the registry for the length of a test.
func withViews(t *testing.T) {
	t.Helper()
	saved := viewCtors
	viewCtors = map[string]viewCtor{}
	t.Cleanup(func() { viewCtors = saved })
}

func TestRegisteredViewIsAvailable(t *testing.T) {
	withViews(t)
	registerView("about", func(Services) screen { return newFake("about") })
	ctor, ok := viewFor("about")
	if !ok || ctor == nil {
		t.Fatalf("viewFor(about) = %v, want the registered constructor", ok)
	}
	if s, ok := ctor(Services{}).(fakeScreen); !ok || s.name != "about" {
		t.Errorf("the constructor built %v, want the about screen", s)
	}
}

func TestUnregisteredViewIsUnavailable(t *testing.T) {
	withViews(t)
	if _, ok := viewFor("doctor"); ok {
		t.Errorf("viewFor(doctor) is available with nothing registered")
	}
}

func TestRegisteringTwicePanics(t *testing.T) {
	withViews(t)
	registerView("about", func(Services) screen { return newFake("about") })
	defer func() {
		if recover() == nil {
			t.Errorf("a second registerView(about) did not panic")
		}
	}()
	registerView("about", func(Services) screen { return newFake("about") })
}
