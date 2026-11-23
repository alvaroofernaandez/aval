package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The cost of Claude Code hooks. A hook runs on every event it matches, so its
// cost is paid over and over: measured 2026-11-21, `gentle-ai sdd-preflight-hook`
// took 657 ms on every subagent launch and `code-review-graph update` 154 ms on
// every edit. SDD has been dropped from this harness, so its hooks must not
// come back, and work that only needs to happen eventually must not block the
// agent.

func TestAgentHooksSettingsDeclareNoSDDAndSyncGraphAsync(t *testing.T) {
	s := repoSettings(t) // fails when settings.json is not valid JSON
	_, commands := s.allHooks()
	var sdd, registry []string
	for _, c := range commands {
		if strings.Contains(strings.ToLower(c), "sdd") {
			sdd = append(sdd, c)
		}
		if strings.Contains(c, "skill-registry") {
			registry = append(registry, c)
		}
	}
	eq(t, "hooks that run SDD", "", strings.Join(sdd, " | "))
	eq(t, "hooks that refresh the SDD skill registry", "", strings.Join(registry, " | "))

	// Re-indexing the code graph is bookkeeping: it must not hold up an edit.
	found := false
	for _, g := range s.Hooks["PostToolUse"] {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "code-review-graph update") {
				found = true
				isTrue(t, "code-review-graph update runs async", h.Async)
			}
		}
	}
	isTrue(t, "a code-review-graph update hook exists", found)
}

// hookProgram is the program a settings hook command runs: aval for the
// resolver form, the body of an `if ...; then <program>` guard, else the first
// word.
func hookProgram(cmd string) string {
	if strings.HasPrefix(cmd, `a="$HOME/.local/bin/aval"`) {
		return "aval"
	}
	if _, body, ok := strings.Cut(cmd, "; then "); ok {
		cmd = body
	}
	return strings.Fields(cmd)[0]
}

// Every hook runs aval or one of two tools the harness depends on on purpose.
func TestAgentHooksRunOnlyKnownPrograms(t *testing.T) {
	allowed := map[string]bool{"aval": true, "code-review-graph": true, "rm": true}
	events, commands := repoSettings(t).allHooks()
	for i, c := range commands {
		if p := hookProgram(c); !allowed[p] {
			t.Errorf("%s hook runs %q: %s", events[i], p, c)
		}
	}
}

// The fmt hook formats the file an agent just edited, so the agent's next read
// sees the formatted file: it must run synchronously, after Edit and Write.
func TestAgentHooksFormatAfterEveryEdit(t *testing.T) {
	s := repoSettings(t)
	found := false
	for _, g := range s.Hooks["PostToolUse"] {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "hook fmt") {
				found = true
				eq(t, "the fmt hook matcher", "Edit|Write", g.Matcher)
				isTrue(t, "the fmt hook runs synchronously", !h.Async)
			}
		}
	}
	isTrue(t, "a PostToolUse hook runs aval hook fmt", found)
}

// 21 commits in 60 days carried Co-Authored-By despite the rule: Claude Code
// itself injects the trailer unless the setting hides it. A rule the harness
// contradicts on every commit is a coin flip; the setting is not.
func TestAgentHooksAttributionIsHidden(t *testing.T) {
	a := repoSettings(t).Attribution
	isTrue(t, "aval hides commit attribution (set to empty)", a.Commit != nil && *a.Commit == "")
	isTrue(t, "aval hides PR attribution (set to empty)", a.PR != nil && *a.PR == "")
}

// Terax is not installed; its three hooks still forked a shell on every
// prompt, notification and stop just to test an empty variable.
func TestAgentHooksNoHookServesTheUninstalledTerax(t *testing.T) {
	_, commands := repoSettings(t).allHooks()
	n := 0
	for _, c := range commands {
		if strings.Contains(c, "TERAX") {
			n++
		}
	}
	eq(t, "hooks serving the uninstalled Terax terminal", 0, n)
}

// Agents pinned retired IDs (claude-opus-4-6) or OpenCode's provider/model form
// (anthropic/claude-sonnet-4-5), which Claude Code does not resolve. Aliases
// always track the current model.
func TestAgentHooksEveryClaudeAgentUsesAModelAlias(t *testing.T) {
	files, _ := filepath.Glob(p("config", "claude", "agents", "*"))
	ok := regexp.MustCompile(`model: *(opus|sonnet|haiku|inherit) *$`)
	var bad []string
	for _, f := range files {
		if st, err := os.Stat(f); err != nil || st.IsDir() {
			continue
		}
		for _, l := range strings.Split(readAbs(t, f), "\n") {
			if strings.HasPrefix(l, "model:") && !ok.MatchString(l) {
				bad = append(bad, filepath.Base(f))
			}
		}
	}
	sort.Strings(bad)
	eq(t, "agents with a non-alias model", "", strings.Join(bad, " "))
}

// The live settings file is a separate copy, not a symlink: gentle-ai rewrites
// it, and it holds personal keys that cannot be published. Hooks must never
// drift: on 2026-11-21 the live copy still called a retired
// `gentle-ai sdd-preflight-hook` and every Agent call failed. So aval owns the
// hooks and the live copy must match them. It inspects the real
// ~/.claude/settings.json of the machine running the suite, so it is opt-in
// (AVAL_LIVE_CHECKS=1; AVAL_LIVE_SETTINGS overrides the path).
func TestAgentHooksLiveSettingsRunExactlyAvalsHooks(t *testing.T) {
	if os.Getenv("AVAL_LIVE_CHECKS") != "1" {
		t.Skip("reads the real $HOME; set AVAL_LIVE_CHECKS=1 to run")
	}
	live := os.Getenv("AVAL_LIVE_SETTINGS")
	if live == "" {
		home, _ := os.UserHomeDir()
		live = filepath.Join(home, ".claude", "settings.json")
	}
	if !exists(live) {
		t.Skipf("%s absent", live)
	}
	pairs := func(s settingsDoc) []string {
		evs, cmds := s.allHooks()
		var out []string
		for i := range evs {
			out = append(out, evs[i]+" "+cmds[i])
		}
		sort.Strings(out)
		return out
	}
	l := loadSettings(t, live)
	eq(t, "the live settings run exactly aval's hooks",
		strings.Join(pairs(repoSettings(t)), "\n"), strings.Join(pairs(l), "\n"))
	_, cmds := l.allHooks()
	for _, c := range cmds {
		mustNotHave(t, `sdd-|skill-registry`, c, "a live hook calls a retired SDD command")
	}
	isTrue(t, "the live settings hide commit and PR attribution",
		l.Attribution.Commit != nil && *l.Attribution.Commit == "" && l.Attribution.PR != nil && *l.Attribution.PR == "")
}
