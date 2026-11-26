package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const floorMain = `{"type":"assistant","isSidechain":false,"timestamp":"2026-11-26T10:00:05Z","message":{"usage":{"input_tokens":2,"cache_creation_input_tokens":37489,"cache_read_input_tokens":31177}}}
{"type":"assistant","isSidechain":false,"timestamp":"2026-11-26T10:05:00Z","message":{"usage":{"input_tokens":2,"cache_creation_input_tokens":100,"cache_read_input_tokens":90000}}}
`
const floorSub = `{"type":"assistant","isSidechain":true,"timestamp":"2026-11-26T10:01:00Z","message":{"usage":{"input_tokens":3,"cache_creation_input_tokens":1000,"cache_read_input_tokens":9000}}}
`

func contextFloorRun(t *testing.T, home, wd string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	env := map[string]string{"HOME": home, "USERPROFILE": home}
	err := runContext(append([]string{"floor"}, args...), wd, cmdIO{stdout: &out, stderr: &bytes.Buffer{}, env: func(k string) string { return env[k] }})
	return out.String(), err
}

func TestContextFloor_DefaultsToTheNewestSessionOfTheCwd(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, ".claude", "projects", "-work-app")
	writeFile(t, filepath.Join(proj, "old.jsonl"), `{"type":"assistant","timestamp":"2026-11-26T09:00:00Z","message":{"usage":{"input_tokens":1}}}`+"\n")
	writeFile(t, filepath.Join(proj, "new.jsonl"), floorMain)
	writeFile(t, filepath.Join(proj, "new", "subagents", "agent-a.jsonl"), floorSub)
	touch(t, filepath.Join(proj, "old.jsonl"), -3600)
	got, err := contextFloorRun(t, home, "/work/app")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"68668", "10003"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q lacks %s", got, want)
		}
	}
}

func TestContextFloor_SessionAndProjectDirFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "abc.jsonl"), floorMain)
	got, err := contextFloorRun(t, t.TempDir(), "/elsewhere", "--project-dir", dir, "--session", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "68668") || strings.Contains(got, "10003") {
		t.Errorf("got %q: want the main floor and no sub-agent floor", got)
	}
}

func TestContextFloor_NoSessionIsAnError(t *testing.T) {
	if _, err := contextFloorRun(t, t.TempDir(), "/nowhere"); err == nil {
		t.Fatal("want an error when there is no session")
	}
}

func TestContext_UnknownSubcommandIsUsage(t *testing.T) {
	err := runContext([]string{"nope"}, "/x", cmdIO{stdout: &bytes.Buffer{}, env: func(string) string { return "" }})
	var u *usageError
	if !errors.As(err, &u) {
		t.Fatalf("err = %v, want a usage error", err)
	}
}

// touch sets the mtime of path secs seconds from now.
func touch(t *testing.T, path string, secs int) {
	t.Helper()
	at := time.Now().Add(time.Duration(secs) * time.Second)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// Claude Code names a project dir after the cwd with every character that is
// not a letter or digit replaced by "-", on every OS.
func TestClaudeProjectDirName(t *testing.T) {
	for in, want := range map[string]string{
		"/work/app":                     "-work-app",
		`C:\Users\a.b\my_app`:           "C--Users-a-b-my-app",
		"/Users/x/Proyectos/sumi-panel": "-Users-x-Proyectos-sumi-panel",
	} {
		if got := claudeProjectDirName(in); got != want {
			t.Errorf("claudeProjectDirName(%q) = %q, want %q", in, got, want)
		}
	}
}
