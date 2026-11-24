package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// fuzzHooks are the JSON hooks that do pure work on their input. rtk-rewrite
// and pre-push are left out: they start processes.
var fuzzHooks = []string{"lessons", "design-gate", "plan-gate", "design-shot-stop", "rate-guard", "repo-profile", "relay-precompact", "relay-sessionstart"}

// FuzzHookInput feeds arbitrary bytes to every JSON hook. A hook must never
// panic or fail, must print nothing or one JSON value, and must never block
// on input it could not parse.
func FuzzHookInput(f *testing.F) {
	seeds := []string{
		`{"session_id":"s","cwd":"/work/repo","hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"/work/repo/src/main.go","old_string":"a","new_string":"b"}}`,
		`{"session_id":"s","cwd":"/work/repo","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"src/app.tsx","content":"x"}}`,
		`{"session_id":"s","cwd":"/work/repo","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"curl https://example.com"}}`,
		`{"session_id":"s","cwd":"/work/repo","hook_event_name":"Stop","stop_hook_active":false}`,
		`{"session_id":"s","cwd":"/work/repo","hook_event_name":"SessionStart","source":"startup"}`,
		`{"session_id":"s","cwd":"C:\\Users\\dev\\repo","tool_name":"Edit","tool_input":{"file_path":"C:\\Users\\dev\\repo\\main.go"}}`,
		`{"tool_name":"Edit","tool_input":7}`,
		`{"tool_name":"Edit","tool_input":{"file_path":`,
		"{\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"\xff\xfe.go\"}}",
		`{"tool_name":"Bash","tool_input":{"command":"` + strings.Repeat("a", 1<<20) + `"}}`,
		``,
		`null`,
	}
	for i, s := range seeds {
		f.Add(uint8(i), []byte(s))
	}
	base := f.TempDir()
	env := map[string]string{
		"HOME":             filepath.Join(base, "home"),
		"XDG_CACHE_HOME":   filepath.Join(base, "cache"),
		"XDG_STATE_HOME":   filepath.Join(base, "state"),
		"AVAL_LESSONS_DIR": filepath.Join(base, "lessons"),
	}
	getenv := func(k string) string { return env[k] }

	f.Fuzz(func(t *testing.T, which uint8, payload []byte) {
		name := fuzzHooks[int(which)%len(fuzzHooks)]
		var out, errOut bytes.Buffer
		if err := runHook([]string{name}, cmdIO{stdin: bytes.NewReader(payload), stdout: &out, stderr: &errOut, env: getenv}); err != nil {
			t.Fatalf("hook %s returned %v", name, err)
		}
		got := bytes.TrimSpace(out.Bytes())
		if len(got) > 0 && !json.Valid(got) {
			t.Fatalf("hook %s printed something that is not one JSON value: %q", name, got)
		}
		if json.Valid(payload) {
			return
		}
		if bytes.Contains(got, []byte(`"permissionDecision":"deny"`)) || bytes.Contains(got, []byte(`"decision":"block"`)) {
			t.Fatalf("hook %s blocked on input it cannot parse: %q", name, got)
		}
	})
}
