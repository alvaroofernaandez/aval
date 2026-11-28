// Package budget measures what the harness costs before the user types
// anything: always-on context per agent, and the latency of every hook.
//
// Context is counted in characters and converted with chars/4, which is an
// approximation: it is good enough to compare before/after and to catch growth,
// not to bill against. Hook latency is measured by actually running each hook
// command with a representative payload.
package budget

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Item is one line of an agent's always-on context.
type Item struct {
	Label  string `json:"label"`
	Count  int    `json:"count,omitempty"`
	Chars  int    `json:"chars"`
	Tokens int    `json:"approx_tokens"`
}

// Agent is the always-on context of one agent.
type Agent struct {
	Name   string `json:"name"`
	Items  []Item `json:"items"`
	Chars  int    `json:"chars"`
	Tokens int    `json:"approx_tokens"`
}

// Hook is the measured latency of one hook command.
type Hook struct {
	Event    string  `json:"event"`
	Matcher  string  `json:"matcher,omitempty"`
	Command  string  `json:"command"`
	Async    bool    `json:"async"`
	Budgeted bool    `json:"budgeted"`
	BudgetMs float64 `json:"budget_ms,omitempty"`
	Runs     int     `json:"runs"`
	P50Ms    float64 `json:"p50_ms"`
	P95Ms    float64 `json:"p95_ms"`
	TimedOut bool    `json:"timed_out"`
	Over     bool    `json:"over"`
}

// Report is the result of Run.
type Report struct {
	Agents []Agent `json:"agents"`
	Hooks  []Hook  `json:"hooks"`
	Over   bool    `json:"over_budget"`
	Note   string  `json:"note"`
}

// Options configures Run. Everything is a parameter so tests use fixtures.
type Options struct {
	Home string
	// Runs is how many times each hook is executed (default 5).
	Runs int
	// Timeout caps one execution (default 2s); hitting it counts as over.
	Timeout time.Duration
	// HookBudget is the p95 ceiling for synchronous PreToolUse/PostToolUse
	// hooks (default 20 ms).
	HookBudget time.Duration
	// SkipHooks measures context only: hooks are real commands with side
	// effects, so this is the safe mode.
	SkipHooks bool
}

const note = "tokens are approximate (chars/4); skills from plugins and project files are not counted"

// Run measures everything. It reads Home and, unless SkipHooks, executes the
// live hook commands.
func Run(opt Options) (*Report, error) {
	if opt.Runs <= 0 {
		opt.Runs = 5
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 2 * time.Second
	}
	if opt.HookBudget <= 0 {
		opt.HookBudget = 20 * time.Millisecond
	}

	settings := readSettings(opt.Home)
	r := &Report{
		Agents: []Agent{claudeContext(opt.Home, settings), opencodeContext(opt.Home)},
		Note:   note,
	}
	if !opt.SkipHooks {
		for _, h := range hookSpecs(settings) {
			m := measure(h, opt)
			r.Over = r.Over || m.Over
			r.Hooks = append(r.Hooks, m)
		}
	}
	return r, nil
}

// --- context ---------------------------------------------------------------

func tokens(chars int) int { return (chars + 3) / 4 }

func total(name string, items []Item) Agent {
	a := Agent{Name: name, Items: items}
	for i := range a.Items {
		a.Items[i].Tokens = tokens(a.Items[i].Chars)
		a.Chars += a.Items[i].Chars
	}
	a.Tokens = tokens(a.Chars)
	return a
}

type settingsFile struct {
	SkillOverrides map[string]string `json:"skillOverrides"`
	Hooks          map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
			Async   bool     `json:"async"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func readSettings(home string) settingsFile {
	var s settingsFile
	if b, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func claudeContext(home string, s settingsFile) Agent {
	root := filepath.Join(home, ".claude")
	md := filepath.Join(root, "CLAUDE.md")
	var items []Item

	items = append(items, Item{Label: "CLAUDE.md", Chars: fileChars(md)})
	imp, n := importChars(md, map[string]bool{md: true}, 0)
	items = append(items, Item{Label: "@imports", Count: n, Chars: imp})

	sk, n := skillChars(filepath.Join(root, "skills"), s.SkillOverrides)
	items = append(items, Item{Label: "skills", Count: n, Chars: sk})

	ag, n := descChars(filepath.Join(root, "agents"))
	items = append(items, Item{Label: "agents", Count: n, Chars: ag})

	cm, n := descChars(filepath.Join(root, "commands"))
	items = append(items, Item{Label: "commands", Count: n, Chars: cm})
	return total("claude", items)
}

func opencodeContext(home string) Agent {
	root := filepath.Join(home, ".config", "opencode")
	sk, n := skillChars(filepath.Join(root, "skills"), nil)
	return total("opencode", []Item{
		{Label: "AGENTS.md", Chars: fileChars(filepath.Join(root, "AGENTS.md"))},
		{Label: "skills", Count: n, Chars: sk},
	})
}

func fileChars(p string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	return len([]rune(string(b)))
}

var importLine = regexp.MustCompile(`^@(\S+)\s*$`)

// importChars sums the files a CLAUDE.md pulls in with `@path` lines, which
// Claude Code expands in full on every turn. Imports of imports are followed.
func importChars(file string, seen map[string]bool, depth int) (chars, count int) {
	if depth > 4 {
		return 0, 0
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return 0, 0
	}
	for _, ln := range strings.Split(string(b), "\n") {
		m := importLine.FindStringSubmatch(strings.TrimSpace(ln))
		if m == nil {
			continue
		}
		p := m[1]
		if strings.HasPrefix(p, "~/") {
			home, _ := os.UserHomeDir()
			p = filepath.Join(home, p[2:])
		} else if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(file), p)
		}
		if seen[p] {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		seen[p] = true
		c := fileChars(p)
		cc, nn := importChars(p, seen, depth+1)
		chars += c + cc
		count += 1 + nn
	}
	return chars, count
}

// skillChars sums name+description of every visible skill: that is the part
// of a skill the model always sees. A skill switched off in skillOverrides is
// not visible and costs nothing.
func skillChars(dir string, overrides map[string]string) (chars, count int) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range ents {
		fm, ok := frontmatter(filepath.Join(dir, e.Name(), "SKILL.md"))
		if !ok {
			continue
		}
		name := fm["name"]
		if name == "" {
			name = e.Name()
		}
		if overrides[name] == "off" || overrides[e.Name()] == "off" {
			continue
		}
		chars += len([]rune(name)) + len([]rune(fm["description"]))
		count++
	}
	return chars, count
}

// descChars sums name+description of the .md files in dir (agents, commands).
func descChars(dir string) (chars, count int) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		fm, ok := frontmatter(filepath.Join(dir, e.Name()))
		if !ok {
			continue
		}
		name := fm["name"]
		if name == "" {
			name = strings.TrimSuffix(e.Name(), ".md")
		}
		chars += len([]rune(name)) + len([]rune(fm["description"]))
		count++
	}
	return chars, count
}

// frontmatter reads the flat key: value pairs of a markdown file's YAML
// header. A folded or literal block (`>`, `|`) is joined with spaces.
func frontmatter(path string) (map[string]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return nil, false
	}
	out := map[string]string{}
	key := ""
	for sc.Scan() {
		ln := sc.Text()
		if strings.TrimSpace(ln) == "---" {
			return out, true
		}
		if key != "" && (strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t")) {
			out[key] = strings.TrimSpace(out[key] + " " + strings.TrimSpace(ln))
			continue
		}
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			key = ""
			continue
		}
		key = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if v == ">" || v == "|" || v == ">-" || v == "|-" {
			v = ""
		}
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out[key] = v
	}
	return nil, false
}

// --- hooks -----------------------------------------------------------------

type hookSpec struct {
	event, matcher, command string
	// args is set for the exec form (command + args, no shell), which is how
	// `aval sync` writes hooks on Windows.
	args  []string
	async bool
}

func hookSpecs(s settingsFile) []hookSpec {
	events := make([]string, 0, len(s.Hooks))
	for e := range s.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)
	var out []hookSpec
	for _, e := range events {
		for _, g := range s.Hooks[e] {
			for _, h := range g.Hooks {
				if h.Type == "command" && h.Command != "" {
					out = append(out, hookSpec{e, g.Matcher, h.Command, h.Args, h.Async})
				}
			}
		}
	}
	return out
}

var simpleTool = regexp.MustCompile(`^[A-Za-z]+$`)

// payload is a representative stdin for the event: the tool the matcher is
// about, or an Edit for PreToolUse/PostToolUse hooks without one.
func payload(h hookSpec, home string) []byte {
	p := map[string]any{
		"session_id":      "aval-budget",
		"hook_event_name": h.event,
		"cwd":             home,
		"transcript_path": filepath.Join(home, "transcript.jsonl"),
	}
	if h.event == "PreToolUse" || h.event == "PostToolUse" {
		tool := "Edit"
		for _, alt := range strings.Split(h.matcher, "|") {
			if simpleTool.MatchString(alt) {
				tool = alt
				break
			}
		}
		p["tool_name"] = tool
		if tool == "Bash" {
			p["tool_input"] = map[string]any{"command": "ls"}
		} else {
			p["tool_input"] = map[string]any{"file_path": filepath.Join(home, "aval-budget.txt")}
		}
	}
	b, _ := json.Marshal(p)
	return b
}

// guardBudget is the ceiling of `aval hook guard`: it runs before every Bash
// call, so it gets half of the generic budget.
const guardBudget = 10 * time.Millisecond

var guardCommand = regexp.MustCompile(`\bhook guard\b`)

// budgetFor is the p95 ceiling of one hook: the generic one, or the guard's
// when that is stricter.
func budgetFor(command string, args []string, global time.Duration) time.Duration {
	if guardBudget < global && guardCommand.MatchString(command+" "+strings.Join(args, " ")) {
		return guardBudget
	}
	return global
}

func measure(h hookSpec, opt Options) Hook {
	m := Hook{Event: h.event, Matcher: h.matcher, Command: h.command, Async: h.async, Runs: opt.Runs}
	m.Budgeted = (h.event == "PreToolUse" || h.event == "PostToolUse") && !h.async
	if m.Budgeted {
		m.BudgetMs = float64(budgetFor(h.command, h.args, opt.HookBudget)) / float64(time.Millisecond)
	}
	in := payload(h, opt.Home)

	// One discarded run first, like hyperfine --warmup: the first exec of a
	// script pays page-cache and code-signing costs (140 ms measured on macOS
	// for a fresh file) that a steady-state hook never does. Capped by Timeout.
	runOnce(h, in, opt)

	var samples []float64
	timedOut := false
	for i := 0; i < opt.Runs; i++ {
		d, t := runOnce(h, in, opt)
		timedOut = timedOut || t
		samples = append(samples, float64(d)/float64(time.Millisecond))
	}
	return classify(m, samples, timedOut)
}

func runOnce(h hookSpec, stdin []byte, opt Options) (time.Duration, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), opt.Timeout)
	defer cancel()
	name, args := hookArgv(h)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(string(stdin))
	cmd.Dir = os.TempDir()
	// USERPROFILE is what os.UserHomeDir (and so aval itself) reads on Windows.
	cmd.Env = append(os.Environ(), "HOME="+opt.Home, "USERPROFILE="+opt.Home)
	// No pipes for output: a grandchild holding one would make Wait outlive
	// the timeout.
	cmd.WaitDelay = 50 * time.Millisecond
	start := time.Now()
	_ = cmd.Run()
	d := time.Since(start)
	if ctx.Err() != nil {
		return opt.Timeout, true
	}
	return d, false
}

// --- output ----------------------------------------------------------------

// Render prints the report as tables.
func Render(w io.Writer, r *Report) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tITEM\tCOUNT\tCHARS\t≈TOKENS")
	for _, a := range r.Agents {
		for _, it := range a.Items {
			count := ""
			if it.Count > 0 {
				count = fmt.Sprint(it.Count)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\n", a.Name, it.Label, count, it.Chars, it.Tokens)
		}
		fmt.Fprintf(tw, "%s\ttotal\t\t%d\t%d\n", a.Name, a.Chars, a.Tokens)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%s\n", r.Note)

	if len(r.Hooks) == 0 {
		return
	}
	fmt.Fprintln(w)
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "HOOK\tMATCHER\tP50 ms\tP95 ms\tBUDGET\tSTATUS\tCOMMAND")
	for _, h := range r.Hooks {
		budget, status := "async", "reported"
		switch {
		case h.Budgeted:
			budget = fmt.Sprintf("%g ms", h.BudgetMs)
			status = "ok"
			if h.Over {
				status = "OVER"
			}
			if h.TimedOut {
				status = "OVER (timeout)"
			}
		case !h.Async:
			budget, status = "-", "reported"
		}
		fmt.Fprintf(tw, "%s\t%s\t%.1f\t%.1f\t%s\t%s\t%s\n", h.Event, h.Matcher, h.P50Ms, h.P95Ms, budget, status, clip(h.Command, 60))
	}
	tw.Flush()
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

// RenderJSON prints the report as indented JSON.
func RenderJSON(w io.Writer, r *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// hookArgv is how a hook is launched, the way Claude Code launches it: the
// exec form (command + args, what `aval sync` writes on Windows) runs the
// program directly; the shell form goes through `sh -c` on every OS (Git for
// Windows ships sh; without it on PATH a shell-form hook cannot be measured).
func hookArgv(h hookSpec) (string, []string) {
	if h.args != nil {
		return h.command, h.args
	}
	return "sh", []string{"-c", h.command}
}
