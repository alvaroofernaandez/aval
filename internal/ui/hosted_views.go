package ui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// The hub's Install and Settings entries host the existing installer and
// settings screens (flow.go) instead of reimplementing them.
func init() {
	registerView(viewInstall, newInstallView)
	registerView(viewSettings, newSettingsView)
}

func newInstallView(svc Services) screen {
	if svc.Installer == nil {
		return errorView{title: "Install", err: errors.New("the installer is not available in this build")}
	}
	f, err := installFlow(svc)
	if err != nil {
		return errorView{title: "Install", err: err}
	}
	return f
}

func newSettingsView(svc Services) screen {
	var env SettingsEnv
	if svc.Settings != nil {
		env = svc.Settings()
	}
	m := NewModel(&plan.Plan{}, nil, Options{Settings: env}).openSettings()
	return newFlow(m, nil)
}

// errorView names what failed and leaves the router's esc to go back.
type errorView struct {
	title string
	err   error
}

func (e errorView) init() tea.Cmd                    { return nil }
func (e errorView) update(tea.Msg) (screen, tea.Cmd) { return e, nil }
func (e errorView) keys() keyMap                     { return keyMap{} }
func (e errorView) busy() busyKind                   { return busyIdle }
func (e errorView) view() view {
	return view{
		status: "Nothing was changed",
		body: func(l layout, _ int) []string {
			return []string{e.title, "", "× " + e.err.Error()}
		},
	}
}
