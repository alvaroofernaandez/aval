package relay

import (
	"fmt"
	"regexp"
	"slices"
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
	Plans       []PlanFacts // open plan tasks of the plans in play
	Tree        *TreeFacts  // working tree facts; nil when unknown
	Work        []string    // running work, one line each
	Note        string      // optional next-step note
}

// RepoFacts are read by an adapter; a nil value means no repository.
type RepoFacts struct {
	Branch  string `json:"branch"`
	Changed int    `json:"changed"` // files with uncommitted changes; 0 when unknown
}

// CapsuleHeader starts every capsule; adapters use it to recognise a capsule
// from an earlier relay among the messages.
const CapsuleHeader = "# aval relay capsule"

// CapsuleFirstRequest is the label of the original first request in a capsule.
const CapsuleFirstRequest = "First request: "

// CapsuleFirst reports whether text is a capsule and returns the original
// first request it carries (empty when it carries none). Adapters use it so a
// capsule from an earlier relay is never taken as a request.
func CapsuleFirst(text string) (string, bool) {
	if !isCapsule(text) {
		return "", false
	}
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(line, CapsuleFirstRequest); ok {
			return v, true
		}
	}
	return "", true
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
		func(c *Capsule) { c.Tree = nil },
		func(c *Capsule) { c.Files = nil },
		func(c *Capsule) { c.Plans = nil },
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
	// maxPromptRunes caps one request in the capsule: a long paste says little
	// about what to do next and would eat the budget.
	maxPromptRunes = 600
)

// capPrompt cuts a request to maxPromptRunes and says how much it dropped.
func capPrompt(s string) string {
	if utf8.RuneCountInString(s) <= maxPromptRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxPromptRunes]) + fmt.Sprintf("… [truncated %d chars]", len(r)-maxPromptRunes)
}

func capPrompts(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = capPrompt(s)
	}
	return out
}

func render(c Capsule) string {
	var b strings.Builder
	b.WriteString(CapsuleHeader + "\n\n## Objective\n")
	fmt.Fprintf(&b, "%s%s\n", CapsuleFirstRequest, c.FirstPrompt)
	if c.LastPrompt != "" && c.LastPrompt != c.FirstPrompt {
		fmt.Fprintf(&b, "Latest request: %s\n", c.LastPrompt)
	}
	if c.Repo != nil {
		fmt.Fprintf(&b, "\n## Repository\nBranch: %s", c.Repo.Branch)
		if c.Repo.Changed > 0 {
			fmt.Fprintf(&b, " · changed files: %d", c.Repo.Changed)
		}
		b.WriteString("\n")
	}
	renderPlans(&b, c.Plans)
	renderTree(&b, c.Tree)
	list(&b, "Running work", c.Work)
	list(&b, "Recent requests", lastN(c.Prompts, maxPrompts))
	list(&b, "Files edited", c.Files[:min(len(c.Files), maxFiles)])
	if c.Note != "" {
		fmt.Fprintf(&b, "\n## Note\n%s\n", c.Note)
	}
	return b.String()
}

func renderPlans(b *strings.Builder, plans []PlanFacts) {
	var out strings.Builder
	for _, p := range plans {
		if len(p.Items) == 0 {
			continue
		}
		if p.Title != "" {
			out.WriteString(p.Title + "\n")
		}
		for _, it := range p.Items[:min(len(p.Items), maxPlanItems)] {
			fmt.Fprintf(&out, "- [ ] %s\n", it)
		}
	}
	if out.Len() > 0 {
		b.WriteString("\n## Open plan tasks\n" + out.String())
	}
}

func renderTree(b *strings.Builder, t *TreeFacts) {
	if t == nil || len(t.Dirty)+len(t.Commits) == 0 {
		return
	}
	b.WriteString("\n## Working tree\n")
	for _, g := range []struct {
		label string
		items []string
	}{{"Uncommitted:", head(t.Dirty, maxDirty)}, {"Commits this session:", head(t.Commits, maxCommits)}} {
		if len(g.items) == 0 {
			continue
		}
		b.WriteString(g.label + "\n")
		for _, it := range g.items {
			fmt.Fprintf(b, "- %s\n", strings.TrimSpace(it))
		}
	}
}

func head(items []string, n int) []string { return items[:min(len(items), n)] }

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
	c.FirstPrompt, c.LastPrompt, c.Prompts = capPrompt(c.FirstPrompt), capPrompt(c.LastPrompt), capPrompts(c.Prompts)
	c.Prompts = dedupeLast(c.Prompts, c.FirstPrompt, c.LastPrompt)
	c.Files = dedupe(c.Files)
	plans := make([]PlanFacts, len(c.Plans))
	for i, p := range c.Plans {
		plans[i] = PlanFacts{Title: Redact(p.Title), Items: capPrompts(redactAll(p.Items))}
	}
	c.Plans = plans
	if c.Tree != nil {
		c.Tree = &TreeFacts{Dirty: redactAll(c.Tree.Dirty), Commits: redactAll(c.Tree.Commits)}
	}
	return c
}

// dedupe drops repeats, keeping the first occurrence of each item and the
// order of the rest, and drops every item equal to one of skip.
func dedupe(in []string, skip ...string) []string {
	seen := map[string]bool{}
	for _, s := range skip {
		seen[s] = true
	}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// dedupeLast is dedupe for an oldest-first list: a repeat keeps its newest
// place.
func dedupeLast(in []string, skip ...string) []string {
	rev := slices.Clone(in)
	slices.Reverse(rev)
	out := dedupe(rev, skip...)
	slices.Reverse(out)
	return out
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

// isCapsule: the header starts the text, or its second line when the runtime
// wraps a plugin-submitted prompt in one line of its own (Claude Code writes
// "The aval-relay plugin sent a message:"; T11 sandbox run).
func isCapsule(text string) bool {
	if strings.HasPrefix(text, CapsuleHeader) {
		return true
	}
	_, rest, ok := strings.Cut(text, "\n")
	return ok && strings.HasPrefix(rest, CapsuleHeader)
}

// notRequests are the prefixes of user messages a runtime writes itself:
// Claude Code's summary after its own compaction and its slash-command
// markers (recorded on 2.1.331, T11 sandbox run).
var notRequests = []string{
	"This session is being continued from a previous conversation",
	"<command-",
	"<local-command-",
}

// attachmentOnly matches a message that is only placeholders of attachments
// ("[Image #11]", "[Pasted text #2 +40 lines]") and whitespace.
var attachmentOnly = regexp.MustCompile(`^\s*(?:\[(?:Image|Pasted text) #\d+[^\]]*\]\s*)+$`)

// IsRequest reports whether a user message is a request of the person: not
// empty, not a capsule and not a message the runtime wrote itself.
func IsRequest(text string) bool {
	if text == "" || isCapsule(text) || attachmentOnly.MatchString(text) {
		return false
	}
	for _, p := range notRequests {
		if strings.HasPrefix(text, p) {
			return false
		}
	}
	return true
}

// PlanFacts is one plan file: its title and its next unchecked items.
type PlanFacts struct {
	Title string
	Items []string
}

// TreeFacts is the working tree as git sees it, read by an adapter.
type TreeFacts struct {
	Dirty   []string // `git status --porcelain` lines
	Commits []string // commits made during the session, one line each, newest first
}

const (
	maxPlanItems = 8
	maxDirty     = 20
	maxCommits   = 10
)

var openItem = regexp.MustCompile(`^\s*- \[ \] (.+)$`)

// ParsePlan reads a plan file: its first heading and the first line of its
// next unchecked items (at most maxPlanItems).
func ParsePlan(md string) PlanFacts {
	var p PlanFacts
	for _, line := range strings.Split(md, "\n") {
		if t, ok := strings.CutPrefix(line, "# "); ok && p.Title == "" {
			p.Title = strings.TrimSpace(t)
		} else if m := openItem.FindStringSubmatch(line); m != nil && len(p.Items) < maxPlanItems {
			p.Items = append(p.Items, strings.TrimSpace(m[1]))
		}
	}
	return p
}
