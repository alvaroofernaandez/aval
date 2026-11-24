package main

import (
	"encoding/json"
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

// relayJSON runs `aval relay <sub> --json` with HOME and the state dir under
// home and decodes stdout into v.
func relayJSON(t *testing.T, home, sub string, v any) {
	t.Helper()
	env := map[string]string{"HOME": home, "USERPROFILE": home, "XDG_STATE_HOME": filepath.Join(home, "state"), "AVAL_NO_UPDATE_CHECK": "1"}
	r := runIn(t, env, "", "relay", sub, "--json")
	if r.code != 0 {
		t.Fatalf("relay %s: exit %d, stderr %q", sub, r.code, r.stderr)
	}
	if err := json.Unmarshal([]byte(r.stdout), v); err != nil {
		t.Fatalf("stdout is not JSON: %q (%v)", r.stdout, err)
	}
}

type statusJSON struct {
	Enabled bool            `json:"enabled"`
	Window  int             `json:"window"`
	Runtime string          `json:"runtime"`
	Tier    string          `json:"tier"`
	Source  string          `json:"source"`
	ErrPct  float64         `json:"err_pct"`
	Paused  bool            `json:"paused"`
	Relays  int             `json:"relays"`
	Last    *statefs.Metric `json:"last"`
}

func TestRelayStatus_PrintsTierSourceAndBound(t *testing.T) {
	home := relayHome(t, `{"env":{"AVAL_RELAY":"1","CLAUDE_CODE_AUTO_COMPACT_WINDOW":"233000"}}`, true)
	state := filepath.Join(home, "state")
	compact(t, state, relayFixture(t), nil)
	commitRelay(t, state)
	var st statusJSON
	relayJSON(t, home, "status", &st)
	if !st.Enabled || st.Window != 233_000 || st.Runtime != "claude-code" || st.Tier != "T0" ||
		st.Source != "exact" || st.ErrPct != 0.02 || st.Relays != 1 || st.Last == nil || st.Last.After != 61_000 {
		t.Fatalf("status %+v", st)
	}
}

func TestRelayStatus_NoStateIsNotAnError(t *testing.T) {
	var st statusJSON
	relayJSON(t, t.TempDir(), "status", &st)
	if st.Enabled || st.Relays != 0 || st.Last != nil || st.Tier != "T0" {
		t.Fatalf("status %+v, want off, no relays, tier still reported", st)
	}
}

func TestRelayLog_ReadsMetrics(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(home, "state")
	compact(t, state, relayFixture(t), nil)
	commitRelay(t, state)
	var got []statefs.Metric
	relayJSON(t, home, "log", &got)
	if len(got) != 1 || got[0].Before != 63_795 || got[0].After != 61_000 {
		t.Fatalf("log %+v, want the one committed relay", got)
	}
}

func TestRelayLog_NoStateIsAnEmptyList(t *testing.T) {
	var got []statefs.Metric
	relayJSON(t, t.TempDir(), "log", &got)
	if got == nil || len(got) != 0 {
		t.Fatalf("log %#v, want an empty list", got)
	}
}
