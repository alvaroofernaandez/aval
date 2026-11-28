package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "claude", "settings.json"))
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
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "opencode", "opencode.json"))
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

func strs(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		out = append(out, x.(string))
	}
	return out
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
