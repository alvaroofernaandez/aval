package rateguard

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const heliceReason = "Helice (innohelp.es / helice.app) bans the IP after rapid requests. Do not crawl it: use the Helice panel via Claude in Chrome, or make 1-2 spaced requests."

var seed = []string{"innohelp.es", "helice.app"}

func TestDecide(t *testing.T) {
	cases := []struct {
		name, cmd string
		deny      bool
	}{
		// deny: a batch of HTTP requests to a guarded host
		{"for loop curl on innohelp.es", `for p in a b c d e; do curl -s "https://www.innohelp.es/$p"; done`, true},
		{"cat urls | xargs curl on helice.app", `cat urls.txt | xargs -I{} curl -s https://helice.app{}`, true},
		{"xargs curl with a guarded host in the command", `cat urls.txt | xargs curl -s --resolve helice.app:443:1.2.3.4`, true},
		{"xargs curl reading a list named after innohelp", `xargs curl -s < innohelp-urls.txt`, true},
		{"4 innohelp URLs in one curl", `curl -s https://www.innohelp.es/a https://www.innohelp.es/b https://innohelp.es/c https://innohelp.es/d`, true},
		{"while read loop with wget", `while read u; do wget "https://www.innohelp.es/$u"; done < list`, true},
		{"xargs -P on a subdomain", `seq 1 50 | xargs -P 8 -I{} curl -s https://innohelp.helice.app/p/{}`, true},
		{"parallel on a subdomain", `parallel curl -s https://innohelp.helice.app/{} ::: a b c`, true},
		{"until loop with httpie", `until http GET https://helice.app/health; do :; done`, true},
		{"wget -i list on a guarded host", `wget -i innohelp.txt --base=https://www.innohelp.es/`, true},
		{"curl -K config with a guarded host", `curl -K innohelp.cfg https://innohelp.es/`, true},
		{"absolute client path in a loop", `for i in 1 2; do /usr/bin/curl https://innohelp.es/$i; done`, true},
		{"uppercase host in a loop", `for i in 1 2; do curl https://WWW.InnoHelp.ES/$i; done`, true},

		// allow: one-off requests, other hosts, no client
		{"a single curl", `curl -sL https://www.innohelp.es/`, false},
		{"curl -i on one URL is not an input file", `curl -i https://www.innohelp.es/`, false},
		{"3 URLs in one curl", `curl -s https://innohelp.es/a https://innohelp.es/b https://helice.app/c`, false},
		{"a loop over an unrelated host", `for p in a b c; do curl -s "https://example.com/$p"; done`, false},
		{"lookalike notinnohelp.es in a loop", `for p in a b c; do curl -s "https://notinnohelp.es/$p"; done`, false},
		{"lookalike suffix host in a loop", `for p in a b; do curl -s "https://innohelp.es.evil.com/$p"; done`, false},
		{"echo a guarded host", `echo innohelp.es`, false},
		{"grep a guarded host", `grep helice.app file`, false},
		{"grep a guarded host in a loop, no client", `for f in *.log; do grep helice.app "$f"; done`, false},
		{"xargs curl with nothing guarded visible", `cat urls.txt | xargs curl -s`, false},
		{"an empty command", ``, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deny, reason := Decide(c.cmd, seed)
			if deny != c.deny {
				t.Fatalf("Decide(%q) deny = %v, want %v", c.cmd, deny, c.deny)
			}
			if deny && reason != heliceReason {
				t.Errorf("reason = %q, want %q", reason, heliceReason)
			}
			if !deny && reason != "" {
				t.Errorf("allow carries a reason: %q", reason)
			}
		})
	}
}

func TestDecideNamesAUserAddedHost(t *testing.T) {
	deny, reason := Decide(`for i in 1 2 3; do curl https://api.shop.test/$i; done`, append(seed, "shop.test"))
	if !deny {
		t.Fatal("a loop on a user-added host must be denied")
	}
	want := "shop.test bans the IP after rapid requests. Do not crawl it: use its web UI via Claude in Chrome, or make 1-2 spaced requests."
	if reason != want {
		t.Errorf("reason = %q, want %q", reason, want)
	}
}

func TestDecideWithNoHostsAllows(t *testing.T) {
	if deny, _ := Decide(`for i in 1 2; do curl https://innohelp.es/$i; done`, nil); deny {
		t.Error("with no guarded hosts nothing is denied")
	}
}

func TestMatchesHost(t *testing.T) {
	cases := []struct {
		host, guard string
		want        bool
	}{
		{"innohelp.es", "innohelp.es", true},
		{"www.innohelp.es", "innohelp.es", true},
		{"a.b.innohelp.es", "innohelp.es", true},
		{"innohelp.helice.app", "helice.app", true},
		{"notinnohelp.es", "innohelp.es", false},
		{"innohelp.es.evil.com", "innohelp.es", false},
		{"innohelp.e", "innohelp.es", false},
	}
	for _, c := range cases {
		if got := matchesHost(c.host, c.guard); got != c.want {
			t.Errorf("matchesHost(%q, %q) = %v, want %v", c.host, c.guard, got, c.want)
		}
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := m[p]; ok {
			return []byte(s), nil
		}
		return nil, fs.ErrNotExist
	}
}

func TestHosts(t *testing.T) {
	home := filepath.Join("/", "home", "u")
	xdgFile := filepath.Join("/", "x", "aval", "rate-guard-hosts")
	homeFile := filepath.Join(home, ".config", "aval", "rate-guard-hosts")
	winFile := filepath.Join(`C:\AppData`, "aval", "rate-guard-hosts")
	content := "# guarded\nshop.test   # the shop\n\n  https://API.Other.test/  \n*.wild.test\nhelice.app\n"
	cases := []struct {
		name  string
		env   map[string]string
		files map[string]string
		goos  string
		want  []string
	}{
		{"seed only", nil, nil, "linux", []string{"innohelp.es", "helice.app"}},
		{"file under XDG_CONFIG_HOME, comments and dedupe", map[string]string{"XDG_CONFIG_HOME": filepath.Join("/", "x")},
			map[string]string{xdgFile: content}, "linux",
			[]string{"innohelp.es", "helice.app", "shop.test", "api.other.test", "wild.test"}},
		{"file under ~/.config", nil, map[string]string{homeFile: "shop.test\n"}, "darwin",
			[]string{"innohelp.es", "helice.app", "shop.test"}},
		{"file under APPDATA on Windows", map[string]string{"APPDATA": `C:\AppData`}, map[string]string{winFile: "shop.test\r\n"}, "windows",
			[]string{"innohelp.es", "helice.app", "shop.test"}},
		{"env var, comma separated", map[string]string{"AVAL_RATE_GUARD_HOSTS": " a.test, ,B.test ,innohelp.es"}, nil, "linux",
			[]string{"innohelp.es", "helice.app", "a.test", "b.test"}},
		{"file and env together", map[string]string{"AVAL_RATE_GUARD_HOSTS": "e.test"}, map[string]string{homeFile: "f.test"}, "linux",
			[]string{"innohelp.es", "helice.app", "f.test", "e.test"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Hosts(env(c.env), files(c.files), c.goos, home)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Hosts = %v, want %v", got, c.want)
			}
		})
	}
}

func TestHostsUnreadableFileIsAnError(t *testing.T) {
	boom := func(string) ([]byte, error) { return nil, errors.New("permission denied") }
	if _, err := Hosts(env(nil), boom, "linux", "/home/u"); err == nil {
		t.Error("an unreadable (not missing) hosts file must be reported so the hook can fail open")
	}
}

// The hook is a hot path: it must not load ~/.config/aval/config, exec or
// touch the network. Its only imports are the standard library, minus
// os/exec and net.
func TestRateGuardImportsOnlySafeStdlib(t *testing.T) {
	fset := token.NewFileSet()
	matches, _ := filepath.Glob("*.go")
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(strings.SplitN(p, "/", 2)[0], ".") || p == "os/exec" || strings.HasPrefix(p, "net") {
				t.Errorf("%s imports %s", f, p)
			}
		}
	}
}
