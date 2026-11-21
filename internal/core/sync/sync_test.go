package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

var fixedNow = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, s)
	}
	return m
}

func TestMergeSettings(t *testing.T) {
	cases := []struct {
		name        string
		repo, live  string
		overlay     string
		want        string
		wantChanged []string
	}{
		{
			name:        "managed keys are replaced",
			repo:        `{"hooks":{"Stop":[1]},"attribution":{"commit":""},"worktree":{"a":1},"spinnerTipsEnabled":false,"subagentPromptCacheTtl":"1h","statusLine":{"type":"command","command":"aval statusline"}}`,
			live:        `{"hooks":{"Old":[2]},"attribution":{"commit":"x"},"worktree":{"a":0},"spinnerTipsEnabled":true,"subagentPromptCacheTtl":"5m"}`,
			want:        `{"hooks":{"Stop":[1]},"attribution":{"commit":""},"worktree":{"a":1},"spinnerTipsEnabled":false,"subagentPromptCacheTtl":"1h","statusLine":{"type":"command","command":"aval statusline"}}`,
			wantChanged: []string{"attribution", "hooks", "spinnerTipsEnabled", "statusLine", "subagentPromptCacheTtl", "worktree"},
		},
		{
			name:        "maps merge, repo wins per key, live-only kept",
			repo:        `{"env":{"A":"repo","B":"repo"},"enabledPlugins":{"p":true}}`,
			live:        `{"env":{"A":"live","C":"live"},"enabledPlugins":{"q":false}}`,
			want:        `{"env":{"A":"repo","B":"repo","C":"live"},"enabledPlugins":{"p":true,"q":false}}`,
			wantChanged: []string{"enabledPlugins", "env"},
		},
		{
			name:        "everything else in live is untouched",
			repo:        `{"model":"opus","permissions":{"allow":["x"]}}`,
			live:        `{"model":"sonnet","pluginConfigs":{"k":1}}`,
			want:        `{"model":"sonnet","pluginConfigs":{"k":1}}`,
			wantChanged: nil,
		},
		{
			name:        "overlay wins over repo and deep-merges",
			repo:        `{"skillOverrides":{"a":"off"},"env":{"A":"1"}}`,
			live:        `{}`,
			overlay:     `{"skillOverrides":{"a":"on","b":"off"},"model":"haiku"}`,
			want:        `{"skillOverrides":{"a":"on","b":"off"},"env":{"A":"1"},"model":"haiku"}`,
			wantChanged: []string{"env", "model", "skillOverrides"},
		},
		{
			name:        "already in sync reports nothing",
			repo:        `{"hooks":{"Stop":[1]},"env":{"A":"1"}}`,
			live:        `{"hooks":{"Stop":[1]},"env":{"A":"1","Z":"live"}}`,
			want:        `{"hooks":{"Stop":[1]},"env":{"A":"1","Z":"live"}}`,
			wantChanged: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var overlay map[string]any
			if c.overlay != "" {
				overlay = decode(t, c.overlay)
			}
			got, changed := MergeSettings(decode(t, c.repo), decode(t, c.live), overlay)
			if !reflect.DeepEqual(got, decode(t, c.want)) {
				gb, _ := json.Marshal(got)
				t.Errorf("merged = %s\nwant     %s", gb, c.want)
			}
			if !reflect.DeepEqual(changed, c.wantChanged) {
				t.Errorf("changed = %v, want %v", changed, c.wantChanged)
			}
		})
	}
}

func fakeEnv(t *testing.T) Options {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, "config/claude/settings.json"), `{"hooks":{"Stop":[1]},"env":{"A":"1"}}`)
	write(t, filepath.Join(repo, "config/claude/CLAUDE.md"),
		"intro\n<!-- shared:agent-rules -->\nrule one\nrule two\n<!-- /shared:agent-rules -->\noutro\n")
	return Options{Repo: repo, Home: home, PrivateDir: filepath.Join(home, ".config/aval/private"), Now: fixedNow}
}

func TestRunCreatesLiveSettingsWithoutBackup(t *testing.T) {
	opt := fakeEnv(t)
	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(opt.Home, ".claude/settings.json")
	got := read(t, live)
	if !rep.Settings.Created || !rep.Settings.Written || rep.Settings.Backup != "" {
		t.Errorf("settings result = %+v, want created+written, no backup", rep.Settings)
	}
	want := "{\n  \"env\": {\n    \"A\": \"1\"\n  },\n  \"hooks\": {\n    \"Stop\": [\n      1\n    ]\n  }\n}\n"
	if got != want {
		t.Errorf("live file =\n%q\nwant\n%q", got, want)
	}
}

func TestRunBacksUpAndKeepsPersonalKeys(t *testing.T) {
	opt := fakeEnv(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	old := `{"model":"sonnet","hooks":{"Old":[2]}}`
	write(t, live, old)

	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	wantBackup := filepath.Join(opt.Home, ".aval-backup/20260102-030405/claude-settings.json")
	if rep.Settings.Backup != wantBackup {
		t.Fatalf("backup = %q, want %q", rep.Settings.Backup, wantBackup)
	}
	if read(t, wantBackup) != old {
		t.Error("backup does not hold the previous live content")
	}
	m := decode(t, read(t, live))
	if m["model"] != "sonnet" {
		t.Errorf("personal key lost: %v", m)
	}
	if _, ok := m["hooks"].(map[string]any)["Stop"]; !ok {
		t.Errorf("repo hooks not applied: %v", m["hooks"])
	}
	if !strings.HasSuffix(read(t, live), "}\n") {
		t.Error("missing trailing newline")
	}
}

func TestRunIsANoOpWhenInSync(t *testing.T) {
	opt := fakeEnv(t)
	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(opt.Home, ".claude/settings.json")
	before := read(t, live)
	if err := os.RemoveAll(filepath.Join(opt.Home, ".aval-backup")); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Settings.Written || rep.Settings.Backup != "" || len(rep.Settings.Changed) != 0 {
		t.Errorf("second run = %+v, want no-op", rep.Settings)
	}
	if read(t, live) != before {
		t.Error("file changed on a no-op run")
	}
	if _, err := os.Stat(filepath.Join(opt.Home, ".aval-backup")); err == nil {
		t.Error("backup dir created on a no-op run")
	}
}

func TestRunDryRunWritesNothing(t *testing.T) {
	opt := fakeEnv(t)
	opt.DryRun = true
	live := filepath.Join(opt.Home, ".claude/settings.json")
	write(t, live, `{"model":"sonnet"}`)
	write(t, filepath.Join(opt.Home, ".codex/AGENTS.md"), "mine\n")

	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"env", "hooks"}; !reflect.DeepEqual(rep.Settings.Changed, want) {
		t.Errorf("changed = %v, want %v", rep.Settings.Changed, want)
	}
	if rep.Settings.Written {
		t.Error("dry run reported a write")
	}
	if read(t, live) != `{"model":"sonnet"}` {
		t.Error("dry run modified the live file")
	}
	if read(t, filepath.Join(opt.Home, ".codex/AGENTS.md")) != "mine\n" {
		t.Error("dry run modified AGENTS.md")
	}
	if _, err := os.Stat(filepath.Join(opt.Home, ".aval-backup")); err == nil {
		t.Error("dry run created a backup")
	}
}

func TestSettingsDrift(t *testing.T) {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.Home, ".claude/settings.json"), `{"env":{"A":"1"}}`)
	got, err := SettingsDrift(opt)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"hooks"}; !reflect.DeepEqual(got, want) {
		t.Errorf("drift = %v, want %v", got, want)
	}
}

func TestApplyRules(t *testing.T) {
	const b, e = "<!-- aval:agent-rules -->", "<!-- /aval:agent-rules -->"
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{"append when no markers", "mine\n", "mine\n\n" + b + "\nrules\n" + e + "\n", true},
		{"replace existing block, keep outside", "top\n" + b + "\nold\n" + e + "\nbottom\n", "top\n" + b + "\nrules\n" + e + "\nbottom\n", true},
		{"identical block is a no-op", "top\n" + b + "\nrules\n" + e + "\nbottom\n", "top\n" + b + "\nrules\n" + e + "\nbottom\n", false},
		{"empty file", "", "\n" + b + "\nrules\n" + e + "\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := ApplyRules("rules", c.in)
			if got != c.want || changed != c.changed {
				t.Errorf("got (%q, %v)\nwant (%q, %v)", got, changed, c.want, c.changed)
			}
		})
	}
}

func TestRunMirrorsAgentRules(t *testing.T) {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.Home, ".codex/AGENTS.md"), "mine\n")
	// ~/.gemini does not exist: skipped silently, not created.

	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(opt.Home, ".codex/AGENTS.md"))
	want := "mine\n\n<!-- aval:agent-rules -->\nrule one\nrule two\n<!-- /aval:agent-rules -->\n"
	if got != want {
		t.Errorf("AGENTS.md = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(opt.Home, ".gemini")); err == nil {
		t.Error("created a directory for an agent that is not installed")
	}
	var skipped int
	for _, r := range rep.Rules {
		if r.Skipped {
			skipped++
		}
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (%+v)", skipped, rep.Rules)
	}
}

func TestRunCreatesRulesFileWhenDirExists(t *testing.T) {
	opt := fakeEnv(t)
	if err := os.MkdirAll(filepath.Join(opt.Home, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, filepath.Join(opt.Home, ".gemini/GEMINI.md")), "rule one") {
		t.Error("GEMINI.md not created with the block")
	}
}

func TestRunFailsWithoutSharedBlock(t *testing.T) {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.Repo, "config/claude/CLAUDE.md"), "no block here\n")
	if _, err := Run(opt); err == nil {
		t.Error("expected an error when CLAUDE.md has no shared block")
	}
}

func TestRunMergesPrivateOverlaySettings(t *testing.T) {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.PrivateDir, "settings.json"), `{"skillOverrides":{"secret-skill":"off"},"env":{"A":"private"}}`)
	if _, err := Run(opt); err != nil {
		t.Fatal(err)
	}
	m := decode(t, read(t, filepath.Join(opt.Home, ".claude/settings.json")))
	if m["skillOverrides"].(map[string]any)["secret-skill"] != "off" {
		t.Errorf("overlay skillOverrides missing: %v", m)
	}
	if m["env"].(map[string]any)["A"] != "private" {
		t.Errorf("overlay must win over the repo: %v", m["env"])
	}
}

func TestRunWithoutOverlayIsFine(t *testing.T) {
	opt := fakeEnv(t)
	opt.PrivateDir = filepath.Join(opt.Home, "nope")
	if _, err := Run(opt); err != nil {
		t.Fatalf("absent overlay must not fail: %v", err)
	}
}

func TestRunRejectsInvalidOverlay(t *testing.T) {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.PrivateDir, "settings.json"), `{broken`)
	if _, err := Run(opt); err == nil {
		t.Error("expected an error for an invalid overlay fragment")
	}
}

func overlayLinks(t *testing.T) Options {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.PrivateDir, "skills/secret/SKILL.md"), "x")
	write(t, filepath.Join(opt.PrivateDir, "links.manifest"), "# c\nskills/secret  .claude/skills/secret\n")
	return opt
}

func TestRunLinksPrivateSkill(t *testing.T) {
	testenv.NeedSymlinks(t)
	opt := overlayLinks(t)
	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(opt.Home, ".claude/skills/secret")
	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("not a symlink: %v", err)
	}
	if want := filepath.Join(opt.PrivateDir, "skills/secret"); got != want {
		t.Errorf("link -> %q, want %q", got, want)
	}
	if len(rep.Links) != 1 || rep.Links[0].Status != "linked" {
		t.Errorf("links = %+v", rep.Links)
	}

	rep, err = Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Links[0].Status != "already linked" {
		t.Errorf("second run status = %q, want already linked", rep.Links[0].Status)
	}
}

func TestRunBacksUpARealFileAtLinkDest(t *testing.T) {
	testenv.NeedSymlinks(t)
	opt := overlayLinks(t)
	dest := filepath.Join(opt.Home, ".claude/skills/secret")
	write(t, filepath.Join(dest, "SKILL.md"), "mine")

	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Links[0].Backup == "" {
		t.Fatal("expected a backup of the displaced directory")
	}
	if read(t, filepath.Join(rep.Links[0].Backup, "SKILL.md")) != "mine" {
		t.Error("backup lost the original content")
	}
	if _, err := os.Readlink(dest); err != nil {
		t.Errorf("dest is not a symlink after the run: %v", err)
	}
}

func TestDryRunDoesNotLink(t *testing.T) {
	opt := overlayLinks(t)
	opt.DryRun = true
	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(opt.Home, ".claude/skills/secret")); err == nil {
		t.Error("dry run created a link")
	}
	if rep.Links[0].Status != "would link" {
		t.Errorf("status = %q, want would link", rep.Links[0].Status)
	}
}

func TestRunErrorsOnMissingLinkSource(t *testing.T) {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.PrivateDir, "links.manifest"), "gone .claude/gone\n")
	if _, err := Run(opt); err == nil {
		t.Error("expected an error for a link whose source is missing")
	}
}

func TestPrivateDir(t *testing.T) {
	t.Setenv("AVAL_PRIVATE_DIR", "")
	if got, want := PrivateDir("/h"), filepath.Join("/h", ".config", "aval", "private"); got != want {
		t.Errorf("default = %q, want %q", got, want)
	}
	t.Setenv("AVAL_PRIVATE_DIR", "/elsewhere")
	if got := PrivateDir("/h"); got != "/elsewhere" {
		t.Errorf("override = %q", got)
	}
}
