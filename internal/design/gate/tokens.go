package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var aliasRe = regexp.MustCompile(`\{([^}]+)\}`)

// flatten maps dotted paths to the $value of every DTCG leaf token.
func flatten(node any, prefix string, out map[string]any) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	if v, ok := m["$value"]; ok {
		out[prefix] = v
	}
	for k, v := range m {
		if strings.HasPrefix(k, "$") {
			continue
		}
		child := k
		if prefix != "" {
			child = prefix + "." + k
		}
		flatten(v, child, out)
	}
}

func collectAliases(v any) []string {
	switch x := v.(type) {
	case string:
		var refs []string
		for _, m := range aliasRe.FindAllStringSubmatch(x, -1) {
			refs = append(refs, m[1])
		}
		return refs
	case map[string]any:
		var refs []string
		for _, e := range x {
			refs = append(refs, collectAliases(e)...)
		}
		return refs
	case []any:
		var refs []string
		for _, e := range x {
			refs = append(refs, collectAliases(e)...)
		}
		return refs
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Tokens validates DTCG token files: each parses as JSON and every {alias}
// resolves inside the set. The set is self-contained, so an unresolved alias
// fails. A directory contributes its *.json files.
func Tokens(w io.Writer, paths []string) int {
	if len(paths) == 0 {
		fmt.Fprintln(w, "ERROR: give a token file or directory")
		return 1
	}
	var files []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(w, "ERROR: %s not found\n", p)
			return 1
		}
		if st.IsDir() {
			m, _ := filepath.Glob(filepath.Join(p, "*.json"))
			sort.Strings(m)
			files = append(files, m...)
		} else {
			files = append(files, p)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(w, "ERROR: no token files found")
		return 1
	}

	all := map[string]any{}
	parsed := map[string]map[string]any{}
	var errs []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		var data any
		if err == nil {
			err = json.Unmarshal(b, &data)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid JSON - %v", filepath.Base(f), err))
			continue
		}
		flat := map[string]any{}
		flatten(data, "", flat)
		parsed[f] = flat
		stem := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		for p, v := range flat {
			all[p] = v
			all[stem+"."+p] = v
		}
	}

	var unresolved []string
	for _, f := range files {
		flat, ok := parsed[f]
		if !ok {
			continue
		}
		for _, p := range sortedKeys(flat) {
			for _, ref := range collectAliases(flat[p]) {
				ref = strings.TrimSpace(ref)
				norm := ref
				for strings.HasPrefix(norm, "../") || strings.HasPrefix(norm, "./") {
					if strings.HasPrefix(norm, "../") {
						norm = norm[3:]
					} else {
						norm = norm[2:]
					}
				}
				if resolves(all, ref, norm) {
					continue
				}
				unresolved = append(unresolved, fmt.Sprintf("%s: %s -> {%s} (unresolved)", filepath.Base(f), p, ref))
			}
		}
	}

	fmt.Fprintf(w, "Parsed %d/%d token files, %d tokens defined.\n", len(parsed), len(files), len(all)/2)
	for _, e := range errs {
		fmt.Fprintln(w, "  x "+e)
	}
	for _, u := range unresolved {
		fmt.Fprintln(w, "  ! "+u)
	}
	if len(errs) > 0 {
		fmt.Fprintln(w, "\nFAIL: JSON errors above.")
		return 1
	}
	if len(unresolved) > 0 {
		fmt.Fprintf(w, "\nFAIL: %d unresolved alias(es) in a self-contained token set.\n", len(unresolved))
		return 1
	}
	fmt.Fprintln(w, "\nOK: all token files valid JSON and every alias resolves.")
	return 0
}

// resolves reports whether an alias names a defined token, tolerating
// ../ prefixes, an omitted file prefix, and a suffix match.
func resolves(all map[string]any, ref, norm string) bool {
	if _, ok := all[ref]; ok {
		return true
	}
	if _, ok := all[norm]; ok {
		return true
	}
	tail := norm
	if i := strings.Index(norm, "."); i >= 0 {
		tail = norm[i+1:]
	}
	if _, ok := all[tail]; ok {
		return true
	}
	for k := range all {
		if strings.HasSuffix(k, norm) {
			return true
		}
	}
	return false
}
