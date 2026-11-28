package claude

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, s)
	}
	return m
}

func decodeObj(t *testing.T, s string) map[string]any {
	t.Helper()
	if s == "" {
		return nil
	}
	return decode(t, s)
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

func strs(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		out = append(out, x.(string))
	}
	return out
}
