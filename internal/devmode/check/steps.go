// Package check is the single table of quality steps behind `aval dev check`
// and the runner that executes it. CI calls the same command, so the local
// verdict and the CI verdict come from one definition (issue #55).
//
// It is a leaf: it imports nothing internal and starts no shell. Every step
// is a binary and its arguments.
package check

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Step is one quality step. Name is what --only takes and what the output
// shows; Tool and Args are launched directly, never through a shell.
type Step struct {
	Name string
	Tool string
	Args []string
	// Needs are the binaries besides Tool that the step relies on.
	Needs []string
	// OnlyOS and SkipOS are GOOS values; both empty means every OS.
	OnlyOS, SkipOS []string
	// Fast puts the step in the --fast set (the edit loop).
	Fast bool
	// FastArgs, when set, replace Args under --fast.
	FastArgs []string
	// OSArgs replace Args on the GOOS they are keyed by.
	OSArgs map[string][]string
	// FailOnOutput makes any output a failure (gofmt -l exits 0 and lists files).
	FailOnOutput bool
}

// SkipReason says why a planned step does not run.
type SkipReason string

const (
	// Runs means the step is not skipped.
	Runs SkipReason = ""
	// SkippedFast: the step is outside the --fast set.
	SkippedFast SkipReason = "fast"
	// SkippedOS: the step does not apply to this operating system.
	SkippedOS SkipReason = "os"
)

// Entry is a step resolved for one run.
type Entry struct {
	Step Step
	// Args are the arguments to launch, after OSArgs and FastArgs.
	Args []string
	Skip SkipReason
}

// PlanOptions selects what runs.
type PlanOptions struct {
	GOOS string
	Fast bool
	// Only, when set, plans just the step with this name.
	Only string
}

// Plan resolves steps for one run, in table order. Steps that do not run stay
// in the result with the reason, so the output can say what was skipped.
// --only wins over --fast: naming a step asks for it.
func Plan(steps []Step, o PlanOptions) ([]Entry, error) {
	if o.Only != "" && !slices.ContainsFunc(steps, func(s Step) bool { return s.Name == o.Only }) {
		known := make([]string, len(steps))
		for i, s := range steps {
			known[i] = s.Name
		}
		return nil, &UnknownStepError{Name: o.Only, Known: known}
	}
	out := make([]Entry, 0, len(steps))
	for _, s := range steps {
		e := Entry{Step: s, Args: s.Args}
		if a, ok := s.OSArgs[o.GOOS]; ok {
			e.Args = a
		}
		switch {
		case len(s.OnlyOS) > 0 && !slices.Contains(s.OnlyOS, o.GOOS), slices.Contains(s.SkipOS, o.GOOS):
			e.Skip = SkippedOS
		case o.Only != "":
			if s.Name != o.Only {
				e.Skip = SkippedOnly
			}
		case o.Fast && !s.Fast:
			e.Skip = SkippedFast
		}
		if o.Fast && s.FastArgs != nil && e.Skip == Runs {
			e.Args = s.FastArgs
		}
		out = append(out, e)
	}
	return out, nil
}

// SkippedOnly: --only named another step.
const SkippedOnly SkipReason = "only"

// ErrUnknownStep is wrapped by UnknownStepError.
var ErrUnknownStep = errors.New("unknown step")

// UnknownStepError is --only naming a step the table does not have.
type UnknownStepError struct {
	Name  string
	Known []string
}

func (e *UnknownStepError) Error() string {
	return fmt.Sprintf("unknown step %q (steps: %s)", e.Name, strings.Join(e.Known, ", "))
}
func (e *UnknownStepError) Unwrap() error { return ErrUnknownStep }
