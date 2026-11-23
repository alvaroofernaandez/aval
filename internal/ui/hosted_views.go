package ui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvaroofernaandez/aval/internal/core/plan"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// The hub's Install and Settings entries host the existing installer and
// settings screens (flow.go) instead of reimplementing them.
func init() {
	registerView(viewInstall, newInstallView)
	registerView(viewSettings, newSettingsView)
}

func newInstallView(svc Services) screen {
	if svc.Installer == nil {
		return errorView{title: i18n.Key("hub.install.name"), err: errors.New(svc.tr.T("install.unavailable", nil))}
	}
	f, err := installFlow(svc)
	if err != nil {
		return errorView{title: i18n.Key("hub.install.name"), err: err}
	}
	return f
}

func newSettingsView(svc Services) screen {
	var env SettingsEnv
	if svc.Settings != nil {
		env = svc.Settings()
	}
	m := NewModel(&plan.Plan{}, nil, Options{Settings: env, Lang: svc.tr.Lang()}).openSettings()
	return newFlow(m, nil)
}

// errorView names what failed and leaves the router's esc to go back. title
// is a message key; err is already in the person's language.
type errorView struct {
	title string
	err   error
	c     chrome
}

func (e errorView) init() tea.Cmd { return nil }
func (e errorView) update(msg tea.Msg) (screen, tea.Cmd) {
	if m, ok := msg.(chromeMsg); ok {
		e.c = m.c
	}
	return e, nil
}
func (e errorView) keys() keyMap   { return keyMap{} }
func (e errorView) busy() busyKind { return busyIdle }
func (e errorView) view() view {
	return view{
		status: e.c.tr.T("hosted.unchanged", nil),
		body: func(l layout, _ int) []string {
			return []string{e.c.tr.T(e.title, nil), "", "× " + e.err.Error()}
		},
	}
}
