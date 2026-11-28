package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

// Claude Code and other agent tools rewrite ~/.claude/settings.json, and it
// holds personal keys that cannot be published, so it is a real file rather
// than a link to the repo. The merge below is how the repo-owned part stays
// authoritative: it replaces or merges only the managed keys and leaves
// everything else alone.

// ReplaceKeys are owned by the repo: the live value is overwritten.
var ReplaceKeys = []string{"hooks", "attribution", "worktree", "spinnerTipsEnabled", "subagentPromptCacheTtl", "statusLine"}

// MergeKeys are maps where the repo wins per entry but live-only entries
// survive (plugins and overrides added by hand or by a marketplace).
var MergeKeys = []string{"env", "enabledPlugins", "skillOverrides", "extraKnownMarketplaces"}

// MergeSettings merges repo and then overlay into live and returns the result
// plus the sorted top-level keys whose value changed. Inputs are not mutated.
func MergeSettings(repo, live, overlay map[string]any) (map[string]any, []string) {
	out, changed, _ := MergeSettingsWarn(repo, live, overlay)
	return out, changed
}

// MergeSettingsWarn is MergeSettings plus the warnings it raised.
//
// permissions.deny is a safety floor, not a preference: the result is the union
// of the repo's denies, the live file's own and the overlay's, so the overlay
// can add denies but never remove or replace one. Overlay permissions.allow
// entries are added to the live ones, except those a deny covers: those are
// dropped with a warning instead of silently widening access.
func MergeSettingsWarn(repo, live, overlay map[string]any) (map[string]any, []string, []string) {
	rest, overlayDeny, overlayAllow := splitOverlayPermissions(overlay)
	out, _ := mergeSettings(repo, live, rest)
	warnings := mergePermissions(out, repo, overlayDeny, overlayAllow)

	var changed []string
	for k, v := range out {
		if old, ok := live[k]; !ok || !reflect.DeepEqual(old, v) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return out, changed, warnings
}

// splitOverlayPermissions returns the overlay without permissions.deny and
// permissions.allow (which merge by union, not by replacement) plus those two
// lists. The input is not mutated.
func splitOverlayPermissions(overlay map[string]any) (map[string]any, []string, []string) {
	perms, ok := overlay["permissions"].(map[string]any)
	if !ok {
		return overlay, nil, nil
	}
	rest := cloneMap(overlay)
	rp := rest["permissions"].(map[string]any)
	deny, allow := stringList(perms["deny"]), stringList(perms["allow"])
	delete(rp, "deny")
	delete(rp, "allow")
	if len(rp) == 0 {
		delete(rest, "permissions")
	}
	return rest, deny, allow
}

func stringList(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func union(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, s := range l {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func toAny(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}

func mergePermissions(out, repo map[string]any, overlayDeny, overlayAllow []string) []string {
	var repoDeny []string
	if rp, ok := repo["permissions"].(map[string]any); ok {
		repoDeny = stringList(rp["deny"])
	}
	if len(repoDeny) == 0 && len(overlayDeny) == 0 && len(overlayAllow) == 0 {
		return nil
	}
	perms, _ := out["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	deny := union(repoDeny, stringList(perms["deny"]), overlayDeny)
	if len(deny) > 0 {
		perms["deny"] = toAny(deny)
	}

	var warnings []string
	if len(overlayAllow) > 0 {
		var add []string
		for _, a := range overlayAllow {
			if d, hit := coveredByDeny(a, deny); hit {
				warnings = append(warnings, fmt.Sprintf("permissions: overlay allow %q dropped, denied by %q", a, d))
				continue
			}
			add = append(add, a)
		}
		if merged := union(stringList(perms["allow"]), add); len(merged) > 0 {
			perms["allow"] = toAny(merged)
		}
	}
	if len(perms) > 0 {
		out["permissions"] = perms
	}
	return warnings
}

// coveredByDeny reports whether a deny rule matches the allow rule: the same
// rule, a bare tool name denying every use of it, or a deny glob that matches
// the allow rule's argument ("Read(~/.ssh/**)" covers "Read(~/.ssh/id_rsa)").
func coveredByDeny(allow string, deny []string) (string, bool) {
	at, aa := splitRule(allow)
	for _, d := range deny {
		if d == allow {
			return d, true
		}
		dt, da := splitRule(d)
		if dt != at {
			continue
		}
		if da == "" {
			return d, true
		}
		if aa != "" && ruleGlob(at, da).MatchString(aa) {
			return d, true
		}
	}
	return "", false
}

// splitRule splits "Tool(arg)" into "Tool" and "arg"; a bare "Tool" has no arg.
func splitRule(r string) (tool, arg string) {
	tool, rest, ok := strings.Cut(r, "(")
	if !ok {
		return r, ""
	}
	arg = strings.TrimSuffix(rest, ")")
	return tool, strings.TrimPrefix(arg, "./")
}

// ruleGlob compiles a permission glob: ** crosses directories, "**/" also
// matches none, and * stops at "/" for file tools but not for Bash commands.
func ruleGlob(tool, pattern string) *regexp.Regexp {
	pattern = strings.TrimPrefix(pattern, "./")
	star := "[^/]*"
	if tool == "Bash" {
		star = ".*"
	}
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
			b.WriteString(star)
		default:
			// Whole characters, so a non-ASCII literal matches itself.
			r, size := utf8.DecodeRuneInString(pattern[i:])
			b.WriteString(regexp.QuoteMeta(string(r)))
			i += size - 1
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func mergeSettings(repo, live, overlay map[string]any) (map[string]any, []string) {
	out := cloneMap(live)
	for _, k := range ReplaceKeys {
		if v, ok := repo[k]; ok {
			out[k] = clone(v)
		}
	}
	for _, k := range MergeKeys {
		rv, ok := repo[k].(map[string]any)
		if !ok {
			continue
		}
		dst, _ := out[k].(map[string]any)
		if dst == nil {
			dst = map[string]any{}
		}
		for name, v := range rv {
			dst[name] = clone(v)
		}
		out[k] = dst
	}
	deepMerge(out, overlay)

	var changed []string
	for k, v := range out {
		if old, ok := live[k]; !ok || !reflect.DeepEqual(old, v) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return out, changed
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		sm, srcIsMap := v.(map[string]any)
		dm, dstIsMap := dst[k].(map[string]any)
		if srcIsMap && dstIsMap {
			deepMerge(dm, sm)
			continue
		}
		dst[k] = clone(v)
	}
}

func clone(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneMap(t)
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = clone(t[i])
		}
		return out
	}
	return v
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = clone(v)
	}
	return out
}

func readJSONFile(path string) (map[string]any, []byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	m, err := agentcfg.DecodeJSON(b)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, b, nil
}

// encode writes 2-space-indented JSON with a trailing newline and without
// escaping & < > — hook commands are full of them.
func encode(m map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
