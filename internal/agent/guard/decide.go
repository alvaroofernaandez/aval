package guard

import (
	"strings"
)

// Options carries what a decision needs from outside. ReadAllow is called only
// after a rule has matched, so a command that breaks no rule costs no read.
type Options struct {
	Ctx       Context
	ReadAllow func() ([]string, error)
}

// Decision is the outcome for one Bash command.
type Decision struct {
	Deny         bool
	Rule, Reason string
}

const allowFile = "~/.config/aval/private/guard.allow"

var ruleText = map[string]string{
	ruleRm:      "rm -rf outside the project and home, or on /, ~ or the home folder",
	rulePush:    "a forced push to main or master",
	ruleSQL:     "DROP DATABASE or TRUNCATE against a host that is not local",
	ruleDocker:  "docker volume or system prune",
	ruleKubectl: "kubectl delete",
	rulePipe:    "a download piped into a shell",
	ruleSecret:  "reading SSH keys, AWS credentials or an .env file",
}

const maxShown = 200

// Decide runs the rules over cmd. A command that breaks one is denied unless
// the allow-list names that exact command. A panic is an internal error and
// lets the command through; an allow-list that cannot be read leaves a
// matched command denied.
func Decide(cmd string, o Options) (d Decision) {
	defer func() {
		if recover() != nil {
			d = Decision{}
		}
	}()
	f, hit := Check(cmd, o.Ctx)
	if !hit {
		return Decision{}
	}
	if o.ReadAllow != nil {
		entries, err := o.ReadAllow()
		if err == nil && allowed(cmd, entries) {
			return Decision{}
		}
	}
	return Decision{Deny: true, Rule: f.Rule, Reason: reason(f.Rule, cmd)}
}

func reason(rule, cmd string) string {
	shown := []rune(strings.Join(strings.Fields(cmd), " "))
	text := string(shown)
	if len(shown) > maxShown {
		text = string(shown[:maxShown]) + "..."
	}
	return "aval guard blocked this command (" + rule + ": " + ruleText[rule] + "). Command: " + text +
		". If you really mean it, add the full command as one line to " + allowFile + " (a private file, never committed)."
}

func normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

func allowed(cmd string, entries []string) bool {
	n := normalize(cmd)
	for _, e := range entries {
		if e == n {
			return true
		}
	}
	return false
}

// parseAllow reads guard.allow: one full command per line, blank lines and
// lines starting with # ignored. Whitespace is normalised, so an entry matches
// the whole command and never a prefix of it.
func parseAllow(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = normalize(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}
