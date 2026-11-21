// Package macos holds the macOS-only helpers: the defaults chosen in the habits
// audit, and the screenshot-to-clipboard LaunchAgent with the action it runs.
package macos

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Runner runs an external command. Failures of "best effort" commands (killall)
// are the caller's to ignore.
type Runner func(name string, args ...string) error

// AgentLabel is the LaunchAgent's label and its plist's base name.
const AgentLabel = "dev.aval.screenshot-clipboard"

// ApplyDefaults writes the macOS defaults chosen in the habits audit
// (2026-11-21). Idempotent. Key repeat takes effect after logging out and in.
func ApplyDefaults(run Runner) error {
	writes := [][]string{
		{"NSGlobalDomain", "KeyRepeat", "-int", "2"},         // fastest repeat the UI allows
		{"NSGlobalDomain", "InitialKeyRepeat", "-int", "15"}, // short delay before repeating
		{"NSGlobalDomain", "AppleShowAllExtensions", "-bool", "true"},
		{"com.apple.finder", "ShowPathbar", "-bool", "true"},
	}
	for _, w := range writes {
		if err := run("defaults", append([]string{"write"}, w...)...); err != nil {
			return fmt.Errorf("defaults write %s: %w", w[1], err)
		}
	}
	_ = run("killall", "Finder")
	return nil
}

// CopyOptions configure CopyNewest.
type CopyOptions struct {
	Dir      string                  // folder watched; default ~/Pictures/Screenshots (caller's job)
	StateDir string                  // where the last-copied marker lives
	Settle   time.Duration           // wait for macOS to finish writing the file
	Sleep    func(time.Duration)     // injectable; default time.Sleep
	Clip     func(path string) error // clipboard writer
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".heic": true, ".tiff": true}

// CopyNewest copies the newest screenshot in Dir to the clipboard, once. The
// file stays on disk; the image is also ready to paste. Hidden files are
// skipped: macOS writes ".Captura…" first and renames it when the capture is
// complete.
func CopyNewest(opt CopyOptions) error {
	sleep := opt.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	sleep(opt.Settle)

	entries, err := os.ReadDir(opt.Dir)
	if err != nil {
		return nil // no folder, nothing to copy
	}
	type img struct {
		path string
		mod  time.Time
	}
	var imgs []img
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		imgs = append(imgs, img{filepath.Join(opt.Dir, e.Name()), fi.ModTime()})
	}
	if len(imgs) == 0 {
		return nil
	}
	sort.SliceStable(imgs, func(i, j int) bool { return imgs[i].mod.After(imgs[j].mod) })
	newest := imgs[0].path

	state := filepath.Join(opt.StateDir, "screenshot-clipboard.last")
	if b, err := os.ReadFile(state); err == nil && strings.TrimRight(string(b), "\n") == newest {
		return nil
	}
	if err := opt.Clip(newest); err != nil {
		return err
	}
	if err := os.MkdirAll(opt.StateDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(state, []byte(newest+"\n"), 0o644)
}

// clipScript is the AppleScript that puts an image file on the clipboard.
func clipScript(path string) string {
	class := "TIFF picture"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		class = "«class PNGf»"
	case ".jpg", ".jpeg":
		class = "JPEG picture"
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(path)
	return fmt.Sprintf(`set the clipboard to (read (POSIX file "%s") as %s)`, esc, class)
}

// OsascriptClip is the real clipboard writer.
func OsascriptClip(run Runner) func(string) error {
	return func(path string) error { return run("/usr/bin/osascript", "-e", clipScript(path)) }
}

// InstallOptions configure InstallClipboardAgent.
type InstallOptions struct {
	Home          string
	Bin           string // the aval binary the agent runs, normally $HOME/.local/bin/aval
	Run           Runner
	UID           int
	SkipDefaults  bool // sandboxed tests
	SkipLaunchctl bool
}

// InstallClipboardAgent sends every screenshot to ~/Pictures/Screenshots AND
// the clipboard: it sets the macOS screenshot folder, then installs a
// LaunchAgent that runs `aval macos screenshot-clipboard copy` whenever that
// folder changes. Idempotent.
func InstallClipboardAgent(opt InstallOptions) error {
	shots := filepath.Join(opt.Home, "Pictures", "Screenshots")
	plist := filepath.Join(opt.Home, "Library", "LaunchAgents", AgentLabel+".plist")
	for _, d := range []string{shots, filepath.Dir(plist)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	if !opt.SkipDefaults {
		if err := opt.Run("defaults", "write", "com.apple.screencapture", "location", shots); err != nil {
			return err
		}
		if err := opt.Run("defaults", "write", "com.apple.screencapture", "type", "png"); err != nil {
			return err
		}
		_ = opt.Run("killall", "SystemUIServer")
	}

	if err := os.WriteFile(plist, []byte(agentPlist(AgentLabel, opt.Bin, shots)), 0o644); err != nil {
		return err
	}

	if !opt.SkipLaunchctl {
		domain := fmt.Sprintf("gui/%d", opt.UID)
		_ = opt.Run("launchctl", "bootout", domain+"/"+AgentLabel)
		if err := opt.Run("launchctl", "bootstrap", domain, plist); err != nil {
			return err
		}
	}
	return nil
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func agentPlist(label, bin, shots string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>macos</string>
    <string>screenshot-clipboard</string>
    <string>copy</string>
    <string>%s</string>
  </array>
  <key>WatchPaths</key>
  <array><string>%s</string></array>
  <key>ThrottleInterval</key><integer>1</integer>
</dict>
</plist>
`, xmlEscape(label), xmlEscape(bin), xmlEscape(shots), xmlEscape(shots))
}
