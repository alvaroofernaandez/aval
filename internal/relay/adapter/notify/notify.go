// Package notify builds the OS notification for a relay: osascript on macOS,
// notify-send on Linux, a PowerShell toast on Windows and WSL, and a terminal
// sequence when nothing else is there. The text is always an argument or an
// environment value, never part of a script or a shell line.
package notify

import "strings"

// Platform is what Plan needs to know about the machine.
type Platform struct {
	GOOS     string
	WSL      bool                         // Linux under WSL, with powershell.exe through interop
	LookPath func(string) (string, error) // exec.LookPath in production
	TTY      bool                         // a terminal is attached for the OSC fallback
	WSLEnv   string                       // the WSLENV already set, kept when WSL adds to it
}

// Delivery is one way to show a notification. Argv empty and OSC empty means
// there is no way on this machine.
type Delivery struct {
	Argv []string // the command to start, no shell
	Env  []string // KEY=value added to the command's environment
	OSC  string   // terminal sequence to write instead of a command
}

// toastScript shows a Windows toast. It is fixed: the title and the text
// arrive in AVAL_NOTIFY_TITLE and AVAL_NOTIFY_TEXT, never in the script.
// CreateTextNode escapes them for the toast XML. The app id is PowerShell's
// own, the one Windows shows a toast for without a registered shortcut.
const toastScript = `$ErrorActionPreference='Stop';` +
	`[void][Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime];` +
	`$x=[Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);` +
	`$t=$x.GetElementsByTagName('text');` +
	`[void]$t.Item(0).AppendChild($x.CreateTextNode($env:AVAL_NOTIFY_TITLE));` +
	`[void]$t.Item(1).AppendChild($x.CreateTextNode($env:AVAL_NOTIFY_TEXT));` +
	`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show([Windows.UI.Notifications.ToastNotification]::new($x))`

// Title is the notification title.
const Title = "aval"

// wslValues names the values WSL interop must pass to powershell.exe.
const wslValues = "AVAL_NOTIFY_TITLE/u:AVAL_NOTIFY_TEXT/u"

// Plan returns how to show text on p.
func Plan(p Platform, text string) Delivery {
	found := func(name string) bool {
		_, err := p.LookPath(name)
		return err == nil
	}
	switch {
	case p.GOOS == "darwin" && found("osascript"):
		return Delivery{Argv: []string{"osascript",
			"-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
			"-e", "end run",
			Title, text}}
	case p.GOOS == "windows" && found("powershell"):
		return toast("powershell", text, nil)
	case p.GOOS == "linux" && p.WSL && found("powershell.exe"):
		wslenv := wslValues
		if p.WSLEnv != "" {
			wslenv = p.WSLEnv + ":" + wslValues
		}
		return toast("powershell.exe", text, []string{"WSLENV=" + wslenv})
	case p.GOOS == "linux" && found("notify-send"):
		return Delivery{Argv: []string{"notify-send", "--app-name=" + Title, "--", Title, text}}
	case p.TTY:
		return Delivery{OSC: "\x1b]9;" + Title + ": " + printable(text) + "\x1b\\"}
	}
	return Delivery{}
}

func toast(exe, text string, extra []string) Delivery {
	return Delivery{
		Argv: []string{exe, "-NoProfile", "-NonInteractive", "-Command", toastScript},
		Env:  append([]string{"AVAL_NOTIFY_TITLE=" + Title, "AVAL_NOTIFY_TEXT=" + text}, extra...),
	}
}

// printable drops control characters, so the text cannot end the OSC
// sequence or start another one.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}
