// Package rateguard is the PreToolUse hook that stops an agent from sending a
// burst of HTTP requests to hosts that ban the client IP after rapid requests
// (Helice: innohelp.es and helice.app). It denies a Bash command only when an
// HTTP client contacts a guarded host AND the command is a batch: a loop,
// xargs/parallel, a -P flag, more than 3 guarded URLs, or a URL list fed to
// the client. One-off requests and loops over other hosts pass.
//
// It is a hot path: pure string work, no exec, no network, and it never loads
// ~/.config/aval/config. It fails open: any doubt means no output.
package rateguard

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

// Seed is the built-in list of guarded hosts.
var Seed = []string{"innohelp.es", "helice.app"}

// HeliceReason is the deny reason when a seed host is involved.
const HeliceReason = "Helice (innohelp.es / helice.app) bans the IP after rapid requests. Do not crawl it: use the Helice panel via Claude in Chrome, or make 1-2 spaced requests."

var clients = map[string]bool{
	"curl": true, "wget": true, "wget2": true, "http": true, "https": true,
	"xh": true, "aria2c": true, "lynx": true, "links": true, "w3m": true,
}

var loopWords = map[string]bool{"for": true, "while": true, "until": true, "xargs": true, "parallel": true}

// maxURLs is how many guarded URLs one command may name.
const maxURLs = 3

// Decide reports whether command must be denied, and why.
func Decide(command string, hosts []string) (bool, string) {
	if strings.TrimSpace(command) == "" || len(hosts) == 0 {
		return false, ""
	}
	words := splitWords(command)
	hasClient, batch, listFeed, wgetLike := false, false, false, false
	for _, w := range words {
		name := w
		if !strings.Contains(w, "://") {
			name = w[strings.LastIndex(w, "/")+1:]
		}
		if clients[name] {
			hasClient = true
			if name == "wget" || name == "wget2" || name == "aria2c" {
				wgetLike = true
			}
		}
		switch {
		case loopWords[w]:
			batch = true
			if w == "xargs" {
				listFeed = true
			}
		case isParallelFlag(w):
			batch = true
		case w == "--input-file" || strings.HasPrefix(w, "--input-file=") ||
			w == "-K" || strings.HasPrefix(w, "-K") && len(w) > 2 ||
			w == "--config" || strings.HasPrefix(w, "--config="):
			listFeed = true
		}
	}
	if !hasClient {
		return false, ""
	}
	// -i is wget's --input-file but curl's --include: only the former is a list.
	if wgetLike {
		for _, w := range words {
			if w == "-i" || strings.HasPrefix(w, "-i") && len(w) > 2 && !strings.HasPrefix(w, "-i-") {
				listFeed = true
			}
		}
	}

	matched, count := "", 0
	for _, h := range hostTokens(command) {
		for _, g := range hosts {
			if matchesHost(h, g) {
				count++
				if matched == "" {
					matched = g
				}
				break
			}
		}
	}
	// A URL list read from a file hides its hosts: a file named after a
	// guarded host (innohelp-urls.txt) is evidence enough.
	if matched == "" && listFeed {
		matched = labelMention(command, hosts)
	}
	if matched == "" {
		return false, ""
	}
	if !(batch || listFeed || count > maxURLs) {
		return false, ""
	}
	return true, reasonFor(matched)
}

func reasonFor(host string) string {
	for _, s := range Seed {
		if host == s {
			return HeliceReason
		}
	}
	return host + " bans the IP after rapid requests. Do not crawl it: use its web UI via Claude in Chrome, or make 1-2 spaced requests."
}

// isParallelFlag is -P, -P8 or --max-procs (xargs and friends).
func isParallelFlag(w string) bool {
	if w == "-P" || w == "--max-procs" || strings.HasPrefix(w, "--max-procs=") {
		return true
	}
	if strings.HasPrefix(w, "-P") && len(w) > 2 {
		for _, r := range w[2:] {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	return false
}

// splitWords is a loose shell tokenizer: whitespace and operators separate
// words, quotes are dropped. Precision is not needed, only recall of the
// command names and flags.
func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ';', '|', '&', '(', ')', '<', '>', '`', '"', '\'', '{', '}':
			return true
		}
		return false
	})
}

// hostTokens returns every maximal run of hostname characters, lowercased and
// trimmed of dots and hyphens, that contains a dot.
func hostTokens(s string) []string {
	var out []string
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		t := strings.ToLower(strings.Trim(s[start:end], ".-"))
		if strings.Contains(t, ".") {
			out = append(out, t)
		}
		start = -1
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(s))
	return out
}

// matchesHost: host is guard itself or one of its subdomains.
func matchesHost(host, guard string) bool {
	return host == guard || strings.HasSuffix(host, "."+guard)
}

// labelMention finds a guarded host whose name without the TLD (innohelp for
// innohelp.es) appears as a whole word-part in s.
func labelMention(s string, hosts []string) string {
	ls := strings.ToLower(s)
	for _, g := range hosts {
		i := strings.LastIndex(g, ".")
		if i < 4 {
			continue
		}
		label := g[:i]
		for off := 0; ; {
			j := strings.Index(ls[off:], label)
			if j < 0 {
				break
			}
			j += off
			end := j + len(label)
			if (j == 0 || !alnum(ls[j-1])) && (end == len(ls) || !alnum(ls[end])) {
				return g
			}
			off = j + 1
		}
	}
	return ""
}

func alnum(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' }

// Hosts is the guarded list: the seed, then the hosts file, then
// AVAL_RATE_GUARD_HOSTS (comma-separated). A missing file is fine; any other
// read error is returned so the hook can fail open.
func Hosts(getenv func(string) string, readFile func(string) ([]byte, error), goos, home string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		h = normalize(h)
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, h := range Seed {
		add(h)
	}
	b, err := readFile(HostsFile(getenv, goos, home))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		add(line)
	}
	for _, h := range strings.Split(getenv("AVAL_RATE_GUARD_HOSTS"), ",") {
		add(h)
	}
	return out, nil
}

// HostsFile is ${XDG_CONFIG_HOME:-~/.config}/aval/rate-guard-hosts, or
// %APPDATA%\aval\rate-guard-hosts on Windows (same rule as the aval config).
func HostsFile(getenv func(string) string, goos, home string) string {
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "aval", "rate-guard-hosts")
	}
	if goos == "windows" {
		if a := getenv("APPDATA"); a != "" {
			return filepath.Join(a, "aval", "rate-guard-hosts")
		}
	}
	return filepath.Join(home, ".config", "aval", "rate-guard-hosts")
}

func normalize(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/:"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimPrefix(h, "*")
	return strings.Trim(h, ".")
}

// Run reads a PreToolUse payload on in and writes a deny to out when the Bash
// command is a guarded batch. Anything unexpected, a panic included, writes
// nothing.
func Run(in io.Reader, out io.Writer, hosts func() ([]string, error)) {
	defer func() { _ = recover() }()
	raw, err := io.ReadAll(in)
	if err != nil {
		return
	}
	var p struct {
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if json.Unmarshal(raw, &p) != nil || p.ToolName != "Bash" || p.ToolInput.Command == "" {
		return
	}
	hs, err := hosts()
	if err != nil {
		return
	}
	deny, reason := Decide(p.ToolInput.Command, hs)
	if !deny {
		return
	}
	type specific struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{specific{"PreToolUse", "deny", reason}})
}
