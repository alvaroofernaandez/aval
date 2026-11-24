package main

import (
	"path/filepath"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

const commitInput = `{"version":1,"session_id":"session-a","tokens":61000}`

func commitRelay(t *testing.T, state string) {
	t.Helper()
	if r := runIn(t, map[string]string{"XDG_STATE_HOME": state}, commitInput, "relay", "commit"); r.code != 0 {
		t.Fatalf("commit: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestRelayCommit_AppendsOneMetric(t *testing.T) {
	state := t.TempDir()
	compact(t, state, relayFixture(t), nil)
	commitRelay(t, state)
	commitRelay(t, state) // a second measure of the same relay adds nothing
	got, err := statefs.Store{Dir: filepath.Join(state, "aval", "relay")}.ReadMetrics()
	if err != nil || len(got) != 1 {
		t.Fatalf("metrics %+v, %v; want exactly one", got, err)
	}
	m := got[0]
	if m.Session != "session-a" || m.Trigger != "auto" || m.Action != "replace" || m.Source != "exact" ||
		m.Before != 63_795 || m.After != 61_000 || m.At.IsZero() {
		t.Fatalf("metric %+v", m)
	}
}

func TestRelayCommit_WithoutARelayAppendsNothing(t *testing.T) {
	state := t.TempDir()
	commitRelay(t, state)
	if got, _ := (statefs.Store{Dir: filepath.Join(state, "aval", "relay")}).ReadMetrics(); len(got) != 0 {
		t.Fatalf("metrics %+v, want none", got)
	}
}
