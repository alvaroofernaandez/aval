//go:build integration

package secrets

import "testing"

func TestDefaultRoundTripAgainstTheRealKeychain(t *testing.T) {
	s := Default()
	const n = "aval-it-roundtrip"
	t.Cleanup(func() { _ = s.Delete(n) })
	if err := s.Set(n, `v"a\l`); err != nil {
		t.Skip("keychain not usable here:", err)
	}
	if v, err := s.Get(n); err != nil || v != `v"a\l` {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if names, _ := s.List(); len(names) == 0 {
		t.Error("List is empty after Set")
	}
	if err := s.Delete(n); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(n); err == nil {
		t.Error("still there after Delete")
	}
}
