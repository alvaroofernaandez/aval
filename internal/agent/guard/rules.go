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
type Context struct {
	Cwd, Home string
	// Tmp lists extra temp roots (the resolved $TMPDIR); the fixed ones are
	// /tmp, /private/tmp and /var/folders.
	Tmp []string
	// Real resolves symlinks of a path; nil means the lexical path is trusted.
	Real func(string) string
}

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

// scene is one simple command with everything a rule may look at.
type scene struct {
	name  string
	args  []string
	env   map[string]string
	cs    []command
	i     int
	c     Context
	words []string
}

// rules are tried in order; the first that applies names the finding.
var rules = []struct {
	name    string
	applies func(s scene) bool
}{
	{ruleRm, func(s scene) bool { return s.name == "rm" && rmOutsideRoots(s.args, s.c) }},
	{rulePush, func(s scene) bool { return s.name == "git" && forcePushesProtected(s.args) }},
	{ruleSQL, func(s scene) bool {
		return sqlTools[s.name] && destroysRemoteData(s.name, s.args, s.env, s.cs, s.i)
	}},
	{ruleDocker, func(s scene) bool { return (s.name == "docker" || s.name == "podman") && dockerPrune(s.args) }},
	{ruleKubectl, func(s scene) bool { return (s.name == "kubectl" || s.name == "oc") && kubectlDelete(s.args) }},
	{rulePipe, func(s scene) bool { return shells[s.name] && readsDownload(s.words, s.cs, s.i) }},
	{ruleSecret, func(s scene) bool { return secretReaders[s.name] && readsSecret(s.name, s.args, s.c) }},
}

func checkCommand(cs []command, i int, c Context, depth int, lenient bool) (Finding, bool) {
	text := strings.Join(cs[i].Words, " ")
	for _, r := range cs[i].Redirs {
		if r.Op == "<" && secretPath(r.Target, c) {
			return Finding{ruleSecret, text}, true
		}
	}
	words, env := peel(cs[i].Words)
	if len(words) == 0 {
		return Finding{}, false
	}
	if script, ok := inlineScript(words, lenient); ok {
		if downloadsInto(script) {
			return Finding{rulePipe, text}, true
		}
		if f, ok := check(script, c, depth+1); ok {
			return f, true
		}
	}
	s := scene{name: base(words[0]), args: words[1:], env: env, cs: cs, i: i, c: c, words: words}
	for _, r := range rules {
		if r.applies(s) {
			return Finding{r.name, text}, true
		}
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

// rmTargets reads rm's arguments: whether it is recursive, and what it removes.
func rmTargets(args []string) (recursive bool, targets []string) {
	for i, a := range args {
		switch {
		case a == "--":
			return recursive, append(targets, args[i+1:]...)
		case a == "--recursive":
			recursive = true
		case strings.HasPrefix(a, "--"):
		case strings.HasPrefix(a, "-") && len(a) > 1:
			recursive = recursive || strings.ContainsAny(a, "rR")
		default:
			targets = append(targets, a)
		}
	}
	return recursive, targets
}

func rmOutsideRoots(args []string, c Context) bool {
	recursive, targets := rmTargets(args)
	if !recursive {
		return false
	}
	for _, t := range targets {
		if rmTargetRisky(t, c) {
			return true
		}
	}
	return false
}

// rmTargetRisky: the filesystem root, the home folder, the project itself,
// anything outside both roots, or a glob directly under / or home.
func rmTargetRisky(t string, c Context) bool {
	p, ok := resolve(t, c)
	if !ok {
		return c.Cwd == "" && strings.HasPrefix(t, "..")
	}
	home, cwd := clean(c.Home), clean(c.Cwd)
	switch {
	case p == "/", c.Home != "" && p == home, c.Cwd != "" && p == cwd:
		return true
	case strings.ContainsAny(path.Base(p), "*?[") && (path.Dir(p) == "/" || c.Home != "" && path.Dir(p) == home):
		return true
	}
	if inTemp(p, c) {
		return false
	}
	return !within(p, cwd) && !(c.Home != "" && within(p, home))
}

var tempRoots = []string{"/tmp", "/private/tmp", "/var/folders"}

func (c Context) tempRoots() []string { return append(append([]string{}, tempRoots...), c.Tmp...) }

func (c Context) real(p string) string {
	if c.Real == nil {
		return p
	}
	return clean(c.Real(p))
}

// strictlyIn: p lies below dir, never dir itself.
func strictlyIn(p, dir string) bool { return p != dir && within(p, dir) }

// inTemp: p is below a temp root, lexically and after symlinks, and is not a
// glob that would sweep a whole root. The root itself is never a target.
func inTemp(p string, c Context) bool {
	roots := c.tempRoots()
	var hit string
	for _, r := range roots {
		if r = clean(r); strictlyIn(p, r) && (hit == "" || len(r) > len(hit)) {
			hit = r
		}
	}
	if hit == "" || strings.ContainsAny(path.Base(p), "*?[") && path.Dir(p) == hit {
		return false
	}
	real := c.real(p)
	for _, r := range roots {
		if r = clean(r); strictlyIn(real, r) || strictlyIn(real, c.real(r)) {
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
	force := pushForces(args)
	for _, p := range pos[1:] {
		plus := strings.HasPrefix(p, "+")
		if (force || plus) && protectedRef(strings.TrimPrefix(p, "+")) {
			return true
		}
	}
	return false
}

// pushForces: --force, -f (alone or in a cluster such as -fu) or
// --force-with-lease, among the options after the push subcommand.
func pushForces(args []string) bool {
	after := false
	for _, a := range args {
		switch {
		case !after:
			after = a == "push"
		case a == "--force", a == "-f", strings.HasPrefix(a, "--force-with-lease"):
			return true
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsRune(a, 'f'):
			return true
		}
	}
	return false
}

// protectedRef is main or master as the destination of a refspec.
func protectedRef(ref string) bool {
	if i := strings.LastIndexByte(ref, ':'); i >= 0 {
		ref = ref[i+1:]
	}
	ref = strings.TrimPrefix(ref, "refs/heads/")
	return ref == "main" || ref == "master"
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
	seenPositional := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if h, takesNext, ok := hostFlag(a, mongo); ok {
			if takesNext {
				if i+1 >= len(args) {
					continue
				}
				i++
				h = hostOf(args[i])
			}
			hosts = append(hosts, h)
			continue
		}
		if strings.Contains(a, "://") {
			hosts = append(hosts, hostOf(a))
		} else if mongo && !seenPositional && !strings.HasPrefix(a, "-") && strings.ContainsAny(a, ".:") && !strings.ContainsAny(a, " ()") {
			seenPositional = true
			hosts = append(hosts, hostOf(a))
		}
	}
	return hosts
}

// hostFlag recognises the options that name a host: -h and --host (the value
// is the next word or attached), --uri, --connection-string.
func hostFlag(a string, mongo bool) (host string, takesNext, ok bool) {
	switch {
	case a == "--host" || a == "--uri" || a == "--connection-string" || a == "-h" && !mongo:
		return "", true, true
	case strings.HasPrefix(a, "--host="):
		return hostOf(a[len("--host="):]), false, true
	case strings.HasPrefix(a, "-h") && !strings.HasPrefix(a, "--") && !mongo && len(a) > 2:
		return a[2:], false, true
	}
	return "", false, false
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
