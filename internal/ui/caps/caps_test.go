package caps

import (
	"testing"

	"github.com/muesli/termenv"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetect(t *testing.T) {
	tty := func(e map[string]string) Inputs {
		return Inputs{Getenv: env(e), GOOS: "linux", IsTTY: true, Width: 80, Height: 24}
	}
	tests := []struct {
		name string
		in   Inputs
		want Caps
	}{
		{"utf8 truecolor", tty(map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm-256color", "COLORTERM": "truecolor"}),
			Caps{termenv.TrueColor, true, true, true, 80, 24}},
		{"24bit", tty(map[string]string{"COLORTERM": "24bit", "LANG": "C.utf8"}),
			Caps{termenv.TrueColor, true, true, true, 80, 24}},
		{"256color", tty(map[string]string{"TERM": "xterm-256color", "LANG": "en_US.UTF-8"}),
			Caps{termenv.ANSI256, true, true, true, 80, 24}},
		{"plain ansi", tty(map[string]string{"TERM": "xterm", "LANG": "en_US.UTF-8"}),
			Caps{termenv.ANSI, true, true, true, 80, 24}},
		{"no color any value", tty(map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8"}),
			Caps{termenv.Ascii, true, true, true, 80, 24}},
		{"no color empty-but-set is still unset in env map", tty(map[string]string{"NO_COLOR": "", "TERM": "xterm", "LANG": "en_US.UTF-8"}),
			Caps{termenv.ANSI, true, true, true, 80, 24}},
		{"dumb", tty(map[string]string{"TERM": "dumb", "LANG": "en_US.UTF-8"}),
			Caps{termenv.Ascii, false, true, true, 80, 24}},
		{"linux console", tty(map[string]string{"TERM": "linux", "LANG": "en_US.UTF-8"}),
			Caps{termenv.ANSI, false, true, true, 80, 24}},
		{"no utf8 locale", tty(map[string]string{"LANG": "C", "TERM": "xterm"}),
			Caps{termenv.ANSI, false, true, true, 80, 24}},
		{"LC_ALL beats LANG", tty(map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8", "TERM": "xterm"}),
			Caps{termenv.ANSI, false, true, true, 80, 24}},
		{"LC_CTYPE beats LANG", tty(map[string]string{"LC_CTYPE": "en_US.UTF-8", "LANG": "C", "TERM": "xterm"}),
			Caps{termenv.ANSI, true, true, true, 80, 24}},
		{"utf-8 case-insensitive", tty(map[string]string{"LANG": "en_US.Utf-8", "TERM": "xterm"}),
			Caps{termenv.ANSI, true, true, true, 80, 24}},
		{"not a tty", Inputs{Getenv: env(map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm-256color"}), GOOS: "linux", Width: 80, Height: 24},
			Caps{termenv.Ascii, true, false, true, 80, 24}},
		{"windows bare", Inputs{Getenv: env(nil), GOOS: "windows", IsTTY: true, Width: 80, Height: 24},
			Caps{termenv.ANSI, false, true, true, 80, 24}},
		{"windows terminal", Inputs{Getenv: env(map[string]string{"WT_SESSION": "x"}), GOOS: "windows", IsTTY: true},
			Caps{termenv.ANSI, true, true, true, 0, 0}},
		{"windows TERM_PROGRAM", Inputs{Getenv: env(map[string]string{"TERM_PROGRAM": "vscode"}), GOOS: "windows", IsTTY: true},
			Caps{termenv.ANSI, true, true, true, 0, 0}},
		{"windows ConEmu", Inputs{Getenv: env(map[string]string{"ConEmuANSI": "ON"}), GOOS: "windows", IsTTY: true},
			Caps{termenv.ANSI, true, true, true, 0, 0}},
		{"windows ignores utf8 locale", Inputs{Getenv: env(map[string]string{"LANG": "en_US.UTF-8"}), GOOS: "windows", IsTTY: true},
			Caps{termenv.ANSI, false, true, true, 0, 0}},
		{"glyphs unicode override", Inputs{Getenv: env(map[string]string{"TERM": "dumb"}), GOOS: "linux", IsTTY: true, Glyphs: "unicode"},
			Caps{termenv.Ascii, true, true, true, 0, 0}},
		{"glyphs ascii override", Inputs{Getenv: env(map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm"}), GOOS: "linux", IsTTY: true, Glyphs: "ascii"},
			Caps{termenv.ANSI, false, true, true, 0, 0}},
		{"motion off", Inputs{Getenv: env(map[string]string{"TERM": "xterm"}), GOOS: "linux", IsTTY: true, Motion: "off"},
			Caps{termenv.ANSI, false, false, true, 0, 0}},
		{"background light", Inputs{Getenv: env(map[string]string{"TERM": "xterm"}), GOOS: "linux", IsTTY: true, Background: "light"},
			Caps{termenv.ANSI, false, true, false, 0, 0}},
		{"background dark", Inputs{Getenv: env(map[string]string{"TERM": "xterm"}), GOOS: "linux", IsTTY: true, Background: "dark"},
			Caps{termenv.ANSI, false, true, true, 0, 0}},
		{"color off override", Inputs{Getenv: env(map[string]string{"COLORTERM": "truecolor"}), GOOS: "linux", IsTTY: true, ColorOff: true},
			Caps{termenv.Ascii, false, true, true, 0, 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.in); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDetectNilGetenv(t *testing.T) {
	got := Detect(Inputs{GOOS: "linux", IsTTY: true})
	if got.Profile != termenv.ANSI || !got.Dark {
		t.Fatalf("got %+v", got)
	}
}
