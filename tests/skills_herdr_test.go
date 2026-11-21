package tests

import (
	"encoding/json"
	"strings"
	"testing"
)

// The Herdr workspace setup. Herdr (herdr-app window + herdr-core `hc`)
// replaced Orca on 2026-11-19. Claude Code reports its status to Herdr natively
// (OSC 7501), so no agent hooks may come back: Orca's 16 hooks ran a shell on
// every single tool call. The skill is a stub that loads the version-matched
// manuals (`hc --skill`, `herdr-app --skill`) so it never drifts from the
// installed release.

func TestHerdrSkill(t *testing.T) {
	s := skill(t, "herdr")
	for _, c := range []struct{ what, re string }{
		{"frontmatter name matches dir", `^name: herdr$`},
		{"description is one quoted line", `^description: ".+"$`},
		{"loads the version-matched hc manual", `hc --skill`},
		{"loads the version-matched window manual", `herdr-app --skill`},
		{"agents always start with --bg", `hc run --bg`},
		{"waits with hc wait, never sleep", `hc wait`},
		{"never stops or restarts the user's core", `(?i)never .*hc server (stop|restart)`},
		{"forbids broad process kills", `(?i)never .*pkill`},
		{"states status needs no hooks", `(?i)no (agent )?hooks`},
		{"Codex runs with --no-daemon", `no-daemon`},
		{"wraps one-off commands in an interactive zsh for .zshrc PATH", `zsh -ic`},
	} {
		mustHave(t, c.re, s, "skill: "+c.what)
	}
	mustNotHave(t, `^license:`, s, "skill: own skills carry no license field")
	mustNotHave(t, `(?i)orca`, s, "skill: drives Orca")
}

func TestHerdrTemplates(t *testing.T) {
	for _, name := range []string{"reviewer", "implementer"} {
		f := read(t, "config/herdr-core/templates/"+name+".md")
		mustHave(t, `^name: `+name+`$`, f, "template "+name+": frontmatter name")
		mustHave(t, `^description: .+`, f, "template "+name+": has description")
		mustHave(t, `^command: .+`, f, "template "+name+": has command")
		mustHave(t, `hc send`, f, "template "+name+": reports back through hc send")
	}
	mustHave(t, `(?i)read-only`, read(t, "config/herdr-core/templates/reviewer.md"), "template reviewer: is read-only")
	mustHave(t, `(?m)^config/herdr-core/templates +\.config/herdr-core/templates$`, read(t, "install.manifest"), "manifest links hc templates")
}

func TestHerdrNoOrcaLeftBehind(t *testing.T) {
	mustNotHave(t, `(?i)orca`, read(t, "config/claude/settings.json"), "claude settings carry Orca hooks")
	isTrue(t, "old Orca snapshot is gone", !exists(p("config/orca")))
}

// The rule is path/command-scoped, so it lives in a lesson (injected when `hc`
// or `herdr-app` runs) and the shared block only points at the lessons dir.
func TestHerdrLessonAndSharedBlockPointer(t *testing.T) {
	lesson := read(t, "config/claude/lessons/herdr-workspace.md")
	mustContain(t, lesson, "`herdr`", "the herdr lesson points to the herdr skill")
	isTrue(t, "the herdr lesson triggers on hc and herdr-app", has(`commands:`, lesson) && has(`hc\|herdr-app`, lesson))
	blk := sharedBlock(read(t, "config/claude/CLAUDE.md"))
	isTrue(t, "CLAUDE.md: shared block points to the lessons", has(`Herdr`, blk) && has(`lessons`, blk))
	mustHave(t, `Herdr`, sharedBlock(read(t, "config/opencode/AGENTS.md")), "AGENTS.md mirrors the pointer")
}

// herdr-app keys are versioned: git and PR views one key away from any agent.
func TestHerdrAppKeys(t *testing.T) {
	var keys struct {
		Global map[string]json.RawMessage `json:"global"`
	}
	if err := json.Unmarshal([]byte(read(t, "config/herdr-app/keys.json")), &keys); err != nil {
		t.Fatalf("herdr-app keys.json is versioned and valid JSON: %v", err)
	}
	// A binding is either a bare string or an object carrying args.argv.
	argvOf := func(key string) []string {
		var b struct {
			Args struct {
				Argv []string `json:"argv"`
			} `json:"args"`
		}
		_ = json.Unmarshal(keys.Global[key], &b)
		return b.Args.Argv
	}
	argv := argvOf("cmd-shift-g")
	isTrue(t, "cmd-shift-g opens lazygit in a new tab", len(argv) > 0 && argv[0] == "lazygit")
	eq(t, "cmd-shift-o opens the branch PR", "gh pr view --web", strings.Join(argvOf("cmd-shift-o"), " "))
	mustHave(t, `(?m)^config/herdr-app/keys\.json\s+\.config/herdr-app/keys\.json$`, read(t, "install.manifest"), "the manifest links it")
}
