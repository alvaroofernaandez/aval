package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedHereNamesTheCommandAndWSL(t *testing.T) {
	for cmd := range unixOnly {
		err := unsupportedHere("windows", cmd)
		if err == nil {
			t.Errorf("aval %s should be refused on windows", cmd)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "aval "+cmd) || !strings.Contains(msg, "not supported on windows") || !strings.Contains(msg, "WSL") {
			t.Errorf("unclear message for %s: %q", cmd, msg)
		}
		for _, goos := range []string{"darwin", "linux"} {
			if err := unsupportedHere(goos, cmd); err != nil {
				t.Errorf("aval %s must run on %s, got %v", cmd, goos, err)
			}
		}
	}
	// What does work natively must never be refused.
	for _, cmd := range []string{"doctor", "sync", "restore", "uninstall", "hook", "statusline", "budget", "review", "design", "update", "pdf", ""} {
		if err := unsupportedHere("windows", cmd); err != nil {
			t.Errorf("aval %s is supported on windows, got %v", cmd, err)
		}
	}
}

// Copy mode is what Windows runs without symlink rights; it is tested here on
// every OS by forcing it (--copy), so CI on Windows exercises the same path a
// user without Developer Mode takes.
func TestCopyModeInstallDoctorUninstallRoundTrip(t *testing.T) {
	repo, home := fakeMachine(t)
	dest := filepath.Join(home, ".claude", "agents")

	if err := runInstall(repo, home, false, true, true, uiSettings{}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(dest); err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		t.Fatalf("copy mode must leave a real directory, got %v (err %v)", fi, err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "a.md")); err != nil || !strings.Contains(string(b), "model: opus") {
		t.Fatalf("the copy is missing its content: %q %v", b, err)
	}

	// An identical copy is healthy, and a second install does not displace it.
	var out bytes.Buffer
	_ = runDoctor(repo, home, nil, &out, uiSettings{})
	if strings.Contains(out.String(), "manifest links") && strings.Contains(out.String(), "need attention") {
		t.Errorf("doctor flags a healthy copy:\n%s", out.String())
	}
	if err := runInstall(repo, home, false, true, true, uiSettings{}); err != nil {
		t.Fatal(err)
	}

	// An edited copy is the user's: uninstall must leave it alone.
	edited := filepath.Join(dest, "a.md")
	if err := os.WriteFile(edited, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runUninstall(repo, home, []string{"--yes"}, false, false, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(edited); string(b) != "mine" {
		t.Fatalf("uninstall touched an edited copy: %q", b)
	}

	// Put the pristine copy back and uninstall removes it.
	if err := os.WriteFile(edited, []byte("---\nmodel: opus\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runUninstall(repo, home, []string{"--yes"}, false, false, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dest); err == nil {
		t.Errorf("an untouched copy should be removed:\n%s", out.String())
	}
}
