package doctor

// Security checks (aval doctor --security). They are off by default because
// they judge policy, not breakage, and a deliberate choice (a broad allow
// entry, an MCP server wired with a literal token) is the user's to make.
// Every finding can be suppressed:
//
//	aval doctor --security --allow <check-id>[:<subject>]
//	~/.config/aval/private/doctor-allow   (one "<check-id>[:<subject>]" per line)
//
// Secret values are never printed: findings name the server and the key only.

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	idPermissionsBroad = "sec-permissions-broad"
	idHooksRemoteExec  = "sec-hooks-remote-exec"
	idHooksNpxLatest   = "sec-hooks-npx-latest"
	idHooksOutside     = "sec-hooks-outside"
	idMCPSecrets       = "sec-mcp-inline-secrets"
)

// finding is one problem; subject is what an allow entry can name.
type finding struct{ subject, text string }

type secCheck struct {
	id       string
	status   Status
	okMsg    string
	failFmt  string
	findings []finding
}

func checkSecurity(opt Options) []Result {
	settings := securitySettings(opt)
	allow := loadAllow(opt)

	checks := []secCheck{
		{id: idPermissionsBroad, status: Fail, okMsg: "no wildcard-broad permissions.allow entries",
			failFmt: "wildcard-broad permissions.allow", findings: broadPermissions(settings)},
		{id: idHooksRemoteExec, status: Fail, okMsg: "no hook downloads and executes remote code",
			failFmt: "hook downloads and executes remote code", findings: remoteExecHooks(settings)},
		{id: idHooksNpxLatest, status: Warn, okMsg: "no hook runs an unpinned npx @latest package",
			failFmt: "hook runs an unpinned npx @latest package", findings: npxLatestHooks(settings)},
		{id: idHooksOutside, status: Warn, okMsg: "hook scripts live in the repo or ~/.claude/hooks",
			failFmt: "hook script outside the repo and ~/.claude/hooks", findings: outsideHooks(opt, settings)},
		{id: idMCPSecrets, status: Fail, okMsg: "no inline secrets in MCP server config",
			failFmt: "MCP server with an inline secret (value not shown)", findings: mcpSecrets(opt)},
	}

	var out []Result
	for _, c := range checks {
		var kept, suppressed []finding
		for _, f := range c.findings {
			if allow[c.id] || allow[c.id+":"+f.subject] {
				suppressed = append(suppressed, f)
			} else {
				kept = append(kept, f)
			}
		}
		switch {
		case len(kept) > 0:
			texts := make([]string, len(kept))
			for i, f := range kept {
				texts[i] = f.text
			}
			out = append(out, result(c.id, c.status, false, "%s: %s (suppress: aval doctor --security --allow %s)",
				c.failFmt, strings.Join(texts, ", "), c.id))
		case len(suppressed) > 0:
			out = append(out, result(c.id, OK, false, "%d finding(s) allowed by the allow list", len(suppressed)))
		default:
			out = append(out, result(c.id, OK, false, "%s", c.okMsg))
		}
	}
	return out
}

// loadAllow merges --allow with the private overlay's doctor-allow file.
func loadAllow(opt Options) map[string]bool {
	allow := map[string]bool{}
	for _, a := range opt.Allow {
		if a = strings.TrimSpace(a); a != "" {
			allow[a] = true
		}
	}
	if opt.PrivateDir == "" {
		return allow
	}
	f, err := os.Open(filepath.Join(opt.PrivateDir, "doctor-allow"))
	if err != nil {
		return allow
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			allow[line] = true
		}
	}
	return allow
}

// securitySettings is every settings file whose content reaches the agent:
// the live one and the repo one that sync pushes to every machine.
func securitySettings(opt Options) []map[string]any {
	var out []map[string]any
	for _, p := range []string{
		filepath.Join(opt.Home, ".claude", "settings.json"),
		filepath.Join(opt.Repo, "config", "claude", "settings.json"),
	} {
		if m, err := readJSON(p); err == nil {
			out = append(out, m)
		}
	}
	return out
}

// --- permissions ------------------------------------------------------------

var (
	broadTools   = map[string]bool{"Bash": true, "Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true, "WebFetch": true, "PowerShell": true}
	broadPattern = regexp.MustCompile(`^[A-Za-z_]+\((\*+|:\*|\*:\*)?\)$`)
)

func isBroadPermission(e string) bool {
	e = strings.TrimSpace(e)
	if e == "*" || e == "**" {
		return true
	}
	if broadTools[e] {
		return true
	}
	return broadPattern.MatchString(e)
}

func broadPermissions(settings []map[string]any) []finding {
	seen := map[string]bool{}
	var out []finding
	for _, s := range settings {
		perms, _ := s["permissions"].(map[string]any)
		allow, _ := perms["allow"].([]any)
		for _, a := range allow {
			e, _ := a.(string)
			if isBroadPermission(e) && !seen[e] {
				seen[e] = true
				out = append(out, finding{subject: e, text: e})
			}
		}
	}
	return out
}

// --- hooks ------------------------------------------------------------------

func allHookCommands(settings []map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range settings {
		for _, h := range hookCommands(s) {
			if !seen[h.command] {
				seen[h.command] = true
				out = append(out, h.command)
			}
		}
	}
	sort.Strings(out)
	return out
}

var (
	pipeToShell  = regexp.MustCompile(`(?i)\b(curl|wget)\b[^|;&\n]*\|\s*(sudo\s+(-\S+\s+)*)?(ba|z|da)?sh\b`)
	shellOfFetch = regexp.MustCompile(`(?i)\b(ba|z|da)?sh\b[^|;&\n]*(\$\(|<\(|` + "`" + `)\s*(curl|wget)\b`)
	npxLatest    = regexp.MustCompile(`\bnpx\b\s+(-\S+\s+)*\S+@latest\b`)
)

func shortCmd(c string) string {
	c = strings.Join(strings.Fields(c), " ")
	if len(c) > 60 {
		c = c[:57] + "..."
	}
	return c
}

func remoteExecHooks(settings []map[string]any) []finding {
	var out []finding
	for _, c := range allHookCommands(settings) {
		if pipeToShell.MatchString(c) || shellOfFetch.MatchString(c) {
			out = append(out, finding{subject: shortCmd(c), text: "`" + shortCmd(c) + "`"})
		}
	}
	return out
}

func npxLatestHooks(settings []map[string]any) []finding {
	var out []finding
	for _, c := range allHookCommands(settings) {
		if npxLatest.MatchString(c) {
			out = append(out, finding{subject: shortCmd(c), text: "`" + shortCmd(c) + "`"})
		}
	}
	return out
}

var interpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true,
	"node": true, "python": true, "python3": true, "ruby": true, "bun": true, "deno": true}

// systemDirs hold the OS's own tools, not user scripts.
var systemDirs = []string{"/bin/", "/usr/bin/", "/sbin/", "/usr/sbin/"}

// scriptPaths lists the files a hook command runs by path: the command itself
// when it has a directory part, or the script handed to an interpreter.
func scriptPaths(cmd, home string) []string {
	var out []string
	atStart, interp := true, false
	for _, t := range tokenize(cmd) {
		if t.op {
			atStart, interp = true, false
			continue
		}
		w := expand(t.text, home)
		switch {
		case atStart:
			if keywords[t.text] || assignment.MatchString(t.text) {
				continue
			}
			atStart = false
			if interpreters[filepath.Base(w)] {
				interp = true
			}
			if strings.ContainsAny(w, `/\`) && !interpreters[filepath.Base(w)] {
				out = append(out, w)
			}
		case interp && !strings.HasPrefix(t.text, "-"):
			interp = false
			if strings.ContainsAny(w, `/\`) {
				out = append(out, w)
			}
		}
	}
	return out
}

func within(path string, roots ...string) bool {
	for _, r := range roots {
		if r == "" {
			continue
		}
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func outsideHooks(opt Options, settings []map[string]any) []finding {
	trusted := []string{opt.Repo, filepath.Join(opt.Home, ".claude", "hooks")}
	if r, err := filepath.EvalSymlinks(opt.Repo); err == nil {
		trusted = append(trusted, r)
	}
	var out []finding
	seen := map[string]bool{}
	for _, c := range allHookCommands(settings) {
		for _, p := range scriptPaths(c, opt.Home) {
			p = filepath.Clean(p)
			sys := false
			for _, d := range systemDirs {
				if strings.HasPrefix(p, d) {
					sys = true
				}
			}
			if sys || within(p, trusted...) || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, finding{subject: p, text: tilde(opt.Home, p)})
		}
	}
	return out
}

// --- MCP servers ------------------------------------------------------------

var (
	secretName = regexp.MustCompile(`(?i)(token|secret|key|passw(or)?d)`)
	flagEq     = regexp.MustCompile(`^-{1,2}([A-Za-z0-9_.-]+)=(.*)$`)
	envEq      = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
)

// isReference reports whether a value defers to the environment instead of
// holding the secret itself.
func isReference(v string) bool {
	return v == "" || strings.Contains(v, "${") || strings.HasPrefix(v, "$")
}

func literalSecret(name, value string) bool {
	return secretName.MatchString(name) && !isReference(value)
}

// serverSecretKeys names the config keys of one MCP server that hold a
// literal secret. Values are inspected, never returned.
func serverSecretKeys(srv map[string]any) []string {
	var keys []string
	for _, field := range []string{"env", "headers"} {
		m, _ := srv[field].(map[string]any)
		for k, v := range m {
			if s, ok := v.(string); ok && literalSecret(k, s) {
				keys = append(keys, k)
			}
		}
	}
	args, _ := srv["args"].([]any)
	for i, a := range args {
		s, _ := a.(string)
		if m := flagEq.FindStringSubmatch(s); m != nil {
			if literalSecret(m[1], m[2]) {
				keys = append(keys, "--"+m[1])
			}
			continue
		}
		if m := envEq.FindStringSubmatch(s); m != nil {
			if literalSecret(m[1], m[2]) {
				keys = append(keys, m[1])
			}
			continue
		}
		if strings.HasPrefix(s, "-") && secretName.MatchString(s) && i+1 < len(args) {
			next, _ := args[i+1].(string)
			if !strings.HasPrefix(next, "-") && !isReference(next) {
				keys = append(keys, strings.TrimLeft(s, "-")) // flag name only
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func mcpSecrets(opt Options) []finding {
	var out []finding
	scan := func(origin string, servers map[string]any) {
		names := make([]string, 0, len(servers))
		for n := range servers {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			srv, _ := servers[n].(map[string]any)
			for _, k := range serverSecretKeys(srv) {
				out = append(out, finding{subject: n, text: n + " (" + origin + ") " + k})
			}
		}
	}

	projectFiles := map[string]bool{filepath.Join(opt.Repo, ".mcp.json"): true}
	claudeJSON := filepath.Join(opt.Home, ".claude.json")
	if m, err := readJSON(claudeJSON); err == nil {
		servers, _ := m["mcpServers"].(map[string]any)
		scan("~/.claude.json", servers)
		projects, _ := m["projects"].(map[string]any)
		paths := make([]string, 0, len(projects))
		for p := range projects {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			pm, _ := projects[p].(map[string]any)
			ps, _ := pm["mcpServers"].(map[string]any)
			scan("~/.claude.json project "+tilde(opt.Home, p), ps)
			projectFiles[filepath.Join(p, ".mcp.json")] = true
		}
	}

	files := make([]string, 0, len(projectFiles))
	for f := range projectFiles {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if m, err := readJSON(f); err == nil {
			servers, _ := m["mcpServers"].(map[string]any)
			scan(tilde(opt.Home, f), servers)
		}
	}
	return out
}
