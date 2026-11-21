package designhook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/designack"
)

var epoch0 = time.Unix(1_700_000_000, 0)

type env struct {
	cache string
	now   time.Time
}

func newEnv(t *testing.T) env { return env{cache: t.TempDir(), now: epoch0} }

// project makes a git project (physical path, as the gate resolves it).
func project(t *testing.T, design string) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if design != "" {
		if err := os.MkdirAll(filepath.Join(d, ".agents"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, ".agents", "DESIGN.md"), []byte(design), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func (e env) ack(t *testing.T, proj string, at time.Time) {
	t.Helper()
	var sink bytes.Buffer
	if err := designack.Ack(&sink, designack.Options{Surface: "component", Dir: proj, Cache: e.cache, Now: at}); err != nil {
		t.Fatal(err)
	}
}

func (e env) gate(payload string, mut ...func(*GateOptions)) string {
	o := GateOptions{Cache: e.cache, TTL: designack.DefaultTTL, Now: e.now}
	for _, m := range mut {
		m(&o)
	}
	var out bytes.Buffer
	Gate(strings.NewReader(payload), &out, o)
	return out.String()
}

func edit(tool, file, cwd, sid string) string {
	b, _ := json.Marshal(map[string]any{
		"session_id": sid, "transcript_path": "/x.jsonl", "cwd": cwd,
		"hook_event_name": "PreToolUse", "tool_name": tool,
		"tool_input": map[string]string{"file_path": file},
	})
	return string(b)
}

func deny(name, why string) string {
	return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Blocked: ` +
		name + ` is a UI surface and ` + why + `. Do the design pipeline (invoke the design-pipeline skill if you have not: laws of UX, direction from .agents/DESIGN.md, gates), then run ` +
		"`aval design ack <surface>`" + ` and retry."}}` + "\n"
}

func TestGateDecisions(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, e env, p string)
		file  string // relative to project
		tool  string
		opts  []func(*GateOptions)
		want  string // "" = allowed, otherwise the "why" of a denial
	}{
		{name: "UI file with no marker is denied", file: "src/Button.tsx", want: "no design-ack for this project yet"},
		{name: "fresh marker allows", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) { e.ack(t, p, e.now) }},
		{name: "a stylesheet is covered by the same marker", file: "src/app.css", tool: "Write",
			setup: func(t *testing.T, e env, p string) { e.ack(t, p, e.now) }},
		{name: "older than the 4 h default is denied", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) { e.ack(t, p, e.now.Add(-5*time.Hour)) },
			want:  "the last design-ack is older than 240 min"},
		{name: "exactly at the TTL is still allowed", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) { e.ack(t, p, e.now.Add(-4*time.Hour)) }},
		{name: "a longer TTL lengthens the window", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) { e.ack(t, p, e.now.Add(-5*time.Hour)) },
			opts:  []func(*GateOptions){func(o *GateOptions) { o.TTL = 86400 }}},
		{name: "a shorter TTL shortens it", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) { e.ack(t, p, e.now.Add(-30*time.Second)) },
			opts:  []func(*GateOptions){func(o *GateOptions) { o.TTL = 5 }},
			want:  "the last design-ack is older than 0 min"},
		{name: "DESIGN.md changed after the ack", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) {
				e.ack(t, p, e.now)
				os.WriteFile(filepath.Join(p, ".agents", "DESIGN.md"), []byte("v2"), 0o644)
			},
			want: ".agents/DESIGN.md changed since the last design-ack"},
		{name: "an unchanged DESIGN.md keeps the marker valid", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) { e.ack(t, p, e.now) }},
		{name: "a marker without a number epoch is unreadable", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) {
				m := designack.MarkerPath(e.cache, p)
				os.MkdirAll(filepath.Dir(m), 0o755)
				os.WriteFile(m, []byte("surface=x\nepoch=soon\n"), 0o644)
			},
			want: "the design-ack marker is unreadable"},
		{name: "another project's marker does not open this one", file: "src/Button.tsx",
			setup: func(t *testing.T, e env, p string) { e.ack(t, project(t, ""), e.now) },
			want:  "no design-ack for this project yet"},
		{name: "non-UI source needs no marker", file: "main.go"},
		{name: "test files are exempt", file: "src/Button.test.tsx", tool: "Write"},
		{name: "spec files are exempt", file: "src/Button.spec.tsx"},
		{name: "__tests__ is exempt", file: "src/__tests__/Button.tsx"},
		{name: "node_modules is exempt", file: "node_modules/x/y.css"},
		{name: "tools other than Edit/Write pass through", file: "src/Button.tsx", tool: "Bash"},
		{name: "the bypass allows", file: "src/Button.tsx",
			opts: []func(*GateOptions){func(o *GateOptions) { o.Off = true }}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			p := project(t, "v1")
			if c.setup != nil {
				c.setup(t, e, p)
			}
			tool := c.tool
			if tool == "" {
				tool = "Edit"
			}
			got := e.gate(edit(tool, filepath.Join(p, c.file), p, "s1"), c.opts...)
			want := ""
			if c.want != "" {
				want = deny(filepath.Base(c.file), c.want)
			}
			if got != want {
				t.Errorf("stdout =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestGateFailsOpenOnMalformedInput(t *testing.T) {
	e := newEnv(t)
	for _, bad := range []string{"", "not json at all", `{"tool_name":"Edit","tool_input":{"file_path":`, `{"tool_name":"Edit"}`, `{}`,
		`{"tool_name":"Edit","tool_input":{"file_path":""}}`, `{"tool_name":"Edit","tool_input":"x"}`} {
		if got := e.gate(bad); got != "" {
			t.Errorf("input %q: stdout = %q, want silence", bad, got)
		}
	}
}

func TestGateStampsSessionOnAllowedUIEditOnly(t *testing.T) {
	e := newEnv(t)
	p := project(t, "v1")
	stamp := filepath.Join(e.cache, "design-gate", "ui-edited.sess-stamp")
	has := func() bool { _, err := os.Stat(stamp); return err == nil }

	e.gate(edit("Edit", filepath.Join(p, "src/Button.tsx"), p, "sess-stamp"))
	if has() {
		t.Fatal("a denied UI edit must leave no stamp")
	}
	e.ack(t, p, e.now)
	e.gate(edit("Edit", filepath.Join(p, "main.go"), p, "sess-stamp"))
	if has() {
		t.Fatal("a non-UI edit must leave no stamp")
	}
	e.gate(edit("Edit", filepath.Join(p, "src/Button.tsx"), p, "bad/sid"))
	if _, err := os.Stat(filepath.Join(e.cache, "design-gate", "ui-edited.bad", "sid")); err == nil {
		t.Fatal("a session id outside [A-Za-z0-9._-] must not be used in a path")
	}
	if out := e.gate(edit("Edit", filepath.Join(p, "src/Button.tsx"), p, "sess-stamp")); out != "" {
		t.Fatalf("allowed edit printed %q", out)
	}
	b, err := os.ReadFile(stamp)
	if err != nil || string(b) != p {
		t.Fatalf("stamp = %q (%v), want the project root %q with no newline", b, err, p)
	}
}

func TestGateResolvesRelativePathsAndGitRoot(t *testing.T) {
	e := newEnv(t)
	p := project(t, "")
	deep := filepath.Join(p, "src", "deep")
	os.MkdirAll(deep, 0o755)
	e.ack(t, p, e.now) // marker is for the git toplevel
	if got := e.gate(edit("Edit", "Card.tsx", deep, "s")); got != "" {
		t.Errorf("a relative file resolved against cwd under the git root must use the root marker, got %q", got)
	}
	// No .git anywhere: the project is cwd.
	bare, _ := filepath.EvalSymlinks(t.TempDir())
	got := e.gate(edit("Edit", "Card.tsx", bare, "s"))
	if got != deny("Card.tsx", "no design-ack for this project yet") {
		t.Errorf("no git root: got %q", got)
	}
	e.ack(t, bare, e.now)
	if got := e.gate(edit("Edit", "Card.tsx", bare, "s")); got != "" {
		t.Errorf("ack for a non-git cwd must open it, got %q", got)
	}
}

func TestGateDenialIsOneLineAndNamesTheFile(t *testing.T) {
	e := newEnv(t)
	p := project(t, "")
	got := e.gate(edit("Edit", filepath.Join(p, `src/we"ird\.tsx`), p, "s"))
	if strings.Count(got, "\n") != 1 || !json.Valid([]byte(got)) {
		t.Fatalf("denial must be one valid JSON line, got %q", got)
	}
	if !strings.Contains(got, "we_ird_.tsx is a UI surface") {
		t.Errorf("quotes and backslashes in the name become _, got %q", got)
	}
}
