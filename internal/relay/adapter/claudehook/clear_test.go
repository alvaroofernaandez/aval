package claudehook

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// fakeHandoffs is an in-memory Handoffs.
type fakeHandoffs struct {
	cwd, owner, text string
	at               time.Time
}

func (f *fakeHandoffs) PutHandoff(cwd, owner, text string, at time.Time) error {
	f.cwd, f.owner, f.text, f.at = cwd, owner, text, at
	return nil
}

func (f *fakeHandoffs) TakeHandoff(cwd, owner string, now time.Time, maxAge time.Duration) (string, bool) {
	if f.cwd != cwd || f.owner != owner || f.text == "" || now.Sub(f.at) > maxAge {
		return "", false
	}
	text := f.text
	f.text = ""
	return text, true
}

const clearTranscript = `{"type":"user","message":{"role":"user","content":"Add retries to the upload client."}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done: retries are in client.go."}]}}
{"type":"user","message":{"role":"user","content":"Cap the backoff."}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/w/backoff.go"}}]}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Capped at 30 s."}]}}
`

func hookIn(t *testing.T, fields map[string]string) *bytes.Reader {
	t.Helper()
	b, _ := json.Marshal(fields)
	return bytes.NewReader(b)
}

func writeTranscript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var clearAt = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

func TestSessionEnd_ClearStoresTheCapsuleWithTheLastExchange(t *testing.T) {
	h := &fakeHandoffs{}
	tr := writeTranscript(t, clearTranscript)
	SessionEnd(hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "clear"}), Options{Handoff: h, Now: clearAt, CapsuleTokens: 8000})
	if h.cwd != "/w" || !h.at.Equal(clearAt) {
		t.Fatalf("stored for %q at %v", h.cwd, h.at)
	}
	for _, want := range []string{"Add retries to the upload client.", "Cap the backoff.", "Capped at 30 s.", "/w/backoff.go"} {
		if !strings.Contains(h.text, want) {
			t.Errorf("handoff lacks %q:\n%s", want, h.text)
		}
	}
}

func TestSessionEnd_OtherReasonsAndBadInputStoreNothing(t *testing.T) {
	tr := writeTranscript(t, clearTranscript)
	for name, in := range map[string]*bytes.Reader{
		"logout":        hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "logout"}),
		"no reason":     hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr}),
		"no transcript": hookIn(t, map[string]string{"cwd": "/w", "transcript_path": "/nope", "reason": "clear"}),
		"garbage":       bytes.NewReader([]byte("x")),
	} {
		h := &fakeHandoffs{}
		SessionEnd(in, Options{Handoff: h, Now: clearAt})
		if h.text != "" {
			t.Errorf("%s: stored %q", name, h.text)
		}
	}
	h := &fakeHandoffs{}
	SessionEnd(hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "clear"}), Options{Handoff: h, Now: clearAt, Paused: func(string) bool { return true }})
	if h.text != "" {
		t.Error("stored for a paused repo")
	}
}

func TestSessionStart_ClearInjectsTheHandoffOnce(t *testing.T) {
	h := &fakeHandoffs{cwd: "/w", text: "CAPSULE", at: clearAt}
	run := func(now time.Time) string {
		var out bytes.Buffer
		SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "clear"}), &out, Options{Handoff: h, Now: now, Owner: func() string { return "" }})
		return out.String()
	}
	var o struct {
		H struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	got := run(clearAt.Add(time.Minute))
	if err := json.Unmarshal([]byte(got), &o); err != nil || o.H.Event != "SessionStart" || o.H.Context != "CAPSULE" {
		t.Fatalf("got %q (%v)", got, err)
	}
	if again := run(clearAt.Add(2 * time.Minute)); again != "" {
		t.Errorf("second start printed %q", again)
	}
}

func TestSessionStart_ClearIgnoresStaleAndForeignHandoffs(t *testing.T) {
	var out bytes.Buffer
	h := &fakeHandoffs{cwd: "/w", text: "CAPSULE", at: clearAt}
	SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "clear"}), &out, Options{Handoff: h, Now: clearAt.Add(25 * time.Hour)})
	SessionStart(hookIn(t, map[string]string{"cwd": "/other", "source": "clear"}), &out, Options{Handoff: &fakeHandoffs{cwd: "/w", text: "C", at: clearAt}, Now: clearAt})
	SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "startup"}), &out, Options{Handoff: &fakeHandoffs{cwd: "/w", text: "C", at: clearAt}, Now: clearAt})
	if out.Len() != 0 {
		t.Errorf("printed %q, want nothing", out.String())
	}
}

func TestSessionEnd_AndStart_UseTheOwnerOfTheClaudeProcess(t *testing.T) {
	h := &fakeHandoffs{}
	owner := func() string { return "4242" }
	tr := writeTranscript(t, clearTranscript)
	SessionEnd(hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "clear"}), Options{Handoff: h, Now: clearAt, Owner: owner})
	if h.owner != "4242" {
		t.Fatalf("stored for owner %q, want 4242", h.owner)
	}
	var out bytes.Buffer
	SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "clear"}), &out, Options{Handoff: h, Now: clearAt.Add(time.Minute), Owner: func() string { return "9999" }})
	if out.Len() != 0 {
		t.Fatalf("another session took the handoff: %q", out.String())
	}
	SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "clear"}), &out, Options{Handoff: h, Now: clearAt.Add(time.Minute), Owner: owner})
	if out.Len() == 0 {
		t.Fatal("the owner did not get its handoff")
	}
}

func TestSessionStart_HandoffSurvivesHoursButNotADay(t *testing.T) {
	for age, want := range map[time.Duration]bool{12 * time.Hour: true, 25 * time.Hour: false} {
		var out bytes.Buffer
		h := &fakeHandoffs{cwd: "/w", text: "CAPSULE", at: clearAt}
		SessionStart(hookIn(t, map[string]string{"cwd": "/w", "source": "clear"}), &out, Options{Handoff: h, Now: clearAt.Add(age)})
		if (out.Len() > 0) != want {
			t.Errorf("after %v: printed=%v, want %v", age, out.Len() > 0, want)
		}
	}
}

func TestFromTranscript_ImageOnlyPromptKeepsTheLastTextRequest(t *testing.T) {
	c := FromTranscript(strings.NewReader(clearTranscript +
		`{"type":"user","message":{"role":"user","content":"[Image #11]"}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"[Image #12]"},{"type":"text","text":"[Pasted text #1 +9 lines]"}]}}` + "\n"))
	if c.LastPrompt != "Cap the backoff." {
		t.Fatalf("latest request %q, want the last text request", c.LastPrompt)
	}
}

const stampedTranscript = `{"type":"user","timestamp":"2026-11-26T10:00:00.000Z","message":{"role":"user","content":"Plan the work."}}
{"type":"assistant","timestamp":"2026-11-26T10:00:05.000Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/w/odd/tasks/x.md"}}]}}
{"type":"user","timestamp":"2026-11-26T10:05:00.000Z","message":{"role":"user","content":"Go on."}}
`

func TestSessionEnd_AddsPlanAndWorkingTreeFacts(t *testing.T) {
	h := &fakeHandoffs{}
	var gotFiles []string
	var gotBranch string
	var gotSince time.Time
	opt := Options{
		Handoff: h, Now: clearAt, CapsuleTokens: 8000,
		Repo: func(string) *relay.RepoFacts { return &relay.RepoFacts{Branch: "feat/x"} },
		Plans: func(cwd string, files []string, branch string) []relay.PlanFacts {
			gotFiles, gotBranch = files, branch
			return []relay.PlanFacts{{Title: "Plan X", Items: []string{"do the thing"}}}
		},
		Tree: func(cwd string, since time.Time) *relay.TreeFacts {
			gotSince = since
			return &relay.TreeFacts{Dirty: []string{" M a.go"}, Commits: []string{"abc1234 feat: y"}}
		},
	}
	tr := writeTranscript(t, stampedTranscript)
	SessionEnd(hookIn(t, map[string]string{"cwd": "/w", "transcript_path": tr, "reason": "clear"}), opt)
	if len(gotFiles) != 1 || gotFiles[0] != "/w/odd/tasks/x.md" || gotBranch != "feat/x" {
		t.Errorf("plans asked with %v on %q", gotFiles, gotBranch)
	}
	if want := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC); !gotSince.Equal(want) {
		t.Errorf("tree since %v, want the first transcript timestamp", gotSince)
	}
	for _, want := range []string{"## Open plan tasks\nPlan X\n- [ ] do the thing", "## Working tree", "abc1234 feat: y"} {
		if !strings.Contains(h.text, want) {
			t.Errorf("handoff lacks %q:\n%s", want, h.text)
		}
	}
}

var updateGolden = flag.Bool("update", false, "rewrite the goldens under testdata")

// A realistic session: image-only and pasted-text prompts, a repeated request,
// a long log, tool results, meta and sidechain entries, a touched plan.
func TestSessionEnd_GoldenOnARealisticTranscript(t *testing.T) {
	h := &fakeHandoffs{}
	const budget = 8000
	opt := Options{
		Handoff: h, Now: clearAt, CapsuleTokens: budget,
		Repo: func(string) *relay.RepoFacts { return &relay.RepoFacts{Branch: "feat/relay-autoclear", Changed: 3} },
		Plans: func(string, []string, string) []relay.PlanFacts {
			return []relay.PlanFacts{{Title: "Relay auto-clear + leaner floor", Items: []string{"Open plan tasks section", "Working tree section", "Golden test on a real transcript fixture"}}}
		},
		Tree: func(string, time.Time) *relay.TreeFacts {
			return &relay.TreeFacts{Dirty: []string{" M internal/relay/capsule.go", "?? internal/relay/adapter/workfacts/"}, Commits: []string{"c298e4d feat(relay): robust handoff"}}
		},
	}
	SessionEnd(hookIn(t, map[string]string{"cwd": "/w/aval", "transcript_path": filepath.Join("testdata", "session.jsonl"), "reason": "clear"}), opt)
	path := filepath.Join("testdata", "handoff.golden")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(h.text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if h.text != string(want) {
		t.Fatalf("handoff differs from %s:\n--- got\n%s\n--- want\n%s", path, h.text, want)
	}
	if max := int(budget*1.75) + maxReply + 100; len(h.text) > max {
		t.Errorf("handoff is %d bytes, over the budget of %d", len(h.text), max)
	}
	for _, gone := range []string{"[Image #1]", "[Pasted text", "sub/agent.go", "system-reminder"} {
		if strings.Contains(h.text, gone) {
			t.Errorf("handoff keeps %q", gone)
		}
	}
	if strings.Count(h.text, "- run the tests\n")+strings.Count(h.text, "Latest request: run the tests") > 1 {
		t.Error("a repeated request was kept twice")
	}
}
