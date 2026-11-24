package relay

import (
	"maps"
	"slices"
	"testing"
)

func states(l Ledger) map[string]JobState {
	out := map[string]JobState{}
	for _, j := range l.Jobs {
		out[j.ID] = j.State
	}
	return out
}

func TestLedger_RunningJobIsNeverRestartable(t *testing.T) {
	for state, want := range map[JobState]bool{JobRunning: false, JobFinished: false, JobLost: true, JobOrphaned: true} {
		if got := (Job{State: state}).Restartable(); got != want {
			t.Errorf("state %d: restartable %v, want %v", state, got, want)
		}
	}
}

func TestLedger_Replay(t *testing.T) {
	l := Replay([]Event{{JobID: "a", Kind: "bash"}, {JobID: "b", Kind: "subagent"}, {JobID: "a", Finished: true}})
	want := map[string]JobState{"a": JobFinished, "b": JobRunning}
	if got := states(l); !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if l.Jobs[0].Kind != "bash" {
		t.Errorf("finishing a job dropped its kind: %+v", l.Jobs[0])
	}
}

func TestLedger_Merge(t *testing.T) {
	a := Replay([]Event{{JobID: "x"}, {JobID: "y"}})
	b := Replay([]Event{{JobID: "x"}, {JobID: "x", Finished: true}, {JobID: "z"}})
	want := map[string]JobState{"x": JobFinished, "y": JobRunning, "z": JobRunning}
	if got := states(Merge(a, b)); !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLedger_Reconcile_LostAndOrphaned(t *testing.T) {
	l := Replay([]Event{{JobID: "alive"}, {JobID: "gone"}, {JobID: "done"}, {JobID: "done", Finished: true}})
	got := states(l.Reconcile([]string{"alive", "stranger"}))
	want := map[string]JobState{"alive": JobRunning, "gone": JobLost, "done": JobFinished, "stranger": JobOrphaned}
	if !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLedger_EpochCarriesLedger(t *testing.T) {
	l := Replay([]Event{{JobID: "a"}, {JobID: "b"}, {JobID: "b", Finished: true}})
	next := l.Next()
	if next.Epoch != l.Epoch+1 {
		t.Fatalf("epoch %d, want %d", next.Epoch, l.Epoch+1)
	}
	ids := []string{}
	for _, j := range next.Jobs {
		ids = append(ids, j.ID)
		if j.Epoch != next.Epoch {
			t.Errorf("job %s carried with epoch %d, want %d", j.ID, j.Epoch, next.Epoch)
		}
	}
	if !slices.Equal(ids, []string{"a"}) {
		t.Fatalf("carried %v, want only the unfinished job [a]", ids)
	}
}
