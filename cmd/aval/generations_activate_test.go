package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

// genRepo is a minimal aval repo whose settings set env AVAL_GEN=tag.
func genRepo(t *testing.T, repo, tag string) {
	t.Helper()
	for p, body := range map[string]string{
		"config/claude/settings.json": `{"env":{"AVAL_GEN":"` + tag + `"}}`,
		"config/claude/CLAUDE.md":     "x\n<!-- shared:agent-rules -->\nrule\n<!-- /shared:agent-rules -->\n",
		"install.manifest":            "[claude]\nlabel = c\ndetail = d\nplatforms = all\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runGenIn(t *testing.T, repo, home string, args ...string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	env := func(k string) string {
		switch k {
		case "HOME", "USERPROFILE":
			return home
		}
		return ""
	}
	err := dispatch(append(args, "--repo", repo), cmdIO{stdin: strings.NewReader(""), stdout: &out, stderr: &errb, env: env})
	return out.String(), err
}

func buildID(t *testing.T, repo, home string) string {
	t.Helper()
	if _, err := runGenIn(t, repo, home, "generations", "build"); err != nil {
		t.Fatal(err)
	}
	gs, _ := generation.List(home)
	return gs[0].ID
}

// activate and rollback go through the sync transaction; a bare rollback
// returns to the generation active before, and two of them toggle (G7 = A).
func TestGenerationsActivateAndRollback(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	live := filepath.Join(home, ".claude", "settings.json")
	genRepo(t, repo, "a")
	a := buildID(t, repo, home)
	if _, err := runGenIn(t, repo, home, "generations", "rollback"); !errors.Is(err, generation.ErrNoPrevious) {
		t.Errorf("rollback with no history = %v, want ErrNoPrevious", err)
	}
	out, err := runGenIn(t, repo, home, "generations", "activate", a)
	if err != nil || !strings.Contains(out, "activated "+a) {
		t.Fatalf("activate a: %q, %v", out, err)
	}
	first, _ := os.ReadFile(live)

	genRepo(t, repo, "b")
	b := buildID(t, repo, home)
	if _, err := runGenIn(t, repo, home, "generations", "activate", b); err != nil {
		t.Fatal(err)
	}
	if out, err := runGenIn(t, repo, home, "generations", "rollback"); err != nil || !strings.Contains(out, "activated "+a) {
		t.Fatalf("rollback: %q, %v", out, err)
	}
	if got, _ := os.ReadFile(live); !bytes.Equal(got, first) {
		t.Errorf("rollback to a is not byte-identical to its first activation:\n%s\nwant\n%s", got, first)
	}
	if _, err := runGenIn(t, repo, home, "generations", "rollback"); err != nil {
		t.Fatal(err)
	}
	if id, _ := generation.Current(home); id != b {
		t.Errorf("a second bare rollback left %s active, want %s", id, b)
	}
	if _, err := runGenIn(t, repo, home, "generations", "rollback", a); err != nil {
		t.Fatal(err)
	}
	if id, _ := generation.Current(home); id != a {
		t.Errorf("rollback %s left %s active", a, id)
	}
}
