package relay

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Capsule is what the new context starts with: the work's objective, facts
// about the repository, the work still running and an optional note. The
// verbatim tail of messages travels beside it, as handles.
type Capsule struct {
	FirstPrompt string   // the request that started the work
	LastPrompt  string   // the latest request
	Prompts     []string // earlier requests, oldest first (at most 15 kept)
	Files       []string // recently edited files, newest first (at most 25 kept)
	Repo        *RepoFacts
	Work        []string // running work, one line each
	Note        string   // optional next-step note
}

// RepoFacts are read by an adapter; a nil value means no repository.
type RepoFacts struct {
	Branch  string `json:"branch"`
	Changed int    `json:"changed"` // files with uncommitted changes
}

// tokensOf is a conservative token count for capsule text (E1 of #63).
func tokensOf(s string) int { return int(float64(len(s))/charsPerToken + 0.999) }

// Render returns the capsule text, redacted, within budget tokens. Over
// budget it drops the note, then the earlier requests, then the edited files,
// then shortens the objective; it never returns more than budget tokens.
func Render(c Capsule, budget int) string {
	c = redactCapsule(c)
	for _, drop := range []func(*Capsule){
		func(c *Capsule) {},
		func(c *Capsule) { c.Note = "" },
		func(c *Capsule) { c.Prompts = nil },
		func(c *Capsule) { c.Files = nil },
	} {
		drop(&c)
		if s := render(c); tokensOf(s) <= budget {
			return s
		}
	}
	return truncateTokens(render(c), budget)
}

const (
	maxPrompts = 15
	maxFiles   = 25
)

func render(c Capsule) string {
	var b strings.Builder
	b.WriteString("# aval relay capsule\n\n## Objective\n")
	fmt.Fprintf(&b, "First request: %s\n", c.FirstPrompt)
	if c.LastPrompt != "" && c.LastPrompt != c.FirstPrompt {
		fmt.Fprintf(&b, "Latest request: %s\n", c.LastPrompt)
	}
	if c.Repo != nil {
		fmt.Fprintf(&b, "\n## Repository\nBranch: %s · changed files: %d\n", c.Repo.Branch, c.Repo.Changed)
	}
	list(&b, "Running work", c.Work)
	list(&b, "Recent requests", lastN(c.Prompts, maxPrompts))
	list(&b, "Files edited", c.Files[:min(len(c.Files), maxFiles)])
	if c.Note != "" {
		fmt.Fprintf(&b, "\n## Note\n%s\n", c.Note)
	}
	return b.String()
}

// list writes a section of bullet lines.
func list(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
}

// lastN returns the last n items of an oldest-first list.
func lastN(items []string, n int) []string {
	return items[max(0, len(items)-n):]
}

func redactCapsule(c Capsule) Capsule {
	c.FirstPrompt, c.LastPrompt, c.Note = Redact(c.FirstPrompt), Redact(c.LastPrompt), Redact(c.Note)
	c.Prompts, c.Work, c.Files = redactAll(c.Prompts), redactAll(c.Work), redactAll(c.Files)
	return c
}

func redactAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = Redact(s)
	}
	return out
}

const cut = "\n[…]\n"

// truncateTokens cuts s to at most budget tokens, on a rune boundary.
func truncateTokens(s string, budget int) string {
	limit := int(float64(budget)*charsPerToken) - len(cut)
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit] + cut
}
