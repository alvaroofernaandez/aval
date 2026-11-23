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

// Running the sync must be a no-op when things are already in sync; if it is
// not idempotent it churns the file on every install. Run on a copy, so the
// suite never writes into the checkout it is testing. In process through run,
// with --repo resolved like the binary does; moved from tests/rules_sync_test.go,
// which keeps the real-binary smoke (`rules sync --check` on this checkout).
func TestAgentRulesSyncIsIdempotentAndCheckSeesDrift(t *testing.T) {
	src := repoRoot(t)
	env := func() map[string]string {
		home := t.TempDir()
		return map[string]string{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config")}
	}
	copyIn := func(dst, rel string) {
		b, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dst, rel), string(b))
	}

	tmp := t.TempDir()
	copyIn(tmp, "config/claude/CLAUDE.md")
	copyIn(tmp, "config/opencode/AGENTS.md")
	writeFile(t, filepath.Join(tmp, "install.manifest"), "")
	ents, _ := os.ReadDir(filepath.Join(src, "config", "claude", "lessons"))
	for _, e := range ents {
		copyIn(tmp, filepath.Join("config", "claude", "lessons", e.Name()))
	}
	before := readFile(t, filepath.Join(src, "config/opencode/AGENTS.md"))
	r := runIn(t, env(), "", "--repo", tmp, "rules", "sync")
	if r.code != 0 {
		t.Fatalf("the sync runs: exit %d (%s%s)", r.code, r.stdout, r.stderr)
	}
	if got := readFile(t, filepath.Join(tmp, "config/opencode/AGENTS.md")); got != before {
		t.Error("the sync is idempotent (the committed AGENTS.md is already in sync)")
	}

	// --check on the real repo is read-only by contract.
	if r := runIn(t, env(), "", "--repo", src, "rules", "sync", "--check"); r.code != 0 {
		t.Errorf("--check reports no drift after a sync: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}

	// --check must see drift and must not write.
	stale := t.TempDir()
	copyIn(stale, "config/claude/CLAUDE.md")
	staleAgents := "# AGENTS\n<!-- shared:agent-rules -->\nstale\n<!-- /shared:agent-rules -->\n"
	writeFile(t, filepath.Join(stale, "config/opencode/AGENTS.md"), staleAgents)
	writeFile(t, filepath.Join(stale, "install.manifest"), "")
	r = runIn(t, env(), "", "--repo", stale, "rules", "sync", "--check")
	if r.code != 1 {
		t.Errorf("--check exits 1 on drift, got %d", r.code)
	}
	if got := readFile(t, filepath.Join(stale, "config/opencode/AGENTS.md")); got != staleAgents {
		t.Error("--check does not write")
	}
}
