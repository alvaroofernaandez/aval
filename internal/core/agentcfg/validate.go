package agentcfg

import (
	"errors"
	"strings"
)

// Validation failures, each wrapped in a *ValidationError naming the element.
var (
	ErrHookWithoutCommand = errors.New("hook handler has no command")
	ErrUnknownSkill       = errors.New("references a skill that does not exist")
	ErrDuplicate          = errors.New("defined twice")
	ErrNoDescription      = errors.New("has no description")
)

// ValidationError is one broken element.
type ValidationError struct {
	ID  string // the element id, e.g. "agent.planner"
	Ref string // what it points at, when the failure is a reference
	Err error
}

func (e *ValidationError) Error() string {
	if e.Ref != "" {
		return e.ID + " " + e.Err.Error() + ": " + e.Ref
	}
	return e.ID + " " + e.Err.Error()
}

func (e *ValidationError) Unwrap() error { return e.Err }

// Validate reports broken references in the model, all of them, joined.
func (c Config) Validate() error {
	var errs []error
	fail := func(id, ref string, err error) { errs = append(errs, &ValidationError{ID: id, Ref: ref, Err: err}) }

	skills := map[string]bool{}
	for _, s := range c.Skills {
		id := "skill." + s.Name
		if skills[s.Name] {
			fail(id, "", ErrDuplicate)
		}
		skills[s.Name] = true
		if strings.TrimSpace(s.Description) == "" {
			fail(id, "", ErrNoDescription)
		}
	}
	seen := map[string]bool{}
	for _, cm := range c.Commands {
		if id := "command." + cm.Name; seen[id] {
			fail(id, "", ErrDuplicate)
		} else {
			seen[id] = true
		}
	}
	for _, a := range c.Agents {
		id := "agent." + a.Name
		if seen[id] {
			fail(id, "", ErrDuplicate)
		}
		seen[id] = true
		for _, ref := range a.Skills {
			if !skills[ref] {
				fail(id, ref, ErrUnknownSkill)
			}
		}
	}
	perEvent := map[string]int{}
	for _, h := range c.Hooks {
		id := HookID(h.Event, perEvent[h.Event])
		perEvent[h.Event]++
		for _, hh := range h.Handlers {
			if hh.Type == "command" && strings.TrimSpace(hh.Command) == "" {
				fail(id, "", ErrHookWithoutCommand)
			}
		}
	}
	return errors.Join(errs...)
}
