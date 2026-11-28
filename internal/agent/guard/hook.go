package guard

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
)

// RunOptions is the machine a hook run needs, all injectable.
type RunOptions struct {
	Home     string
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
}

// AllowPath is where the allow-list lives: $AVAL_PRIVATE_DIR/guard.allow, else
// ~/.config/aval/private/guard.allow (the private overlay, outside the repo).
func AllowPath(getenv func(string) string, home string) string {
	if d := getenv("AVAL_PRIVATE_DIR"); d != "" {
		return filepath.Join(d, "guard.allow")
	}
	return filepath.Join(home, ".config", "aval", "private", "guard.allow")
}

// readAllow returns the entries of the allow-list; a missing file has none.
func readAllow(o RunOptions) func() ([]string, error) {
	return func() ([]string, error) {
		b, err := o.ReadFile(AllowPath(o.Getenv, o.Home))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return parseAllow(string(b)), nil
	}
}

// Run reads a PreToolUse payload and writes a deny when a Bash command breaks
// a rule. Anything unexpected, a panic included, writes nothing.
func Run(in io.Reader, out io.Writer, o RunOptions) {
	defer func() { _ = recover() }()
	raw, err := io.ReadAll(in)
	if err != nil {
		return
	}
	var p struct {
		Cwd       string `json:"cwd"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if json.Unmarshal(raw, &p) != nil || p.ToolName != "Bash" || p.ToolInput.Command == "" {
		return
	}
	d := Decide(p.ToolInput.Command, Options{Ctx: Context{Cwd: p.Cwd, Home: o.Home}, ReadAllow: readAllow(o)})
	if !d.Deny {
		return
	}
	type specific struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{specific{"PreToolUse", "deny", d.Reason}})
}
