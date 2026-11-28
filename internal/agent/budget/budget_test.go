package budget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

func skill(t *testing.T, root, name, desc string) {
	t.Helper()
	write(t, filepath.Join(root, name, "SKILL.md"),
		fmt.Sprintf("---\nname: %s\ndescription: %s\n---\nbody that must not count\n", name, desc))
}

func item(a Agent, label string) (Item, bool) {
	for _, it := range a.Items {
		if it.Label == label {
			return it, true
		}
	}
	return Item{}, false
}

func agent(t *testing.T, r *Report, name string) Agent {
	t.Helper()
	for _, a := range r.Agents {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("no agent %q in %+v", name, r.Agents)
	return Agent{}
}

func contextHome(t *testing.T) string {
	home := t.TempDir()
	c := filepath.Join(home, ".claude")
	write(t, filepath.Join(c, "CLAUDE.md"), strings.Repeat("a", 100)+"\n@RTK.md\n")
	write(t, filepath.Join(c, "RTK.md"), strings.Repeat("r", 40))
	skill(t, filepath.Join(c, "skills"), "alpha", "does alpha") // 5+10 = 15
	skill(t, filepath.Join(c, "skills"), "beta", "does beta")   // 4+9  = 13
	skill(t, filepath.Join(c, "skills"), "gamma", "does gamma") // off
	write(t, filepath.Join(c, "skills", "junk", "notes.txt"), "no SKILL.md here")
	write(t, filepath.Join(c, "settings.json"), `{"skillOverrides":{"gamma":"off","beta":"on"}}`)
	write(t, filepath.Join(c, "agents", "rev.md"), "---\nname: rev\ndescription: reviews code\nmodel: sonnet\n---\nlong prompt body\n") // 3+13
	write(t, filepath.Join(c, "commands", "go.md"), "---\ndescription: go go\n---\nbody\n")                                             // go + 5
	o := filepath.Join(home, ".config", "opencode")
	write(t, filepath.Join(o, "AGENTS.md"), strings.Repeat("o", 60))
	skill(t, filepath.Join(o, "skills"), "alpha", "does alpha")
	return home
}

func TestAlwaysOnContextPerAgent(t *testing.T) {
	r, err := Run(Options{Home: contextHome(t), SkipHooks: true})
	if err != nil {
		t.Fatal(err)
	}
	cl := agent(t, r, "claude")

	md, _ := item(cl, "CLAUDE.md")
	if md.Chars != 109 { // 100 a's, newline, "@RTK.md", newline
		t.Errorf("CLAUDE.md chars = %d, want 109", md.Chars)
	}
	imp, ok := item(cl, "@imports")
	if !ok || imp.Chars != 40 || imp.Count != 1 {
		t.Errorf("@imports = %+v, want 1 file of 40 chars", imp)
	}
	sk, _ := item(cl, "skills")
	if sk.Count != 2 || sk.Chars != 28 {
		t.Errorf("skills = %+v, want 2 visible (gamma is off) totalling 28 chars", sk)
	}
	ag, _ := item(cl, "agents")
	if ag.Count != 1 || ag.Chars != 15 {
		t.Errorf("agents = %+v, want 1 totalling 15", ag)
	}
	cm, _ := item(cl, "commands")
	if cm.Count != 1 || cm.Chars != 7 {
		t.Errorf("commands = %+v, want 1 totalling 7", cm)
	}
	if want := 109 + 40 + 28 + 15 + 7; cl.Chars != want {
		t.Errorf("claude total = %d, want %d", cl.Chars, want)
	}
	if cl.Tokens != (cl.Chars+3)/4 {
		t.Errorf("tokens = %d, want chars/4 rounded up = %d", cl.Tokens, (cl.Chars+3)/4)
	}

	oc := agent(t, r, "opencode")
	if a, _ := item(oc, "AGENTS.md"); a.Chars != 60 {
		t.Errorf("AGENTS.md = %+v", a)
	}
	if s, _ := item(oc, "skills"); s.Count != 1 || s.Chars != 15 {
		t.Errorf("opencode skills = %+v", s)
	}
	if !strings.Contains(r.Note, "approx") {
		t.Errorf("note must say token counts are approximate: %q", r.Note)
	}
}

func TestMissingAgentFilesAreZeroNotErrors(t *testing.T) {
	r, err := Run(Options{Home: t.TempDir(), SkipHooks: true})
	if err != nil {
		t.Fatal(err)
	}
	if a := agent(t, r, "claude"); a.Chars != 0 {
		t.Errorf("empty home should cost 0, got %d", a.Chars)
	}
}

func hooksHome(t *testing.T, settings string) string {
	home := t.TempDir()
	write(t, filepath.Join(home, ".claude", "settings.json"), settings)
	return home
}

func TestHookLatencyAgainstBudget(t *testing.T) {
	testenv.SkipWindows(t, "the fixture hooks are POSIX shell scripts")
	home := t.TempDir()
	slow := filepath.Join(home, "slow.sh")
	fast := filepath.Join(home, "fast.sh")
	write(t, slow, "#!/bin/sh\nsleep 0.3\n")
	write(t, fast, "#!/bin/sh\ncat >/dev/null\n")
	write(t, filepath.Join(home, ".claude", "settings.json"), fmt.Sprintf(`{"hooks":{
	  "PreToolUse":[{"matcher":"Edit|Write","hooks":[{"type":"command","command":%q}]}],
	  "PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":%q}]}],
	  "Stop":[{"hooks":[{"type":"command","command":%q,"async":true}]}]}}`, fast, slow, slow))

	r, err := Run(Options{Home: home, Runs: 3, Timeout: 2 * time.Second, HookBudget: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hooks) != 3 {
		t.Fatalf("hooks = %d, want 3: %+v", len(r.Hooks), r.Hooks)
	}
	by := map[string]Hook{}
	for _, h := range r.Hooks {
		by[h.Event] = h
	}
	pre, post, stop := by["PreToolUse"], by["PostToolUse"], by["Stop"]

	if pre.Runs != 3 || !pre.Budgeted || pre.BudgetMs != 100 {
		t.Errorf("pre = %+v", pre)
	}
	// Only lower bounds on the wall clock: a loaded machine makes every hook
	// slower, never faster, so "the fast hook is under budget" belongs to
	// TestClassifyVerdict, which needs no clock (#125).
	if !post.Over || post.P50Ms < 280 || post.P95Ms < post.P50Ms {
		t.Errorf("slow sync hook must be over budget with p50>=280ms: %+v", post)
	}
	if stop.Budgeted || stop.Over || !stop.Async || stop.P50Ms < 280 {
		t.Errorf("an async hook is reported (p50 measured) but never budgeted: %+v", stop)
	}
	if !r.Over {
		t.Error("report must be over budget because of the slow PostToolUse hook")
	}
}

func TestHookTimeoutCountsAsOver(t *testing.T) {
	testenv.SkipWindows(t, "the fixture hooks are POSIX shell scripts")
	home := t.TempDir()
	hang := filepath.Join(home, "hang.sh")
	write(t, hang, "#!/bin/sh\nexec sleep 5\n")
	write(t, filepath.Join(home, ".claude", "settings.json"),
		fmt.Sprintf(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":%q}]}]}}`, hang))

	start := time.Now()
	r, err := Run(Options{Home: home, Runs: 2, Timeout: 150 * time.Millisecond, HookBudget: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("the timeout was not enforced: took %v", time.Since(start))
	}
	if len(r.Hooks) != 1 {
		t.Fatalf("hooks = %+v", r.Hooks)
	}
	if h := r.Hooks[0]; !h.TimedOut || !h.Over {
		t.Errorf("hook = %+v, want timed out and over", h)
	}
}

func TestHookReceivesRepresentativeStdin(t *testing.T) {
	testenv.SkipWindows(t, "the fixture hooks are POSIX shell scripts")
	home := t.TempDir()
	probe := filepath.Join(home, "probe.sh")
	out := filepath.Join(home, "stdin.json")
	write(t, probe, fmt.Sprintf("#!/bin/sh\ncat > %s\n", out))
	write(t, filepath.Join(home, ".claude", "settings.json"),
		fmt.Sprintf(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":%q}]}]}}`, probe))
	if _, err := Run(Options{Home: home, Runs: 1, Timeout: 2 * time.Second, HookBudget: time.Second}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Event string `json:"hook_event_name"`
		Tool  string `json:"tool_name"`
		In    struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if json.Unmarshal(b, &p) != nil || p.Event != "PreToolUse" || p.Tool != "Bash" || p.In.Command == "" {
		t.Errorf("stdin payload = %s", b)
	}
}

func TestSkipHooksAndNoSettings(t *testing.T) {
	r, err := Run(Options{Home: t.TempDir()})
	if err != nil || len(r.Hooks) != 0 || r.Over {
		t.Errorf("no settings: %+v %v", r, err)
	}
}

func TestPercentile(t *testing.T) {
	s := []float64{1, 2, 3, 4, 5}
	for _, c := range []struct {
		p    float64
		want float64
	}{{0.5, 3}, {0.95, 5}, {0, 1}, {1, 5}} {
		if got := percentile(s, c.p); got != c.want {
			t.Errorf("percentile(%v) = %v, want %v", c.p, got, c.want)
		}
	}
	if percentile(nil, 0.5) != 0 {
		t.Error("empty sample must be 0")
	}
}

func TestRender(t *testing.T) {
	r := &Report{
		Agents: []Agent{{Name: "claude", Chars: 400, Tokens: 100, Items: []Item{{Label: "CLAUDE.md", Chars: 400, Tokens: 100}}}},
		Hooks:  []Hook{{Event: "PreToolUse", Command: "x.sh", Budgeted: true, BudgetMs: 20, Runs: 5, P50Ms: 4.2, P95Ms: 31, Over: true}},
		Over:   true, Note: "tokens are approximate (chars/4)",
	}
	var b bytes.Buffer
	Render(&b, r)
	for _, want := range []string{"claude", "CLAUDE.md", "400", "approx", "PreToolUse", "x.sh", "OVER", "31"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("table lacks %q:\n%s", want, b.String())
		}
	}
	b.Reset()
	if err := RenderJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(b.Bytes(), &back); err != nil || !back.Over || len(back.Agents) != 1 {
		t.Errorf("json round trip failed: %v\n%s", err, b.String())
	}
}

func TestHookArgvExecFormSkipsTheShell(t *testing.T) {
	name, args := hookArgv(hookSpec{command: "C:/bin/aval.exe", args: []string{"hook", "lessons"}})
	if name != "C:/bin/aval.exe" || len(args) != 2 || args[0] != "hook" {
		t.Errorf("exec form = %q %v", name, args)
	}
	name, args = hookArgv(hookSpec{command: "echo hi"})
	if name != "sh" || len(args) != 2 || args[0] != "-c" || args[1] != "echo hi" {
		t.Errorf("shell form = %q %v", name, args)
	}
}

// An exec-form hook is measured on every OS, Windows included: the test
// binary itself is the program, so no shell or script is needed.
func TestExecFormHookIsMeasured(t *testing.T) {
	home := t.TempDir()
	settings := fmt.Sprintf(`{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":%q,"args":["-test.run=^$"]}]}]}}`, os.Args[0])
	write(t, filepath.Join(home, ".claude", "settings.json"), settings)
	r, err := Run(Options{Home: home, Runs: 2, Timeout: 5 * time.Second, HookBudget: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hooks) != 1 || r.Hooks[0].TimedOut || r.Hooks[0].P50Ms <= 0 {
		t.Errorf("hooks = %+v, want one measured exec-form hook", r.Hooks)
	}
}

// The verdict, on synthetic samples: no wall clock, so no load can flip it.
func TestClassifyVerdict(t *testing.T) {
	sync := Hook{Event: "PreToolUse", Budgeted: true, BudgetMs: 100}
	async := Hook{Event: "Stop", Async: true}
	for _, c := range []struct {
		name     string
		h        Hook
		samples  []float64
		timedOut bool
		over     bool
		p50, p95 float64
	}{
		{"under budget", sync, []float64{3, 1, 2, 4, 90}, false, false, 3, 90},
		{"p95 over budget", sync, []float64{1, 2, 3, 4, 101}, false, true, 3, 101},
		{"exactly at budget is under", sync, []float64{100, 100}, false, false, 100, 100},
		{"a timeout is over even when fast", sync, []float64{1, 1}, true, true, 1, 1},
		{"an async hook is never over", async, []float64{900, 900}, true, false, 900, 900},
	} {
		got := classify(c.h, c.samples, c.timedOut)
		if got.Over != c.over || got.P50Ms != c.p50 || got.P95Ms != c.p95 || got.TimedOut != c.timedOut {
			t.Errorf("%s: %+v, want over=%v p50=%v p95=%v timedOut=%v", c.name, got, c.over, c.p50, c.p95, c.timedOut)
		}
	}
}

// The guard hook runs before every Bash call, so it has its own, tighter
// ceiling (10 ms) than the generic hook budget.
func TestGuardHookHasATenMillisecondBudget(t *testing.T) {
	testenv.SkipWindows(t, "the fixture hooks are POSIX shell scripts")
	home := t.TempDir()
	write(t, filepath.Join(home, ".claude", "settings.json"), `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[
	  {"type":"command","command":"cat >/dev/null; sleep 0.06 # aval hook guard"},
	  {"type":"command","command":"cat >/dev/null; sleep 0.06 # another hook"}]}]}}`)

	r, err := Run(Options{Home: home, Runs: 2, Timeout: 2 * time.Second, HookBudget: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hooks) != 2 {
		t.Fatalf("hooks = %d, want 2: %+v", len(r.Hooks), r.Hooks)
	}
	guard, other := r.Hooks[0], r.Hooks[1]
	if guard.BudgetMs != 10 || !guard.Over {
		t.Errorf("the guard hook must be held to 10 ms and be over with a 60 ms sleep: %+v", guard)
	}
	if other.BudgetMs != 500 || other.Over {
		t.Errorf("another hook keeps the generic budget: %+v", other)
	}
	if !r.Over {
		t.Error("an over-budget guard fails the report")
	}
}

func TestGuardBudgetNeverRaisesAStricterGlobalBudget(t *testing.T) {
	if got := budgetFor("aval hook guard", nil, 5*time.Millisecond); got != 5*time.Millisecond {
		t.Errorf("budgetFor = %v, want the stricter 5ms", got)
	}
	if got := budgetFor("a=x; $a hook guard || true", nil, 20*time.Millisecond); got != 10*time.Millisecond {
		t.Errorf("budgetFor = %v, want 10ms", got)
	}
	if got := budgetFor("aval", []string{"hook", "guard"}, 20*time.Millisecond); got != 10*time.Millisecond {
		t.Errorf("exec form: budgetFor = %v, want 10ms", got)
	}
	if got := budgetFor("aval hook guardian", nil, 20*time.Millisecond); got != 20*time.Millisecond {
		t.Errorf("a lookalike keeps the generic budget, got %v", got)
	}
}
