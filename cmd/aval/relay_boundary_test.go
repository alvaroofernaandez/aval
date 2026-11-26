package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

const boundaryCommit = `{"version":1,"tokens":150000,"turn":{"reply":"Committed.","tools":[{"tool":"Bash","input":{"command":"git commit -m x"}}]}}`

func boundary(t *testing.T, env map[string]string, stdin string) relay.BoundaryAnswer {
	t.Helper()
	r := runIn(t, env, stdin, "relay", "boundary")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	var a relay.BoundaryAnswer
	if err := json.Unmarshal([]byte(r.stdout), &a); err != nil {
		t.Fatalf("stdout is not an answer: %q (%v)", r.stdout, err)
	}
	return a
}

func TestRelayBoundary_ClearsOnACommitAboveTheThreshold(t *testing.T) {
	env := map[string]string{"XDG_STATE_HOME": t.TempDir(), "AVAL_RELAY": "1"}
	a := boundary(t, env, boundaryCommit)
	if a.Action != relay.AnswerClear || a.Reason != "commit" || !strings.Contains(a.Text, "Committed.") {
		t.Fatalf("got %+v, want clear for commit with the reply", a)
	}
}

func TestRelayBoundary_NoneWhenItMustNotClear(t *testing.T) {
	state := t.TempDir()
	store := statefs.Store{Dir: filepath.Join(state, "aval", "relay"), Alive: func(int, int64) bool { return false }}
	if err := store.Prepare("relay-abc", statefs.Entry{Trigger: "auto", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, stdin, reason string
		env                 map[string]string
	}{
		{"relay off", boundaryCommit, relay.ReasonDisabled, map[string]string{"XDG_STATE_HOME": t.TempDir()}},
		{"AVAL_RELAY=0", boundaryCommit, relay.ReasonDisabled, map[string]string{"XDG_STATE_HOME": t.TempDir(), "AVAL_RELAY": "0"}},
		{"not json", "nope", relay.ReasonMalformed, map[string]string{"XDG_STATE_HOME": t.TempDir(), "AVAL_RELAY": "1"}},
		{"empty stdin", "", relay.ReasonMalformed, map[string]string{"XDG_STATE_HOME": t.TempDir(), "AVAL_RELAY": "1"}},
		{"below the threshold", strings.Replace(boundaryCommit, "150000", "5000", 1), relay.ReasonBelowThreshold, map[string]string{"XDG_STATE_HOME": t.TempDir(), "AVAL_RELAY": "1"}},
		{"a relay just happened", boundaryCommit, reasonRecent, map[string]string{"XDG_STATE_HOME": state, "AVAL_RELAY": "1"}},
	} {
		if a := boundary(t, c.env, c.stdin); a.Action != relay.AnswerNone || a.Reason != c.reason || a.Text != "" {
			t.Errorf("%s: got %+v, want none/%s", c.name, a, c.reason)
		}
	}
}

func TestRelayBoundary_ThresholdFromTheEnvironment(t *testing.T) {
	small := strings.Replace(boundaryCommit, "150000", "5000", 1)
	env := map[string]string{"XDG_STATE_HOME": t.TempDir(), "AVAL_RELAY": "1", "AVAL_RELAY_BOUNDARY_TOKENS": "4000"}
	if a := boundary(t, env, small); a.Action != relay.AnswerClear {
		t.Fatalf("got %+v, want clear with the threshold at 4000", a)
	}
	env["AVAL_RELAY_BOUNDARY_TOKENS"] = "junk"
	if a := boundary(t, env, small); a.Action != relay.AnswerNone {
		t.Fatalf("got %+v, want none: junk keeps the default threshold", a)
	}
}
