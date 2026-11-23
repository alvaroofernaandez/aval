package deps

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// entry is a valid manifest section; tests change one line of it.
func entry(name string, override ...string) string {
	fields := map[string]string{
		"layer":             "harness",
		"used_by":           "install, hook pre-push",
		"if_missing":        "degrades",
		"native_substitute": "none",
		"probe":             "bin " + name,
	}
	for _, o := range override {
		k, v, _ := strings.Cut(o, "=")
		fields[k] = v
	}
	var b strings.Builder
	b.WriteString("[" + name + "]\n")
	for _, k := range []string{"layer", "used_by", "if_missing", "native_substitute", "probe"} {
		if v, ok := fields[k]; ok && v != "-" {
			b.WriteString(k + " = " + v + "\n")
		}
	}
	return b.String()
}

func TestParseReadsAnEntry(t *testing.T) {
	m, err := Parse(strings.NewReader("# comment\n" + entry("git", "if_missing=blocks", "probe=bin git --version") + "\n[personal]\nnotion\nwhisky\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("entries = %+v", m.Entries)
	}
	e := m.Entries[0]
	if e.Name != "git" || e.Layer != Harness || e.IfMissing != Blocks || e.NativeSubstitute != "none" {
		t.Errorf("entry = %+v", e)
	}
	if strings.Join(e.UsedBy, "|") != "install|hook pre-push" {
		t.Errorf("used_by = %q", e.UsedBy)
	}
	if e.Probe.Kind != ProbeBin || e.Probe.Target != "git" || strings.Join(e.Probe.Args, " ") != "--version" {
		t.Errorf("probe = %+v", e.Probe)
	}
	if strings.Join(m.Personal, " ") != "notion whisky" {
		t.Errorf("personal = %q", m.Personal)
	}
}

func TestParseRejectsAnUnknownLayer(t *testing.T) {
	_, err := Parse(strings.NewReader(entry("x", "layer=kernel")))
	if !errors.Is(err, ErrUnknownLayer) {
		t.Fatalf("want ErrUnknownLayer, got %v", err)
	}
}

func TestParseRejectsAnUnknownIfMissing(t *testing.T) {
	_, err := Parse(strings.NewReader(entry("x", "if_missing=explodes")))
	if !errors.Is(err, ErrUnknownIfMissing) {
		t.Fatalf("want ErrUnknownIfMissing, got %v", err)
	}
}

func TestParseRequiresANativeSubstitute(t *testing.T) {
	for _, c := range []struct {
		value string
		want  error
	}{
		{"-", ErrNativeSubstitute},
		{"", ErrNativeSubstitute},
		{"later", ErrNativeSubstitute},
		{"none", nil},
		{"#6", nil},
	} {
		_, err := Parse(strings.NewReader(entry("x", "native_substitute="+c.value)))
		if !errors.Is(err, c.want) {
			t.Errorf("native_substitute %q: want %v, got %v", c.value, c.want, err)
		}
	}
}

func TestParseRejectsAMalformedManifest(t *testing.T) {
	for _, c := range []struct {
		name, text string
		want       error
	}{
		{"missing probe", entry("x", "probe=-"), ErrMissingField},
		{"missing used_by", entry("x", "used_by=-"), ErrMissingField},
		{"unknown probe kind", entry("x", "probe=url x"), ErrBadProbe},
		{"probe without a target", entry("x", "probe=bin"), ErrBadProbe},
		{"unknown key", entry("x") + "colour = red\n", ErrUnknownKey},
		{"duplicate entry", entry("x") + entry("x"), ErrDuplicate},
		{"personal also an entry", entry("x") + "[personal]\nx\n", ErrDuplicate},
		{"line before any section", "layer = env\n", errOutsideSection},
	} {
		if _, err := Parse(strings.NewReader(c.text)); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
	}
}

func TestParseLayer(t *testing.T) {
	for _, l := range Layers {
		if got, err := ParseLayer(string(l)); err != nil || got != l {
			t.Errorf("ParseLayer(%q) = %q, %v", l, got, err)
		}
	}
	if _, err := ParseLayer("kernel"); !errors.Is(err, ErrUnknownLayer) {
		t.Errorf("want ErrUnknownLayer, got %v", err)
	}
}

func TestEmbeddedManifestIsValid(t *testing.T) {
	m, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) < 10 {
		t.Fatalf("the inventory has %d entries", len(m.Entries))
	}
	seen := map[string]bool{}
	for _, e := range m.Entries {
		if seen[e.Name] {
			t.Errorf("%s: listed twice", e.Name)
		}
		seen[e.Name] = true
		if e.Layer == "" || len(e.UsedBy) == 0 || e.IfMissing == "" || e.NativeSubstitute == "" || e.Probe.Target == "" {
			t.Errorf("%s: incomplete entry %+v", e.Name, e)
		}
	}
	for _, p := range m.Personal {
		if seen[p] {
			t.Errorf("%s: both an entry and personal", p)
		}
	}
}

// Owner decision D9: only git blocks.
func TestOnlyGitBlocks(t *testing.T) {
	m, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	var blocking []string
	for _, e := range m.Entries {
		if e.IfMissing == Blocks {
			blocking = append(blocking, e.Name)
		}
	}
	if strings.Join(blocking, " ") != "git" {
		t.Errorf("blocking entries = %q, want only git", blocking)
	}
}

func TestFilterByLayer(t *testing.T) {
	es := []Entry{{Name: "a", Layer: Harness}, {Name: "b", Layer: Env}, {Name: "c", Layer: Harness}}
	var names []string
	for _, e := range Filter(es, Harness) {
		names = append(names, e.Name)
	}
	if strings.Join(names, " ") != "a c" {
		t.Errorf("Filter(harness) = %q", names)
	}
	if got := Filter(es, Agent); len(got) != 0 {
		t.Errorf("Filter(agent) = %+v", got)
	}
}

// fakeProber answers from a table: a name it does not know is missing, one in
// skip is skipped.
type fakeProber struct {
	present map[string]Found
	skip    map[string]bool
}

func (f fakeProber) Probe(_ context.Context, p Probe) (Found, error) {
	if f.skip[p.Target] {
		return Found{}, ErrSkipped
	}
	if r, ok := f.present[p.Target]; ok {
		return r, nil
	}
	return Found{}, ErrNotFound
}

func bins(names ...string) []Entry {
	var es []Entry
	for _, n := range names {
		es = append(es, Entry{Name: n, Layer: Harness, IfMissing: Degrades, Probe: Probe{Kind: ProbeBin, Target: n}})
	}
	return es
}

func TestProbeMarksPresentAndMissing(t *testing.T) {
	p := fakeProber{
		present: map[string]Found{"git": {Path: "/bin/git", Version: "git version 2"}},
		skip:    map[string]bool{"app": true},
	}
	rs := Check(context.Background(), bins("git", "rtk", "app"), p, time.Minute)
	if len(rs) != 3 {
		t.Fatalf("reports = %+v", rs)
	}
	want := []struct {
		name   string
		status Status
		path   string
	}{{"git", Present, "/bin/git"}, {"rtk", Missing, ""}, {"app", Skipped, ""}}
	for i, w := range want {
		if rs[i].Name != w.name || rs[i].Status != w.status || rs[i].Path != w.path {
			t.Errorf("report %d = %+v, want %s %s %q", i, rs[i], w.name, w.status, w.path)
		}
	}
	if rs[0].Version != "git version 2" {
		t.Errorf("version = %q", rs[0].Version)
	}
}

// barrierProber answers only once every probe has started, so a Check that
// probed one tool at a time would never see the barrier open: each probe would
// run into its timeout and report missing.
type barrierProber struct {
	mu      sync.Mutex
	waiting int
	n       int
	open    chan struct{}
}

func (b *barrierProber) Probe(ctx context.Context, p Probe) (Found, error) {
	b.mu.Lock()
	b.waiting++
	if b.waiting == b.n {
		close(b.open)
	}
	b.mu.Unlock()
	select {
	case <-b.open:
		return Found{Path: "/bin/" + p.Target}, nil
	case <-ctx.Done():
		return Found{}, ctx.Err()
	}
}

// stuckProber never answers before its timeout: it returns only when ctx is
// done.
type stuckProber struct{ fast fakeProber }

func (s stuckProber) Probe(ctx context.Context, p Probe) (Found, error) {
	if p.Target == "slow" {
		<-ctx.Done()
		return Found{Path: "/bin/slow"}, ctx.Err()
	}
	return s.fast.Probe(ctx, p)
}

func TestProbesRunConcurrentlyAndTimeOut(t *testing.T) {
	t.Run("every probe starts before one returns", func(t *testing.T) {
		b := &barrierProber{n: 4, open: make(chan struct{})}
		// The minute is a cap for a broken Check, not a measurement.
		rs := Check(context.Background(), bins("a", "b", "c", "d"), b, time.Minute)
		if len(rs) != 4 {
			t.Fatalf("reports = %+v", rs)
		}
		for _, r := range rs {
			if r.Status != Present {
				t.Errorf("%s: %s, want present: the probes did not run at once", r.Name, r.Status)
			}
		}
	})
	t.Run("a probe that outlives its timeout is missing", func(t *testing.T) {
		p := stuckProber{fast: fakeProber{present: map[string]Found{"git": {Path: "/bin/git"}}}}
		rs := Check(context.Background(), bins("git", "slow"), p, time.Millisecond)
		if len(rs) != 2 {
			t.Fatalf("reports = %+v", rs)
		}
		if rs[0].Status != Present {
			t.Errorf("git: %s, want present", rs[0].Status)
		}
		if rs[1].Status != Missing || rs[1].Path != "" {
			t.Errorf("slow: %+v, want missing with no path", rs[1])
		}
	})
}

func TestBlockingMissing(t *testing.T) {
	rs := []Report{
		{Entry: Entry{Name: "git", IfMissing: Blocks}, Status: Missing},
		{Entry: Entry{Name: "rtk", IfMissing: Degrades}, Status: Missing},
		{Entry: Entry{Name: "go", IfMissing: Blocks}, Status: Present},
	}
	if got := BlockingMissing(rs); len(got) != 1 || got[0].Name != "git" {
		t.Errorf("BlockingMissing = %+v", got)
	}
	if got := BlockingMissing(rs[1:]); len(got) != 0 {
		t.Errorf("BlockingMissing without git = %+v", got)
	}
}
