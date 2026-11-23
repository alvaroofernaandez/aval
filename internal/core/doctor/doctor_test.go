package doctor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func w(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

const repoSettings = `{
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "\"$HOME/.claude/hooks/ok.sh\""}]}]},
  "enabledPlugins": {"p@m": true, "off@m": false}
}`

const manifestText = `[claude]
label = c
detail = d
platforms = all

config/claude/hooks         .claude/hooks
config/claude/settings.json .claude/settings.json
`

// healthy builds a repo and a HOME where every check passes.
func healthy(t *testing.T) Options {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	// macOS resolves /var to /private/var; keep both sides canonical.
	repo, _ = filepath.EvalSymlinks(repo)
	home, _ = filepath.EvalSymlinks(home)

	w(t, filepath.Join(repo, "install.manifest"), manifestText)
	w(t, filepath.Join(repo, "config/claude/settings.json"), repoSettings)
	w(t, filepath.Join(repo, "config/claude/CLAUDE.md"), "<!-- shared:agent-rules -->\nr\n<!-- /shared:agent-rules -->\n")
	w(t, filepath.Join(repo, "config/claude/hooks/ok.sh"), "#!/bin/sh\n")

	link(t, filepath.Join(repo, "config/claude/hooks"), filepath.Join(home, ".claude/hooks"))
	w(t, filepath.Join(home, ".claude/settings.json"), repoSettings)
	w(t, filepath.Join(home, ".claude/agents/a.md"), "---\nname: a\nmodel: sonnet\n---\nbody\n")
	w(t, filepath.Join(home, ".claude/skills/s/SKILL.md"), "---\nname: s\ndescription: does s\n---\n")
	w(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"), `{"version":2,"plugins":{"p@m":[{"scope":"user"}]}}`)

	return Options{
		Repo: repo, Home: home, GOOS: runtime.GOOS,
		PrivateDir: filepath.Join(home, ".config/aval/private"),
		LookPath: func(name string) (string, error) {
			if name == "git" || name == "rm" {
				return "/bin/" + name, nil
			}
			return "", fmt.Errorf("not found: %s", name)
		},
	}
}

func get(t *testing.T, rs []Result, id string) Result {
	t.Helper()
	for _, r := range rs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no result with id %q in %+v", id, rs)
	return Result{}
}

func expect(t *testing.T, rs []Result, id string, st Status, fixable bool) Result {
	t.Helper()
	r := get(t, rs, id)
	if r.Status != st || r.Fixable != fixable {
		t.Errorf("%s = {%s fixable=%v %q}, want {%s fixable=%v}", id, r.Status, r.Fixable, r.Message, st, fixable)
	}
	return r
}

func TestHealthyMachineIsAllOK(t *testing.T) {
	rs := Run(healthy(t))
	if len(rs) < 9 {
		t.Fatalf("only %d results: %+v", len(rs), rs)
	}
	for _, r := range rs {
		if r.Status != OK {
			t.Errorf("%s = %s: %s", r.ID, r.Status, r.Message)
		}
	}
	if HasFail(rs) {
		t.Error("HasFail on a healthy machine")
	}
}

func TestManifestLinks(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, o Options)
		want   Status
		in     string
	}{
		{"missing destination", func(t *testing.T, o Options) { os.Remove(filepath.Join(o.Home, ".claude/hooks")) }, Fail, ".claude/hooks"},
		{"replaced by a real directory", func(t *testing.T, o Options) {
			os.Remove(filepath.Join(o.Home, ".claude/hooks"))
			w(t, filepath.Join(o.Home, ".claude/hooks/x"), "")
		}, Warn, ".claude/hooks"},
		{"points outside the repo", func(t *testing.T, o Options) {
			os.Remove(filepath.Join(o.Home, ".claude/hooks"))
			link(t, t.TempDir(), filepath.Join(o.Home, ".claude/hooks"))
		}, Warn, ".claude/hooks"},
		{"dangling link", func(t *testing.T, o Options) {
			os.Remove(filepath.Join(o.Home, ".claude/hooks"))
			link(t, filepath.Join(o.Home, "nowhere"), filepath.Join(o.Home, ".claude/hooks"))
		}, Fail, ".claude/hooks"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := healthy(t)
			c.mutate(t, o)
			r := get(t, Run(o), "links")
			if r.Status != c.want || !strings.Contains(r.Message, c.in) {
				t.Errorf("links = %s %q, want %s mentioning %q", r.Status, r.Message, c.want, c.in)
			}
		})
	}
}

func TestSettingsJSONIsNotExpectedToBeALink(t *testing.T) {
	// It is a real file on purpose (sync owns it), so it must not warn.
	r := get(t, Run(healthy(t)), "links")
	if r.Status != OK {
		t.Errorf("links = %s %q", r.Status, r.Message)
	}
}

func TestBrokenLinksAreFoundAndFixed(t *testing.T) {
	o := healthy(t)
	dead := filepath.Join(o.Home, ".claude/skills/dead")
	link(t, filepath.Join(o.Home, "gone"), dead)
	deadBin := filepath.Join(o.Home, ".local/bin/kiri")
	link(t, filepath.Join(o.Home, "gone2"), deadBin)

	r := expect(t, Run(o), "broken-links", Fail, true)
	if !strings.Contains(r.Message, "2") {
		t.Errorf("message should count the links: %q", r.Message)
	}

	removed, err := Fix(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Errorf("removed = %v", removed)
	}
	for _, p := range []string{dead, deadBin} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s still exists after Fix", p)
		}
	}
	expect(t, Run(o), "broken-links", OK, false)
}

func TestExecutables(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{`"$HOME/.claude/hooks/x.sh"`, []string{"/h/.claude/hooks/x.sh"}},
		{`code-review-graph status`, []string{"code-review-graph"}},
		{`if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then code-review-graph update --skip-flows; fi`, []string{"git", "code-review-graph"}},
		{`[ -x "$HOME/.tool/bin/g" ] && "$HOME/.tool/bin/g" || true`, nil},
		{`rm -f /tmp/dk_* /tmp/.dk_* 2>/dev/null`, []string{"rm"}},
		{`gentle-ai telemetry runtime claude --json`, []string{"gentle-ai"}},
		{`~/bin/tool --x && other | third`, []string{"/h/bin/tool", "other", "third"}},
		{`echo "a && b"; true`, nil},
	}
	for _, c := range cases {
		t.Run(c.cmd, func(t *testing.T) {
			if got := executables(c.cmd, "/h"); !reflect.DeepEqual(got, c.want) {
				t.Errorf("executables = %q, want %q", got, c.want)
			}
		})
	}
}

func liveWithCommand(t *testing.T, o Options, cmd string) {
	b, _ := json.Marshal(map[string]any{"hooks": map[string]any{"Stop": []any{
		map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd}}}}},
		"enabledPlugins": map[string]any{"p@m": true, "off@m": false}})
	w(t, filepath.Join(o.Home, ".claude/settings.json"), string(b))
}

func TestHookResolution(t *testing.T) {
	o := healthy(t)
	liveWithCommand(t, o, `"$HOME/.claude/hooks/missing.sh"`)
	r := expect(t, Run(o), "hooks-resolve", Fail, false)
	if !strings.Contains(r.Message, "missing.sh") {
		t.Errorf("message should name the command: %q", r.Message)
	}

	liveWithCommand(t, o, `if git rev-parse x; then nonexistent-tool update; fi`)
	r = expect(t, Run(o), "hooks-resolve", Fail, false)
	if !strings.Contains(r.Message, "nonexistent-tool") {
		t.Errorf("message = %q", r.Message)
	}

	liveWithCommand(t, o, `[ -x "$HOME/.nope" ] && "$HOME/.nope" || true`)
	expect(t, Run(o), "hooks-resolve", OK, false)
}

func TestRetiredHooks(t *testing.T) {
	for _, cmd := range []string{"git sdd-preflight-hook", "git skill-registry refresh"} {
		o := healthy(t)
		liveWithCommand(t, o, cmd)
		expect(t, Run(o), "hooks-retired", Fail, false)
	}
	expect(t, Run(healthy(t)), "hooks-retired", OK, false)
}

func TestHooksDriftFromRepo(t *testing.T) {
	o := healthy(t)
	liveWithCommand(t, o, "git status")
	expect(t, Run(o), "hooks-drift", Fail, true)
	expect(t, Run(healthy(t)), "hooks-drift", OK, false)
}

func TestMissingLiveSettings(t *testing.T) {
	o := healthy(t)
	os.Remove(filepath.Join(o.Home, ".claude/settings.json"))
	rs := Run(o)
	expect(t, rs, "hooks-drift", Fail, true)
	expect(t, rs, "settings-drift", Fail, true)
}

func TestAgentModels(t *testing.T) {
	cases := map[string]Status{
		"opus": OK, "haiku": OK, "inherit": OK, `"sonnet"`: OK,
		"claude-opus-4-6": Fail, "anthropic/claude-sonnet-4-5": Fail,
	}
	for model, want := range cases {
		o := healthy(t)
		w(t, filepath.Join(o.Home, ".claude/agents/b.md"), "---\nname: b\nmodel: "+model+"\n---\n")
		r := get(t, Run(o), "agents")
		if r.Status != want {
			t.Errorf("model %q: %s %q, want %s", model, r.Status, r.Message, want)
		}
		if want == Fail && !strings.Contains(r.Message, "b.md") {
			t.Errorf("message should name the agent: %q", r.Message)
		}
	}
}

func TestSkills(t *testing.T) {
	cases := map[string]string{
		"no SKILL.md":        "",
		"no description":     "---\nname: x\n---\n",
		"no frontmatter":     "just text\n",
		"no name":            "---\ndescription: d\n---\n",
		"folded description": "---\nname: x\ndescription: >\n  long\n---\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			o := healthy(t)
			if body == "" {
				if err := os.MkdirAll(filepath.Join(o.Home, ".claude/skills/x"), 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				w(t, filepath.Join(o.Home, ".claude/skills/x/SKILL.md"), body)
			}
			r := get(t, Run(o), "skills")
			want := Warn
			if name == "folded description" {
				want = OK
			}
			if r.Status != want {
				t.Errorf("skills = %s %q, want %s", r.Status, r.Message, want)
			}
			if want == Warn && !strings.Contains(r.Message, "x") {
				t.Errorf("message should name the skill: %q", r.Message)
			}
		})
	}
}

func TestSkillThroughASymlinkIsFollowed(t *testing.T) {
	o := healthy(t)
	target := filepath.Join(o.Repo, "skills/linked")
	w(t, filepath.Join(target, "SKILL.md"), "---\nname: linked\ndescription: d\n---\n")
	link(t, target, filepath.Join(o.Home, ".claude/skills/linked"))
	expect(t, Run(o), "skills", OK, false)
}

// _shared is a support dir by convention (leading underscore) and synced is
// Claude Code's own state for claude.ai-synced skills: neither is a skill, and a
// doctor that always warns is a doctor nobody reads.
func TestSkillsIgnoresSupportDirs(t *testing.T) {
	o := healthy(t)
	w(t, filepath.Join(o.Home, ".claude/skills/_shared/README.md"), "support")
	w(t, filepath.Join(o.Home, ".claude/skills/synced/state"), "x")
	expect(t, Run(o), "skills", OK, false)
}

func TestPlugins(t *testing.T) {
	o := healthy(t)
	repo := `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "\"$HOME/.claude/hooks/ok.sh\""}]}]},
  "enabledPlugins": {"p@m": true, "ghost@m": true, "off@m": false}}`
	w(t, filepath.Join(o.Home, ".claude/settings.json"), repo)
	r := expect(t, Run(o), "plugins", Warn, false)
	if !strings.Contains(r.Message, "ghost@m") || strings.Contains(r.Message, "off@m") {
		t.Errorf("message = %q", r.Message)
	}
}

func TestSettingsDrift(t *testing.T) {
	o := healthy(t)
	w(t, filepath.Join(o.Home, ".claude/settings.json"), `{"hooks":{},"enabledPlugins":{"p@m":true}}`)
	r := expect(t, Run(o), "settings-drift", Fail, true)
	if !strings.Contains(r.Message, "hooks") {
		t.Errorf("message should list the keys: %q", r.Message)
	}
}

func TestOverlayReporting(t *testing.T) {
	o := healthy(t)
	r := expect(t, Run(o), "overlay", OK, false)
	if !strings.Contains(r.Message, "absent") {
		t.Errorf("message = %q", r.Message)
	}

	w(t, filepath.Join(o.PrivateDir, "settings.json"), `{}`)
	w(t, filepath.Join(o.PrivateDir, "skills/p/SKILL.md"), "x")
	w(t, filepath.Join(o.PrivateDir, "links.manifest"), "skills/p .claude/skills/p\n")
	rs := Run(o)
	r = expect(t, rs, "overlay", OK, false)
	if !strings.Contains(r.Message, "present") {
		t.Errorf("message = %q", r.Message)
	}
	// The overlay link has not been applied yet.
	rl := get(t, rs, "overlay-links")
	if rl.Status != Fail || !strings.Contains(rl.Message, ".claude/skills/p") {
		t.Errorf("overlay-links = %s %q", rl.Status, rl.Message)
	}

	link(t, filepath.Join(o.PrivateDir, "skills/p"), filepath.Join(o.Home, ".claude/skills/p"))
	expect(t, Run(o), "overlay-links", OK, false)
}

func TestFixRepairsEverythingItCan(t *testing.T) {
	o := healthy(t)
	os.Remove(filepath.Join(o.Home, ".claude/settings.json"))
	link(t, filepath.Join(o.Home, "gone"), filepath.Join(o.Home, ".claude/skills/dead"))

	if _, err := Fix(o); err != nil {
		t.Fatal(err)
	}
	rs := Run(o)
	expect(t, rs, "broken-links", OK, false)
	expect(t, rs, "settings-drift", OK, false)
	expect(t, rs, "hooks-drift", OK, false)
}

func TestRender(t *testing.T) {
	rs := []Result{
		{ID: "links", Status: OK, Message: "12 links ok"},
		{ID: "broken-links", Status: Fail, Message: "2 broken", Fixable: true},
		{ID: "skills", Status: Warn, Message: "1 without SKILL.md"},
	}
	var b bytes.Buffer
	Render(&b, rs)
	out := b.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "STATUS") || !strings.Contains(lines[0], "CHECK") || !strings.Contains(lines[0], "MESSAGE") {
		t.Errorf("header = %q", lines[0])
	}
	col := strings.Index(lines[0], "CHECK")
	for i, id := range []string{"links", "broken-links", "skills"} {
		if strings.Index(lines[i+1], id) != col {
			t.Errorf("row %d not aligned: %q", i, lines[i+1])
		}
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, "1 ok") || !strings.Contains(last, "1 warn") || !strings.Contains(last, "1 fail") {
		t.Errorf("summary = %q", last)
	}
	if !HasFail(rs) {
		t.Error("HasFail = false with a failing result")
	}
	if HasFail(rs[:1]) || HasFail(rs[2:]) {
		t.Error("HasFail true without a fail")
	}
}

func TestRenderJSON(t *testing.T) {
	var b bytes.Buffer
	if err := RenderJSON(&b, []Result{{ID: "x", Status: Warn, Message: "m", Fixable: true}}); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, b.String())
	}
	if len(got) != 1 || got[0]["id"] != "x" || got[0]["status"] != "warn" || got[0]["fixable"] != true {
		t.Errorf("got %v", got)
	}
	b.Reset()
	_ = RenderJSON(&b, nil)
	if strings.TrimSpace(b.String()) != "[]" {
		t.Errorf("empty = %q, want []", b.String())
	}
}

// TestRenderPlainIsByteStable pins the exact bytes scripts parse today.
func TestRenderPlainIsByteStable(t *testing.T) {
	rs := []Result{
		{ID: "links", Status: OK, Message: "12 links ok"},
		{ID: "broken-links", Status: Fail, Message: "2 broken", Fixable: true},
		{ID: "skills", Status: Warn, Message: "1 without SKILL.md", Fixable: true},
	}
	want := "STATUS  CHECK         MESSAGE\n" +
		"OK      links         12 links ok\n" +
		"FAIL    broken-links  2 broken (fixable: aval doctor --fix)\n" +
		"WARN    skills        1 without SKILL.md (fixable: aval doctor --fix)\n" +
		"\n1 ok, 1 warn, 1 fail\n"
	var b bytes.Buffer
	Render(&b, rs)
	if b.String() != want {
		t.Errorf("Render changed.\n got: %q\nwant: %q", b.String(), want)
	}
	b.Reset()
	RenderStyled(&b, rs, nil)
	if b.String() != want {
		t.Errorf("RenderStyled(nil) differs from Render.\n got: %q", b.String())
	}
}

type fakeStyler struct{}

func (fakeStyler) OK(w string) string   { return "V " + w }
func (fakeStyler) Warn(w string) string { return "! " + w }
func (fakeStyler) Fail(w string) string { return "X " + w }
func (fakeStyler) Width(s string) int   { return len(s) }

func TestRenderStyledAddsGlyphAndKeepsWord(t *testing.T) {
	rs := []Result{
		{ID: "links", Status: OK, Message: "fine"},
		{ID: "broken-links", Status: Fail, Message: "bad"},
		{ID: "skills", Status: Warn, Message: "hm"},
	}
	want := "STATUS  CHECK         MESSAGE\n" +
		"V OK    links         fine\n" +
		"X FAIL  broken-links  bad\n" +
		"! WARN  skills        hm\n" +
		"\n1 ok, 1 warn, 1 fail\n"
	var b bytes.Buffer
	RenderStyled(&b, rs, fakeStyler{})
	if b.String() != want {
		t.Errorf("got: %q\nwant: %q", b.String(), want)
	}
}
