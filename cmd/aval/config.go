package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/ui"
	"github.com/alvaroofernaandez/aval/internal/ui/caps"
)

// openSettings opens the interactive settings view. Bare `aval config` calls
// it when stdout is a terminal; a nil value, or no terminal, lists instead.
var openSettings = runSettingsView

// runSettingsView draws the settings view full screen until the user saves,
// discards or quits.
func runSettingsView(path string, getenv func(string) string) error {
	home, _ := os.UserHomeDir()
	s, warns, err := resolveUI(uiFlags{}, getenv, path)
	if err != nil {
		return err
	}
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	fd := os.Stdout.Fd()
	width, height, _ := term.GetSize(fd)
	m := ui.NewSettings(ui.Options{
		Version:  version,
		Platform: ui.PlatformLabel(runtime.GOOS, runtime.GOARCH),
		Caps:     caps.Detect(s.capsInputs(getenv, runtime.GOOS, true, width, height)),
		Theme:    s.Theme,
		Lang:     s.Lang,
		Settings: s.settingsEnv(path, home, getenv, true, width, height),
	})
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

const configUsage = "usage: aval config [list | get <key> | set <key> <value> | path]"

// runConfig implements `aval config`. Output streams, the environment and the
// home directory are injected so tests never touch the real HOME.
func runConfig(args []string, stdout, stderr io.Writer, getenv func(string) string, home string) error {
	path := config.Path(getenv, home, runtime.GOOS)
	tr := i18n.New(langAt(path, getenv))
	sub := "list"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	} else if openSettings != nil && isTerminal(stdout) {
		return openSettings(path, getenv)
	}

	switch sub {
	case "list":
		if len(args) > 0 {
			return newUsage(tr.T("cli.config.usage", nil))
		}
		return configList(path, stdout, stderr, getenv, tr)
	case "get":
		if len(args) != 1 {
			return newUsage(tr.T("cli.config.usage", nil))
		}
		return configGet(path, args[0], stdout, stderr, getenv, tr)
	case "set":
		if len(args) != 2 {
			return newUsage(tr.T("cli.config.usage", nil))
		}
		if err := config.Set(path, args[0], args[1]); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s = %s  (%s)\n", args[0], args[1], tildePath(path, home))
		return nil
	case "path":
		if len(args) > 0 {
			return newUsage(tr.T("cli.config.usage", nil))
		}
		fmt.Fprintln(stdout, path)
		return nil
	}
	return newUsage(tr.T("cli.config.unknown_command", i18n.Args{"command": strconv.Quote(sub)}))
}

// langAt resolves the language for a config command from the file at path:
// --lang reaches here as AVAL_LANG (see dispatch), so the usual precedence
// holds. A file that cannot be read is skipped; loadConfig reports it.
func langAt(path string, getenv func(string) string) i18n.Lang {
	f, _, err := config.Load(path)
	if err != nil {
		f, _, _ = config.Load("")
	}
	return resolveLang(f, "", getenv)
}

func loadConfig(path string, stderr io.Writer, getenv func(string) string) (*config.File, error) {
	f, warns, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	tr := i18n.New(resolveLang(f, "", getenv))
	for _, w := range configWarnings(tr, path, warns) {
		fmt.Fprintln(stderr, "warning:", w)
	}
	return f, nil
}

func configList(path string, stdout, stderr io.Writer, getenv func(string) string, tr i18n.Translator) error {
	f, err := loadConfig(path, stderr, getenv)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0)
	for _, k := range config.Keys() {
		v, src := f.Resolve(k, nil, getenv)
		fmt.Fprintf(tw, "%s\t%s\t%s\n", k, v, src)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if unknown := f.Unknown(); len(unknown) > 0 {
		fmt.Fprintln(stdout)
		for _, k := range unknown {
			fmt.Fprintln(stdout, tr.T("cli.config.unknown_in_file", i18n.Args{"key": strconv.Quote(k), "source": unknownSource(path, k)}))
		}
	}
	return nil
}

// unknownSource finds the line of an unknown key, since the config package
// only reports its name.
func unknownSource(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "file"
	}
	for i, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, _, ok := strings.Cut(l, "=")
		if ok && strings.TrimSpace(k) == key {
			return fmt.Sprintf("file:%d", i+1)
		}
	}
	return "file"
}

func configGet(path, key string, stdout, stderr io.Writer, getenv func(string) string, tr i18n.Translator) error {
	valid := false
	for _, k := range config.Keys() {
		valid = valid || k == key
	}
	if !valid {
		err := fmt.Errorf("%w %q. Valid: %s", errUnknownKey, key, strings.Join(config.Keys(), ", "))
		return localizedError{err: err, text: tr.T("cli.config.unknown_key", i18n.Args{"key": strconv.Quote(key), "valid": strings.Join(config.Keys(), ", ")})}
	}
	f, err := loadConfig(path, stderr, getenv)
	if err != nil {
		return err
	}
	v, _ := f.Resolve(key, nil, getenv)
	fmt.Fprintln(stdout, v)
	return nil
}

// tildePath shortens a path under home for display.
func tildePath(path, home string) string {
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}

// isTerminal reports whether w is an interactive terminal.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// errUnknownKey is a config key aval does not know.
var errUnknownKey = errors.New("unknown config key")
