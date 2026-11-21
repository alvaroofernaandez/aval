package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func put(t *testing.T, dir, rel string, lines int) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "line %d of %s\n", i, rel)
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// repoWith creates a repo with one commit on main, then a feat branch where
// `change` is applied and committed.
func repoWith(t *testing.T, change func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	put(t, dir, "README.md", 1)
	put(t, dir, "src/app.go", 3)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "init")
	git(t, dir, "checkout", "-q", "-b", "feat")
	change(dir)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")
	return dir
}

var (
	none        = []string(nil)
	reliability = []string{"review-reliability"}
	all4        = []string{"review-risk", "review-resilience", "review-reliability", "review-readability"}
)

func TestTiers(t *testing.T) {
	cases := []struct {
		name   string
		change func(dir string)
		tier   string
		lenses []string
		tests  bool
		reason string // substring expected in some reason
	}{
		{"docs only", func(d string) { put(t, d, "docs/guide.md", 40); put(t, d, "README.md", 5) }, "low", none, false, "docs"},
		{"tests only", func(d string) { put(t, d, "src/app_test.go", 30) }, "low", none, true, "tests"},
		{"small src with tests", func(d string) { put(t, d, "src/app.go", 20); put(t, d, "src/app_test.go", 10) }, "low", none, true, "tests changed"},
		{"src without tests", func(d string) { put(t, d, "src/app.go", 20) }, "medium", reliability, false, "without test changes"},
		{"large src with tests", func(d string) { put(t, d, "src/big.go", 150); put(t, d, "src/big_test.go", 30) }, "medium", reliability, true, "100"},
		{"hooks", func(d string) { put(t, d, "config/claude/hooks/gate.sh", 3) }, "high", all4, false, "hooks"},
		{"settings", func(d string) { put(t, d, "config/claude/settings.json", 3) }, "high", all4, false, "settings"},
		{"manifest", func(d string) { put(t, d, "install.manifest", 3) }, "high", all4, false, "install.manifest"},
		{"installer", func(d string) { put(t, d, "scripts/install.sh", 3) }, "high", all4, false, "installer"},
		{"auth", func(d string) { put(t, d, "src/auth/login.ts", 3); put(t, d, "src/auth/login.test.ts", 3) }, "high", all4, true, "auth"},
		{"migrations", func(d string) { put(t, d, "db/migrations/001_add.sql", 3) }, "high", all4, false, "migrations"},
		{"payments", func(d string) { put(t, d, "api/payments/charge.py", 3) }, "high", all4, false, "payments"},
		{"workflows", func(d string) { put(t, d, ".github/workflows/ci.yml", 3) }, "high", all4, false, ".github/workflows"},
		{"permissions", func(d string) { put(t, d, "src/permissions.ts", 3) }, "high", all4, false, "permissions"},
		{"over 400 production lines", func(d string) { put(t, d, "src/huge.go", 401); put(t, d, "src/huge_test.go", 5) }, "high", all4, true, "400"},
		{"author is not auth", func(d string) { put(t, d, "docs/author-notes.md", 3) }, "low", none, false, "docs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := repoWith(t, c.change)
			r, err := Assess(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			if r.Tier != c.tier {
				t.Errorf("tier = %s, want %s (reasons %v)", r.Tier, c.tier, r.Reasons)
			}
			if !reflect.DeepEqual(r.Lenses, c.lenses) {
				t.Errorf("lenses = %v, want %v", r.Lenses, c.lenses)
			}
			if r.TestsChanged != c.tests {
				t.Errorf("tests_changed = %v, want %v", r.TestsChanged, c.tests)
			}
			found := false
			for _, why := range r.Reasons {
				found = found || strings.Contains(why, c.reason)
			}
			if !found {
				t.Errorf("no reason mentions %q: %v", c.reason, r.Reasons)
			}
			if len(r.Reasons) == 0 {
				t.Error("a tier without reasons is unexplained")
			}
		})
	}
}

func TestCountsAndBase(t *testing.T) {
	dir := repoWith(t, func(d string) {
		put(t, d, "src/new.go", 10)     // +10
		put(t, d, "src/app.go", 1)      // 3 lines -> 1 line: +1 -3 (rewritten)
		put(t, d, "src/new_test.go", 4) // +4, test
		put(t, d, "docs/a.md", 6)       // +6, docs
	})
	r, err := Assess(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if r.Base != "main" || r.Files != 4 {
		t.Errorf("base=%q files=%d", r.Base, r.Files)
	}
	if r.Added != 20 || r.Removed != 2 {
		t.Errorf("added=%d removed=%d, want +20 -2", r.Added, r.Removed)
	}
	// Production lines exclude tests and docs: src/new.go (10) + src/app.go.
	if r.ProductionLines < 11 || r.ProductionLines > 14 {
		t.Errorf("production lines = %d", r.ProductionLines)
	}
}

func TestUncommittedAndUntrackedCount(t *testing.T) {
	dir := repoWith(t, func(d string) { put(t, d, "docs/a.md", 1) })
	put(t, dir, "config/claude/hooks/new.sh", 4) // untracked, never added
	r, err := Assess(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Tier != "high" {
		t.Errorf("an untracked hook must count: tier = %s %v", r.Tier, r.Reasons)
	}
}

func TestNoChangesIsLow(t *testing.T) {
	dir := repoWith(t, func(d string) {})
	r, err := Assess(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Tier != "low" || r.Files != 0 || len(r.Lenses) != 0 || len(r.Reasons) == 0 {
		t.Errorf("empty diff = %+v", r)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Assess(t.TempDir(), ""); err == nil {
		t.Error("a directory that is not a git repository must be an error")
	}
	dir := repoWith(t, func(d string) {})
	if _, err := Assess(dir, "no-such-ref"); err == nil {
		t.Error("an unknown base must be an error")
	}
}

func TestRender(t *testing.T) {
	dir := repoWith(t, func(d string) { put(t, d, "config/claude/hooks/x.sh", 2) })
	r, _ := Assess(dir, "")
	var b bytes.Buffer
	Render(&b, r)
	for _, want := range []string{"risk", "high", "hooks", "review-risk", "review-readability", "config/claude/hooks/x.sh"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, b.String())
		}
	}
	b.Reset()
	if err := RenderJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	var back Result
	if err := json.Unmarshal(b.Bytes(), &back); err != nil || back.Tier != "high" || len(back.Lenses) != 4 {
		t.Errorf("json = %v\n%s", err, b.String())
	}
	if !strings.Contains(b.String(), `"lenses": [`) {
		t.Error("lenses must be an array even when empty")
	}
}

func TestEmptyLensesSerializeAsArray(t *testing.T) {
	dir := repoWith(t, func(d string) { put(t, d, "docs/a.md", 1) })
	r, _ := Assess(dir, "")
	var b bytes.Buffer
	if err := RenderJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"lenses": []`) {
		t.Errorf("want \"lenses\": [], got:\n%s", b.String())
	}
}
