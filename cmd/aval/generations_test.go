package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/fake"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

// genHome is a home whose read-only generations the test can still delete.
func genHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	return home
}

func runGen(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	env := func(k string) string {
		switch k {
		case "HOME", "USERPROFILE":
			return home
		}
		return ""
	}
	err := dispatch(append(args, "--repo", filepath.Join("..", "..")), cmdIO{stdin: strings.NewReader(""), stdout: &out, stderr: &errb, env: env})
	return out.String(), err
}

func TestGenerationsBuildRendersEveryAdapterOfTheRepo(t *testing.T) {
	home := genHome(t)
	out, err := runGen(t, home, "generations", "build", "--json")
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	var r struct {
		generation.Generation
		Created bool `json:"created"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if !r.Created || !strings.HasPrefix(r.ID, "gen-") || len(r.Adapters) != 2 {
		t.Fatalf("record = %+v", r)
	}
	for _, p := range []string{"claude-code/settings.json", "pi/AGENTS.md", "pi/mcp.json"} {
		if _, err := os.Stat(filepath.Join(generation.Root(home), r.ID, filepath.FromSlash(p))); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	again, err := runGen(t, home, "generations", "build")
	if err != nil || !strings.Contains(again, r.ID) || !strings.Contains(again, "already built") {
		t.Errorf("second build = %q, %v; want %s already built", again, err, r.ID)
	}
}

// goldenGenerations builds two generations with a fixed clock and fake
// adapters, so list and diff print the same bytes on every machine.
func goldenGenerations(t *testing.T) (home, a, b string) {
	t.Helper()
	home = genHome(t)
	at := time.Date(2026, 11, 28, 15, 0, 0, 0, time.UTC)
	for i, rule := range []string{"one", "two"} {
		c := agentcfg.Config{Rules: []agentcfg.Rule{{ID: "agent-rules", Body: rule}}}
		if i == 1 {
			c.Commands = []agentcfg.Command{{Name: "ship", Description: "Ship"}}
		}
		when := at.Add(time.Duration(i) * time.Hour)
		g, _, err := generation.Build(generation.Options{Home: home, Config: c, Adapters: []agentcfg.Adapter{fake.New()}, Source: "abc123", Now: func() time.Time { return when }})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			a = g.ID
		} else {
			b = g.ID
		}
	}
	return home, a, b
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s changed (go test ./cmd/aval -run TestGenerations -update):\n%s", name, got)
	}
}

func TestGenerationsListAndDiffMatchTheGoldens(t *testing.T) {
	home, a, b := goldenGenerations(t)
	for _, c := range []struct {
		golden string
		args   []string
	}{
		{"generations_list.golden.json", []string{"generations", "list", "--json"}},
		{"generations_list.golden.txt", []string{"generations", "list"}},
		{"generations_diff.golden.json", []string{"generations", "diff", a, b, "--json"}},
		{"generations_diff.golden.txt", []string{"generations", "diff", a, b}},
	} {
		out, err := runGen(t, home, c.args...)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		checkGolden(t, c.golden, out)
	}
}

func TestGenerationsRejectsBadUse(t *testing.T) {
	home := genHome(t)
	for _, args := range [][]string{{"generations"}, {"generations", "nope"}, {"generations", "diff", "gen-1"}} {
		if _, err := runGen(t, home, args...); err == nil || !strings.Contains(err.Error(), "aval generations") {
			t.Errorf("%v: err = %v, want the usage", args, err)
		}
	}
	if _, err := runGen(t, home, "generations", "diff", "gen-0000000000000000", "gen-0000000000000001"); err == nil || !strings.Contains(err.Error(), "unknown generation") {
		t.Errorf("unknown ids: %v", err)
	}
}
