package check

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// call is one launch the fake recorded.
type call struct {
	Dir, Tool string
	Args      []string
}

// fakeExec scripts outcomes by tool name and records every launch; nothing
// real starts.
type fakeExec struct {
	calls []call
	out   map[string]string
	code  map[string]int
	err   map[string]error
}

func (f *fakeExec) Run(_ context.Context, dir, tool string, args []string) ([]byte, int, error) {
	f.calls = append(f.calls, call{dir, tool, slices.Clone(args)})
	return []byte(f.out[tool]), f.code[tool], f.err[tool]
}

// ticker is a clock that advances one second per reading.
func ticker() func() time.Time {
	t := time.Unix(0, 0)
	return func() time.Time { t = t.Add(time.Second); return t }
}

func three() []Entry {
	es, _ := Plan([]Step{
		{Name: "a", Tool: "ta", Args: []string{"1"}},
		{Name: "b", Tool: "tb", Args: []string{"2"}},
		{Name: "c", Tool: "tc", Args: []string{"3"}},
	}, PlanOptions{GOOS: "linux"})
	return es
}

func statuses(r Report) []Status {
	var out []Status
	for _, x := range r.Results {
		out = append(out, x.Status)
	}
	return out
}

func TestRunLaunchesEachStepInOrderInTheRepoDir(t *testing.T) {
	f := &fakeExec{}
	r := Run(context.Background(), three(), Options{Dir: "/repo", Exec: f, Now: ticker()})
	want := []call{{"/repo", "ta", []string{"1"}}, {"/repo", "tb", []string{"2"}}, {"/repo", "tc", []string{"3"}}}
	if len(f.calls) != 3 || !slices.EqualFunc(f.calls, want, func(a, b call) bool {
		return a.Dir == b.Dir && a.Tool == b.Tool && slices.Equal(a.Args, b.Args)
	}) {
		t.Errorf("calls = %+v, want %+v", f.calls, want)
	}
	if r.Code != 0 || !slices.Equal(statuses(r), []Status{Passed, Passed, Passed}) {
		t.Errorf("code %d statuses %v, want green", r.Code, statuses(r))
	}
}

func TestRunStopsAtTheFirstFailureAndReturnsItsExitCode(t *testing.T) {
	f := &fakeExec{code: map[string]int{"tb": 3, "tc": 9}, out: map[string]string{"tb": "boom\n"}}
	r := Run(context.Background(), three(), Options{Exec: f, Now: ticker()})
	if r.Code != 3 {
		t.Errorf("code = %d, want the failing step's 3", r.Code)
	}
	if got, want := statuses(r), []Status{Passed, Failed, NotRun}; !slices.Equal(got, want) {
		t.Errorf("statuses = %v, want %v", got, want)
	}
	if len(f.calls) != 2 {
		t.Errorf("launched %d steps, want 2", len(f.calls))
	}
	if r.Results[1].Output != "boom\n" {
		t.Errorf("failed output = %q", r.Results[1].Output)
	}
}

func TestKeepGoingRunsEverythingAndKeepsTheFirstCode(t *testing.T) {
	f := &fakeExec{code: map[string]int{"ta": 4, "tc": 9}}
	r := Run(context.Background(), three(), Options{Exec: f, KeepGoing: true, Now: ticker()})
	if r.Code != 4 {
		t.Errorf("code = %d, want the first failure's 4", r.Code)
	}
	if got, want := statuses(r), []Status{Failed, Passed, Failed}; !slices.Equal(got, want) {
		t.Errorf("statuses = %v, want %v", got, want)
	}
}

func TestSkippedStepsAreNotLaunchedAndKeepTheirReason(t *testing.T) {
	es, _ := Plan(Steps, PlanOptions{GOOS: "linux", Fast: true})
	f := &fakeExec{}
	r := Run(context.Background(), es, Options{Exec: f, Now: ticker()})
	for _, x := range r.Results {
		if x.Name == "gitleaks" && (x.Status != Skipped || x.Skip != SkippedFast) {
			t.Errorf("gitleaks = %+v, want skipped by fast", x)
		}
	}
	for _, c := range f.calls {
		if c.Tool == "gitleaks" || c.Tool == "npx" {
			t.Errorf("launched %s under --fast", c.Tool)
		}
	}
}

func TestDurationsComeFromTheClock(t *testing.T) {
	r := Run(context.Background(), three(), Options{Exec: &fakeExec{}, Now: ticker()})
	for _, x := range r.Results {
		if x.Duration != time.Second {
			t.Errorf("%s took %v, want 1s from the fake clock", x.Name, x.Duration)
		}
	}
	if r.Duration != 3*time.Second {
		t.Errorf("total = %v, want 3s", r.Duration)
	}
}

func TestOutputFromAFailOnOutputStepIsAFailureEvenWithExitZero(t *testing.T) {
	es, _ := Plan([]Step{{Name: "fmt", Tool: "gofmt", FailOnOutput: true}}, PlanOptions{GOOS: "linux"})
	f := &fakeExec{out: map[string]string{"gofmt": "a.go\n"}}
	r := Run(context.Background(), es, Options{Exec: f, Now: ticker()})
	if r.Code != 1 || r.Results[0].Status != Failed {
		t.Errorf("code %d result %+v, want a failure with code 1", r.Code, r.Results[0])
	}
}

func TestAStepThatCannotStartFailsWithTheError(t *testing.T) {
	f := &fakeExec{err: map[string]error{"ta": exec.ErrNotFound}}
	r := Run(context.Background(), three(), Options{Exec: f, Now: ticker()})
	if r.Code == 0 || !errors.Is(r.Results[0].Err, exec.ErrNotFound) {
		t.Errorf("code %d err %v, want a failure carrying ErrNotFound", r.Code, r.Results[0].Err)
	}
}

func TestACancelledContextStopsTheRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeExec{}
	r := Run(ctx, three(), Options{Exec: f, Now: ticker()})
	if len(f.calls) != 0 || r.Code == 0 {
		t.Errorf("launched %d, code %d; want nothing launched and a failure", len(f.calls), r.Code)
	}
}

func TestSystemExecCapturesOutputAndTheExitCode(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	dir := t.TempDir()
	out, code, err := SystemExec{}.Run(context.Background(), dir, "go", []string{"env", "GOOS"})
	if err != nil || code != 0 || strings.TrimSpace(string(out)) != runtime.GOOS {
		t.Errorf("go env GOOS: out %q code %d err %v", out, code, err)
	}
	_, code, err = SystemExec{}.Run(context.Background(), dir, "go", []string{"definitely-not-a-subcommand"})
	if err != nil || code == 0 {
		t.Errorf("failing go: code %d err %v, want a non-zero code and no start error", code, err)
	}
	_, _, err = SystemExec{}.Run(context.Background(), dir, "aval-no-such-tool-xyz", nil)
	if err == nil {
		t.Error("a missing binary must be a start error")
	}
}
