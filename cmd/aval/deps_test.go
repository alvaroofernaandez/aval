package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/deps"
)

var updateGolden = flag.Bool("update", false, "rewrite the goldens under testdata")

// fakeDepsProber finds the tools it is given, skips app probes like a machine
// that is not macOS, and finds nothing else.
type fakeDepsProber struct{ present map[string]bool }

func (f fakeDepsProber) Probe(_ context.Context, p deps.Probe) (deps.Found, error) {
	if p.Kind == deps.ProbeApp {
		return deps.Found{}, deps.ErrSkipped
	}
	if f.present[p.Target] {
		return deps.Found{Path: "/fake/bin/" + p.Target, Version: p.Target + " 1.0"}, nil
	}
	return deps.Found{}, deps.ErrNotFound
}

func presentProber(names ...string) fakeDepsProber {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return fakeDepsProber{present: m}
}

// runDepsIn runs `aval <args>` in process with p as the prober and a scratch
// HOME with no repository anywhere.
func runDepsIn(t *testing.T, p deps.Prober, args ...string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	env := envMap(withHomeVars(map[string]string{"HOME": t.TempDir(), "PATH": ""}))
	err := dispatch(args, cmdIO{stdin: strings.NewReader(""), stdout: &out, stderr: &errb, env: env, prober: p})
	return out.String(), err
}

type depsJSONRow struct {
	Name             string   `json:"name"`
	Layer            string   `json:"layer"`
	UsedBy           []string `json:"used_by"`
	IfMissing        string   `json:"if_missing"`
	NativeSubstitute string   `json:"native_substitute"`
	Probe            string   `json:"probe"`
	Status           string   `json:"status"`
	Path             string   `json:"path"`
	Version          string   `json:"version"`
}

func TestDepsJSONMatchesTheGolden(t *testing.T) {
	out, err := runDepsIn(t, presentProber("git", "go", "rtk", "tmux", "zsh"), "deps", "--json")
	if err != nil {
		t.Fatalf("deps --json: %v", err)
	}
	const golden = "testdata/deps.golden.json"
	if *updateGolden {
		if err := os.WriteFile(golden, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Errorf("deps --json changed (go test ./cmd/aval -run TestDepsJSONMatchesTheGolden -update):\n%s", out)
	}
}

func TestDepsJSONListsTheWholeInventoryByLayer(t *testing.T) {
	m, err := deps.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	out, err := runDepsIn(t, presentProber("git"), "deps", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []depsJSONRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(rows) != len(m.Entries) {
		t.Fatalf("%d rows for %d entries", len(rows), len(m.Entries))
	}
	order := map[string]int{"harness": 0, "agent": 1, "env": 2}
	for i := 1; i < len(rows); i++ {
		if order[rows[i].Layer] < order[rows[i-1].Layer] {
			t.Errorf("row %d (%s, %s) is listed after a later layer", i, rows[i].Name, rows[i].Layer)
		}
	}
	for _, r := range rows {
		want := "missing"
		switch {
		case r.Name == "git":
			want = "present"
		case strings.HasPrefix(r.Probe, "app "):
			want = "skipped"
		}
		if r.Status != want {
			t.Errorf("%s: status %q, want %q", r.Name, r.Status, want)
		}
	}
}

func TestDepsFiltersByLayer(t *testing.T) {
	m, err := deps.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range deps.Layers {
		out, err := runDepsIn(t, presentProber("git"), "deps", "--json", "--layer", string(l))
		if err != nil {
			t.Fatalf("--layer %s: %v", l, err)
		}
		var rows []depsJSONRow
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatal(err)
		}
		if want := len(deps.Filter(m.Entries, l)); len(rows) != want || want == 0 {
			t.Errorf("--layer %s: %d rows, want %d", l, len(rows), want)
		}
		for _, r := range rows {
			if r.Layer != string(l) {
				t.Errorf("--layer %s listed %s (%s)", l, r.Name, r.Layer)
			}
		}
	}
}

func TestDepsUnknownLayerIsAUsageError(t *testing.T) {
	_, err := runDepsIn(t, presentProber("git"), "deps", "--layer", "kernel")
	var ue *usageError
	if !errors.As(err, &ue) {
		t.Fatalf("want a usage error, got %v", err)
	}
	if !errors.Is(err, deps.ErrUnknownLayer) {
		t.Errorf("want ErrUnknownLayer inside, got %v", err)
	}
	_, err = runDepsIn(t, presentProber("git"), "deps", "--bogus")
	if !errors.As(err, &ue) {
		t.Errorf("an unknown flag: want a usage error, got %v", err)
	}
	_, err = runDepsIn(t, presentProber("git"), "deps", "extra")
	if !errors.As(err, &ue) {
		t.Errorf("a stray argument: want a usage error, got %v", err)
	}
}

func TestDepsExitsOneWhenABlockingToolIsMissing(t *testing.T) {
	for _, args := range [][]string{{"deps"}, {"deps", "--json"}} {
		out, err := runDepsIn(t, presentProber("go", "rtk"), args...)
		if !errors.Is(err, errChecksFailed) {
			t.Errorf("%v without git: want errChecksFailed, got %v", args, err)
		}
		if !strings.Contains(out, "git") {
			t.Errorf("%v: the report was not printed before failing:\n%s", args, out)
		}
	}
	// Only blocking tools fail the run: everything but git missing is fine.
	if _, err := runDepsIn(t, presentProber("git"), "deps"); err != nil {
		t.Errorf("git present: want success, got %v", err)
	}
	// A layer without git does not fail on git.
	if _, err := runDepsIn(t, presentProber(), "deps", "--layer", "env"); err != nil {
		t.Errorf("--layer env without git: want success, got %v", err)
	}
}

func TestDepsTableHasARowPerTool(t *testing.T) {
	m, err := deps.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	out, err := runDepsIn(t, presentProber("git"), "deps")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	// header, one row per entry, a blank line, the summary
	if len(lines) != len(m.Entries)+3 {
		t.Fatalf("%d lines for %d entries:\n%s", len(lines), len(m.Entries), out)
	}
	for _, e := range m.Entries {
		found := false
		for _, l := range lines[1 : len(lines)-2] {
			if f := strings.Fields(l); len(f) > 1 && f[1] == e.Name {
				found = true
			}
		}
		if !found {
			t.Errorf("no row for %s:\n%s", e.Name, out)
		}
	}
}

// No repository, no AVAL_REPO, an empty PATH and a scratch HOME: deps still
// runs, on the real prober, and fails only because git (blocking) is not on
// that PATH. With nothing on PATH it cannot start a program, let alone reach
// the network.
func TestDepsNeedsNoRepositoryAndNoNetwork(t *testing.T) {
	out, err := runDepsIn(t, nil, "deps", "--json")
	if !errors.Is(err, errChecksFailed) {
		t.Fatalf("want errChecksFailed (git is not on an empty PATH), got %v", err)
	}
	var rows []depsJSONRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) == 0 {
		t.Fatalf("no report: %v\n%s", err, out)
	}
	for _, r := range rows {
		if strings.HasPrefix(r.Probe, "bin ") && r.Status != "missing" {
			t.Errorf("%s: %s on an empty PATH", r.Name, r.Status)
		}
	}
}
