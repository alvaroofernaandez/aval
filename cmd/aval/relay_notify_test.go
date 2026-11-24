package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/relay/adapter/notify"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

const notifyInput = `{"version":1,"session_id":"session-a"}`

// fakeNotifier replaces the platform with a Linux that has notify-send and
// records every command started; fail makes each start fail.
func fakeNotifier(t *testing.T, fail error) *[][]string {
	t.Helper()
	var started [][]string
	platform, run := notifyPlatform, notifyRun
	t.Cleanup(func() { notifyPlatform, notifyRun = platform, run })
	notifyPlatform = func(func(string) string) notify.Platform {
		return notify.Platform{GOOS: "linux", LookPath: func(string) (string, error) { return "/usr/bin/notify-send", nil }}
	}
	notifyRun = func(_ context.Context, argv, _ []string) error {
		started = append(started, argv)
		return fail
	}
	return &started
}

func notifyCmd(t *testing.T, state string, extra map[string]string) invocation {
	t.Helper()
	env := map[string]string{"XDG_STATE_HOME": state}
	for k, v := range extra {
		env[k] = v
	}
	return runIn(t, env, notifyInput, "relay", "notify")
}

func TestRelayNotify_ShowsTheTierText(t *testing.T) {
	started := fakeNotifier(t, nil)
	state := t.TempDir()
	compact(t, state, relayFixture(t), nil)
	commitRelay(t, state)
	if r := notifyCmd(t, state, nil); r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Fatalf("notify: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if len(*started) != 1 || (*started)[0][len((*started)[0])-1] != "Relayed 64k → 61k" {
		t.Fatalf("started %q, want one notify-send with the T0 text", *started)
	}
}

func TestNotify_FailureNeverFailsTheRelay(t *testing.T) {
	fakeNotifier(t, errors.New("notify-send: no session bus"))
	state := t.TempDir()
	compact(t, state, relayFixture(t), nil)
	commitRelay(t, state)
	if r := notifyCmd(t, state, nil); r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Fatalf("a failed notifier: exit %d, stdout %q, stderr %q; want silent exit 0", r.code, r.stdout, r.stderr)
	}
	store := statefs.Store{Dir: state + "/aval/relay"}
	if e, err := store.Read("session-a"); err != nil || e.State != statefs.Committed {
		t.Fatalf("journal after a failed notifier: %+v, %v", e, err)
	}
}

func TestRelayNotify_OffOrNothingToShow(t *testing.T) {
	started := fakeNotifier(t, nil)
	state := t.TempDir()
	notifyCmd(t, state, nil) // no relay in the journal
	compact(t, state, relayFixture(t), nil)
	notifyCmd(t, state, nil) // prepared, not yet committed: no after figure
	commitRelay(t, state)
	notifyCmd(t, state, map[string]string{"AVAL_RELAY_NOTIFY": "0"})
	if len(*started) != 0 {
		t.Fatalf("started %q, want nothing", *started)
	}
}

func TestRelayNotify_IsAHookPath(t *testing.T) {
	for _, in := range []string{"", "{}", "not json", `{"version":2,"session_id":"a"}`, `{"version":1,"session_id":"../x"}`} {
		r := runIn(t, map[string]string{"XDG_STATE_HOME": t.TempDir()}, in, "relay", "notify")
		if r.code != 0 || r.stdout != "" || r.stderr != "" {
			t.Errorf("input %q: exit %d, stdout %q, stderr %q", in, r.code, r.stdout, r.stderr)
		}
	}
}
