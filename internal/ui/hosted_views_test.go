package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/plan"
)

// The hub's Install and Settings entries open the hosted installer and the
// hosted settings screen; without a registered view the hub shows them as
// "not in this build yet".
func TestInstallAndSettingsAreRegisteredHubViews(t *testing.T) {
	for _, name := range []string{viewInstall, viewSettings} {
		if _, ok := viewFor(name); !ok {
			t.Errorf("%s has no registered view", name)
		}
	}
}

func TestInstallViewHostsTheInstaller(t *testing.T) {
	ctor, ok := viewFor(viewInstall)
	if !ok {
		t.Fatal("install is not registered")
	}
	svc := Services{
		Installer: func() (Model, error) { return NewModel(&plan.Plan{}, nil, Options{}), nil },
	}
	s := ctor(svc)
	f, ok := s.(flow)
	if !ok {
		t.Fatalf("install view is %T, want a hosted flow", s)
	}
	if !f.m.hosted {
		t.Error("the installer is not hosted: closing it would quit the program")
	}
}

func TestInstallViewShowsTheInstallerError(t *testing.T) {
	ctor, ok := viewFor(viewInstall)
	if !ok {
		t.Fatal("install is not registered")
	}
	svc := Services{Installer: func() (Model, error) { return Model{}, errors.New("cannot read install.manifest") }}
	s := ctor(svc)
	if _, isFlow := s.(flow); isFlow {
		t.Fatal("a failing installer still opened a flow")
	}
	c := chrome{width: 80, height: 24, theme: DefaultTheme()}
	got := strings.Join(s.view().body(c.layout(), 24), "\n")
	if !strings.Contains(got, "cannot read install.manifest") {
		t.Errorf("error view does not name the error:\n%s", got)
	}
}

func TestSettingsViewHostsTheSettingsScreen(t *testing.T) {
	ctor, ok := viewFor(viewSettings)
	if !ok {
		t.Fatal("settings is not registered")
	}
	svc := Services{Settings: func() SettingsEnv { return SettingsEnv{Path: t.TempDir() + "/config"} }}
	s := ctor(svc)
	f, ok := s.(flow)
	if !ok {
		t.Fatalf("settings view is %T, want a hosted flow", s)
	}
	if !f.m.hosted || f.m.Screen() != ScreenSettings {
		t.Errorf("hosted=%v screen=%v, want a hosted settings screen", f.m.hosted, f.m.Screen())
	}
}
