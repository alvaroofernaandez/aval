package ui

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/config"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// The Language row (DESIGN.md decision of 2026-11-23): left and right cycle
// auto, en and es, and the screen repaints in the chosen language before
// anything is saved.

// languageRow is the cursor position of the language row among the rows.
const languageRow = 5

func toLanguageRow(m Model) Model {
	for range languageRow {
		m = send(m, "down")
	}
	return m
}

func TestSettingsLanguageRowPreviewsTheLanguageAtOnce(t *testing.T) {
	m, path := settingsModel(t, "", nil)
	m = toLanguageRow(m)
	if v := renderAtSize(m, 100, 30); !strings.Contains(v, "Language") || !strings.Contains(v, "auto · en · es") {
		t.Fatalf("the language row is not listed with its choices:\n%s", v)
	}

	m = send(send(m, "right"), "right") // auto -> en -> es
	v := renderAtSize(m, 100, 30)
	for _, want := range []string{"Ajustes", "Idioma", "Vista previa", "ayuda"} {
		if !strings.Contains(v, want) {
			t.Errorf("after es, the screen lacks %q:\n%s", want, v)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("previewing a language wrote the config file (stat err = %v)", err)
	}

	m = send(m, "left") // back to en
	if v := renderAtSize(m, 100, 30); !strings.Contains(v, "Settings") || strings.Contains(v, "Ajustes") {
		t.Errorf("after en, the screen is not English:\n%s", v)
	}
}

func TestSettingsLanguageEnterWritesTheLanguageKey(t *testing.T) {
	m, path := settingsModel(t, "", nil)
	m = send(send(toLanguageRow(m), "right"), "right")
	if _, cmd := m.Update(key("enter")); cmd == nil {
		t.Fatal("enter did not end the standalone settings screen")
	}
	f, _, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Resolve("language", nil, envOf(nil)); got != "es" {
		t.Errorf("language in the file = %q, want es", got)
	}
}

func TestSettingsLanguageEscRestoresTheLanguageItOpenedIn(t *testing.T) {
	m := screenFixture(t, ScreenSelect, true)
	m.settingsEnv.Path = t.TempDir() + "/config"
	open := send(m, "s")
	chosen := send(send(toLanguageRow(open), "right"), "right")
	if v := renderAt(chosen, 80); !strings.Contains(v, "Ajustes") {
		t.Fatalf("the preview is not Spanish:\n%s", v)
	}
	back := send(chosen, "esc")
	if back.Screen() != ScreenSelect {
		t.Fatalf("esc went to %v, want the select screen", back.Screen())
	}
	if got := back.tr.Lang(); got != i18n.English {
		t.Errorf("esc left the screen speaking %q, want en", got)
	}
	if v := renderAt(back, 80); !strings.Contains(v, "Choose what to install") {
		t.Errorf("the select screen is not back in English:\n%s", v)
	}
}

func TestSettingsLanguageAutoFollowsTheLocale(t *testing.T) {
	m, _ := settingsModel(t, "", map[string]string{"LANG": "es_ES.UTF-8"})
	m = toLanguageRow(m)
	m = send(m, "right") // auto -> en
	if got := m.tr.Lang(); got != i18n.English {
		t.Errorf("en previews %q", got)
	}
	m = send(send(m, "right"), "right") // en -> es -> auto
	if got := m.tr.Lang(); got != i18n.Spanish {
		t.Errorf("auto with LANG=es_ES.UTF-8 previews %q, want es", got)
	}
	m, _ = settingsModel(t, "", map[string]string{"LANG": "en_US.UTF-8"})
	m = send(send(send(toLanguageRow(m), "right"), "right"), "right") // en -> es -> auto
	if got := m.tr.Lang(); got != i18n.English {
		t.Errorf("auto with LANG=en_US.UTF-8 previews %q, want en", got)
	}
}

// A language set by $AVAL_LANG wins over the file, so previewing the file
// value would show a screen that is not what the next run shows: the screen
// keeps its language and says so, like the theme.
func TestSettingsLanguageOverrideKeepsTheScreenLanguage(t *testing.T) {
	m, _ := settingsModel(t, "", map[string]string{"AVAL_LANG": "en"})
	m = send(toLanguageRow(m), "right")
	m = send(m, "right") // en -> es in the file
	v := renderAtSize(m, 120, 30)
	if strings.Contains(v, "Ajustes") {
		t.Errorf("an overridden language was previewed:\n%s", v)
	}
	if !strings.Contains(v, "language is set by $AVAL_LANG in this shell") {
		t.Errorf("the override is not noted:\n%s", v)
	}
}

// Hosted by the hub, a save hands the router a frame in the new language, so
// the hub and every later view repaint in it.
func TestSettingsLanguageSaveReachesTheRouter(t *testing.T) {
	ctor, ok := viewFor(viewSettings)
	if !ok {
		t.Fatal("settings is not registered")
	}
	path := t.TempDir() + "/config"
	s := ctor(Services{Settings: func() SettingsEnv { return SettingsEnv{Path: path} }})
	for _, k := range []string{"down", "down", "down", "down", "down", "right", "right"} {
		s, _ = s.update(key(k))
	}
	_, c := s.update(key("enter"))
	var got i18n.Lang
	for _, msg := range drainCmd(c) {
		if cm, ok := msg.(chromeMsg); ok {
			got = cm.c.tr.Lang()
		}
	}
	if got != i18n.Spanish {
		t.Errorf("the save handed the router a frame in %q, want es", got)
	}
}
