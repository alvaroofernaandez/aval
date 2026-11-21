package tests

import (
	"bytes"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

// SDD (Spec-Driven Development) is dropped from this harness, 2026-11-21. It
// cost on every turn: instruction text loaded whether or not a session touched
// it, a 657 ms hook on every subagent launch, eleven skills and eleven agents
// competing for triggers. gentle-ai 4.6.0 reports the component as retired, so
// nothing reinstalls it, which also means nothing removes the leftovers but this
// repo. Retired material lives only in git history.

var sddName = regexp.MustCompile(`(?i)^(sdd([-_.].*)?|skill-registry|openspec.*)$`)

func TestNoSDDLeftovers(t *testing.T) {
	var found []string
	for _, f := range trackedAndUntracked(t) {
		underRoot := false
		for _, r := range []string{"skills/", "config/claude/", "config/opencode/"} {
			underRoot = underRoot || strings.HasPrefix(f, r)
		}
		if !underRoot {
			continue
		}
		for _, part := range strings.Split(f, "/") {
			if sddName.MatchString(part) {
				found = append(found, f)
				break
			}
		}
	}
	eq(t, "SDD skills, agents, prompts or profiles that remain", "", strings.Join(found, " "))

	var shared []string
	for _, f := range trackedAndUntracked(t) {
		if strings.HasPrefix(f, "skills/_shared/") &&
			regexp.MustCompile(`(?i)^(sdd-|openspec-)`).MatchString(path.Base(f)) {
			shared = append(shared, f)
		}
	}
	eq(t, "_shared SDD/openspec contracts that remain", "", strings.Join(shared, " "))
}

// Active instructions, skills, hooks, plugins and scripts must not mention it.
func TestNoSDDMentionsInActiveFiles(t *testing.T) {
	re := regexp.MustCompile(`(?i)\bsdd\b|sdd-|openspec|skill-registry`)
	roots := []string{"config/", "skills/", "docs/"}
	exact := map[string]bool{"install.manifest": true, "README.md": true}
	var hits []string
	for _, f := range trackedAndUntracked(t) {
		in := exact[f]
		for _, r := range roots {
			in = in || strings.HasPrefix(f, r)
		}
		if !in || strings.HasPrefix(f, "skills-retired/") || regexp.MustCompile(`\.bak-`).MatchString(f) {
			continue
		}
		hidden := false // ripgrep skips dot-files and dot-dirs by default
		for _, part := range strings.Split(f, "/") {
			hidden = hidden || strings.HasPrefix(part, ".")
		}
		if hidden {
			continue
		}
		b, err := os.ReadFile(p(f))
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			continue
		}
		if re.Match(b) {
			hits = append(hits, f)
		}
	}
	eq(t, "active files that mention SDD, openspec or skill-registry", "", strings.Join(hits, " "))
}
