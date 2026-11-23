package main

import (
	"errors"
	"fmt"
	"io"
	"runtime"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// loadUIConfig is the seam tests use to prove which commands read the config.
var loadUIConfig = config.Load

// uiSettings is every user-facing look-and-feel choice, resolved once per
// command. The zero value is not valid for resolveUI callers, but every
// consumer treats empty strings as "auto"/default, so tests may pass it.
type uiSettings struct {
	Theme      string    // a config.Themes name
	Background string    // auto | dark | light
	Glyphs     string    // auto | unicode | ascii
	Motion     string    // auto | off
	Lang       i18n.Lang // the resolved language for human output

	UpdateCheck bool // false when update_check = off or AVAL_NO_UPDATE_CHECK is set
	ColorOff    bool // NO_COLOR: above every other setting

	// flags are the command-line values these came from, so the settings
	// view can say which keys a flag overrides.
	flags uiFlags
}

// uiFlags are the global flags that reach the UI settings.
type uiFlags struct {
	Theme string // --theme
	Plain bool   // --plain
	Lang  string // --lang
}

// resolveUI applies flag > env > file > default to the UI keys, then --plain
// and NO_COLOR on top; the language adds detection before the default
// (resolveLang). A bad file value comes back as a warning (file, line and a
// suggestion) and the next layer applies; a bad --theme or --lang is an
// error. Only
// commands that draw call it: hooks, statusline and tmux never read config.
func resolveUI(flags uiFlags, getenv func(string) string, cfgPath string) (uiSettings, []string, error) {
	if flags.Theme != "" {
		if msg := config.Validate("theme", flags.Theme); msg != "" {
			return uiSettings{}, nil, fmt.Errorf("%w: %s", errBadThemeFlag, msg)
		}
	}

	if flags.Lang != "" {
		if msg := config.Validate("language", flags.Lang); msg != "" {
			return uiSettings{}, nil, fmt.Errorf("%w: %s", errBadLangFlag, msg)
		}
	}

	var warns []string
	f, loadWarns, err := loadUIConfig(cfgPath)
	if err != nil {
		warns = append(warns, "config: "+err.Error()+"; using defaults")
		f, _, _ = config.Load("") // empty path: no file, defaults only
	}
	lang := resolveLang(f, flags.Lang, getenv)
	warns = append(warns, configWarnings(i18n.New(lang), cfgPath, loadWarns)...)

	pick := func(key string) string {
		v, _ := f.Resolve(key, map[string]string{"theme": flags.Theme}, getenv)
		return v
	}
	update, _ := f.Resolve("update_check", nil, getenv)
	s := uiSettings{
		Theme:       pick("theme"),
		Background:  pick("background"),
		Glyphs:      pick("glyphs"),
		Motion:      pick("motion"),
		Lang:        lang,
		UpdateCheck: update != "off",
		ColorOff:    getenv("NO_COLOR") != "",
		flags:       flags,
	}
	if flags.Plain {
		s.Theme, s.Glyphs, s.Motion = "mono", "ascii", "off"
	}
	return s, warns, nil
}

// capsInputs builds the detection inputs for a terminal of the given size.
func (s uiSettings) capsInputs(getenv func(string) string, goos string, isTTY bool, width, height int) caps.Inputs {
	return caps.Inputs{
		Getenv: getenv, GOOS: goos, IsTTY: isTTY, Width: width, Height: height,
		Glyphs: s.Glyphs, Motion: s.Motion, Background: s.Background, ColorOff: s.ColorOff,
	}
}

// settingsForCommand resolves the UI settings for commands that draw. Hooks,
// statusline and tmux never get here: they run on every tool call or refresh,
// so they must not touch the config file at all. Warnings go to stderr, except
// for doctor, whose config check reports them.
func settingsForCommand(cmd string, flags uiFlags, getenv func(string) string, home string, stderr io.Writer) (uiSettings, error) {
	switch cmd {
	case "install", "doctor", "":
	default:
		return uiSettings{}, nil
	}
	s, warns, err := resolveUI(flags, getenv, config.Path(getenv, home, runtime.GOOS))
	if err != nil {
		return uiSettings{}, err
	}
	if cmd != "doctor" {
		for _, w := range warns {
			fmt.Fprintln(stderr, "warning:", w)
		}
	}
	return s, nil
}

// settingsEnv is what the settings view needs to read and write path and to
// preview candidate values on this terminal.
func (s uiSettings) settingsEnv(path, home string, getenv func(string) string, isTTY bool, width, height int) ui.SettingsEnv {
	env := ui.SettingsEnv{
		Path: path, Home: home, Getenv: getenv,
		Flags: map[string]string{}, FlagNames: map[string]string{},
		Detect: func(glyphs, background string) caps.Caps {
			c := s
			c.Glyphs, c.Background = glyphs, background
			return caps.Detect(c.capsInputs(getenv, runtime.GOOS, isTTY, width, height))
		},
	}
	if s.flags.Theme != "" {
		env.Flags["theme"], env.FlagNames["theme"] = s.flags.Theme, "--theme"
	}
	if s.flags.Plain {
		for k, v := range map[string]string{"theme": "mono", "glyphs": "ascii", "motion": "off"} {
			env.Flags[k], env.FlagNames[k] = v, "--plain"
		}
	}
	return env
}

// errBadThemeFlag is a --theme that names no theme.
var errBadThemeFlag = errors.New("invalid --theme")

// errBadLangFlag is a --lang that names no language.
var errBadLangFlag = errors.New("invalid --lang")
