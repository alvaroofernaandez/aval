package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/relay"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

func relayFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "relay", "compact-auto.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// compact runs `aval relay compact` with stdin and a state dir under state.
func compact(t *testing.T, state, stdin string, extra map[string]string) (relay.Response, invocation) {
	t.Helper()
	env := map[string]string{"XDG_STATE_HOME": state, "AVAL_NO_UPDATE_CHECK": "1"}
	for k, v := range extra {
		env[k] = v
	}
	r := runIn(t, env, stdin, "relay", "compact")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	var resp relay.Response
	if err := json.Unmarshal([]byte(r.stdout), &resp); err != nil {
		t.Fatalf("stdout is not a response: %q (%v)", r.stdout, err)
	}
	return resp, r
}

func TestRelayCompact_AnswersWithCapsuleAndHandles(t *testing.T) {
	resp, _ := compact(t, t.TempDir(), relayFixture(t), nil)
	if resp.Action != relay.AnswerReplace || !strings.Contains(resp.Capsule, "First request: Turn 1. Write about a lighthouse.") {
		t.Fatalf("got %+v, want replace with a capsule holding the first request", resp)
	}
	if len(resp.Keep) == 0 || resp.Keep[len(resp.Keep)-1] != "h-06" {
		t.Fatalf("kept %v, want a tail ending at the last handle h-06", resp.Keep)
	}
}

func TestRelayCompact_ManualTrigger(t *testing.T) {
	in := strings.Replace(relayFixture(t), `"trigger": "auto"`, `"trigger": "manual"`, 1)
	if resp, _ := compact(t, t.TempDir(), in, nil); resp.Action != relay.AnswerReplace {
		t.Fatalf("manual compaction: got %+v, want replace", resp)
	}
}

func TestRelayCompact_Passthrough(t *testing.T) {
	cases := []struct {
		name, stdin, reason string
		env                 map[string]string
	}{
		{"ProtocolVersionMismatch", `{"version":2,"event":"compact","session_id":"s"}`, relay.ReasonVersion, nil},
		{"BadJSON", `{"version":`, relay.ReasonMalformed, nil},
		{"Empty", ``, relay.ReasonMalformed, nil},
		{"DisabledByEnv", "", reasonDisabled, map[string]string{"AVAL_RELAY": "0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdin := c.stdin
			if c.name == "DisabledByEnv" {
				stdin = relayFixture(t)
			}
			resp, r := compact(t, t.TempDir(), stdin, c.env)
			if resp.Action != relay.AnswerPassthrough || resp.Reason != c.reason {
				t.Fatalf("got %+v, want passthrough/%s", resp, c.reason)
			}
			if r.stderr != "" {
				t.Errorf("stderr %q: a hook path prints nothing", r.stderr)
			}
		})
	}
}

func TestRelayCompact_SecondRelayInSessionSkips(t *testing.T) {
	state := t.TempDir()
	store := statefs.Store{Dir: filepath.Join(state, "aval", "relay"), PID: 1, Start: 1 << 62}
	release, err := store.Lock("session-a")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if resp, _ := compact(t, state, relayFixture(t), nil); resp.Action != relay.AnswerPassthrough || resp.Reason != reasonLocked {
		t.Fatalf("got %+v, want passthrough/%s", resp, reasonLocked)
	}
}

func TestRelayCompact_RecordsTheRelay(t *testing.T) {
	state := t.TempDir()
	compact(t, state, relayFixture(t), nil)
	store := statefs.Store{Dir: filepath.Join(state, "aval", "relay")}
	e, err := store.Read("session-a")
	if err != nil || e.State != statefs.Prepared || e.TokensBefore != 63_795 || e.Trigger != "auto" {
		t.Fatalf("journal: %+v, %v", e, err)
	}
	sum, err := store.ReadSummary("session-a")
	if err != nil || sum.Relays != 1 || sum.Before != 63_795 {
		t.Fatalf("summary: %+v, %v", sum, err)
	}
	chain, err := store.ReadChain("session-a")
	if err != nil || chain.Epoch() != 1 || chain.Links[0].Fingerprint != "h-06" {
		t.Fatalf("chain: %+v, %v", chain, err)
	}
}

func TestRelayCompact_NoProgressBrakesTheSecondRelay(t *testing.T) {
	state := t.TempDir()
	compact(t, state, relayFixture(t), nil)
	resp, _ := compact(t, state, relayFixture(t), nil)
	if resp.Action != relay.AnswerPassthrough || resp.Reason != "brake:no_progress" {
		t.Fatalf("got %+v, want passthrough/brake:no_progress", resp)
	}
}

func TestRelayUnknownSubcommandIsAUsageError(t *testing.T) {
	for _, sub := range [][]string{nil, {"bogus"}} {
		var out bytes.Buffer
		err := runRelay(sub, uiFlags{}, cmdIO{stdin: strings.NewReader(""), stdout: &out, stderr: &out, env: envMap(nil)})
		var u *usageError
		if !errors.As(err, &u) {
			t.Errorf("relay %v: got %v, want a usage error", sub, err)
		}
	}
}

func TestRelayPauseAndResume(t *testing.T) {
	state, repo := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := relayFixture(t)
	t.Chdir(repo)
	env := map[string]string{"XDG_STATE_HOME": state, "AVAL_NO_UPDATE_CHECK": "1", "AVAL_LANG": "en"}

	if r := runIn(t, env, "", "relay", "pause"); r.code != 0 {
		t.Fatalf("pause: exit %d, stderr %q", r.code, r.stderr)
	}
	if resp, _ := compact(t, state, fixture, nil); resp.Action != relay.AnswerPassthrough || resp.Reason != reasonPaused {
		t.Fatalf("paused: got %+v, want passthrough/%s", resp, reasonPaused)
	}
	if r := runIn(t, env, "", "relay", "resume"); r.code != 0 {
		t.Fatalf("resume: exit %d, stderr %q", r.code, r.stderr)
	}
	if resp, _ := compact(t, state, fixture, nil); resp.Action != relay.AnswerReplace {
		t.Fatalf("resumed: got %+v, want replace", resp)
	}
}
