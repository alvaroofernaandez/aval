package tests

import (
	"path/filepath"
	"strings"
	"testing"
)

// The generic engineering standards ship to every aval user through skills/
// (the manifest fans every directory under skills/ out to each agent). These
// guards keep the two skills loadable, installed, complete, and free of this
// repository's own paths, which mean nothing in a user's project.

var distributedStandards = []string{"strict-tdd", "architecture-evidence"}

func TestDistributedSkillArchitectureEvidenceHasValidFrontmatter(t *testing.T) {
	text := read(t, "skills/architecture-evidence/SKILL.md")
	name, desc, ok := projectSkillFrontmatter(text)
	isTrue(t, "SKILL.md opens with a closed frontmatter block", ok)
	eq(t, "frontmatter name is the directory name", "architecture-evidence", name)
	isTrue(t, "description says when to use it", strings.HasPrefix(desc, "Use when"))
}

func TestDistributedSkillsAreFannedOutByTheManifest(t *testing.T) {
	m := parseManifest(read(t, "install.manifest"))
	eq(t, "the skills group fans out the whole skills/ directory", "skills", m.fanoutSource)
	for _, s := range distributedStandards {
		isTrue(t, s+" has a SKILL.md for the fan-out to link", exists(p("skills", s, "SKILL.md")))
	}
}

func TestDistributedStrictTddCarriesTheTestingChecklist(t *testing.T) {
	text := read(t, "skills/strict-tdd/SKILL.md")
	for what, re := range map[string]string{
		"levels, with one real-process smoke per entry point": `(?i)real-process smoke`,
		"error identity, not text":                            `errors\.Is`,
		"user-facing text pinned in one place":                `(?i)one place`,
		"no fixed sleeps":                                     `(?i)no fixed sleeps`,
		"the flake protocol":                                  `(?i)flake protocol`,
		"the differential check":                              `(?i)differential check`,
		"the output contract":                                 `(?i)output contract`,
		"skipped is not passed":                               `(?i)skipped is not passed`,
	} {
		mustHave(t, re, text, "strict-tdd: "+what)
	}
}

func TestDistributedStandardsNameNoAvalInternals(t *testing.T) {
	for _, s := range distributedStandards {
		files, _ := filepath.Glob(p("skills", s, "*.md"))
		refs, _ := filepath.Glob(p("skills", s, "references", "*.md"))
		for _, f := range append(files, refs...) {
			rel, _ := filepath.Rel(repo, f)
			text := read(t, rel)
			for _, bad := range []string{"AVAL_TEST_REPO", "internal/", "cmd/aval", "aval-testing", "aval-architecture"} {
				if strings.Contains(text, bad) {
					t.Errorf("%s names the aval-internal %q", rel, bad)
				}
			}
		}
	}
}

func TestDistributedStandardsStayCompact(t *testing.T) {
	const maxBytes = 9000
	for _, s := range distributedStandards {
		n := len(read(t, "skills/"+s+"/SKILL.md"))
		isTrue(t, s+" stays under "+itoa(maxBytes)+" B (is "+itoa(n)+" B)", n <= maxBytes)
	}
}
