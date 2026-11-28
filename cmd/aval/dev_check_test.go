package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/devmode/check"
)

// recExec records every launch and scripts exit codes by "tool firstArg" (or
// by "tool" alone); nothing real starts.
type recExec struct {
	calls []string
	code  map[string]int
	out   map[string]string
}

func (r *recExec) Run(_ context.Context, _, tool string, args []string) ([]byte, int, error) {
	key := tool
	if len(args) > 0 {
		key += " " + args[0]
	}
	r.calls = append(r.calls, strings.TrimSpace(tool+" "+strings.Join(args, " ")))
	return []byte(r.out[key]), r.code[key], nil
}

func tick() func() time.Time {
	t := time.Unix(0, 0)
	return func() time.Time { t = t.Add(1500 * time.Millisecond); return t }
}

func everything(string) (string, error) { return "/bin/x", nil }

// devCheckIn runs `aval dev check ...` in process over a fake machine.
func devCheckIn(t *testing.T, goos string, ex *recExec, look func(string) (string, error), env map[string]string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	c := cmdIO{
		stdin: strings.NewReader(""), stdout: &out, stderr: &errb,
		env:      envMap(withHomeVars(mergeEnv(map[string]string{"HOME": t.TempDir()}, env))),
		devcheck: &devCheckEnv{exec: ex, look: look, goos: goos, dir: t.TempDir(), now: tick()},
	}
	err = dispatch(append([]string{"dev", "check"}, args...), c)
	return out.String(), errb.String(), err
}

func mergeEnv(a, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func lineWith(out, needle string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

func TestDevCheckListShowsEveryStepWithToolPinAndOS(t *testing.T) {
	ex := &recExec{}
	out, _, err := devCheckIn(t, "linux", ex, everything, nil, "--list")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range check.Steps {
		if lineWith(out, s.Name) == "" {
			t.Errorf("--list lacks step %q:\n%s", s.Name, out)
		}
	}
	if l := lineWith(out, "actionlint"); !strings.Contains(l, check.ActionlintVersion) {
		t.Errorf("actionlint row %q lacks the pinned %s", l, check.ActionlintVersion)
	}
	if l := lineWith(out, "agents"); !strings.Contains(l, "not windows") {
		t.Errorf("agents row %q must say it skips windows", l)
	}
	if len(ex.calls) != 0 {
		t.Errorf("--list launched %v", ex.calls)
	}
}

func TestDevCheckGreenRunPrintsTheTableAndAGreenVerdict(t *testing.T) {
	ex := &recExec{}
	out, _, err := devCheckIn(t, "linux", ex, everything, nil)
	if err != nil {
		t.Fatalf("err = %v, want green", err)
	}
	if len(ex.calls) != len(check.Steps) {
		t.Errorf("launched %d steps, want %d: %v", len(ex.calls), len(check.Steps), ex.calls)
	}
	if l := lineWith(out, "vet"); !strings.Contains(l, "passed") || !strings.Contains(l, "1.5s") {
		t.Errorf("vet row = %q, want result and duration", l)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "green") {
		t.Errorf("last line = %q, want the green verdict", last)
	}
}

func TestDevCheckFailureExitsWithTheStepCodeAndShowsItsOutput(t *testing.T) {
	ex := &recExec{code: map[string]int{"go vet": 3}, out: map[string]string{"go vet": "vet: unreachable code\n"}}
	out, _, err := devCheckIn(t, "linux", ex, everything, nil)
	var code exitCode
	if !errors.As(err, &code) || int(code) != 3 {
		t.Fatalf("err = %v, want exitCode(3)", err)
	}
	if !strings.Contains(out, "vet: unreachable code") {
		t.Errorf("the failing step's output is missing:\n%s", out)
	}
	if l := lineWith(out, "test"); !strings.Contains(l, "not run") {
		t.Errorf("test row = %q, want it not run after the failure", l)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "red") {
		t.Errorf("last line = %q, want the red verdict", last)
	}
}

func TestDevCheckKeepGoingRunsTheRest(t *testing.T) {
	ex := &recExec{code: map[string]int{"go vet": 3}}
	_, _, err := devCheckIn(t, "linux", ex, everything, nil, "--keep-going")
	var code exitCode
	if !errors.As(err, &code) || int(code) != 3 {
		t.Fatalf("err = %v, want exitCode(3)", err)
	}
	if len(ex.calls) != len(check.Steps) {
		t.Errorf("launched %d steps, want all %d", len(ex.calls), len(check.Steps))
	}
}

func TestDevCheckFastSaysWhatItSkipped(t *testing.T) {
	ex := &recExec{}
	out, _, err := devCheckIn(t, "linux", ex, everything, nil, "--fast")
	if err != nil {
		t.Fatal(err)
	}
	if l := lineWith(out, "gitleaks"); !strings.Contains(l, "skipped (--fast)") {
		t.Errorf("gitleaks row = %q, want skipped (--fast)", l)
	}
	if !strings.Contains(out, "--fast") || !strings.Contains(out, "agents, gitleaks") {
		t.Errorf("no note naming what --fast left to CI:\n%s", out)
	}
	if slices.ContainsFunc(ex.calls, func(c string) bool { return strings.HasPrefix(c, "gitleaks") }) {
		t.Errorf("gitleaks ran under --fast: %v", ex.calls)
	}
}

func TestDevCheckWindowsRunsTestWithoutRace(t *testing.T) {
	ex := &recExec{}
	if _, _, err := devCheckIn(t, "windows", ex, everything, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ex.calls, "go test ./...") {
		t.Errorf("windows calls = %v, want `go test ./...`", ex.calls)
	}
	if slices.ContainsFunc(ex.calls, func(c string) bool { return strings.Contains(c, "-race") }) {
		t.Errorf("-race ran on windows: %v", ex.calls)
	}
}

func TestDevCheckMissingToolFailsBeforeRunningAnythingAndNamesTheInstall(t *testing.T) {
	ex := &recExec{}
	look := func(n string) (string, error) {
		if n == "actionlint" {
			return "", exec.ErrNotFound
		}
		return "/bin/" + n, nil
	}
	_, _, err := devCheckIn(t, "linux", ex, look, nil)
	var m *check.MissingToolsError
	if !errors.As(err, &m) || len(m.Tools) != 1 || m.Tools[0].Name != "actionlint" {
		t.Fatalf("err = %v, want one missing tool: actionlint", err)
	}
	if want := "go install github.com/rhysd/actionlint/cmd/actionlint@" + check.ActionlintVersion; m.Tools[0].Install != want {
		t.Errorf("install = %q, want %q", m.Tools[0].Install, want)
	}
	if len(ex.calls) != 0 {
		t.Errorf("launched %v before the preflight passed", ex.calls)
	}
}

func TestDevCheckOnlyRunsOneStepAndRejectsUnknownOnes(t *testing.T) {
	ex := &recExec{}
	if _, _, err := devCheckIn(t, "linux", ex, everything, nil, "--only", "vet"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ex.calls, []string{"go vet ./..."}) {
		t.Errorf("calls = %v, want only go vet", ex.calls)
	}
	_, _, err := devCheckIn(t, "linux", &recExec{}, everything, nil, "--only", "nope")
	if !errors.Is(err, check.ErrUnknownStep) {
		t.Errorf("err = %v, want ErrUnknownStep", err)
	}
}

func TestDevCheckSpeaksSpanishWhenAsked(t *testing.T) {
	ex := &recExec{code: map[string]int{"go vet": 1}}
	out, _, _ := devCheckIn(t, "linux", ex, everything, map[string]string{"AVAL_LANG": "es"})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "rojo") {
		t.Errorf("last line = %q, want the rojo verdict", last)
	}
}

func TestDevCheckRejectsStrayArguments(t *testing.T) {
	_, _, err := devCheckIn(t, "linux", &recExec{}, everything, nil, "extra")
	var u *usageError
	if !errors.As(err, &u) {
		t.Errorf("err = %v, want a usage error", err)
	}
}

func TestDevCheckNamesTheSimulatedOSOfAnOSSkip(t *testing.T) {
	out, _, err := devCheckIn(t, "windows", &recExec{}, everything, nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := lineWith(out, "agents"); !strings.Contains(l, "skipped (windows)") {
		t.Errorf("agents row = %q, want skipped (windows)", l)
	}
}
