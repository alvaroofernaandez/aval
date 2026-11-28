package guard

import (
	"reflect"
	"testing"
)

func words(cs []command) [][]string {
	var out [][]string
	for _, c := range cs {
		out = append(out, c.Words)
	}
	return out
}

func TestParseSplitsSimpleCommands(t *testing.T) {
	cases := []struct {
		name, in string
		want     [][]string
	}{
		{"one command", `rm -rf build`, [][]string{{"rm", "-rf", "build"}}},
		{"semicolon", `cd x; rm -rf y`, [][]string{{"cd", "x"}, {"rm", "-rf", "y"}}},
		{"and or", `a && b || c`, [][]string{{"a"}, {"b"}, {"c"}}},
		{"newline", "a\nb", [][]string{{"a"}, {"b"}}},
		{"pipe", `cat f | grep x`, [][]string{{"cat", "f"}, {"grep", "x"}}},
		{"single quotes are literal", `echo 'a  b; c'`, [][]string{{"echo", "a  b; c"}}},
		{"double quotes keep operators", `echo "a && b"`, [][]string{{"echo", "a && b"}}},
		{"escaped space", `rm a\ b`, [][]string{{"rm", "a b"}}},
		{"adjacent quotes join", `echo a"b"'c'`, [][]string{{"echo", "abc"}}},
		{"subshell parens", `(cd x; rm y)`, [][]string{{"cd", "x"}, {"rm", "y"}}},
		{"command substitution is its own command", `echo $(rm -rf /)`, [][]string{{"echo", "$(rm -rf /)"}, {"rm", "-rf", "/"}}},
		{"backticks", "echo `id`", [][]string{{"echo", "`id`"}, {"id"}}},
		{"substitution inside double quotes", `echo "x $(date)"`, [][]string{{"echo", "x $(date)"}, {"date"}}},
		{"keywords are peeled later, kept here", `for i in 1 2; do rm x; done`, [][]string{{"for", "i", "in", "1", "2"}, {"do", "rm", "x"}, {"done"}}},
		{"a comment ends the line", "ls # rm -rf /", [][]string{{"ls"}}},
		{"empty", ``, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parse(c.in, false)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(words(got), c.want) {
				t.Errorf("parse(%q) = %q, want %q", c.in, words(got), c.want)
			}
		})
	}
}

func TestParsePipesAndRedirections(t *testing.T) {
	got, err := parse(`curl -s x | sh > out 2>&1 < in.txt`, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].PipeNext || got[1].PipeNext {
		t.Fatalf("pipe flags wrong: %+v", got)
	}
	if !reflect.DeepEqual(got[1].Words, []string{"sh"}) {
		t.Errorf("redirections leaked into the words: %q", got[1].Words)
	}
	want := []redirect{{">", "out"}, {"<", "in.txt"}}
	if !reflect.DeepEqual(got[1].Redirs, want) {
		t.Errorf("redirs = %+v, want %+v", got[1].Redirs, want)
	}
}

func TestParseHeredocBodyIsDataNotCommands(t *testing.T) {
	got, err := parse("psql <<EOF\nDROP DATABASE x;\nEOF\nls", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Stdin != "DROP DATABASE x;\n" || got[1].Words[0] != "ls" {
		t.Errorf("heredoc handled wrong: %+v", got)
	}
}

func TestParseUnbalancedIsAnErrorUnlessLenient(t *testing.T) {
	for _, in := range []string{`echo "open`, `echo 'open`, `echo $(open`, "echo `open"} {
		if _, err := parse(in, false); err == nil {
			t.Errorf("parse(%q) must fail", in)
		}
	}
	got, err := parse(`echo "x; rm -rf /`, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(words(got), [][]string{{"echo", "x"}, {"rm", "-rf", "/"}}) {
		t.Errorf("lenient parse = %q", words(got))
	}
}
