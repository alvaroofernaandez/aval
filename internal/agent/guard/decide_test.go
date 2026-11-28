package guard

import (
	"errors"
	"strings"
	"testing"
)

func noAllow() ([]string, error) { return nil, nil }

func TestDecideDeniesWithARuleACommandAndAWayOut(t *testing.T) {
	d := Decide("rm -rf /", Options{Ctx: ctx, ReadAllow: noAllow})
	if !d.Deny || d.Rule != rRm {
		t.Fatalf("Decide = %+v, want a deny by %s", d, rRm)
	}
	want := "aval guard blocked this command (rm-outside-roots: rm -rf outside the project and home, or on /, ~ or the home folder). " +
		"Command: rm -rf /. If you really mean it, add the full command as one line to ~/.config/aval/private/guard.allow (a private file, never committed)."
	if d.Reason != want {
		t.Errorf("reason =\n%q\nwant\n%q", d.Reason, want)
	}
}

func TestEveryRuleHasADescription(t *testing.T) {
	for _, r := range []string{rRm, rPush, rSQL, rDocker, rKubectl, rPipe, rSecret} {
		if ruleText[r] == "" {
			t.Errorf("rule %s has no description for the reason", r)
		}
	}
}

func TestReasonTruncatesALongCommand(t *testing.T) {
	d := Decide("rm -rf / "+strings.Repeat("x", 500), Options{Ctx: ctx, ReadAllow: noAllow})
	if len([]rune(d.Reason)) > 600 || !strings.Contains(d.Reason, "...") {
		t.Errorf("reason not truncated (%d runes)", len([]rune(d.Reason)))
	}
}

func TestAllowListIsReadOnlyAfterARuleMatched(t *testing.T) {
	reads := 0
	read := func() ([]string, error) { reads++; return nil, nil }
	for _, cmd := range []string{"ls -la", "go test ./...", "git push origin feature", "rm -rf ./build", ""} {
		if d := Decide(cmd, Options{Ctx: ctx, ReadAllow: read}); d.Deny {
			t.Fatalf("%q denied", cmd)
		}
	}
	if reads != 0 {
		t.Fatalf("the no-op path read the allow-list %d times", reads)
	}
	Decide("rm -rf /", Options{Ctx: ctx, ReadAllow: read})
	if reads != 1 {
		t.Errorf("a matched rule must read the allow-list once, read %d", reads)
	}
}

func TestAnAllowEntryAllowsExactlyThatCommand(t *testing.T) {
	entries := []string{"# cache cleanup", "", "rm -rf /opt/cache"}
	read := func() ([]string, error) { return entries, nil }
	cases := []struct {
		cmd   string
		allow bool
	}{
		{"rm -rf /opt/cache", true},
		{"  rm   -rf   /opt/cache  ", true},
		{"rm -rf /opt/cache2", false},
		{"rm -rf /opt/cache /etc", false},
		{"rm -rf /opt/cache; rm -rf /", false},
		{"rm -rf /opt", false},
		{"sudo rm -rf /opt/cache", false},
	}
	for _, c := range cases {
		if d := Decide(c.cmd, Options{Ctx: ctx, ReadAllow: read}); d.Deny == c.allow {
			t.Errorf("Decide(%q).Deny = %v, want %v", c.cmd, d.Deny, !c.allow)
		}
	}
}

func TestAnUnreadableAllowListDeniesAMatchedCommand(t *testing.T) {
	read := func() ([]string, error) { return nil, errors.New("permission denied") }
	if d := Decide("rm -rf /", Options{Ctx: ctx, ReadAllow: read}); !d.Deny {
		t.Error("a matched rule with an unreadable allow-list must stay denied")
	}
}

func TestAnInternalErrorAllows(t *testing.T) {
	boom := func() ([]string, error) { panic("boom") }
	if d := Decide("rm -rf /", Options{Ctx: ctx, ReadAllow: boom}); d.Deny {
		t.Error("a panic is an internal error: the command must pass")
	}
}

func TestParseAllowList(t *testing.T) {
	got := parseAllow("# c\n\n  rm   -rf  /opt/cache  \r\nkubectl delete pod x # note kept\n")
	want := []string{"rm -rf /opt/cache", "kubectl delete pod x # note kept"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("parseAllow = %q, want %q", got, want)
	}
}
