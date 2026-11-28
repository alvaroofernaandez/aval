package guard

import (
	"path"
	"regexp"
	"strings"
)

var envAssign = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// keywords and builtins that only precede the real command.
var prefixWords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "do": true, "while": true,
	"until": true, "!": true, "{": true, "}": true, "time": true, "nohup": true,
	"exec": true, "builtin": true, "command": true,
}

// wrapperValued maps a wrapper to the options that take a separate value.
var wrapperValued = map[string]map[string]bool{
	"sudo":    set("-u", "-g", "-h", "-p", "-C", "-D", "-R", "-T", "-U", "-r", "-t"),
	"doas":    set("-u", "-C"),
	"env":     set("-u", "-C"),
	"nice":    set("-n"),
	"timeout": set("-s", "-k"),
	"xargs":   set("-I", "-n", "-P", "-L", "-d", "-E", "-s", "-a", "--max-args", "--max-procs", "--delimiter", "--arg-file"),
	"exec":    set("-a"),
	"stdbuf":  set("-i", "-o", "-e"),
}

func set(k ...string) map[string]bool {
	m := make(map[string]bool, len(k))
	for _, s := range k {
		m[s] = true
	}
	return m
}

// base is the command name without its directory: /bin/rm is rm.
func base(w string) string {
	if i := strings.LastIndexByte(w, '/'); i >= 0 {
		return w[i+1:]
	}
	return w
}

// peel strips what runs a command without changing what it is: environment
// assignments, keywords and the wrappers sudo, env, nohup, command, time,
// nice, timeout and xargs. It returns the real command and the environment
// assignments it saw.
func peel(words []string) (rest []string, env map[string]string) {
	env = map[string]string{}
	for n := 0; n < 24 && len(words) > 0; n++ {
		w := words[0]
		if envAssign.MatchString(w) {
			k, v, _ := strings.Cut(w, "=")
			env[k] = v
			words = words[1:]
			continue
		}
		name := base(w)
		if prefixWords[name] {
			if name == "command" && len(words) > 1 && (words[1] == "-v" || words[1] == "-V") {
				return nil, env // a lookup, not a run
			}
			words = skipFlags(words[1:], wrapperValued[name])
			continue
		}
		if vals, ok := wrapperValued[name]; ok && name != "exec" && name != "stdbuf" {
			words = skipFlags(words[1:], vals)
			if name == "timeout" && len(words) > 0 { // the duration
				words = words[1:]
			}
			continue
		}
		break
	}
	return words, env
}

// skipFlags drops leading options (and the values of those that take one).
func skipFlags(words []string, valued map[string]bool) []string {
	for len(words) > 0 {
		w := words[0]
		if w == "--" {
			return words[1:]
		}
		if !strings.HasPrefix(w, "-") {
			return words
		}
		words = words[1:]
		if valued[w] && len(words) > 0 {
			words = words[1:]
		}
	}
	return words
}

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "ash": true, "fish": true}

// inlineScript returns the script a shell (-c) or eval runs, so it can be
// analysed as a command of its own. In lenient mode quoting was ignored, so
// the script is every word that follows.
func inlineScript(words []string, lenient bool) (string, bool) {
	name := base(words[0])
	if name == "eval" {
		return strings.Join(words[1:], " "), len(words) > 1
	}
	if !shells[name] {
		return "", false
	}
	for i := 1; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "-o" || w == "+o":
			i++
		case strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "--") && strings.ContainsRune(w, 'c'):
			if i+1 >= len(words) {
				return "", false
			}
			if lenient {
				return strings.Join(words[i+1:], " "), true
			}
			return words[i+1], true
		case strings.HasPrefix(w, "-") || strings.HasPrefix(w, "+"):
		default:
			return "", false
		}
	}
	return "", false
}

// positionals are the non-option words, skipping the values of the options in
// valued. Used for "git -C dir push", "docker --context x system prune".
func positionals(words []string, valued map[string]bool) []string {
	var out []string
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "--":
			return append(out, words[i+1:]...)
		case valued[w]:
			i++
		case strings.HasPrefix(w, "-") && len(w) > 1:
		default:
			out = append(out, w)
		}
	}
	return out
}

func isAbs(p string) bool {
	return strings.HasPrefix(p, "/") || len(p) > 2 && p[1] == ':' && p[2] == '/'
}

func clean(p string) string { return path.Clean(strings.ReplaceAll(p, `\`, "/")) }
