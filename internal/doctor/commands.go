package doctor

import (
	"os"
	"regexp"
	"strings"
)

type token struct {
	text string
	op   bool
}

// tokenize splits a shell command into words and control operators. It is not
// a shell: it understands quotes, backslashes and the operators that start a
// new command (; && || | & newline), which is all the hook check needs.
func tokenize(cmd string) []token {
	var out []token
	var cur strings.Builder
	has := false
	flush := func() {
		if has {
			out = append(out, token{text: cur.String()})
			cur.Reset()
			has = false
		}
	}
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			has = true
		case c == '\'':
			has = true
			for i++; i < len(rs) && rs[i] != '\''; i++ {
				cur.WriteRune(rs[i])
			}
		case c == '"':
			has = true
			for i++; i < len(rs) && rs[i] != '"'; i++ {
				if rs[i] == '\\' && i+1 < len(rs) {
					i++
				}
				cur.WriteRune(rs[i])
			}
		case c == ' ' || c == '\t':
			flush()
		case c == '\n' || c == ';':
			flush()
			out = append(out, token{text: ";", op: true})
		case c == '|' || c == '&':
			// 2>&1 and &> are redirections, not control operators.
			cs := cur.String()
			if c == '&' && ((has && (strings.HasSuffix(cs, ">") || strings.HasSuffix(cs, "<"))) || (i+1 < len(rs) && rs[i+1] == '>')) {
				cur.WriteRune(c)
				has = true
				continue
			}
			flush()
			op := string(c)
			if i+1 < len(rs) && rs[i+1] == c {
				op += op
				i++
			}
			out = append(out, token{text: op, op: true})
		default:
			cur.WriteRune(c)
			has = true
		}
	}
	flush()
	return out
}

var (
	keywords = map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true,
		"do": true, "done": true, "while": true, "until": true, "!": true, "{": true, "}": true, "time": true}
	// Builtins and conditionals have nothing to resolve on disk.
	builtins   = map[string]bool{"true": true, "false": true, ":": true, "echo": true, "exit": true, "cd": true, "export": true, "set": true, "unset": true, "printf": true, "source": true, ".": true}
	tests      = map[string]bool{"[": true, "[[": true, "test": true}
	assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

// executables lists the programs a hook command needs on disk, with $HOME and
// a leading ~ expanded. A command guarded by a test ("[ -x f ] && f") is
// skipped: its author already accounted for the file being absent.
func executables(cmd, home string) []string {
	var out []string
	atStart, guarded, skipNext := true, false, false
	for _, t := range tokenize(cmd) {
		if t.op {
			atStart = true
			switch t.text {
			case "&&":
				skipNext = guarded
			default:
				skipNext = false
			}
			guarded = false
			continue
		}
		if !atStart {
			continue
		}
		if keywords[t.text] || assignment.MatchString(t.text) {
			continue
		}
		atStart = false
		switch {
		case tests[t.text]:
			guarded = true
		case builtins[t.text]:
		case skipNext:
			skipNext = false
		default:
			out = append(out, expand(t.text, home))
		}
	}
	return out
}

func expand(word, home string) string {
	if word == "~" || strings.HasPrefix(word, "~/") {
		word = home + word[1:]
	}
	return os.Expand(word, func(k string) string {
		if k == "HOME" {
			return home
		}
		return os.Getenv(k)
	})
}
