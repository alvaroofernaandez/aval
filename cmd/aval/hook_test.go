package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestRunHookLessonsUsesEnvDirsAndXDGState(t *testing.T) {
	dir, state, home := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AVAL_LESSONS_DIR", dir)
	t.Setenv("XDG_STATE_HOME", state)
	writeFile(t, filepath.Join(dir, "commit.md"), "---\ncommands:\n  - 'git commit'\n---\nNo trailers.\n")

	var out bytes.Buffer
	in := `{"session_id":"abc","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`
	if err := runHook([]string{"lessons"}, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No trailers.") {
		t.Errorf("output = %q, want the lesson", out.String())
	}
	if !fileExists(filepath.Join(state, "aval", "lessons", "abc.seen")) {
		t.Error("dedupe state must live under $XDG_STATE_HOME/aval/lessons")
	}
}

func TestRunHookLessonsDefaultsToHomeClaudeLessons(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AVAL_LESSONS_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	writeFile(t, filepath.Join(home, ".claude", "lessons", "x.md"), "---\ntriggers:\n  - '**/*.tsx'\n---\nHi.\n")
	var out bytes.Buffer
	in := `{"session_id":"s","tool_name":"Edit","tool_input":{"file_path":"/p/a.tsx"}}`
	if err := runHook([]string{"lessons"}, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi.") {
		t.Errorf("output = %q", out.String())
	}
	if !fileExists(filepath.Join(home, ".local", "state", "aval", "lessons", "s.seen")) {
		t.Error("default state dir should be ~/.local/state/aval/lessons")
	}
}

func TestRunHookUnknownHookIsAnError(t *testing.T) {
	if err := runHook([]string{"nope"}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Error("an unknown hook name must be reported")
	}
}
