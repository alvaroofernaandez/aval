package check

import (
	"errors"
	"slices"
	"testing"
)

func names(es []Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Step.Name)
	}
	return out
}

func find(t *testing.T, es []Entry, name string) Entry {
	t.Helper()
	for _, e := range es {
		if e.Step.Name == name {
			return e
		}
	}
	t.Fatalf("no step %q in %v", name, names(es))
	return Entry{}
}

func plan(t *testing.T, o PlanOptions) []Entry {
	t.Helper()
	es, err := Plan(Steps, o)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestTableOrderIsTheCIOrder(t *testing.T) {
	want := []string{"gofmt", "vet", "test", "npm", "actionlint", "agents", "gitleaks"}
	if got := names(plan(t, PlanOptions{GOOS: "linux"})); !slices.Equal(got, want) {
		t.Errorf("steps = %v, want %v", got, want)
	}
}

func TestTableNamesAreUniqueAndEveryStepHasATool(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Steps {
		if s.Name == "" || s.Tool == "" {
			t.Errorf("step %+v needs a name and a tool", s)
		}
		if seen[s.Name] {
			t.Errorf("duplicate step name %q", s.Name)
		}
		seen[s.Name] = true
	}
}

func TestFullRunOnLinuxRunsEverythingWithRace(t *testing.T) {
	es := plan(t, PlanOptions{GOOS: "linux"})
	for _, e := range es {
		if e.Skip != Runs {
			t.Errorf("%s skipped (%s) on a full linux run", e.Step.Name, e.Skip)
		}
	}
	if got := find(t, es, "test").Args; !slices.Contains(got, "-race") {
		t.Errorf("test args on linux = %v, want -race", got)
	}
}

func TestRaceIsDroppedOnWindowsByTheTable(t *testing.T) {
	e := find(t, plan(t, PlanOptions{GOOS: "windows"}), "test")
	if slices.Contains(e.Args, "-race") {
		t.Errorf("test args on windows = %v, want no -race", e.Args)
	}
	if e.Skip != Runs {
		t.Errorf("test must still run on windows, skip = %q", e.Skip)
	}
}

func TestFastSkipsTheSlowStepsAndSaysWhy(t *testing.T) {
	es := plan(t, PlanOptions{GOOS: "linux", Fast: true})
	for _, n := range []string{"agents", "gitleaks"} {
		if got := find(t, es, n).Skip; got != SkippedFast {
			t.Errorf("%s skip = %q, want %q", n, got, SkippedFast)
		}
	}
	for _, n := range []string{"gofmt", "vet", "test", "npm", "actionlint"} {
		if got := find(t, es, n).Skip; got != Runs {
			t.Errorf("%s skip = %q under --fast, want it to run", n, got)
		}
	}
	if got := find(t, es, "test").Args; slices.Contains(got, "-race") {
		t.Errorf("test args under --fast = %v, want no -race", got)
	}
}

func TestOSRulesAreInTheTable(t *testing.T) {
	steps := []Step{
		{Name: "a", Tool: "x", OnlyOS: []string{"windows"}},
		{Name: "b", Tool: "x", SkipOS: []string{"windows"}},
		{Name: "c", Tool: "x"},
	}
	for _, c := range []struct {
		goos string
		skip map[string]SkipReason
	}{
		{"windows", map[string]SkipReason{"a": Runs, "b": SkippedOS, "c": Runs}},
		{"linux", map[string]SkipReason{"a": SkippedOS, "b": Runs, "c": Runs}},
	} {
		es, err := Plan(steps, PlanOptions{GOOS: c.goos})
		if err != nil {
			t.Fatal(err)
		}
		for n, want := range c.skip {
			if got := find(t, es, n).Skip; got != want {
				t.Errorf("%s on %s: skip = %q, want %q", n, c.goos, got, want)
			}
		}
	}
}

func TestOnlyRunsOneStep(t *testing.T) {
	es := plan(t, PlanOptions{GOOS: "linux", Only: "vet"})
	for _, e := range es {
		want := SkippedOnly
		if e.Step.Name == "vet" {
			want = Runs
		}
		if e.Skip != want {
			t.Errorf("%s skip = %q, want %q", e.Step.Name, e.Skip, want)
		}
	}
}

func TestOnlyWinsOverFast(t *testing.T) {
	es := plan(t, PlanOptions{GOOS: "linux", Fast: true, Only: "gitleaks"})
	if got := find(t, es, "gitleaks").Skip; got != Runs {
		t.Errorf("--only gitleaks --fast: skip = %q, want it to run", got)
	}
}

func TestOnlyUnknownStepIsAnError(t *testing.T) {
	_, err := Plan(Steps, PlanOptions{GOOS: "linux", Only: "nope"})
	var u *UnknownStepError
	if !errors.As(err, &u) || u.Name != "nope" || !errors.Is(err, ErrUnknownStep) {
		t.Fatalf("err = %v, want *UnknownStepError{nope} wrapping ErrUnknownStep", err)
	}
}
