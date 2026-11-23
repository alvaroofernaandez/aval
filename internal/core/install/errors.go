package install

import (
	"errors"
	"fmt"
	"os"

	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// Error is one failed action: which action, and why in a few words. Error()
// keeps the full message (the headless output prints it); Reason is the short,
// path-free form the installer shows next to the action's label.
type Error struct {
	Action plan.Action
	Reason string
	err    error
}

func (e *Error) Error() string {
	if e.err == nil {
		return "install: " + e.Action.Label + ": " + e.Reason
	}
	return e.err.Error()
}

func (e *Error) Unwrap() error { return errors.Unwrap(e.err) }

// fail wraps err for action a. op names the step that failed ("link",
// "copy"); an empty op means the reason is already the whole story.
func fail(a plan.Action, op string, err error) *Error {
	return &Error{Action: a, Reason: shortReason(op, err), err: err}
}

// shortReason is "<op> failed: <cause>", with the cause stripped of the paths
// an os error carries, since the label already says where.
func shortReason(op string, err error) string {
	cause := err
	var pe *os.PathError
	var le *os.LinkError
	switch {
	case errors.As(err, &pe):
		cause = pe.Err
	case errors.As(err, &le):
		cause = le.Err
	}
	return fmt.Sprintf("%s failed: %v", op, cause)
}
