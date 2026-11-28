package agentcfg

import (
	"strings"
)

// frontMatter is the flat YAML header of a markdown file: scalar values,
// folded or literal blocks (`>`, `|`, joined with spaces) and lists ("- x"
// lines or "[a, b]"). It is the subset aval's skills, commands and agents use;
// anything richer stays in the raw file.
type frontMatter struct {
	values map[string]string
	lists  map[string][]string
}

// splitFrontMatter returns the header and the body after it. A file without a
// header has an empty header and the whole content as body.
func splitFrontMatter(content string) (frontMatter, string) {
	fm := frontMatter{values: map[string]string{}, lists: map[string][]string{}}
	rest, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return fm, content
	}
	head, body, found := strings.Cut(rest, "\n---\n")
	if !found {
		head, found = strings.CutSuffix(rest, "\n---")
		if !found {
			return frontMatter{values: map[string]string{}, lists: map[string][]string{}}, content
		}
		body = ""
	}
	key := ""
	for _, ln := range strings.Split(head, "\n") {
		indented := strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t")
		t := strings.TrimSpace(ln)
		switch {
		case t == "":
		case key != "" && strings.HasPrefix(t, "- "):
			fm.lists[key] = append(fm.lists[key], unquote(strings.TrimPrefix(t, "- ")))
		case key != "" && indented:
			fm.values[key] = strings.TrimSpace(fm.values[key] + " " + t)
		default:
			k, v, _ := strings.Cut(t, ":")
			key = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			switch {
			case v == ">" || v == "|" || v == ">-" || v == "|-":
				v = ""
			case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
				for _, item := range strings.Split(strings.Trim(v, "[]"), ",") {
					if item = unquote(strings.TrimSpace(item)); item != "" {
						fm.lists[key] = append(fm.lists[key], item)
					}
				}
				v = ""
			}
			fm.values[key] = unquote(v)
		}
	}
	return fm, body
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
