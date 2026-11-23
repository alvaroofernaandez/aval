// Package deps is the inventory of the external tools aval, its hooks, its
// skills and its install rely on: what each is used by, what happens when it
// is missing and the native substitute planned for it. Check probes the
// machine for each tool, offline and in parallel; it never installs anything.
package deps

import (
	"context"
	"errors"
)

// Layer groups a tool by what depends on it.
type Layer string

const (
	Harness Layer = "harness" // aval itself and its hooks
	Agent   Layer = "agent"   // the agent runtimes and what their skills call
	Env     Layer = "env"     // the shell and terminal environment install links
)

// Layers is every layer, in the order the inventory is listed.
var Layers = []Layer{Harness, Agent, Env}

// IfMissing is what happens when a tool is absent.
type IfMissing string

const (
	Blocks   IfMissing = "blocks"
	Degrades IfMissing = "degrades"
	Ignored  IfMissing = "ignored"
)

// ProbeKind is how a tool is looked for.
type ProbeKind string

const (
	// ProbeBin looks the executable up on PATH; Args, when given, print its
	// version.
	ProbeBin ProbeKind = "bin"
	// ProbeApp checks a macOS application bundle; elsewhere it is skipped.
	ProbeApp ProbeKind = "app"
	// ProbeShare checks a file under a Homebrew share directory.
	ProbeShare ProbeKind = "share"
)

// Probe says how to find one tool.
type Probe struct {
	Kind   ProbeKind
	Target string
	Args   []string
}

// Entry is one tool of the inventory.
type Entry struct {
	Name             string
	Layer            Layer
	UsedBy           []string
	IfMissing        IfMissing
	NativeSubstitute string
	Probe            Probe
}

// Manifest is the parsed inventory. Personal lists Brewfile lines that are on
// the machine for the person, not for aval, so the coverage test sees them.
type Manifest struct {
	Entries  []Entry
	Personal []string
}

var (
	ErrUnknownLayer     = errors.New("unknown layer")
	ErrUnknownIfMissing = errors.New("unknown if_missing")
	ErrNativeSubstitute = errors.New("native_substitute must be an issue (#N) or none")
	ErrMissingField     = errors.New("missing field")
	ErrUnknownKey       = errors.New("unknown key")
	ErrBadProbe         = errors.New("probe must be bin, app or share and a target")
	ErrDuplicate        = errors.New("duplicate name")
	ErrNotFound         = errors.New("not found")
	ErrSkipped          = errors.New("not probed on this platform")
	errOutsideSection   = errors.New("line outside a section")
)

// Status is what a probe found.
type Status string

const (
	Present Status = "present"
	Missing Status = "missing"
	Skipped Status = "skipped"
)

// Found is what a successful probe reports.
type Found struct{ Path, Version string }

// Prober looks for one tool. It returns ErrSkipped when the probe does not
// apply on this platform and any other error when the tool is missing. It
// should return once ctx is done.
type Prober interface {
	Probe(ctx context.Context, p Probe) (Found, error)
}

// Report is one entry with what its probe found.
type Report struct {
	Entry
	Status  Status
	Path    string
	Version string
}
