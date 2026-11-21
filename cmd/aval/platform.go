package main

import "fmt"

// unixOnly lists the commands that drive software with no native Windows
// build, and why. They run fine under WSL, which takes the Unix path.
var unixOnly = map[string]string{
	"tmux":              "it drives tmux, which has no native Windows build",
	"macos":             "it configures macOS (defaults, LaunchAgents, the screenshot clipboard)",
	"reap-orphan-tests": "it reaps Unix process groups and is scheduled by a macOS LaunchAgent",
	"kiri":              "Kiri is a macOS Docker runtime",
	"docker":            "it manages the Kiri Docker runtime, which is macOS only",
}

// unsupportedHere returns a clear error when cmd cannot run on goos, and nil
// otherwise. goos is a parameter so the message is testable on any machine.
func unsupportedHere(goos, cmd string) error {
	if goos != "windows" {
		return nil
	}
	reason, ok := unixOnly[cmd]
	if !ok {
		return nil
	}
	return fmt.Errorf("aval %s is not supported on windows: %s. Use WSL for it", cmd, reason)
}
