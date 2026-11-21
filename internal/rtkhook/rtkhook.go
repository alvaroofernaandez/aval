// Package rtkhook is the Claude Code PreToolUse hook that rewrites Bash
// commands through `rtk rewrite` for token savings. All rewrite logic lives in
// rtk (its registry is the single source of truth); this only speaks the hook
// protocol.
//
// Exit code protocol of `rtk rewrite`:
//
//	0 + stdout  rewrite found, no deny/ask rule matched: auto-allow
//	1           no RTK equivalent: pass through unchanged
//	2           deny rule matched: pass through (Claude Code's native deny handles it)
//	3 + stdout  ask rule matched: rewrite but let Claude Code prompt the user
//
// The hook never fails the tool call: every problem is a silent pass-through
// (a warning on stderr when rtk itself is unusable).
package rtkhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Rtk is the part of the rtk binary the hook needs.
type Rtk interface {
	Available() bool
	Version() string
	Rewrite(cmd string) (rewritten string, exitCode int)
}

// Run reads a PreToolUse payload on in and writes the hook decision to out.
func Run(in io.Reader, out, errw io.Writer, r Rtk) {
	if !r.Available() {
		fmt.Fprintln(errw, "[rtk] WARNING: rtk is not installed or not in PATH. Hook cannot rewrite commands. Install: https://github.com/rtk-ai/rtk#installation")
		return
	}
	// rtk rewrite was added in 0.23.0; older binaries warn and pass through.
	if v := r.Version(); v != "" {
		if m := versionRe.FindStringSubmatch(v); m != nil {
			major, _ := strconv.Atoi(m[1])
			minor, _ := strconv.Atoi(m[2])
			if major == 0 && minor < 23 {
				fmt.Fprintf(errw, "[rtk] WARNING: rtk %s.%s.%s is too old (need >= 0.23.0). Upgrade: cargo install rtk\n", m[1], m[2], m[3])
				return
			}
		}
	}

	raw, err := io.ReadAll(in)
	if err != nil {
		return
	}
	var payload struct {
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	keys, vals, ok := orderedObject(payload.ToolInput)
	if !ok {
		return
	}
	cmdIdx := -1
	for i, k := range keys {
		if k == "command" {
			cmdIdx = i
		}
	}
	var cmd string
	if cmdIdx < 0 || json.Unmarshal(vals[cmdIdx], &cmd) != nil || cmd == "" {
		return
	}

	rewritten, code := r.Rewrite(cmd)
	rewritten = strings.TrimRight(rewritten, "\n") // $(...) semantics of the shell hook
	switch code {
	case 0:
		if rewritten == cmd {
			return // already using rtk
		}
	case 3: // ask: rewrite, but omit the decision so Claude Code prompts
	default:
		return
	}

	var nb bytes.Buffer
	ne := json.NewEncoder(&nb)
	ne.SetEscapeHTML(false)
	_ = ne.Encode(rewritten)
	vals[cmdIdx] = bytes.TrimRight(nb.Bytes(), "\n")
	updated := ordered{keys: keys, vals: vals}

	type specific struct {
		HookEventName            string  `json:"hookEventName"`
		PermissionDecision       string  `json:"permissionDecision,omitempty"`
		PermissionDecisionReason string  `json:"permissionDecisionReason,omitempty"`
		UpdatedInput             ordered `json:"updatedInput"`
	}
	s := specific{HookEventName: "PreToolUse", UpdatedInput: updated}
	if code == 0 {
		s.PermissionDecision, s.PermissionDecisionReason = "allow", "RTK auto-rewrite"
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ") // jq's layout
	_ = enc.Encode(struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{s})
}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// ordered is a JSON object that keeps the key order it was read with, so the
// rewritten tool_input differs from the original only in .command.
type ordered struct {
	keys []string
	vals []json.RawMessage
}

func (o ordered) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		b.Write(o.vals[i])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func orderedObject(raw json.RawMessage) (keys []string, vals []json.RawMessage, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil, false
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, nil, false
		}
		k, isStr := kt.(string)
		var v json.RawMessage
		if !isStr || dec.Decode(&v) != nil {
			return nil, nil, false
		}
		dup := false
		for i := range keys { // jq keeps the last duplicate, in the first position
			if keys[i] == k {
				vals[i], dup = v, true
			}
		}
		if !dup {
			keys, vals = append(keys, k), append(vals, v)
		}
	}
	return keys, vals, true
}

// Binary is the real rtk on PATH.
type Binary struct{ Path string }

func Lookup() Binary {
	p, _ := exec.LookPath("rtk")
	return Binary{Path: p}
}

func (b Binary) Available() bool { return b.Path != "" }

func (b Binary) run(timeout time.Duration, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var out bytes.Buffer
	c := exec.CommandContext(ctx, b.Path, args...)
	c.Stdout = &out
	err := c.Run()
	if err == nil {
		return out.String(), 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return out.String(), ee.ExitCode()
	}
	return "", -1
}

func (b Binary) Version() string {
	s, _ := b.run(5*time.Second, "--version")
	return s
}

func (b Binary) Rewrite(cmd string) (string, int) { return b.run(5*time.Second, "rewrite", cmd) }
