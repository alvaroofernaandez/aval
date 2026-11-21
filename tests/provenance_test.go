package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Attribution for vendored skills must not silently disappear.
//
// Many skills under skills are third-party work carrying MIT or
// Apache-2.0 licences; both require the notice to travel with the work. Two
// failure modes are guarded: a licensed skill that never made it into
// PROVENANCE.md, and a SKILL.md whose license frontmatter was stripped while
// the skill stayed.

func TestProvenanceDocExists(t *testing.T) {
	isTrue(t, "PROVENANCE.md exists", exists(p("skills/PROVENANCE.md")))
}

func TestProvenanceEveryLicensedSkillIsAttributed(t *testing.T) {
	doc := read(t, "skills/PROVENANCE.md")
	var missing []string
	licensed := 0
	for _, name := range subdirs(t, "skills") {
		b, err := os.ReadFile(filepath.Join(p("skills"), name, "SKILL.md"))
		if err != nil || !has(`^license: `, string(b)) {
			continue
		}
		licensed++
		if !strings.Contains(doc, "`"+name+"`") {
			missing = append(missing, name)
		}
	}
	isTrue(t, "some skills carry a licence", licensed > 0)
	eq(t, "licensed skills missing from PROVENANCE.md", "", strings.Join(missing, " "))
}

// The frontmatter is the authoritative notice; the table only mirrors it.
func TestProvenanceNoAttributedSkillLostItsLicenceFrontmatter(t *testing.T) {
	doc := read(t, "skills/PROVENANCE.md")
	var stripped []string
	rows := regexp.MustCompile("(?m)^\\| `([a-z0-9-]+)` \\|").FindAllStringSubmatch(doc, -1)
	isTrue(t, "the table has rows", len(rows) > 0)
	for _, m := range rows {
		b, err := os.ReadFile(filepath.Join(p("skills"), m[1], "SKILL.md"))
		switch {
		case err != nil:
			stripped = append(stripped, m[1]+"(gone)")
		case !has(`^license: `, string(b)):
			stripped = append(stripped, m[1])
		}
	}
	eq(t, "attributed skills without licence frontmatter", "", strings.Join(stripped, " "))
}
