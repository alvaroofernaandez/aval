package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

func TestMacosScreenshotClipboardCopyUsesClipCmdAndStateDir(t *testing.T) {
	testenv.SkipWindows(t, "macOS LaunchAgent and clipboard helpers")
	dir, state := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(dir, "Captura 1.png"), "x")
	log := filepath.Join(t.TempDir(), "clip.log")
	fake := filepath.Join(t.TempDir(), "fakeclip")
	writeFile(t, fake, "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '"+log+"'\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIP_CMD", fake)
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("SETTLE", "0")

	for i := 0; i < 2; i++ { // the second run must not copy again
		if err := runMacos([]string{"screenshot-clipboard", "copy", dir}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(log)
	if strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), "Captura 1.png") {
		t.Fatalf("clip log = %q", b)
	}
	if _, err := os.Stat(filepath.Join(state, "screenshot-clipboard.last")); err != nil {
		t.Fatalf("state not written under XDG_STATE_HOME: %v", err)
	}
}

func TestMacosScreenshotClipboardInstallPointsAtLocalBinAval(t *testing.T) {
	testenv.SkipWindows(t, "macOS LaunchAgent and clipboard helpers")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SKIP_DEFAULTS", "1")
	t.Setenv("SKIP_LAUNCHCTL", "1")
	if err := runMacos([]string{"screenshot-clipboard", "install"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "Library/LaunchAgents/dev.aval.screenshot-clipboard.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<string>"+filepath.Join(home, ".local", "bin", "aval")+"</string>") {
		t.Fatalf("plist does not run $HOME/.local/bin/aval:\n%s", b)
	}
}

func TestMacosUsage(t *testing.T) {
	testenv.SkipWindows(t, "macOS LaunchAgent and clipboard helpers")
	for _, args := range [][]string{nil, {"nope"}, {"screenshot-clipboard"}, {"screenshot-clipboard", "x"}} {
		if err := runMacos(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: want a usage error", args)
		}
	}
}
