package notify

import (
	"errors"
	"slices"
	"testing"
)

func has(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

const text = "Relayed 212k → 61k"

func TestNotifier_Darwin_Command(t *testing.T) {
	d := Plan(Platform{GOOS: "darwin", LookPath: has("osascript")}, text)
	want := []string{"osascript",
		"-e", "on run argv",
		"-e", `display notification (item 2 of argv) with title (item 1 of argv)`,
		"-e", "end run",
		"aval", text}
	if !slices.Equal(d.Argv, want) {
		t.Fatalf("argv %q, want %q", d.Argv, want)
	}
}

func TestNotifier_Linux_NotifySend(t *testing.T) {
	d := Plan(Platform{GOOS: "linux", LookPath: has("notify-send")}, text)
	if want := []string{"notify-send", "--app-name=aval", "--", "aval", text}; !slices.Equal(d.Argv, want) {
		t.Fatalf("argv %q, want %q", d.Argv, want)
	}
}

func TestNotifier_Linux_FallsBackToOSC(t *testing.T) {
	d := Plan(Platform{GOOS: "linux", LookPath: has(), TTY: true}, text)
	if len(d.Argv) != 0 || d.OSC != "\x1b]9;aval: "+text+"\x1b\\" {
		t.Fatalf("got %+v, want only the OSC 9 sequence", d)
	}
}

func TestNotifier_Linux_NoNotifierNoTTY_Nothing(t *testing.T) {
	if d := Plan(Platform{GOOS: "linux", LookPath: has()}, text); len(d.Argv) != 0 || d.OSC != "" {
		t.Fatalf("got %+v, want no delivery", d)
	}
}

func TestNotifier_Windows_PowerShellArgs(t *testing.T) {
	d := Plan(Platform{GOOS: "windows", LookPath: has("powershell")}, text)
	if len(d.Argv) != 5 || !slices.Equal(d.Argv[:4], []string{"powershell", "-NoProfile", "-NonInteractive", "-Command"}) || d.Argv[4] != toastScript {
		t.Fatalf("argv %q", d.Argv)
	}
	if want := []string{"AVAL_NOTIFY_TITLE=aval", "AVAL_NOTIFY_TEXT=" + text}; !slices.Equal(d.Env, want) {
		t.Fatalf("env %q, want %q", d.Env, want)
	}
}

func TestNotifier_WSL_PowerShellExe(t *testing.T) {
	d := Plan(Platform{GOOS: "linux", WSL: true, LookPath: has("powershell.exe", "notify-send")}, text)
	if len(d.Argv) == 0 || d.Argv[0] != "powershell.exe" || d.Argv[len(d.Argv)-1] != toastScript {
		t.Fatalf("argv %q, want powershell.exe with the toast script", d.Argv)
	}
	if !slices.Contains(d.Env, "WSLENV=AVAL_NOTIFY_TITLE/u:AVAL_NOTIFY_TEXT/u") {
		t.Fatalf("env %q, want WSLENV to carry the two values to Windows", d.Env)
	}
}

func TestNotifier_WSL_KeepsTheExistingWSLENV(t *testing.T) {
	d := Plan(Platform{GOOS: "linux", WSL: true, WSLEnv: "USERPROFILE/p", LookPath: has("powershell.exe")}, text)
	if !slices.Contains(d.Env, "WSLENV=USERPROFILE/p:AVAL_NOTIFY_TITLE/u:AVAL_NOTIFY_TEXT/u") {
		t.Fatalf("env %q", d.Env)
	}
}

func TestNotifier_TextIsNeverShellInterpreted(t *testing.T) {
	nasty := `"; rm -rf ~; $(touch x) ` + "`y` '\x1b]0;pwn\x07"
	mac := Plan(Platform{GOOS: "darwin", LookPath: has("osascript")}, nasty)
	if mac.Argv[len(mac.Argv)-1] != nasty || slices.ContainsFunc(mac.Argv[:len(mac.Argv)-1], func(a string) bool { return a == nasty }) {
		t.Errorf("darwin: the text must be the last argument only: %q", mac.Argv)
	}
	linux := Plan(Platform{GOOS: "linux", LookPath: has("notify-send")}, nasty)
	if linux.Argv[len(linux.Argv)-1] != nasty || linux.Argv[len(linux.Argv)-3] != "--" {
		t.Errorf("linux: the text must follow -- as a plain argument: %q", linux.Argv)
	}
	win := Plan(Platform{GOOS: "windows", LookPath: has("powershell")}, nasty)
	if slices.ContainsFunc(win.Argv, func(a string) bool {
		return a == nasty || len(a) > 0 && a != toastScript && a[0] != '-' && a != "powershell"
	}) {
		t.Errorf("windows: the text must reach PowerShell only through the environment: %q", win.Argv)
	}
	osc := Plan(Platform{GOOS: "linux", LookPath: has(), TTY: true}, nasty)
	if inner := osc.OSC[len("\x1b]9;") : len(osc.OSC)-len("\x1b\\")]; slices.ContainsFunc([]rune(inner), func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		t.Errorf("osc: control characters must be dropped from the text: %q", osc.OSC)
	}
}
