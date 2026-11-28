package generation_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/fake"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

func cfg(rule string) agentcfg.Config {
	return agentcfg.Config{
		Rules:    []agentcfg.Rule{{ID: "agent-rules", Body: rule}},
		Commands: []agentcfg.Command{{Name: "ship", Description: "Ship"}},
	}
}

type failing struct{ *fake.Adapter }

func (failing) Name() string { return "failing" }
func (failing) Render(agentcfg.Config) (agentcfg.Rendered, error) {
	return agentcfg.Rendered{}, errors.New("boom")
}

func clock(t time.Time) func() time.Time { return func() time.Time { return t } }

var t0 = time.Date(2026, 11, 28, 15, 0, 0, 0, time.UTC)

// tempHome is a home whose read-only generations the test can still delete.
func tempHome(t *testing.T) string {
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

func build(t *testing.T, home string, c agentcfg.Config, at time.Time) (generation.Generation, bool) {
	t.Helper()
	g, created, err := generation.Build(generation.Options{Home: home, Config: c, Adapters: []agentcfg.Adapter{fake.New()}, Source: "abc123", Now: clock(at)})
	if err != nil {
		t.Fatal(err)
	}
	return g, created
}

var idRe = regexp.MustCompile(`^gen-[0-9a-f]{16}$`)

func TestSameInputsSameIDDifferentInputsDifferentID(t *testing.T) {
	a, created := build(t, tempHome(t), cfg("one"), t0)
	b, _ := build(t, tempHome(t), cfg("one"), t0.Add(time.Hour))
	c, _ := build(t, tempHome(t), cfg("two"), t0)
	if !idRe.MatchString(a.ID) || !created {
		t.Fatalf("id = %q created = %v, want gen-<16 hex>, created", a.ID, created)
	}
	if a.ID != b.ID {
		t.Errorf("same inputs, two ids: %s, %s", a.ID, b.ID)
	}
	if a.ID == c.ID {
		t.Errorf("different inputs, one id: %s", a.ID)
	}
	if !strings.HasPrefix(a.ContentHash, "sha256:") || !strings.HasPrefix(a.ContentHash, "sha256:"+strings.TrimPrefix(a.ID, "gen-")) {
		t.Errorf("contentHash = %q, want sha256:<hex> that the id abbreviates", a.ContentHash)
	}
}

func TestRebuildIsANoOp(t *testing.T) {
	home := tempHome(t)
	first, _ := build(t, home, cfg("one"), t0)
	again, created := build(t, home, cfg("one"), t0.Add(time.Hour))
	if created || !again.Built.Equal(first.Built) {
		t.Errorf("rebuild created=%v built=%v, want the first record (%v)", created, again.Built, first.Built)
	}
}

// hashTree hashes every path under root except skip.
func hashTree(root, skip string) string {
	var lines []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == skip {
			return filepath.SkipDir
		}
		b, _ := os.ReadFile(p)
		s := sha256.Sum256(b)
		lines = append(lines, p+" "+hex.EncodeToString(s[:]))
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestBuildWritesOnlyUnderTheStoreAndReadOnly(t *testing.T) {
	home := tempHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".aval", "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := generation.Root(home)
	if root != filepath.Join(home, ".aval", "generations") {
		t.Fatalf("Root = %q", root)
	}
	before := hashTree(home, root)
	g, _ := build(t, home, cfg("one"), t0)
	if hashTree(home, root) != before {
		t.Error("Build wrote outside ~/.aval/generations")
	}
	r, _ := fake.New().Render(cfg("one"))
	for _, f := range r.Files {
		p := filepath.Join(root, g.ID, "fake", filepath.FromSlash(f.Path))
		b, err := os.ReadFile(p)
		if err != nil || string(b) != string(f.Data) {
			t.Errorf("%s = %q, %v; want the rendering", p, b, err)
			continue
		}
		if st, _ := os.Stat(p); st.Mode().Perm()&0o222 != 0 {
			t.Errorf("%s is writable (%v); a generation is read-only", p, st.Mode().Perm())
		}
	}
	if len(g.Adapters) != 1 || g.Adapters[0].Name != "fake" || g.Adapters[0].Files != len(r.Files) || g.Source != "abc123" {
		t.Errorf("record = %+v", g)
	}
}

func TestAFailingAdapterBuildsNothing(t *testing.T) {
	home := tempHome(t)
	_, _, err := generation.Build(generation.Options{Home: home, Config: cfg("one"), Adapters: []agentcfg.Adapter{fake.New(), failing{fake.New()}}, Now: clock(t0)})
	if err == nil || !strings.Contains(err.Error(), "failing") {
		t.Fatalf("err = %v, want the failing adapter named", err)
	}
	if ents, _ := os.ReadDir(generation.Root(home)); len(ents) != 0 {
		t.Errorf("store after a failed build = %v, want empty", ents)
	}
}

func TestListNewestFirstAndSkipsUnfinishedBuilds(t *testing.T) {
	home := tempHome(t)
	old, _ := build(t, home, cfg("one"), t0)
	young, _ := build(t, home, cfg("two"), t0.Add(time.Hour))
	if err := os.MkdirAll(filepath.Join(generation.Root(home), ".tmp-123"), 0o755); err != nil {
		t.Fatal(err)
	}
	gs, err := generation.List(home)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, g := range gs {
		ids = append(ids, g.ID)
	}
	if !slices.Equal(ids, []string{young.ID, old.ID}) {
		t.Errorf("list = %v, want [%s %s]", ids, young.ID, old.ID)
	}
}

func TestDiff(t *testing.T) {
	home := tempHome(t)
	a, _ := build(t, home, cfg("one"), t0)
	two := cfg("two")
	two.Commands = append(two.Commands, agentcfg.Command{Name: "debug", Description: "Debug"})
	b, _ := build(t, home, two, t0)
	d, err := generation.Diff(home, a.ID, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, c := range d {
		kinds[c.Kind]++
		if !strings.HasPrefix(c.Path, "fake/") {
			t.Errorf("path %q is not <adapter>/<file>", c.Path)
		}
	}
	if kinds["added"] != 1 || kinds["changed"] != 1 || kinds["removed"] != 0 || len(d) != 2 {
		t.Errorf("diff = %+v, want one added command and one changed rule", d)
	}
	if !slices.IsSortedFunc(d, func(x, y generation.Change) int { return strings.Compare(x.Path, y.Path) }) {
		t.Errorf("diff is not sorted: %+v", d)
	}
	if _, err := generation.Diff(home, a.ID, "gen-0000000000000000"); !errors.Is(err, generation.ErrUnknown) {
		t.Errorf("unknown id: %v, want ErrUnknown", err)
	}
}
