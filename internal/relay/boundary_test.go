package relay

import (
	"strings"
	"testing"
)

func bash(cmd string, isErr bool) ToolCall {
	return ToolCall{Tool: "Bash", Input: ToolInput{Command: cmd}, IsError: isErr}
}

func edit(path, old, new string) ToolCall {
	return ToolCall{Tool: "Edit", Input: ToolInput{FilePath: path, OldString: old, NewString: new}}
}

func req(tokens int, reply string, tools ...ToolCall) BoundaryRequest {
	return BoundaryRequest{Version: ProtocolVersion, Tokens: tokens, Turn: BoundaryTurn{Reply: reply, Tools: tools}}
}

func TestBoundary_Signals(t *testing.T) {
	tests := []struct {
		name string
		tool ToolCall
		want string
	}{
		{"commit", bash(`git commit -m "feat: x"`, false), "commit"},
		{"commit with -C", bash(`git -C /w/repo commit -m x`, false), "commit"},
		{"commit after add", bash(`git add -A && git commit -m x`, false), "commit"},
		{"failed commit", bash(`git commit -m x`, true), ""},
		{"commit-tree is not a commit", bash(`git commit-tree abc`, false), ""},
		{"log mentioning commit", bash(`git log --oneline`, false), ""},
		{"plan box checked", edit("/w/odd/tasks/plan.md", "- [ ] T1", "- [x] T1"), "plan-task"},
		{"relative plan path", edit("odd/tasks/plan.md", "- [ ] T1", "- [x] T1"), "plan-task"},
		{"box text edited only", edit("/w/odd/tasks/plan.md", "- [ ] T1", "- [ ] T1 reworded"), ""},
		{"box already checked", edit("/w/odd/tasks/plan.md", "- [x] T1", "- [x] T1 done"), ""},
		{"not a plan file", edit("/w/README.md", "- [ ] a", "- [x] a"), ""},
		{"nested md is not a plan", edit("/w/odd/tasks/sub/x.md", "- [ ] a", "- [x] a"), ""},
		{"plan write with a checked box", ToolCall{Tool: "Write", Input: ToolInput{FilePath: "/w/odd/tasks/p.md", Content: "- [x] a"}}, "plan-task"},
		{"errored plan edit", ToolCall{Tool: "Edit", IsError: true, Input: ToolInput{FilePath: "/w/odd/tasks/p.md", OldString: "- [ ] a", NewString: "- [x] a"}}, ""},
	}
	for _, tc := range tests {
		got := Signal(BoundaryTurn{Tools: []ToolCall{tc.tool}})
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestBoundary_CommitWinsOverPlanTask(t *testing.T) {
	turn := BoundaryTurn{Tools: []ToolCall{edit("/w/odd/tasks/p.md", "- [ ] a", "- [x] a"), bash("git commit -m x", false)}}
	if got := Signal(turn); got != "commit" {
		t.Fatalf("got %q, want commit", got)
	}
}

func TestDecideBoundary_Clears(t *testing.T) {
	a := DefaultPolicy().DecideBoundary(true, req(120_000, "All done.", bash("git commit -m x", false)))
	if a.Action != AnswerClear || a.Reason != "commit" {
		t.Fatalf("got %+v, want clear for commit", a)
	}
	for _, want := range []string{"120k tokens", "commit", "All done.", "Reply only: relayed."} {
		if !strings.Contains(a.Text, want) {
			t.Errorf("text %q lacks %q", a.Text, want)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(a.Text), "Reply only: relayed.") {
		t.Errorf("the instruction must be the last line: %q", a.Text)
	}
}

func TestDecideBoundary_None(t *testing.T) {
	p := DefaultPolicy()
	commit := bash("git commit -m x", false)
	tests := []struct {
		name    string
		enabled bool
		req     BoundaryRequest
		reason  string
	}{
		{"relay off", false, req(200_000, "r", commit), "disabled"},
		{"below threshold", true, req(99_999, "r", commit), "below_threshold"},
		{"no signal", true, req(200_000, "r", bash("ls", false)), "no_signal"},
		{"wrong version", true, BoundaryRequest{Version: 9, Tokens: 200_000, Turn: BoundaryTurn{Tools: []ToolCall{commit}}}, "version"},
		{"no tokens", true, req(0, "r", commit), "below_threshold"},
	}
	for _, tc := range tests {
		a := p.DecideBoundary(tc.enabled, tc.req)
		if a.Action != AnswerNone || a.Reason != tc.reason || a.Text != "" {
			t.Errorf("%s: got %+v, want none/%s", tc.name, a, tc.reason)
		}
	}
}

func TestDecideBoundary_ThresholdIsInclusiveAndConfigurable(t *testing.T) {
	p := DefaultPolicy()
	if a := p.DecideBoundary(true, req(100_000, "r", bash("git commit -m x", false))); a.Action != AnswerClear {
		t.Fatalf("at the threshold: got %+v, want clear", a)
	}
	p.BoundaryTokens = 5_000
	if a := p.DecideBoundary(true, req(5_000, "r", bash("git commit -m x", false))); a.Action != AnswerClear {
		t.Fatalf("lowered threshold: got %+v, want clear", a)
	}
}

func TestDecideBoundary_ReplyIsRedactedAndCapped(t *testing.T) {
	secret := "ghp_" + strings.Repeat("a", 36)
	reply := "key " + secret + " " + strings.Repeat("x", 6000)
	a := DefaultPolicy().DecideBoundary(true, req(150_000, reply, bash("git commit -m x", false)))
	if strings.Contains(a.Text, secret) {
		t.Fatal("the credential reached the text")
	}
	if len(a.Text) > BoundaryReplyMax+600 {
		t.Fatalf("text is %d bytes, the reply cap is %d", len(a.Text), BoundaryReplyMax)
	}
	if !strings.Contains(a.Text, "[...]") {
		t.Fatal("a capped reply carries the truncation marker")
	}
}

func TestDecideBoundary_EmptyReplyStillClears(t *testing.T) {
	a := DefaultPolicy().DecideBoundary(true, req(150_000, "", bash("git commit -m x", false)))
	if a.Action != AnswerClear || strings.Contains(a.Text, "shown again") {
		t.Fatalf("got %+v: no reply to show, but the relay still happens", a)
	}
}

func TestTruncateBytes(t *testing.T) {
	if got := TruncateBytes("héllo", 2); got != "h [...]" {
		t.Fatalf("got %q, must cut on a rune start", got)
	}
	if got := TruncateBytes("abc", 3); got != "abc" {
		t.Fatalf("got %q", got)
	}
}
