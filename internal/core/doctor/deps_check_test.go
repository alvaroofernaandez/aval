package doctor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/deps"
)

// depsProber finds every tool but the ones in missing; app probes are
// skipped, like on a machine that is not macOS.
type depsProber struct{ missing map[string]bool }

func (f depsProber) Probe(_ context.Context, p deps.Probe) (deps.Found, error) {
	if p.Kind == deps.ProbeApp {
		return deps.Found{}, deps.ErrSkipped
	}
	if f.missing[p.Target] {
		return deps.Found{}, deps.ErrNotFound
	}
	return deps.Found{Path: "/bin/" + p.Target}, nil
}

func withProber(t *testing.T, missing ...string) Options {
	t.Helper()
	o := healthy(t)
	m := map[string]bool{}
	for _, n := range missing {
		m[n] = true
	}
	o.DepsProber = depsProber{missing: m}
	return o
}

func TestDoctorDepsFailsWhenABlockingToolIsMissing(t *testing.T) {
	rs := Run(withProber(t, "git", "rtk"))
	r := get(t, rs, "deps")
	if r.Status != Fail {
		t.Fatalf("deps = %+v, want fail", r)
	}
	if !slices.Equal(r.Details, []string{"git"}) {
		t.Errorf("details = %q, want only the blocking tool", r.Details)
	}
	if !HasFail(rs) {
		t.Error("a missing blocking tool must fail the run")
	}
}

func TestDoctorDepsWarnsWhenADegradingToolIsMissing(t *testing.T) {
	r := get(t, Run(withProber(t, "rtk", "fzf")), "deps")
	if r.Status != Warn {
		t.Fatalf("deps = %+v, want warn", r)
	}
	if !slices.Equal(r.Details, []string{"rtk", "fzf"}) {
		t.Errorf("details = %q, want the missing tools in inventory order", r.Details)
	}
}

func TestDoctorDepsIsOKWhenEverythingIsPresent(t *testing.T) {
	r := get(t, Run(withProber(t)), "deps")
	if r.Status != OK || r.Fixable {
		t.Errorf("deps = %+v, want ok and not fixable", r)
	}
}

// Regression guard: txsync calls RunOnly after every sync with no prober, and
// must not start probing tools.
func TestDoctorWithoutAProberReportsNoDepsRow(t *testing.T) {
	for _, r := range Run(healthy(t)) {
		if r.ID == "deps" {
			t.Fatalf("a run without a prober reported %+v", r)
		}
	}
}

// posixUtilities are on every system aval runs on; the inventory lists what
// can be missing.
var posixUtilities = map[string]bool{"rm": true}

func TestInventoryCoversTheRepoHookCommands(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(b, &settings); err != nil {
		t.Fatal(err)
	}
	m, err := deps.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, e := range m.Entries {
		listed[e.Probe.Target] = true
		listed[e.Name] = true
	}
	cmds := hookCommands(settings)
	if len(cmds) == 0 {
		t.Fatal("no hook commands read from config/claude/settings.json")
	}
	for _, h := range cmds {
		for _, ex := range executables(h.command, "/home") {
			if strings.ContainsAny(ex, `/\`) || posixUtilities[ex] {
				continue
			}
			if !listed[ex] {
				t.Errorf("%s hook calls %q, which deps.manifest does not list", h.event, ex)
			}
		}
	}
}
