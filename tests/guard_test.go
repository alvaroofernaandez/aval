package tests

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// guard (internal/agent/guard) runs before every Bash call and denies
// destructive commands in bypass mode. These tests pin the wiring in
// settings.json, the binary's behaviour and its cost against the 10 ms budget.

const guardRm = `{"session_id":"g","cwd":"/work/proj","tool_name":"Bash","tool_input":{"command":"sh -c \"rm -fr /\""}}`

func TestGuardBinaryDeniesAWrappedRmAndAllowsCommonWork(t *testing.T) {
	aval := avalBin(t)
	home := t.TempDir()
	r := run(repo, isoEnv(home), guardRm, aval, "hook", "guard")
	eq(t, "exit code on deny", 0, r.Code)
	var v struct {
		H struct {
			D string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	_ = json.Unmarshal([]byte(r.Stdout), &v)
	eq(t, "sh -c rm -fr / is denied", "deny", v.H.D)

	ok := `{"cwd":"/work/proj","tool_name":"Bash","tool_input":{"command":"go test ./... && git push origin feature"}}`
	r = run(repo, isoEnv(home), ok, aval, "hook", "guard")
	eq(t, "exit code on allow", 0, r.Code)
	eq(t, "output on allow", "", r.Out())
}

func guardHook(t *testing.T) (hookCmd, bool) {
	t.Helper()
	for _, g := range repoSettings(t).Hooks["PreToolUse"] {
		if g.Matcher != "Bash" {
			continue
		}
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "hook guard") {
				return h, true
			}
		}
	}
	return hookCmd{}, false
}

func TestGuardWiring(t *testing.T) {
	h, ok := guardHook(t)
	isTrue(t, "PreToolUse runs the guard hook for Bash", ok)
	isTrue(t, "the guard hook is synchronous (it must decide before the call)", !h.Async)
	eq(t, "the guard hook timeout", 5.0, h.Timeout)
	mustHave(t, `^a="\$HOME/\.local/bin/aval"; \[ -x "\$a" \] \|\| a="\$\(command -v aval\)"; \[ -x "\$a" \] && "\$a" hook guard \|\| true$`,
		h.Command, "the hook uses the same aval wrapper as lessons")
}

// No aval binary: the hook must be invisible, not an error on every Bash call.
func TestGuardHookWithNoBinaryIsASilentNoOp(t *testing.T) {
	testenv.SkipWindows(t, "runs the shell-form hook command through sh")
	needTool(t, "sh")
	h, ok := guardHook(t)
	if !ok {
		t.Fatal("guard hook is not wired")
	}
	home := t.TempDir()
	r := run(home, withEnv(isoEnv(home), "PATH=/usr/bin:/bin"), guardRm, "sh", "-c", h.Command)
	eq(t, "exit code with no aval binary", 0, r.Code)
	eq(t, "output with no aval binary", "", r.Out())
}

func TestGuardHookLatency(t *testing.T) {
	needTool(t, "hyperfine")
	slow(t)
	aval := avalBin(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	write(t, in, `{"cwd":"/work/proj","tool_name":"Bash","tool_input":{"command":"git status && go test ./... | tail -5"}}`)
	ms := hyperfineMeanMs(t, isoEnv(dir), in, aval+" hook guard")
	t.Logf("guard hook mean: %.1f ms", ms)
	isTrue(t, "the guard hook runs in under 10 ms (is "+strconv.FormatFloat(ms, 'f', 1, 64)+" ms)", ms > 0 && ms < 10)
}
