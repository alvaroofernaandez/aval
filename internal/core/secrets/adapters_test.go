package secrets

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

type call struct {
	name  string
	args  []string
	stdin string
}

type fakeRun struct {
	calls []call
	out   string
	err   error
}

func (f *fakeRun) run(name string, args []string, stdin string) (string, error) {
	f.calls = append(f.calls, call{name, args, stdin})
	return f.out, f.err
}

func TestMacSetSendsTheValueOverStdinNotArgv(t *testing.T) {
	f := &fakeRun{}
	s := &macStore{run: f.run}
	if err := s.Set("GH_TOKEN", `se"cr\et`); err != nil {
		t.Fatal(err)
	}
	c := f.calls[0]
	if c.name != "security" || !slices.Equal(c.args, []string{"-i"}) {
		t.Errorf("call = %s %v, want security -i", c.name, c.args)
	}
	want := `add-generic-password -U -a aval-secrets -s GH_TOKEN -w "se\"cr\\et"` + "\n"
	if c.stdin != want {
		t.Errorf("stdin = %q, want %q", c.stdin, want)
	}
	for _, a := range c.args {
		if strings.Contains(a, "cr") {
			t.Error("value in argv")
		}
	}
}

func TestMacSetRejectsNewlines(t *testing.T) {
	if err := (&macStore{run: (&fakeRun{}).run}).Set("K", "a\nb"); err == nil {
		t.Error("a value with a newline must be rejected")
	}
}

func TestMacGetMapsNotFound(t *testing.T) {
	f := &fakeRun{err: &exitError{code: 44}}
	if _, err := (&macStore{run: f.run}).Get("K"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	f = &fakeRun{out: "tok\n"}
	if v, err := (&macStore{run: f.run}).Get("K"); err != nil || v != "tok" {
		t.Errorf("Get = %q, %v", v, err)
	}
	if got := f.calls[0].args; !slices.Equal(got, []string{"find-generic-password", "-a", "aval-secrets", "-s", "K", "-w"}) {
		t.Errorf("args = %v", got)
	}
}

func TestLinuxStoreUsesStdinAndMapsMissingBinary(t *testing.T) {
	f := &fakeRun{}
	s := &linuxStore{run: f.run}
	if err := s.Set("K", "v"); err != nil {
		t.Fatal(err)
	}
	c := f.calls[0]
	if c.name != "secret-tool" || c.stdin != "v" || slices.Contains(c.args, "v") {
		t.Errorf("call = %+v: value must be stdin only", c)
	}
	missing := &fakeRun{err: ErrUnavailable}
	if _, err := (&linuxStore{run: missing.run}).Get("K"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	empty := &fakeRun{err: &exitError{code: 1}}
	if _, err := (&linuxStore{run: empty.run}).Get("K"); !errors.Is(err, ErrNotFound) {
		t.Errorf("lookup exit 1: err = %v, want ErrNotFound", err)
	}
}

func TestIndexedKeepsNamesInOneEntry(t *testing.T) {
	inner := NewMemory()
	s := Indexed(inner)
	_ = s.Set("B", "2")
	_ = s.Set("A", "1")
	_ = s.Set("A", "1b")
	if got, _ := s.List(); !slices.Equal(got, []string{"A", "B"}) {
		t.Errorf("List = %v", got)
	}
	_ = s.Delete("A")
	if got, _ := s.List(); !slices.Equal(got, []string{"B"}) {
		t.Errorf("List after delete = %v", got)
	}
	if err := s.Set(indexName, "x"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("the index name must be reserved, err = %v", err)
	}
}
