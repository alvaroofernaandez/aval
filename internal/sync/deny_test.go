package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// globRegexp turns a Claude Code permission glob into a regexp: ** crosses
// directories, * does not, and "**/" also matches zero directories.
func globRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case pattern[i] == '*':
			b.WriteString("[^/]*")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// denies reports whether any Read(...) deny rule covers path. Patterns with ~/
// are anchored at home; every other pattern is relative to the project root.
func denies(rules []string, home, project, path string) bool {
	for _, r := range rules {
		arg, ok := strings.CutPrefix(r, "Read(")
		if !ok {
			continue
		}
		arg = strings.TrimSuffix(arg, ")")
		var target string
		switch {
		case strings.HasPrefix(arg, "~/"):
			arg = strings.TrimPrefix(arg, "~/")
			rel, err := filepath.Rel(home, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			target = filepath.ToSlash(rel)
		default:
			rel, err := filepath.Rel(project, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			arg = strings.TrimPrefix(arg, "./")
			target = filepath.ToSlash(rel)
		}
		if globRegexp(arg).MatchString(target) {
			return true
		}
	}
	return false
}

func repoDenyRules(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "config", "claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.Permissions.Deny
}

func TestDenyMatcherItself(t *testing.T) {
	rules := []string{"Read(~/.ssh/**)", "Read(**/.env)"}
	home, project := "/h", "/p"
	for path, want := range map[string]bool{
		"/h/.ssh/id_rsa":    true,
		"/h/.sshx/id_rsa":   false,
		"/p/.env":           true,
		"/p/a/b/.env":       true,
		"/p/.env.example":   false,
		"/elsewhere/.env":   false,
		"/h/other/.ssh/key": false,
	} {
		if got := denies(rules, home, project, path); got != want {
			t.Errorf("denies(%s) = %v, want %v", path, got, want)
		}
	}
}

func TestRepoSettingsDenyReadsOfSecrets(t *testing.T) {
	rules := repoDenyRules(t)
	home, project := "/home/u", "/home/u/code/app"
	secrets := []string{
		home + "/.ssh/id_ed25519",
		home + "/.ssh/config",
		home + "/.aws/credentials",
		home + "/.claude/.credentials.json",
		project + "/.env",
		project + "/.env.local",
		project + "/.env.production",
		project + "/apps/web/.env",
		project + "/apps/web/.env.development.local",
		project + "/.credentials.json",
		project + "/deep/er/.credentials.json",
	}
	for _, p := range secrets {
		if !denies(rules, home, project, p) {
			t.Errorf("no permissions.deny rule covers a read of %s", p)
		}
	}
}

func TestRepoSettingsKeepsItsDestructiveBashDenies(t *testing.T) {
	rules := repoDenyRules(t)
	for _, want := range []string{"Bash(rm -rf /)", "Bash(sudo rm -rf /)", "Bash(rm -rf ~)", "Bash(sudo rm -rf ~)"} {
		found := false
		for _, r := range rules {
			found = found || r == want
		}
		if !found {
			t.Errorf("deny list lost %q", want)
		}
	}
}

func TestOpenCodeConfigDeniesSecretReads(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	perm, ok := cfg["permission"].(map[string]any)
	if !ok {
		t.Skip("config/opencode/opencode.json has no \"permission\" block: nothing to assert yet. " +
			"OpenCode's permission rules are not covered by this repo; add one and replace this skip with an assertion.")
	}
	if _, ok := perm["read"]; !ok {
		t.Error("opencode permission block does not constrain read")
	}
}

func decodeObj(t *testing.T, s string) map[string]any {
	t.Helper()
	if s == "" {
		return nil
	}
	return decode(t, s)
}

func strs(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		out = append(out, x.(string))
	}
	return out
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestMergeUnionsDenyListsAndNeverNarrowsThem(t *testing.T) {
	cases := []struct {
		name               string
		repo, live, overly string
		wantDeny           []string
	}{
		{
			name:     "repo denies are added to a live file that lacks them",
			repo:     `{"permissions":{"deny":["Read(~/.ssh/**)","Bash(rm -rf /)"]}}`,
			live:     `{}`,
			wantDeny: []string{"Bash(rm -rf /)", "Read(~/.ssh/**)"},
		},
		{
			name:     "live-only denies survive",
			repo:     `{"permissions":{"deny":["A"]}}`,
			live:     `{"permissions":{"deny":["B"]}}`,
			wantDeny: []string{"A", "B"},
		},
		{
			name:     "overlay can add denies",
			repo:     `{"permissions":{"deny":["A"]}}`,
			live:     `{}`,
			overly:   `{"permissions":{"deny":["C"]}}`,
			wantDeny: []string{"A", "C"},
		},
		{
			name:     "overlay cannot replace or empty the repo denies",
			repo:     `{"permissions":{"deny":["A","B"]}}`,
			live:     `{}`,
			overly:   `{"permissions":{"deny":[]}}`,
			wantDeny: []string{"A", "B"},
		},
		{
			name:     "duplicates collapse",
			repo:     `{"permissions":{"deny":["A"]}}`,
			live:     `{"permissions":{"deny":["A","A"]}}`,
			overly:   `{"permissions":{"deny":["A"]}}`,
			wantDeny: []string{"A"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := MergeSettingsWarn(decodeObj(t, c.repo), decodeObj(t, c.live), decodeObj(t, c.overly))
			perms, _ := got["permissions"].(map[string]any)
			if gotDeny := sorted(strs(perms["deny"])); !reflect.DeepEqual(gotDeny, c.wantDeny) {
				t.Errorf("deny = %v, want %v", gotDeny, c.wantDeny)
			}
		})
	}
}

func TestMergeDropsOverlayAllowsThatTheRepoDenies(t *testing.T) {
	repo := decodeObj(t, `{"permissions":{"deny":["Read(~/.ssh/**)","Read(**/.env)","Bash(rm -rf /)"]}}`)
	overlay := decodeObj(t, `{"permissions":{"allow":["Read(~/.ssh/id_rsa)","Read(./.env)","Bash(rm -rf /)","Bash(ls)"]}}`)

	got, changed, warns := MergeSettingsWarn(repo, decodeObj(t, `{}`), overlay)
	perms := got["permissions"].(map[string]any)
	if want := []string{"Bash(ls)"}; !reflect.DeepEqual(strs(perms["allow"]), want) {
		t.Errorf("allow = %v, want %v", strs(perms["allow"]), want)
	}
	if len(warns) != 3 {
		t.Errorf("warnings = %v, want one per dropped allow", warns)
	}
	for _, w := range warns {
		if !strings.Contains(w, "denied by") {
			t.Errorf("warning %q does not say why", w)
		}
	}
	if !reflect.DeepEqual(changed, []string{"permissions"}) {
		t.Errorf("changed = %v", changed)
	}
}

func TestMergeKeepsOtherOverlayPermissionKeysAndLiveAllows(t *testing.T) {
	repo := decodeObj(t, `{"permissions":{"deny":["A"]}}`)
	live := decodeObj(t, `{"permissions":{"allow":["mine"],"defaultMode":"acceptEdits"}}`)
	overlay := decodeObj(t, `{"permissions":{"allow":["theirs"],"defaultMode":"plan"}}`)

	got, _, _ := MergeSettingsWarn(repo, live, overlay)
	perms := got["permissions"].(map[string]any)
	if want := []string{"mine", "theirs"}; !reflect.DeepEqual(sorted(strs(perms["allow"])), want) {
		t.Errorf("allow = %v, want %v", strs(perms["allow"]), want)
	}
	if perms["defaultMode"] != "plan" {
		t.Errorf("defaultMode = %v, overlay should still win scalar keys", perms["defaultMode"])
	}
}

func TestRunReportsDroppedOverlayAllows(t *testing.T) {
	opt := fakeEnv(t)
	write(t, filepath.Join(opt.Repo, "config/claude/settings.json"),
		`{"hooks":{"Stop":[1]},"permissions":{"deny":["Read(~/.ssh/**)"]}}`)
	write(t, filepath.Join(opt.PrivateDir, "settings.json"),
		`{"permissions":{"allow":["Read(~/.ssh/id_rsa)"]}}`)

	rep, err := Run(opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Settings.Warnings) != 1 {
		t.Errorf("warnings = %v, want 1", rep.Settings.Warnings)
	}
	live := decode(t, read(t, filepath.Join(opt.Home, ".claude/settings.json")))
	perms := live["permissions"].(map[string]any)
	if got := strs(perms["deny"]); !reflect.DeepEqual(got, []string{"Read(~/.ssh/**)"}) {
		t.Errorf("live deny = %v", got)
	}
	if _, has := perms["allow"]; has && len(strs(perms["allow"])) != 0 {
		t.Errorf("the denied allow was written: %v", perms["allow"])
	}
}
