package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The blocking rules must be identical for Claude Code and OpenCode.
//
// CLAUDE.md and config/opencode/AGENTS.md are two files carrying one set of
// rules to two agents; left to discipline they drift (the README records the
// 400-line PR rule that two copies stopped agreeing on). The blocking summary
// is delimited by markers in CLAUDE.md and injected into AGENTS.md by
// `aval rules sync`; this suite asserts the two are byte-identical.

func TestAgentRulesBothFilesDelimitTheSharedBlock(t *testing.T) {
	for _, f := range []string{"config/claude/CLAUDE.md", "config/opencode/AGENTS.md"} {
		text := read(t, f)
		isTrue(t, f+" delimits the shared block", strings.Contains(text, rulesBegin) && strings.Contains(text, rulesEnd))
	}
}

func TestAgentRulesBlockingRulesAreByteIdentical(t *testing.T) {
	c := sharedBlock(read(t, "config/claude/CLAUDE.md"))
	a := sharedBlock(read(t, "config/opencode/AGENTS.md"))
	isTrue(t, "the shared block is non-empty", strings.TrimSpace(c) != "")
	isTrue(t, "both agents carry byte-identical blocking rules", c == a)
	var missing []string
	for _, rule := range []string{"design-pipeline", "aval design gate", "DESIGN_PIPELINE_OFF", "tdd-gate", "RED", "strict-tdd"} {
		if !strings.Contains(c, rule) {
			missing = append(missing, rule)
		}
	}
	eq(t, "blocking rules missing from the shared block", "", strings.Join(missing, " "))
}

// Running the sync must be a no-op when things are already in sync; if it is
// not idempotent it churns the file on every install. Run on a copy, so the
// suite never writes into the checkout it is testing.
func TestAgentRulesSyncIsIdempotentAndCheckSeesDrift(t *testing.T) {
	aval := avalBin(t)

	tmp := t.TempDir()
	write(t, tmp+"/config/claude/CLAUDE.md", read(t, "config/claude/CLAUDE.md"))
	write(t, tmp+"/config/opencode/AGENTS.md", read(t, "config/opencode/AGENTS.md"))
	write(t, tmp+"/install.manifest", "")
	ents, _ := os.ReadDir(lessonsDir())
	for _, e := range ents {
		write(t, filepath.Join(tmp, "config/claude/lessons", e.Name()), readAbs(t, filepath.Join(lessonsDir(), e.Name())))
	}
	before := read(t, "config/opencode/AGENTS.md")
	r := run(repo, isoEnv(t.TempDir()), "", aval, "--repo", tmp, "rules", "sync")
	eq(t, "the sync runs ("+r.Out()+")", 0, r.Code)
	eq(t, "the sync is idempotent (the committed AGENTS.md is already in sync)", before, readAbs(t, tmp+"/config/opencode/AGENTS.md"))

	// --check on the real repo is read-only by contract.
	r = run(repo, isoEnv(t.TempDir()), "", aval, "--repo", repo, "rules", "sync", "--check")
	eq(t, "--check reports no drift after a sync\n"+r.Out(), 0, r.Code)

	// --check must see drift and must not write.
	stale := t.TempDir()
	write(t, stale+"/config/claude/CLAUDE.md", read(t, "config/claude/CLAUDE.md"))
	staleAgents := "# AGENTS\n" + rulesBegin + "\nstale\n" + rulesEnd + "\n"
	write(t, stale+"/config/opencode/AGENTS.md", staleAgents)
	write(t, stale+"/install.manifest", "")
	r = run(repo, isoEnv(t.TempDir()), "", aval, "--repo", stale, "rules", "sync", "--check")
	eq(t, "--check exits 1 on drift", 1, r.Code)
	eq(t, "--check does not write", staleAgents, readAbs(t, stale+"/config/opencode/AGENTS.md"))
}

// OpenCode has no lessons hook, so the scoped lessons that left the always-on
// block must reach it another way: their bodies are mirrored into AGENTS.md.
func TestAgentRulesEveryScopedLessonReachesOpenCode(t *testing.T) {
	lessonBlk := block(read(t, "config/opencode/AGENTS.md"), "<!-- aval:lessons -->", "<!-- /aval:lessons -->")
	files, _ := filepath.Glob(filepath.Join(lessonsDir(), "*.md"))
	isTrue(t, "there are lessons", len(files) > 0)
	var missing []string
	for _, f := range files {
		// first non-empty line of the body (after the second ---)
		first := ""
		c := 0
		for _, l := range strings.Split(readAbs(t, f), "\n") {
			if c >= 2 && strings.TrimSpace(l) != "" {
				first = l
				break
			}
			if l == "---" {
				c++
			}
		}
		if first == "" || !strings.Contains(lessonBlk, first) {
			missing = append(missing, filepath.Base(f))
		}
	}
	eq(t, "lessons missing from OpenCode's AGENTS.md", "", strings.Join(missing, " "))
}
