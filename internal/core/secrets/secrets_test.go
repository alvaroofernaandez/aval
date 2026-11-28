package secrets

import (
	"errors"
	"slices"
	"testing"
)

func TestMemoryRoundTrip(t *testing.T) {
	m := NewMemory()
	if err := m.Set("GH_TOKEN", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if got, err := m.Get("GH_TOKEN"); err != nil || got != "s3cret" {
		t.Errorf("Get = %q, %v; want the stored value", got, err)
	}
	if err := m.Set("A_KEY", "x"); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.List(); !slices.Equal(got, []string{"A_KEY", "GH_TOKEN"}) {
		t.Errorf("List = %v, want sorted names", got)
	}
	if err := m.Delete("GH_TOKEN"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("GH_TOKEN"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	if err := m.Delete("GH_TOKEN"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete of a missing name: err = %v, want ErrNotFound", err)
	}
}

func TestMemoryRejectsBadNames(t *testing.T) {
	m := NewMemory()
	for _, n := range []string{"", "a b", "a/b", "a=b", "-x", "$(id)", "a\nb"} {
		if err := m.Set(n, "v"); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Set(%q): err = %v, want ErrInvalidName", n, err)
		}
		if _, err := m.Get(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Get(%q): err = %v, want ErrInvalidName", n, err)
		}
	}
	if !ValidName("GH_TOKEN") || !ValidName("a.b-c_1") {
		t.Error("legal names rejected")
	}
}

func TestErrorsNeverCarryTheValue(t *testing.T) {
	m := NewMemory()
	_ = m.Set("K", "SENTINEL-VALUE")
	_, err := m.Get("MISSING")
	if err != ErrNotFound {
		t.Errorf("err = %v, want the bare sentinel, which cannot carry a value", err)
	}
}

func contains(s, sub string) bool { return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestChildEnvAddsValuesOnlyToTheCopy(t *testing.T) {
	m := NewMemory()
	_ = m.Set("A", "1")
	_ = m.Set("B", "2")
	base := []string{"PATH=/bin"}
	got, err := ChildEnv(m, []string{"A", "B"}, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"PATH=/bin", "A=1", "B=2"}; !slices.Equal(got, want) {
		t.Errorf("env = %v, want %v", got, want)
	}
	if len(base) != 1 {
		t.Errorf("base modified: %v", base)
	}
}

func TestChildEnvMissingNameFails(t *testing.T) {
	if _, err := ChildEnv(NewMemory(), []string{"NOPE"}, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestChildEnvMapsAnEnvVarToASecretName(t *testing.T) {
	m := NewMemory()
	_ = m.Set("srv.TOKEN", "v1")
	got, err := ChildEnv(m, []string{"API_TOKEN=srv.TOKEN"}, []string{"PATH=/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"PATH=/bin", "API_TOKEN=v1"}; !slices.Equal(got, want) {
		t.Errorf("env = %v, want %v", got, want)
	}
	if _, err := ChildEnv(m, []string{"=srv.TOKEN"}, nil); !errors.Is(err, ErrInvalidName) {
		t.Errorf("empty env var: err = %v, want ErrInvalidName", err)
	}
}
