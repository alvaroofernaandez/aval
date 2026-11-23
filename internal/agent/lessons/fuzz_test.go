package lessons

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzLessonMatch pins the invariants of a lesson's trigger glob: it always
// compiles, so Parse never drops a trigger silently, and a glob with no
// wildcard matches its own literal path.
func FuzzLessonMatch(f *testing.F) {
	for _, seed := range []string{"src/{a", "**/*.go", "{a,b}/c.md", "docs/ñ.md", "a}b", "{{x}"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, glob string) {
		re, err := regexp.Compile(globRegexp(glob))
		if err != nil {
			t.Fatalf("glob %q compiles to an invalid regexp: %v", glob, err)
		}
		if utf8.ValidString(glob) && !strings.ContainsAny(glob, "*?{},") && !re.MatchString(glob) {
			t.Errorf("literal glob %q does not match itself (regexp %q)", glob, re)
		}
	})
}

// FuzzLessonBody pins that a parsed body is always valid UTF-8 and never
// longer than MaxBody, however the source is cut.
func FuzzLessonBody(f *testing.F) {
	f.Add(strings.Repeat("a", MaxBody-1) + "ñ")
	f.Add("short body")
	f.Fuzz(func(t *testing.T, body string) {
		if !utf8.ValidString(body) {
			t.Skip()
		}
		l, ok := Parse("x", "---\ntriggers:\n  - \"**/*.go\"\n---\n"+body)
		if !ok {
			t.Fatal("a lesson with a valid trigger must parse")
		}
		if !utf8.ValidString(l.Body) {
			t.Errorf("body cut inside a character: %q", l.Body[max(0, len(l.Body)-4):])
		}
		if len(l.Body) > MaxBody {
			t.Errorf("body is %d bytes, want at most %d", len(l.Body), MaxBody)
		}
	})
}
