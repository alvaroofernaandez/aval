// Package rules mirrors the blocking agent rules from CLAUDE.md into OpenCode's
// AGENTS.md: one rule set, two agents, one source.
//
// CLAUDE.md owns the block between the shared markers; it is copied verbatim
// into AGENTS.md between the same markers, appending them if AGENTS.md does not
// have them yet. Only the blocking summary is shared. The detail lives in
// skills, which the manifest fanout already carries to OpenCode. Copying detail
// into two instruction files is how the two drift.
//
// OpenCode has no lessons hook (Claude Code injects ~/.claude/lessons per edit
// or command), so the lesson bodies are mirrored into AGENTS.md between their
// own markers: OpenCode keeps every rule, Claude keeps the token saving.
package rules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	rulesBegin   = "<!-- shared:agent-rules -->"
	rulesEnd     = "<!-- /shared:agent-rules -->"
	lessonsBegin = "<!-- aval:lessons -->"
	lessonsEnd   = "<!-- /aval:lessons -->"
)

// Result describes one sync run.
type Result struct {
	Changed bool // AGENTS.md differs from what the sources say
	Lines   int  // lines of the shared block mirrored
}

// Sync rewrites config/opencode/AGENTS.md from config/claude. With check it
// only reports whether anything would change. Idempotent: with nothing changed
// it rewrites identical bytes (and writes nothing at all).
func Sync(repo string, check bool) (Result, error) {
	src := filepath.Join(repo, "config", "claude", "CLAUDE.md")
	dst := filepath.Join(repo, "config", "opencode", "AGENTS.md")

	claude, err := os.ReadFile(src)
	if err != nil {
		return Result{}, err
	}
	block := between(strings.Split(string(claude), "\n"), rulesBegin, rulesEnd)
	if len(block) == 0 {
		return Result{}, fmt.Errorf("%w in %s", errNoBlock, src)
	}

	agentsRaw, err := os.ReadFile(dst)
	if err != nil {
		return Result{}, err
	}
	agents := string(agentsRaw)

	agents, err = inject(agents, rulesBegin, rulesEnd, strings.Join(block, "\n")+"\n")
	if err != nil {
		return Result{}, err
	}
	lessons, err := lessonsBlock(filepath.Join(repo, "config", "claude", "lessons"))
	if err != nil {
		return Result{}, err
	}
	agents, err = inject(agents, lessonsBegin, lessonsEnd, lessons)
	if err != nil {
		return Result{}, err
	}

	res := Result{Changed: agents != string(agentsRaw), Lines: len(block)}
	if check || !res.Changed {
		return res, nil
	}
	return res, os.WriteFile(dst, []byte(agents), 0o644)
}

// between returns the lines strictly between the first begin marker and the
// next end marker, trailing blank lines dropped (as shell $(...) would).
func between(lines []string, begin, end string) []string {
	var out []string
	in := false
	for _, l := range lines {
		switch {
		case l == begin:
			in = true
		case l == end:
			in = false
		case in:
			out = append(out, l)
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// inject replaces what is between the markers with body (which ends in a
// newline), or appends the whole marked block when the begin marker is absent.
func inject(doc, begin, end, body string) (string, error) {
	if !strings.Contains(doc, begin) {
		return doc + "\n" + begin + "\n" + body + end + "\n", nil
	}
	lines := strings.SplitAfter(doc, "\n")
	var b strings.Builder
	skip, closed := false, false
	for _, l := range lines {
		t := strings.TrimSuffix(l, "\n")
		switch {
		case t == begin:
			b.WriteString(l)
			b.WriteString(body)
			skip = true
			continue
		case t == end:
			skip, closed = false, true
		}
		if !skip {
			b.WriteString(l)
		}
	}
	if skip || !closed {
		return "", fmt.Errorf("rules sync: %s has no matching %s in AGENTS.md", begin, end)
	}
	return b.String(), nil
}

// lessonsBlock renders every lesson body (what follows the front matter), in
// file-name order.
func lessonsBlock(dir string) (string, error) {
	var b strings.Builder
	b.WriteString("## Scoped lessons (mirrored for OpenCode)\n")
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\n### %s\n", strings.TrimSuffix(filepath.Base(f), ".md"))
		b.WriteString(lessonBody(string(raw)))
	}
	return b.String(), nil
}

// lessonBody is the text after the second `---` line, without leading blank
// lines, every line newline-terminated.
func lessonBody(s string) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	var out []string
	fences, started := 0, false
	for _, l := range lines {
		if fences == 2 {
			if !started && strings.TrimSpace(l) == "" {
				continue
			}
			started = true
			out = append(out, l)
			continue
		}
		if l == "---" {
			fences++
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// errNoBlock is a CLAUDE.md without the shared rules markers.
var errNoBlock = errors.New("rules sync: no block found")
