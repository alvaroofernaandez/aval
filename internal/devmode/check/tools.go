package check

import (
	"errors"
	"fmt"
	"strings"
)

// Pin is a tool the table relies on: the version CI and every contributor use
// and the exact command that installs it. The check never installs anything.
type Pin struct {
	Name, Version, Install string
}

// Pins maps a binary name to its pin. The versions are the constants in
// table.go; gofmt ships with Go and npx with Node.
var Pins = map[string]Pin{
	"go":         {"go", GoVersion, "install Go " + GoVersion + " from https://go.dev/dl/"},
	"gofmt":      {"gofmt", GoVersion, "install Go " + GoVersion + " from https://go.dev/dl/ (it ships gofmt)"},
	"node":       {"node", NodeVersion, "install Node.js " + NodeVersion + " from https://nodejs.org/"},
	"npx":        {"npx", NodeVersion, "install Node.js " + NodeVersion + " from https://nodejs.org/ (it ships npx)"},
	"actionlint": {"actionlint", ActionlintVersion, "go install github.com/rhysd/actionlint/cmd/actionlint@" + ActionlintVersion},
	"gitleaks":   {"gitleaks", GitleaksVersion, "go install github.com/zricethezav/gitleaks/v8@" + GitleaksVersion},
}

// ErrMissingTool is wrapped by MissingToolsError.
var ErrMissingTool = errors.New("missing tool")

// MissingToolsError lists every tool a run needs and the PATH lacks, in the
// order the table meets them.
type MissingToolsError struct{ Tools []Pin }

func (e *MissingToolsError) Error() string {
	var b strings.Builder
	b.WriteString("missing tools:")
	for _, t := range e.Tools {
		fmt.Fprintf(&b, "\n  %s %s: %s", t.Name, t.Version, t.Install)
	}
	return b.String()
}
func (e *MissingToolsError) Unwrap() error { return ErrMissingTool }

// Preflight checks, before anything runs, that the PATH has every tool of the
// steps that will run. look is exec.LookPath in production.
func Preflight(entries []Entry, look func(string) (string, error)) error {
	var missing []Pin
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Skip != Runs {
			continue
		}
		for _, tool := range append([]string{e.Step.Tool}, e.Step.Needs...) {
			if seen[tool] {
				continue
			}
			seen[tool] = true
			if _, err := look(tool); err != nil {
				p, ok := Pins[tool]
				if !ok {
					p = Pin{Name: tool, Install: "install " + tool}
				}
				missing = append(missing, p)
			}
		}
	}
	if len(missing) > 0 {
		return &MissingToolsError{Tools: missing}
	}
	return nil
}
