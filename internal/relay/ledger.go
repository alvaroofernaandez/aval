package relay

import "slices"

// JobState is where a piece of running work is.
type JobState int

const (
	JobRunning  JobState = iota
	JobFinished          // it ended and reported
	JobLost              // the ledger says running, the runtime no longer has it
	JobOrphaned          // the runtime has it, the ledger never saw it start
)

// Job is one piece of work that outlives a single turn: a background shell,
// a subagent, a monitor. Kind is an open string: which kinds survive a relay
// on Claude Code is unknown until probe S4 of spike #63.
type Job struct {
	ID    string
	Kind  string
	State JobState
	Epoch int // relay epoch the job was last seen in
}

// Event is a job start or end reported by an adapter.
type Event struct {
	JobID    string
	Kind     string
	Finished bool
}

// Ledger is the work register carried across relays.
type Ledger struct {
	Epoch int
	Jobs  []Job
}

// Restartable reports whether the relay may start j again: only work the
// runtime lost or never told the ledger about. A running job never restarts.
func (j Job) Restartable() bool { return j.State == JobLost || j.State == JobOrphaned }

// Replay builds a ledger from events, in order.
func Replay(events []Event) Ledger {
	var l Ledger
	for _, e := range events {
		state := JobRunning
		if e.Finished {
			state = JobFinished
		}
		l = l.put(Job{ID: e.JobID, Kind: e.Kind, State: state})
	}
	return l
}

// Merge combines two ledgers of the same work. A job finished in either
// stays finished; otherwise b, the newer one, wins.
func Merge(a, b Ledger) Ledger {
	out := Ledger{Epoch: max(a.Epoch, b.Epoch), Jobs: slices.Clone(a.Jobs)}
	for _, j := range b.Jobs {
		if i := out.index(j.ID); i >= 0 && out.Jobs[i].State == JobFinished {
			continue
		}
		out = out.put(j)
	}
	return out
}

// Reconcile compares the ledger with the job ids the runtime reports alive:
// a running job not alive is lost, an alive job the ledger never saw is
// orphaned.
func (l Ledger) Reconcile(alive []string) Ledger {
	out := Ledger{Epoch: l.Epoch, Jobs: slices.Clone(l.Jobs)}
	for i, j := range out.Jobs {
		if j.State == JobRunning && !slices.Contains(alive, j.ID) {
			out.Jobs[i].State = JobLost
		}
	}
	for _, id := range alive {
		if out.index(id) < 0 {
			out.Jobs = append(out.Jobs, Job{ID: id, State: JobOrphaned, Epoch: l.Epoch})
		}
	}
	return out
}

// Next carries the unfinished jobs into the next relay epoch.
func (l Ledger) Next() Ledger {
	out := Ledger{Epoch: l.Epoch + 1}
	for _, j := range l.Jobs {
		if j.State != JobFinished {
			j.Epoch = out.Epoch
			out.Jobs = append(out.Jobs, j)
		}
	}
	return out
}

func (l Ledger) index(id string) int {
	return slices.IndexFunc(l.Jobs, func(j Job) bool { return j.ID == id })
}

// put returns l with j added, or updated in place; an empty kind keeps the
// known one.
func (l Ledger) put(j Job) Ledger {
	out := Ledger{Epoch: l.Epoch, Jobs: slices.Clone(l.Jobs)}
	if j.Epoch == 0 {
		j.Epoch = l.Epoch
	}
	i := out.index(j.ID)
	if i < 0 {
		out.Jobs = append(out.Jobs, j)
		return out
	}
	if j.Kind == "" {
		j.Kind = out.Jobs[i].Kind
	}
	out.Jobs[i] = j
	return out
}
