package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rulesRepo(t *testing.T, agents string) string {
	repo := t.TempDir()
	for p, body := range map[string]string{
		"config/claude/CLAUDE.md":    "<!-- shared:agent-rules -->\nrule\n<!-- /shared:agent-rules -->\n",
		"config/opencode/AGENTS.md":  agents,
		"config/claude/lessons/a.md": "---\nx: y\n---\nbody\n",
	} {
		writeFile(t, filepath.Join(repo, p), body)
	}
	return repo
}

func TestRunRulesSyncAndCheck(t *testing.T) {
	repo := rulesRepo(t, "# A\n")
	var out bytes.Buffer

	err := runRules(repo, []string{"sync", "--check"}, &out)
	if !errors.Is(err, errChecksFailed) || !strings.Contains(out.String(), "drift") {
		t.Fatalf("check on drift: err=%v out=%q", err, out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "config/opencode/AGENTS.md")); string(b) != "# A\n" {
		t.Fatal("--check wrote")
	}

	out.Reset()
	if err := runRules(repo, []string{"sync"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1 lines mirrored") {
		t.Fatalf("out = %q", out.String())
	}
	if err := runRules(repo, []string{"sync", "--check"}, &out); err != nil {
		t.Fatalf("check after sync: %v", err)
	}
}

func TestRunRulesUsage(t *testing.T) {
	if err := runRules(t.TempDir(), nil, &bytes.Buffer{}); err == nil {
		t.Fatal("want a usage error")
	}
}
