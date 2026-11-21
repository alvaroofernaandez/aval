package sync

import "testing"

func TestRemoveRules(t *testing.T) {
	const b, e = "<!-- aval:agent-rules -->", "<!-- /aval:agent-rules -->"
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{"block appended by sync", "top\n\n" + b + "\nrules\n" + e + "\n", "top\n", true},
		{"block in the middle, outside kept", "top\n" + b + "\nx\n" + e + "\nbottom\n", "top\nbottom\n", true},
		{"file made only of the block", "\n" + b + "\nx\n" + e + "\n", "", true},
		{"no markers is untouched", "mine\n", "mine\n", false},
		{"begin without end is untouched", "mine\n" + b + "\nx\n", "mine\n" + b + "\nx\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := RemoveRules(c.in)
			if got != c.want || changed != c.changed {
				t.Errorf("got (%q, %v), want (%q, %v)", got, changed, c.want, c.changed)
			}
		})
	}
}

func TestRemoveRulesUndoesApplyRules(t *testing.T) {
	for _, in := range []string{"", "mine\n", "a\nb\n"} {
		applied, _ := ApplyRules("rules", in)
		got, _ := RemoveRules(applied)
		if got != in {
			t.Errorf("RemoveRules(ApplyRules(%q)) = %q", in, got)
		}
	}
}
