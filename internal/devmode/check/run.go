package check

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Status is the outcome of one step.
type Status string

const (
	Passed  Status = "passed"
	Failed  Status = "failed"
	Skipped Status = "skipped"
	// NotRun: an earlier step failed and --keep-going was off.
	NotRun Status = "not run"
)

// Exec launches a binary with arguments in dir and returns what it printed
// (stdout and stderr together) and its exit code. err is only for a process
// that could not start or was interrupted; a non-zero exit is code, not err.
type Exec interface {
	Run(ctx context.Context, dir, tool string, args []string) (out []byte, code int, err error)
}

// Result is one step of a finished run.
type Result struct {
	Name     string
	Status   Status
	Skip     SkipReason
	Duration time.Duration
	Code     int
	Output   string
	Err      error
}

// Report is a finished run. Code is 0 when green, else the exit code of the
// first step that failed.
type Report struct {
	Results  []Result
	Code     int
	Duration time.Duration
}

// Options configures Run.
type Options struct {
	Dir       string
	Exec      Exec
	KeepGoing bool
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Run executes the planned steps in order. A step that fails stops the run
// unless KeepGoing is set; the report's code is the first failure's.
func Run(ctx context.Context, entries []Entry, o Options) Report {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	var rep Report
	stopped := false
	for _, e := range entries {
		r := Result{Name: e.Step.Name, Skip: e.Skip}
		switch {
		case e.Skip != Runs:
			r.Status = Skipped
		case stopped:
			r.Status = NotRun
		case ctx.Err() != nil:
			r.Status, r.Err, r.Code = Failed, ctx.Err(), 1
		default:
			start := now()
			out, code, err := o.Exec.Run(ctx, o.Dir, e.Step.Tool, e.Args)
			r.Duration = now().Sub(start)
			r.Output, r.Code, r.Err = string(out), code, err
			r.Status = Passed
			if err != nil || code != 0 || e.Step.FailOnOutput && len(bytes.TrimSpace(out)) > 0 {
				r.Status = Failed
				if r.Code == 0 {
					r.Code = 1
				}
			}
		}
		if r.Status == Failed {
			if rep.Code == 0 {
				rep.Code = r.Code
			}
			stopped = !o.KeepGoing
		}
		rep.Duration += r.Duration
		rep.Results = append(rep.Results, r)
	}
	return rep
}

// SystemExec runs real processes, directly: no shell is involved.
type SystemExec struct{}

// Run implements Exec.
func (SystemExec) Run(ctx context.Context, dir, tool string, args []string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ctx.Err() == nil {
		return out, ee.ExitCode(), nil
	}
	return out, 0, err
}
