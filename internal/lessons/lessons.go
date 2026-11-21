// Package lessons implements `aval hook lessons`, a PreToolUse hook that puts
// a short lesson into the model's context only when the tool call it is about
// to make matches the lesson's declared triggers.
//
// A lesson is a markdown file whose frontmatter lists `triggers:` (path globs,
// matched against the file an Edit/Write/NotebookEdit is about to touch) and/or
// `commands:` (regexes, matched against a Bash command). Rules that only matter
// near certain files then cost nothing the rest of the time, instead of riding
// in the always-on instructions on every turn.
//
// The hook is on the hot path of every edit, so it does the least possible:
// one small stdin read, a directory listing, and silence on no match. It never
// blocks: every failure degrades to printing nothing.
package lessons

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MaxBody is the longest lesson body that is injected, in bytes.
const MaxBody = 600

// Options configures Run. Everything is a parameter so tests use temp dirs.
type Options struct {
	// Dir holds the lesson *.md files (default ~/.claude/lessons).
	Dir string
	// StateDir holds the per-session dedupe files.
	StateDir string
}

// Lesson is one parsed lesson file.
type Lesson struct {
	Name     string
	Triggers []*regexp.Regexp
	Commands []*regexp.Regexp
	Body     string
}

type payload struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Command      string `json:"command"`
	} `json:"tool_input"`
}

// Run reads one PreToolUse payload from in and writes the hook response, if
// any, to out. It never returns a reason to block, and returns an error only
// for a failed write to out.
func Run(in io.Reader, out io.Writer, opt Options) error {
	raw, err := io.ReadAll(io.LimitReader(in, 1<<20))
	if err != nil || len(raw) == 0 {
		return nil
	}
	var p payload
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	path := p.ToolInput.FilePath
	if path == "" {
		path = p.ToolInput.NotebookPath
	}
	if path == "" && p.ToolInput.Command == "" {
		return nil
	}

	all := Load(opt.Dir)
	var hits []Lesson
	for _, l := range all {
		if l.matches(path, p.ToolInput.Command) {
			hits = append(hits, l)
		}
	}
	if len(hits) == 0 {
		return nil
	}

	hits = unseen(hits, opt.StateDir, p.SessionID)
	if len(hits) == 0 {
		return nil
	}

	var b strings.Builder
	for i, l := range hits {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[lesson: " + l.Name + "]\n" + l.Body)
	}
	resp := map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":     "PreToolUse",
		"additionalContext": b.String(),
	}}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}

func (l Lesson) matches(path, command string) bool {
	if path != "" {
		p := filepath.ToSlash(path)
		for _, re := range l.Triggers {
			if re.MatchString(p) {
				return true
			}
		}
	}
	if command != "" {
		for _, re := range l.Commands {
			if re.MatchString(command) {
				return true
			}
		}
	}
	return false
}

// Load parses every lesson in dir, sorted by file name. A file without
// frontmatter triggers, or an unparsable one, is skipped, never fatal.
func Load(dir string) []Lesson {
	if dir == "" {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Lesson
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if l, ok := Parse(strings.TrimSuffix(e.Name(), ".md"), string(b)); ok {
			out = append(out, l)
		}
	}
	return out
}

// Parse reads one lesson. ok is false when the file has no frontmatter or
// declares no usable trigger.
func Parse(name, src string) (Lesson, bool) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Lesson{}, false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Lesson{}, false
	}

	l := Lesson{Name: name}
	key := ""
	for _, ln := range lines[1:end] {
		t := strings.TrimSpace(ln)
		switch {
		case t == "" || strings.HasPrefix(t, "#"):
		case strings.HasSuffix(t, ":") && !strings.HasPrefix(t, "-"):
			key = strings.TrimSuffix(t, ":")
		case strings.HasPrefix(t, "- ") || t == "-":
			v := unquote(strings.TrimSpace(strings.TrimPrefix(t, "-")))
			if v == "" {
				continue
			}
			switch key {
			case "triggers":
				if re, err := regexp.Compile(globRegexp(v)); err == nil {
					l.Triggers = append(l.Triggers, re)
				}
			case "commands":
				if re, err := regexp.Compile(v); err == nil {
					l.Commands = append(l.Commands, re)
				}
			}
		default:
			key = ""
		}
	}
	if len(l.Triggers) == 0 && len(l.Commands) == 0 {
		return Lesson{}, false
	}

	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if len(body) > MaxBody {
		body = body[:MaxBody]
	}
	l.Body = body
	return l, true
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// globRegexp turns a path glob into a regexp anchored at a directory boundary
// on the left (so `src/**/*.css` matches `/p/src/a/b.css`) and at the end.
//
//	**/  any number of directories    **  anything    *  within a segment
//	?    one character                {a,b}  alternatives
func globRegexp(glob string) string {
	var b strings.Builder
	b.WriteString(`(?:^|/)`)
	braces := 0
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
					b.WriteString(`(?:.*/)?`)
				} else {
					b.WriteString(`.*`)
				}
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		case '{':
			braces++
			b.WriteString(`(?:`)
		case '}':
			if braces > 0 {
				braces--
				b.WriteString(`)`)
			} else {
				b.WriteString(`\}`)
			}
		case ',':
			if braces > 0 {
				b.WriteString(`|`)
			} else {
				b.WriteString(`,`)
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString(`$`)
	return b.String()
}

// unseen drops lessons already shown in this session and records the rest.
// Without a usable session id there is nothing to key on, so everything shows.
func unseen(hits []Lesson, stateDir, session string) []Lesson {
	session = safeID(session)
	if session == "" || stateDir == "" {
		return hits
	}
	file := filepath.Join(stateDir, session+".seen")
	seen := map[string]bool{}
	if f, err := os.Open(file); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			seen[sc.Text()] = true
		}
		f.Close()
	}

	var fresh []Lesson
	for _, l := range hits {
		if !seen[l.Name] {
			fresh = append(fresh, l)
		}
	}
	if len(fresh) == 0 {
		return nil
	}

	_, statErr := os.Stat(file)
	if os.MkdirAll(stateDir, 0o755) != nil {
		return fresh
	}
	if os.IsNotExist(statErr) {
		prune(stateDir)
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fresh
	}
	defer f.Close()
	names := make([]string, 0, len(fresh))
	for _, l := range fresh {
		names = append(names, l.Name)
	}
	sort.Strings(names)
	_, _ = f.WriteString(strings.Join(names, "\n") + "\n")
	return fresh
}

// prune removes dedupe files of sessions that ended long ago. It runs only
// when a new session file is created, so the common path never pays for it.
func prune(dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-14 * 24 * time.Hour)
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".seen") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// safeID keeps a session id usable as a file name and nothing more.
func safeID(s string) string {
	if s == "" || len(s) > 128 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return ""
		}
	}
	return s
}
