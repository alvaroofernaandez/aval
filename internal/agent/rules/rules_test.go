package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, agents string) string {
	repo := t.TempDir()
	write(t, filepath.Join(repo, "config/claude/CLAUDE.md"),
		"intro\n<!-- shared:agent-rules -->\nrule one\n\nrule two\n<!-- /shared:agent-rules -->\ntail\n")
	write(t, filepath.Join(repo, "config/claude/lessons/b-second.md"), "---\nname: b\n---\n\n\nsecond body\nmore\n")
	write(t, filepath.Join(repo, "config/claude/lessons/a-first.md"), "---\nname: a\n---\nfirst body\n")
	write(t, filepath.Join(repo, "config/claude/lessons/notes.txt"), "ignored")
	write(t, filepath.Join(repo, "config/opencode/AGENTS.md"), agents)
	return repo
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const wantLessons = "## Scoped lessons (mirrored for OpenCode)\n\n### a-first\nfirst body\n\n### b-second\nsecond body\nmore\n"

func TestSyncAppendsBothBlocksWhenMissing(t *testing.T) {
	repo := fixture(t, "# AGENTS\n")
	res, err := Sync(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	want := "# AGENTS\n" +
		"\n<!-- shared:agent-rules -->\nrule one\n\nrule two\n<!-- /shared:agent-rules -->\n" +
		"\n<!-- aval:lessons -->\n" + wantLessons + "<!-- /aval:lessons -->\n"
	if got := read(t, filepath.Join(repo, "config/opencode/AGENTS.md")); got != want {
		t.Fatalf("AGENTS.md:\n%q\nwant\n%q", got, want)
	}
	if !res.Changed || res.Lines != 3 {
		t.Fatalf("result = %+v", res)
	}
}

func TestSyncReplacesBetweenMarkersAndIsIdempotent(t *testing.T) {
	repo := fixture(t, "top\n<!-- shared:agent-rules -->\nstale\n<!-- /shared:agent-rules -->\nmiddle\n<!-- aval:lessons -->\nold\n<!-- /aval:lessons -->\nbottom\n")
	if _, err := Sync(repo, false); err != nil {
		t.Fatal(err)
	}
	want := "top\n<!-- shared:agent-rules -->\nrule one\n\nrule two\n<!-- /shared:agent-rules -->\nmiddle\n" +
		"<!-- aval:lessons -->\n" + wantLessons + "<!-- /aval:lessons -->\nbottom\n"
	got := read(t, filepath.Join(repo, "config/opencode/AGENTS.md"))
	if got != want {
		t.Fatalf("AGENTS.md:\n%q\nwant\n%q", got, want)
	}
	res, err := Sync(repo, false)
	if err != nil || res.Changed {
		t.Fatalf("second run changed something: %+v %v", res, err)
	}
	if read(t, filepath.Join(repo, "config/opencode/AGENTS.md")) != want {
		t.Fatal("second run rewrote different bytes")
	}
}

func TestCheckReportsDriftWithoutWriting(t *testing.T) {
	stale := "top\n<!-- shared:agent-rules -->\nstale\n<!-- /shared:agent-rules -->\n"
	repo := fixture(t, stale)
	res, err := Sync(repo, true)
	if err != nil || !res.Changed {
		t.Fatalf("check on drift: %+v %v", res, err)
	}
	if read(t, filepath.Join(repo, "config/opencode/AGENTS.md")) != stale {
		t.Fatal("--check must not write")
	}
	if _, err := Sync(repo, false); err != nil {
		t.Fatal(err)
	}
	if res, _ := Sync(repo, true); res.Changed {
		t.Fatal("check after sync still reports drift")
	}
}

func TestSyncFailsWithoutSharedBlock(t *testing.T) {
	repo := fixture(t, "x\n")
	write(t, filepath.Join(repo, "config/claude/CLAUDE.md"), "no markers here\n")
	if _, err := Sync(repo, false); err == nil || !strings.Contains(err.Error(), "no block found") {
		t.Fatalf("err = %v", err)
	}
}

func TestSyncFailsOnUnterminatedMarker(t *testing.T) {
	repo := fixture(t, "<!-- shared:agent-rules -->\nstale and never closed\n")
	if _, err := Sync(repo, false); err == nil {
		t.Fatal("an unterminated marker would delete the rest of the file; want an error")
	}
}
