package claudehook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

func fixture(t *testing.T) relay.Capsule {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return FromTranscript(f)
}

func TestFromTranscript_PersonRequestsOnly(t *testing.T) {
	c := fixture(t)
	if c.FirstPrompt != "Add retries to the upload client." {
		t.Errorf("first: got %q", c.FirstPrompt)
	}
	if c.LastPrompt != "And add a test for it." {
		t.Errorf("last: got %q", c.LastPrompt)
	}
	if want := []string{"Now cap the backoff at 30 s."}; !slices.Equal(c.Prompts, want) {
		t.Errorf("prompts: got %q, want %q (no meta, command, tool result or sidechain)", c.Prompts, want)
	}
}

func TestFromTranscript_EditedFilesNewestFirst(t *testing.T) {
	want := []string{"/work/repo/upload/client.go", "/work/repo/upload/backoff.go"}
	if got := fixture(t).Files; !slices.Equal(got, want) {
		t.Errorf("files: got %q, want %q (edits only, newest first, once each)", got, want)
	}
}

func TestFromTranscript_EarlierCapsuleGivesTheFirstRequest(t *testing.T) {
	capsule := relay.CapsuleHeader + "\n" + relay.CapsuleFirstRequest + "Original ask.\n"
	in := `{"type":"user","message":{"role":"user","content":` + quote(capsule) + `}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"Next ask."}}` + "\n"
	c := FromTranscript(strings.NewReader(in))
	if c.FirstPrompt != "Original ask." || c.LastPrompt != "Next ask." {
		t.Errorf("got first %q last %q, want the capsule's first request and the next ask", c.FirstPrompt, c.LastPrompt)
	}
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func hookInput(t *testing.T, extra string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return `{"session_id":"s","cwd":"/work/repo","transcript_path":` + quote(abs) + extra + `}`
}

var repoFacts = &relay.RepoFacts{Branch: "feat/upload-retry"}

func opts() Options {
	return Options{Repo: func(string) *relay.RepoFacts { return repoFacts }, CapsuleTokens: 8000}
}

func want(t *testing.T) relay.Capsule {
	c := fixture(t)
	c.Repo = repoFacts
	return c
}

func TestPreCompact_PrintsTheSteerText(t *testing.T) {
	var out strings.Builder
	PreCompact(strings.NewReader(hookInput(t, `,"hook_event_name":"PreCompact","trigger":"auto"`)), &out, opts())
	if got, w := out.String(), relay.SteerText(want(t)); got != w {
		t.Fatalf("got %q, want %q", got, w)
	}
}

func TestSessionStart_AddsTheCapsuleAfterCompaction(t *testing.T) {
	var out strings.Builder
	SessionStart(strings.NewReader(hookInput(t, `,"hook_event_name":"SessionStart","source":"compact"`)), &out, opts())
	var got struct {
		H struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output %q is not one JSON value: %v", out.String(), err)
	}
	if got.H.Event != "SessionStart" || got.H.Context != relay.Render(want(t), 8000) {
		t.Fatalf("got %+v, want SessionStart with the rendered capsule", got.H)
	}
}

func TestHooks_PrintNothing(t *testing.T) {
	relayed := opts()
	relayed.Relayed = true
	paused := opts()
	paused.Paused = func(dir string) bool { return dir == "/work/repo" }
	cases := []struct {
		name string
		hook func(io.Reader, io.Writer, Options)
		in   string
		opt  Options
	}{
		{"session start after a mod relay", SessionStart, hookInput(t, `,"source":"compact"`), relayed},
		{"session start on startup", SessionStart, hookInput(t, `,"source":"startup"`), opts()},
		{"session start, bad input", SessionStart, `{"source":`, opts()},
		{"pre compact, bad input", PreCompact, `nope`, opts()},
		{"pre compact, paused repo", PreCompact, hookInput(t, ""), paused},
		{"session start, paused repo", SessionStart, hookInput(t, `,"source":"compact"`), paused},
		{"pre compact, no transcript", PreCompact, `{"transcript_path":"/does/not/exist"}`, opts()},
		{"pre compact, empty transcript", PreCompact, `{"transcript_path":""}`, opts()},
	}
	for _, c := range cases {
		var out strings.Builder
		c.hook(strings.NewReader(c.in), &out, c.opt)
		if out.Len() != 0 {
			t.Errorf("%s: printed %q, want nothing", c.name, out.String())
		}
	}
}
