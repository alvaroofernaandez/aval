package sync

import "testing"

func TestRuleGlobMatchesNonASCIILiterals(t *testing.T) {
	cases := []struct{ tool, pattern, arg string }{
		{"Read", "docs/ñ.md", "docs/ñ.md"},
		{"Read", "**/café/*.md", "a/café/b.md"},
		{"Bash", "echo ¿qué*", "echo ¿qué tal"},
	}
	for _, c := range cases {
		if !ruleGlob(c.tool, c.pattern).MatchString(c.arg) {
			t.Errorf("ruleGlob(%q, %q) does not match %q", c.tool, c.pattern, c.arg)
		}
	}
}
