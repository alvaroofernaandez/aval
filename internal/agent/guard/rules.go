// Package guard is the PreToolUse hook that denies destructive Bash commands
// in bypass mode: rm -rf outside the project and home, a forced push to
// main or master, DROP DATABASE or TRUNCATE on a non-local host, docker and
// kubectl prune or delete, a download piped into a shell, and reading secrets.
//
// The rules work on words, not on raw text: a tokenizer splits the command,
// wrappers (sh -c, env, sudo, xargs, ...) are peeled up to four levels, and
// flags are normalised before they are compared. A script that does not parse
// is re-read loosely and denied only if a rule still matches.
//
// It is a hot path: pure string work, no exec, no network. The private
// allow-list is read only after a rule has matched (ADR 0009), so a command
// that breaks no rule does no I/O at all.
package guard

import (
	"path"
	"regexp"
	"strings"
)

// Context is what the rules need to know about where the command runs.
type Context struct{ Cwd, Home string }

// Finding is a rule that matched: its name and the simple command that did.
type Finding struct{ Rule, Command string }

const (
	maxDepth = 4
	hardStop = 12
)

// Check reports the first rule a command breaks. A script that does not parse
// is re-read leniently (quotes ignored) and denied only if a rule still
// matches the loose words.
func Check(cmd string, c Context) (Finding, bool) { return check(cmd, c, 0) }

func check(script string, c Context, depth int) (Finding, bool) {
	if depth > hardStop {
		return Finding{}, false
	}
	lenient := depth > maxDepth
	cs, err := parse(script, lenient)
	if err != nil {
		lenient = true
		cs, _ = parse(script, true)
	}
	for i := range cs {
		if f, ok := checkCommand(cs, i, c, depth, lenient); ok {
			return f, true
		}
	}
	return Finding{}, false
}

func checkCommand(cs []command, i int, c Context, depth int, lenient bool) (Finding, bool) {
	words, env := peel(cs[i].Words)
	text := strings.Join(cs[i].Words, " ")
	hit := func(rule string) (Finding, bool) { return Finding{rule, text}, true }

	for _, r := range cs[i].Redirs {
		if r.Op == "<" && secretPath(r.Target, c) {
			return hit(ruleSecret)
		}
	}
	if len(words) == 0 {
		return Finding{}, false
	}
	if script, ok := inlineScript(words, lenient); ok {
		if downloadsInto(script) {
			return hit(rulePipe)
		}
		if f, ok := check(script, c, depth+1); ok {
			return f, true
		}
	}
	name, args := base(words[0]), words[1:]
	switch {
	case name == "rm" && rmOutsideRoots(args, c):
		return hit(ruleRm)
	case name == "git" && forcePushesProtected(args):
		return hit(rulePush)
	case sqlTools[name] && destroysRemoteData(name, args, env, cs, i):
		return hit(ruleSQL)
	case (name == "docker" || name == "podman") && dockerPrune(args):
		return hit(ruleDocker)
	case (name == "kubectl" || name == "oc") && kubectlDelete(args):
		return hit(ruleKubectl)
	case shells[name] && readsDownload(words, cs, i):
		return hit(rulePipe)
	case secretReaders[name] && readsSecret(name, args, c):
		return hit(ruleSecret)
	}
	return Finding{}, false
}

const (
	ruleRm      = "rm-outside-roots"
	rulePush    = "git-force-push"
	ruleSQL     = "sql-destructive"
	ruleDocker  = "docker-prune"
	ruleKubectl = "kubectl-delete"
	rulePipe    = "pipe-to-shell"
	ruleSecret  = "secret-read"
)

// --- rm ----------------------------------------------------------------

func expand(w string, c Context) string {
	if c.Home == "" {
		return w
	}
	for _, h := range []string{"~", "$HOME", "${HOME}"} {
		if w == h || strings.HasPrefix(w, h+"/") {
			return c.Home + w[len(h):]
		}
	}
	return w
}

// resolve makes w absolute against the working directory. ok is false when it
// cannot be known: an unresolved variable, or a relative path with no cwd.
func resolve(w string, c Context) (string, bool) {
	w = expand(w, c)
	if strings.ContainsAny(w, "$`") {
		return "", false
	}
	w = strings.ReplaceAll(w, `\`, "/")
	if isAbs(w) {
		return clean(w), true
	}
	if c.Cwd == "" {
		return "", false
	}
	return clean(c.Cwd + "/" + w), true
}

func within(p, dir string) bool {
	return dir != "" && (p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/"))
}

func rmOutsideRoots(args []string, c Context) bool {
	recursive := false
	var targets []string
parse:
	for i, a := range args {
		switch {
		case a == "--":
			targets = append(targets, args[i+1:]...)
			break parse
		case a == "--recursive":
			recursive = true
		case strings.HasPrefix(a, "--"):
		case strings.HasPrefix(a, "-") && len(a) > 1:
			recursive = recursive || strings.ContainsAny(a, "rR")
		default:
			targets = append(targets, a)
		}
	}
	if !recursive {
		return false
	}
	home, cwd := clean(c.Home), clean(c.Cwd)
	for _, t := range targets {
		p, ok := resolve(t, c)
		if !ok {
			if c.Cwd == "" && strings.HasPrefix(t, "..") {
				return true
			}
			continue
		}
		switch {
		case p == "/", c.Home != "" && p == home, c.Cwd != "" && p == cwd:
			return true
		case strings.ContainsAny(path.Base(p), "*?[") && (path.Dir(p) == "/" || c.Home != "" && path.Dir(p) == home):
			return true
		case !within(p, cwd) && !(c.Home != "" && within(p, home)):
			return true
		}
	}
	return false
}

// --- git push ----------------------------------------------------------

var gitValued = set("-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path")

func forcePushesProtected(args []string) bool {
	pos := positionals(args, gitValued)
	if len(pos) == 0 || pos[0] != "push" {
		return false
	}
	force := false
	start := 0
	for i, a := range args { // options after the subcommand
		if a == "push" {
			start = i + 1
			break
		}
	}
	for _, a := range args[start:] {
		switch {
		case a == "--force", strings.HasPrefix(a, "--force-with-lease"), a == "-f":
			force = true
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--"):
			force = force || strings.ContainsRune(a, 'f')
		}
	}
	for _, p := range pos[1:] {
		plus := strings.HasPrefix(p, "+")
		ref := strings.TrimPrefix(p, "+")
		if i := strings.LastIndexByte(ref, ':'); i >= 0 {
			ref = ref[i+1:]
		}
		ref = strings.TrimPrefix(ref, "refs/heads/")
		if (force || plus) && (ref == "main" || ref == "master") {
			return true
		}
	}
	return false
}

// --- sql ---------------------------------------------------------------

var (
	sqlTools   = set("psql", "pgcli", "mysql", "mycli", "mariadb", "mongosh", "mongo")
	destructiv = regexp.MustCompile(`(?i)\bdrop\s+database\b|\btruncate\b|\bdropDatabase\s*\(`)
)

func destroysRemoteData(name string, args []string, env map[string]string, cs []command, i int) bool {
	text := strings.Join(args, " ") + "\n" + cs[i].Stdin
	for j := i - 1; j >= 0 && cs[j].PipeNext; j-- {
		text += "\n" + strings.Join(cs[j].Words, " ") + "\n" + cs[j].Stdin
	}
	if !destructiv.MatchString(text) {
		return false
	}
	hosts := sqlHosts(name, args)
	for _, k := range []string{"PGHOST", "MYSQL_HOST"} {
		if env[k] != "" {
			hosts = append(hosts, env[k])
		}
	}
	for _, h := range hosts {
		if !localHost(h) {
			return true
		}
	}
	return false
}

func sqlHosts(name string, args []string) []string {
	var hosts []string
	mongo := strings.HasPrefix(name, "mongo")
	positional := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" && !mongo || a == "--host" || a == "--uri" || a == "--connection-string":
			if i+1 < len(args) {
				hosts = append(hosts, hostOf(args[i+1]))
				i++
			}
		case strings.HasPrefix(a, "--host="):
			hosts = append(hosts, hostOf(a[len("--host="):]))
		case strings.HasPrefix(a, "-h") && !strings.HasPrefix(a, "--") && !mongo && len(a) > 2:
			hosts = append(hosts, a[2:])
		case strings.Contains(a, "://"):
			hosts = append(hosts, hostOf(a))
		case strings.HasPrefix(a, "-"):
		case mongo && positional == 0 && (strings.ContainsAny(a, ".:")) && !strings.ContainsAny(a, " ()"):
			positional++
			hosts = append(hosts, hostOf(a))
		}
	}
	return hosts
}

// hostOf extracts the host from a bare host, host:port/db or a URI. Several
// comma-separated hosts come back joined, and localHost checks each.
func hostOf(s string) string {
	if _, rest, ok := strings.Cut(s, "://"); ok {
		s = rest
	}
	if i := strings.IndexAny(s, "/?"); i > 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	parts := strings.Split(s, ",")
	for i, p := range parts {
		p = strings.TrimPrefix(strings.TrimSuffix(strings.SplitN(p, "]", 2)[0], "]"), "[")
		if !strings.Contains(s, "[") {
			if j := strings.LastIndexByte(p, ':'); j >= 0 && strings.Count(p, ":") == 1 {
				p = p[:j]
			}
		}
		parts[i] = p
	}
	return strings.Join(parts, ",")
}

func localHost(h string) bool {
	for _, p := range strings.Split(h, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		local := p == "" || p == "localhost" || p == "::1" || strings.HasPrefix(p, "127.") ||
			strings.HasSuffix(p, ".localhost") || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "%2f")
		if !local {
			return false
		}
	}
	return true
}

// --- docker, kubectl ---------------------------------------------------

func dockerPrune(args []string) bool {
	pos := positionals(args, set("--context", "-c", "-H", "--host", "--config", "-l", "--log-level"))
	return len(pos) >= 2 && (pos[0] == "volume" || pos[0] == "system") && pos[1] == "prune"
}

func kubectlDelete(args []string) bool {
	pos := positionals(args, set("-n", "--namespace", "--context", "--cluster", "--kubeconfig", "--user", "-s", "--server", "--token"))
	return len(pos) > 0 && pos[0] == "delete"
}

// --- download piped into a shell ----------------------------------------

var (
	downloaders = set("curl", "wget", "wget2", "fetch", "aria2c", "http", "https", "xh")
	downloadSub = regexp.MustCompile("^\\s*(?:<\\(|\\$\\(|`)\\s*(?:sudo\\s+)?(?:curl|wget|wget2|fetch)\\s")
)

func downloadsInto(script string) bool { return downloadSub.MatchString(script) }

// readsDownload is a shell that runs what a downloader printed: a pipe from
// curl or wget, or a process substitution bash <(curl ...).
func readsDownload(words []string, cs []command, i int) bool {
	for _, w := range words[1:] {
		if downloadSub.MatchString(w) {
			return true
		}
	}
	if _, isScript := inlineScript(words, false); isScript {
		return false
	}
	for _, w := range words[1:] { // sh script.sh reads a file, not the pipe
		if !strings.HasPrefix(w, "-") {
			return false
		}
	}
	for j := i - 1; j >= 0 && cs[j].PipeNext; j-- {
		w, _ := peel(cs[j].Words)
		if len(w) > 0 && downloaders[base(w[0])] {
			return true
		}
	}
	return false
}

// --- secrets -----------------------------------------------------------

var (
	secretReaders = set("cat", "head", "tail", "less", "more", "bat", "batcat", "cp", "tac", "nl", "grep", "egrep", "fgrep", "rg")
	envFile       = regexp.MustCompile(`^\.env(rc)?([.\-_].*)?$`)
	envExample    = set(".env.example", ".env.sample", ".env.template", ".env.dist")
)

func secretPath(w string, c Context) bool {
	w = expand(w, c)
	b := path.Base(strings.ReplaceAll(w, `\`, "/"))
	if envFile.MatchString(b) && !envExample[b] {
		return true
	}
	if c.Home == "" {
		return false
	}
	p, ok := resolve(w, c)
	if !ok {
		return false
	}
	home := clean(c.Home)
	return within(p, home+"/.ssh") || within(p, home+"/.aws")
}

func readsSecret(name string, args []string, c Context) bool {
	var files []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") || a == "-" {
			files = append(files, a)
		}
	}
	switch name {
	case "cp":
		if len(files) > 0 {
			files = files[:len(files)-1] // the destination is written, not read
		}
	case "grep", "egrep", "fgrep", "rg":
		if len(files) > 0 {
			files = files[1:] // the pattern
		}
	}
	for _, f := range files {
		if secretPath(f, c) {
			return true
		}
	}
	return false
}
