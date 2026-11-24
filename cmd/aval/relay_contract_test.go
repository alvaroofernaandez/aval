package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/agent/statusline"
	"github.com/alvaroofernaandez/aval/internal/relay/adapter/statefs"
)

// The status line reads metrics.jsonl itself (it imports nothing internal):
// this pins that what statefs writes is what statusline counts.
func TestRelayContract_StatuslineCountsWhatStatefsWrites(t *testing.T) {
	dir, repo := t.TempDir(), t.TempDir()
	store := statefs.Store{Dir: dir}
	now := time.Now()
	for _, m := range []statefs.Metric{
		{At: now.Add(-time.Hour), Session: "a", Trigger: "auto", Action: "replace", Before: 200_000, After: 40_000, Repo: repo},
		{At: now.Add(-time.Minute), Session: "b", Trigger: "manual", Action: "replace", Before: 90_000, After: 30_000, Repo: repo},
		{At: now, Session: "c", Repo: filepath.Join(repo, "other")},
	} {
		if err := store.AppendMetric(m); err != nil {
			t.Fatal(err)
		}
	}
	in := `{"workspace":{"current_dir":` + jsonString(repo) + `,"project_dir":` + jsonString(repo) + `}}`
	if got := statusline.RenderWith(strings.NewReader(in), statusline.Options{RelayDir: dir, Now: now}); !strings.HasSuffix(got, "relay 2") {
		t.Fatalf("status line %q, want relay 2", got)
	}
}

// `aval statusline` finds the relay state where the relay writes it.
func TestStatuslineShowsTheRelaySegment(t *testing.T) {
	state, repo := t.TempDir(), t.TempDir()
	store := statefs.Store{Dir: filepath.Join(state, "aval", "relay")}
	if err := store.AppendMetric(statefs.Metric{At: time.Now(), Session: "a", Action: "replace", Repo: repo}); err != nil {
		t.Fatal(err)
	}
	in := `{"workspace":{"current_dir":` + jsonString(repo) + `,"project_dir":` + jsonString(repo) + `}}`
	r := runIn(t, map[string]string{"XDG_STATE_HOME": state}, in, "statusline")
	if r.code != 0 || !strings.HasSuffix(strings.TrimSpace(r.stdout), "relay 1") {
		t.Fatalf("exit %d, stdout %q; want the relay 1 segment", r.code, r.stdout)
	}
}
