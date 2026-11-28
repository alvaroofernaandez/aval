package guard

import (
	"errors"
	"regexp"
	"strings"
)

type redirect struct{ Op, Target string }

// command is one simple command of a script. Redirections are kept apart from
// Words; Stdin is the body of a heredoc attached to it.
type command struct {
	Words    []string
	PipeNext bool
	Redirs   []redirect
	Stdin    string
}

var errUnbalanced = errors.New("guard: unbalanced quote or substitution")

var redirectWord = regexp.MustCompile(`^(?:[0-9]*|&)(>>|>\||>&|&>|<<<|<<-|<<|<&|<>|>|<)(.*)$`)

type heredoc struct {
	delim string
	strip bool
	cmd   int // index in out, resolved when the line ends
}

type lexer struct {
	s       []rune
	i       int
	lenient bool
	out     []command
	cur     command
	w       []rune
	have    bool // a word is being built (an empty "" still counts)
	quoted  bool
	op      string // a redirect operator waiting for its target
	opHere  heredoc
	pend    []heredoc
	subs    []string
}

// parse splits a shell script into simple commands. Substitutions ($(...),
// backticks, <(...)) become commands of their own, after the command that
// holds them. Strict mode fails on an unterminated quote or substitution;
// lenient mode ignores quoting altogether and never fails, which is what the
// guard falls back to when the text does not parse.
func parse(s string, lenient bool) ([]command, error) {
	p := &lexer{s: []rune(s), lenient: lenient}
	if err := p.run(); err != nil {
		return nil, err
	}
	return p.out, nil
}

func (p *lexer) run() error {
	for p.i < len(p.s) {
		r := p.s[p.i]
		switch r {
		case ' ', '\t', '\r':
			p.endWord()
			p.i++
		case '\n':
			p.endWord()
			p.endCmd(false)
			p.readHeredocs()
			p.i++
		case '#':
			if p.have {
				p.add(r)
				continue
			}
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
		case ';':
			p.endWord()
			p.endCmd(false)
			p.i++
		case '&':
			p.amp()
		case '|':
			p.pipe()
		case '(':
			if err := p.openParen(); err != nil {
				return err
			}
		case ')':
			p.endWord()
			p.endCmd(false)
			p.i++
		case '`':
			if err := p.backtick(); err != nil {
				return err
			}
		case '$':
			if err := p.dollar(); err != nil {
				return err
			}
		case '\\':
			p.escape()
		case '\'':
			if err := p.singleQuote(); err != nil {
				return err
			}
		case '"':
			if err := p.doubleQuote(); err != nil {
				return err
			}
		default:
			p.add(r)
		}
	}
	p.endWord()
	p.endCmd(false)
	p.readHeredocs()
	return p.flushSubs()
}

func (p *lexer) add(r rune) {
	p.w = append(p.w, r)
	p.have = true
	p.i++
}

func (p *lexer) peek(n int) rune {
	if p.i+n < len(p.s) {
		return p.s[p.i+n]
	}
	return 0
}

func (p *lexer) amp() {
	if p.peek(1) == '&' {
		p.endWord()
		p.endCmd(false)
		p.i += 2
		return
	}
	if p.have && len(p.w) > 0 && (p.w[len(p.w)-1] == '>' || p.w[len(p.w)-1] == '<') || p.peek(1) == '>' && !p.have {
		p.add('&')
		return
	}
	p.endWord()
	p.endCmd(false)
	p.i++
}

func (p *lexer) pipe() {
	if p.have && len(p.w) > 0 && p.w[len(p.w)-1] == '>' { // >|
		p.add('|')
		return
	}
	p.endWord()
	switch p.peek(1) {
	case '|':
		p.endCmd(false)
		p.i += 2
	case '&':
		p.endCmd(true)
		p.i += 2
	default:
		p.endCmd(true)
		p.i++
	}
}

func (p *lexer) escape() {
	if p.peek(1) == '\n' { // line continuation
		p.i += 2
		return
	}
	p.quoted = true
	if p.lenient || p.i+1 >= len(p.s) {
		p.i++
		return
	}
	p.w = append(p.w, p.s[p.i+1])
	p.have = true
	p.i += 2
}

func (p *lexer) singleQuote() error {
	p.i++
	if p.lenient {
		return nil
	}
	p.quoted, p.have = true, true
	for p.i < len(p.s) && p.s[p.i] != '\'' {
		p.w = append(p.w, p.s[p.i])
		p.i++
	}
	if p.i >= len(p.s) {
		return errUnbalanced
	}
	p.i++
	return nil
}

func (p *lexer) doubleQuote() error {
	p.i++
	if p.lenient {
		return nil
	}
	p.quoted, p.have = true, true
	for p.i < len(p.s) && p.s[p.i] != '"' {
		switch r := p.s[p.i]; {
		case r == '\\' && p.i+1 < len(p.s):
			if n := p.s[p.i+1]; n == '$' || n == '"' || n == '\\' || n == '`' {
				p.w = append(p.w, n)
			} else {
				p.w = append(p.w, r, n)
			}
			p.i += 2
		case r == '$' && p.peek(1) == '(':
			if err := p.dollar(); err != nil {
				return err
			}
		case r == '`':
			if err := p.backtick(); err != nil {
				return err
			}
		default:
			p.w = append(p.w, r)
			p.i++
		}
	}
	if p.i >= len(p.s) {
		return errUnbalanced
	}
	p.i++
	return nil
}

// dollar handles $(...), ${...} and a lone $.
func (p *lexer) dollar() error {
	if p.lenient {
		p.add('$')
		return nil
	}
	switch p.peek(1) {
	case '(':
		end, ok := closeParen(p.s, p.i+2)
		if !ok {
			return errUnbalanced
		}
		return p.substitute("$(", p.s[p.i+2:end], end+1)
	case '{':
		for j := p.i; j < len(p.s); j++ {
			if p.s[j] == '}' {
				p.w = append(p.w, p.s[p.i:j+1]...)
				p.have = true
				p.i = j + 1
				return nil
			}
		}
		return errUnbalanced
	}
	p.add('$')
	return nil
}

func (p *lexer) backtick() error {
	if p.lenient {
		p.endWord()
		p.endCmd(false)
		p.i++
		return nil
	}
	for j := p.i + 1; j < len(p.s); j++ {
		if p.s[j] == '\\' {
			j++
		} else if p.s[j] == '`' {
			return p.substitute("`", p.s[p.i+1:j], j+1)
		}
	}
	return errUnbalanced
}

// openParen is a subshell paren, or the start of <(...) / >(...).
func (p *lexer) openParen() error {
	if !p.lenient && p.have && len(p.w) == 1 && (p.w[0] == '<' || p.w[0] == '>') {
		end, ok := closeParen(p.s, p.i+1)
		if !ok {
			return errUnbalanced
		}
		p.op = ""
		return p.substitute("(", p.s[p.i+1:end], end+1)
	}
	p.endWord()
	p.endCmd(false)
	p.i++
	return nil
}

// substitute records the inner script as a command of its own and keeps the
// raw text in the word, so a rule can still see "bash <(curl x)".
func (p *lexer) substitute(open string, inner []rune, next int) error {
	closer := ")"
	if open == "`" {
		closer = "`"
	}
	p.w = append(p.w, []rune(open+string(inner)+closer)...)
	p.have = true
	p.subs = append(p.subs, string(inner))
	p.i = next
	return nil
}

func (p *lexer) flushSubs() error {
	for len(p.subs) > 0 {
		sub := p.subs[0]
		p.subs = p.subs[1:]
		cs, err := parse(sub, false)
		if err != nil {
			return err
		}
		p.out = append(p.out, cs...)
	}
	return nil
}

// closeParen finds the ")" matching an already opened "(", skipping quotes and
// nested parentheses. from is the index just after the opening paren.
func closeParen(s []rune, from int) (int, bool) {
	depth := 1
	for j := from; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '\'':
			for j++; j < len(s) && s[j] != '\''; j++ {
			}
		case '"':
			for j++; j < len(s) && s[j] != '"'; j++ {
				if s[j] == '\\' {
					j++
				}
			}
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return j, true
			}
		}
	}
	return 0, false
}

func (p *lexer) endWord() {
	if !p.have {
		return
	}
	w, quoted := string(p.w), p.quoted
	p.w, p.have, p.quoted = p.w[:0], false, false
	if p.op != "" {
		p.target(w)
		return
	}
	if !quoted && !strings.HasPrefix(w, "<(") && !strings.HasPrefix(w, ">(") {
		if m := redirectWord.FindStringSubmatch(w); m != nil {
			p.redirectOp(m[1], m[2])
			return
		}
	}
	p.cur.Words = append(p.cur.Words, w)
}

func (p *lexer) redirectOp(op, rest string) {
	if (op == ">&" || op == "<&") && (rest == "-" || rest != "" && strings.Trim(rest, "0123456789") == "") {
		return // fd duplication: 2>&1
	}
	p.op = op
	if rest != "" {
		p.target(rest)
	}
}

func (p *lexer) target(t string) {
	op := p.op
	p.op = ""
	switch op {
	case "<<", "<<-":
		p.pend = append(p.pend, heredoc{delim: t, strip: op == "<<-", cmd: -1})
		p.cur.Redirs = append(p.cur.Redirs, redirect{"<<", t})
		return
	case "<<<":
	case "<", "<&", "<>":
		op = "<"
	default:
		op = ">"
	}
	p.cur.Redirs = append(p.cur.Redirs, redirect{op, t})
}

func (p *lexer) endCmd(pipe bool) {
	if len(p.cur.Words) == 0 && len(p.cur.Redirs) == 0 {
		return
	}
	p.cur.PipeNext = pipe
	p.out = append(p.out, p.cur)
	for i := range p.pend {
		if p.pend[i].cmd < 0 {
			p.pend[i].cmd = len(p.out) - 1
		}
	}
	p.cur = command{}
}

// readHeredocs consumes the bodies of the heredocs opened on the line that
// just ended and attaches each to its command.
func (p *lexer) readHeredocs() {
	for _, h := range p.pend {
		var body strings.Builder
		for p.i+1 < len(p.s) {
			start := p.i + 1
			end := start
			for end < len(p.s) && p.s[end] != '\n' {
				end++
			}
			line := string(p.s[start:end])
			p.i = end
			if h.strip {
				line = strings.TrimLeft(line, "\t")
			}
			if line == h.delim {
				break
			}
			body.WriteString(line + "\n")
		}
		if h.cmd >= 0 && h.cmd < len(p.out) {
			p.out[h.cmd].Stdin += body.String()
		}
	}
	p.pend = nil
}
