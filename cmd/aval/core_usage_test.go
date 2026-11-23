package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// A bare `aval` prints this instead of installing, so it must point at the
// install command, the repo override and every subcommand.
func TestUsage(t *testing.T) {
	var out bytes.Buffer
	printUsage(&out, englishEnv)
	text := out.String()
	for _, want := range []string{"Usage:", "aval install", "--dry-run", "--yes", "--copy", "--repo", "AVAL_REPO"} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not mention %q:\n%s", want, text)
		}
	}
	for _, c := range commands {
		if !strings.Contains(text, "aval "+c.name) {
			t.Errorf("usage does not list %q", c.name)
		}
	}
}

func TestKnownCommand(t *testing.T) {
	for _, name := range []string{"install", "update", "doctor", "sync", "uninstall", "version", "release", "hook", "dev", "help"} {
		if !knownCommand(name) {
			t.Errorf("%q should be known", name)
		}
	}
	for _, name := range []string{"instal", "nope", "--yes"} {
		if knownCommand(name) {
			t.Errorf("%q should be unknown", name)
		}
	}
}

func TestParseInstallArgs(t *testing.T) {
	cases := []struct {
		name    string
		base    installOpts
		args    []string
		want    installOpts
		wantErr bool
	}{
		{"none", installOpts{}, nil, installOpts{}, false},
		{"all after the subcommand", installOpts{}, []string{"--yes", "--dry-run", "--copy"}, installOpts{dryRun: true, yes: true, copy: true}, false},
		{"single dash", installOpts{}, []string{"-yes"}, installOpts{yes: true}, false},
		{"global flags carry over", installOpts{dryRun: true}, []string{"--yes"}, installOpts{dryRun: true, yes: true}, false},
		{"unknown flag", installOpts{}, []string{"--force"}, installOpts{}, true},
		{"stray argument", installOpts{}, []string{"extra"}, installOpts{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseInstallArgs(c.base, c.args)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil {
				if !isUsage(err) {
					t.Errorf("err = %v, want a usage error", err)
				}
				return
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func renderForTest(version string, configured bool) string {
	var out bytes.Buffer
	renderUsage(&out, i18n.New(i18n.English), version, configured)
	return out.String()
}

func TestUsageLockup(t *testing.T) {
	lines := strings.Split(renderForTest("1.2.3", true), "\n")
	if lines[0] != "aval 1.2.3" || lines[1] != "Nothing passes without evidence." {
		t.Errorf("lockup = %q, %q", lines[0], lines[1])
	}
}

func TestUsageGroupsInOrder(t *testing.T) {
	text := renderForTest("dev", true)
	last := -1
	for _, g := range usageGroups {
		i := strings.Index(text, "\n"+g+"\n")
		if i < 0 {
			t.Fatalf("missing group heading %q:\n%s", g, text)
		}
		if i < last {
			t.Errorf("group %q is out of order", g)
		}
		last = i
	}
	want := []string{"Setup", "Health", "Undo", "Agent tooling", "Plumbing"}
	if strings.Join(usageGroups, ",") != strings.Join(want, ",") {
		t.Errorf("groups = %v, want %v", usageGroups, want)
	}
	if !strings.Contains(text, "Called by hooks and scripts") {
		t.Error("plumbing has no note")
	}
}

func TestEveryCommandHasAGroupAndIsListedOnce(t *testing.T) {
	text := renderForTest("dev", true)
	for _, c := range commands {
		ok := false
		for _, g := range usageGroups {
			ok = ok || c.group == g
		}
		if !ok {
			t.Errorf("%q has unknown group %q", c.name, c.group)
		}
		if n := strings.Count(text, "  aval "+c.name+" "); n != 1 {
			t.Errorf("%q listed %d times", c.name, n)
		}
	}
}

func TestUsageFirstRunHint(t *testing.T) {
	const hint = "Not set up yet. Run: aval install"
	if got := renderForTest("dev", false); !strings.HasSuffix(got, hint+"\n") {
		t.Errorf("no hint at the end:\n%s", got)
	}
	if got := renderForTest("dev", true); strings.Contains(got, "Not set up yet") {
		t.Error("hint shown although a config exists")
	}
}

func TestUsageCopyRules(t *testing.T) {
	text := renderForTest("dev", false)
	if strings.Contains(text, "!") || strings.Contains(text, "\u2014") || strings.Contains(text, "\x1b") {
		t.Errorf("usage breaks the copy rules:\n%s", text)
	}
}

func TestUsageDocumentsThemeAndPlain(t *testing.T) {
	var b strings.Builder
	printUsage(&b, englishEnv)
	for _, want := range []string{"--theme <name>", "--plain", "NO_COLOR"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("usage lacks %q", want)
		}
	}
}

// isUsage reports whether err is a command called with arguments it does not
// take: the error that prints the command's usage.
func isUsage(err error) bool {
	var u *usageError
	return errors.As(err, &u)
}

// englishTr is the English translator the logic tests run the commands with;
// the wording itself is pinned in tests/messages_test.go.
var englishTr = i18n.New(i18n.English)

// englishEnv is the process environment with the language pinned, so a usage
// assertion does not depend on the machine's locale.
func englishEnv(k string) string {
	if k == "AVAL_LANG" {
		return "en"
	}
	return os.Getenv(k)
}
