package ui

import "github.com/alvaroofernaandez/aval/internal/i18n"

// The key map is the single source for both Update and the hint line: Update
// only acts on keys a screen's map lists, and the hint line is generated from
// the same map. A `?` overlay (help.go) renders the full map, hidden rows included.

// action is what a key does, independent of which key triggers it.
type action int

const (
	actNone action = iota
	actUp
	actDown
	actToggle
	actAll
	actClear
	actInstall
	actClose
	actQuit
	// actStop ends an install after the step in flight: quitting there would
	// leave a half-applied run with no summary.
	actStop
	// actHelp toggles the help overlay.
	actHelp
)

// Label tokens resolved through the glyph tier, so ASCII terminals see
// "up/down" instead of arrows.
const (
	labelUpDown    = "{updown}"
	labelLeftRight = "{leftright}"
)

// binding ties tea key strings to one action. label and desc make up its hint;
// an empty label keeps the binding out of the hint line. Bindings that share a
// label and desc (up and down) collapse into one hint.
type binding struct {
	keys   []string
	action action
	label  string
	desc   string
	// core hints survive when the hint line has no room to wrap, and are the
	// only ones shown when compact.
	core bool
	// help, when set, replaces desc in the `?` overlay, so bindings that
	// collapse into one hint (the hub letters: "jump") still read apart there.
	// helpName, when set, is the message key of the {name} it takes.
	help, helpName string
}

// hint is one rendered "key desc" pair.
type hint struct{ key, desc string }

type keyMap []binding

// lookup returns the action bound to a tea key string.
func (k keyMap) lookup(key string) (action, bool) {
	for _, b := range k {
		for _, bk := range b.keys {
			if bk == key {
				return b.action, true
			}
		}
	}
	return actNone, false
}

// hints lists the visible hints in map order, with label tokens resolved for the glyph tier and
// descriptions in the chrome language.
func (k keyMap) hints(c chrome) []hint { return k.hintsWhere(c, false) }

func (k keyMap) hintsWhere(c chrome, coreOnly bool) []hint {
	var out []hint
	seen := map[hint]bool{}
	for _, b := range k {
		if b.label == "" || (coreOnly && !b.core) {
			continue
		}
		h := hint{key: c.theme.Glyphs.keyLabel(b.label), desc: c.tr.T(b.desc, nil)}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

func (g Glyphs) keyLabel(label string) string {
	switch label {
	case labelUpDown:
		return g.UpDown
	case labelLeftRight:
		return g.LeftRight
	}
	return label
}

// helpBinding is on every screen, so `?` is always the way to see every key.
var helpBinding = binding{keys: []string{"?"}, action: actHelp, label: "?", desc: i18n.Key("key.help"), core: true}

var quitBinding = binding{keys: []string{"q", "ctrl+c"}, action: actQuit, label: "q", desc: i18n.Key("key.quit")}

var selectKeys = keyMap{
	{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{" ", "x"}, action: actToggle, label: "space", desc: i18n.Key("key.toggle"), core: true},
	{keys: []string{"right", "l"}, action: actExpand, label: labelLeftRight, desc: i18n.Key("key.details")},
	{keys: []string{"left", "h"}, action: actCollapse, label: labelLeftRight, desc: i18n.Key("key.details")},
	{keys: []string{"a"}, action: actAll, label: "a", desc: i18n.Key("key.all")},
	{keys: []string{"n"}, action: actClear, label: "n", desc: i18n.Key("key.none")},
	{keys: []string{"enter"}, action: actInstall, label: "enter", desc: i18n.Key("key.install"), core: true},
	helpBinding,
	quitBinding,
}

var installKeys = keyMap{
	{keys: []string{"q", "ctrl+c"}, action: actStop, label: "ctrl+c", desc: i18n.Key("key.stop"), core: true},
	helpBinding,
}

// stoppingKeys keeps q and ctrl+c bound once a stop is pending, so a second
// press is swallowed, but drops the hint: the status line says it instead.
var stoppingKeys = keyMap{
	{keys: []string{"q", "ctrl+c"}, action: actStop, desc: i18n.Key("key.stop")},
	{keys: []string{"?"}, action: actHelp, desc: i18n.Key("key.help")},
}

var doneKeys = keyMap{
	{keys: []string{"enter", "esc"}, action: actClose, label: "enter", desc: i18n.Key("key.close"), core: true},
	helpBinding,
	quitBinding,
}

// keyMap is the map Update acts on and the hint line shows.
func (m Model) keyMap() keyMap {
	if m.help {
		return helpKeysFor(m.screen)
	}
	return m.screenKeys()
}

// screenKeys is the map for the current screen.
func (m Model) screenKeys() keyMap {
	switch m.screen {
	case ScreenInstall:
		if m.stopping {
			return stoppingKeys
		}
		return installKeys
	case ScreenDone:
		return m.withSettings(doneKeys)
	case ScreenSettings:
		return settingsKeys
	case ScreenWelcome:
		return m.welcomeKeys()
	default:
		return m.withSettings(selectKeys)
	}
}

// withSettings adds `s settings` before the help binding when the model
// knows where the config file is.
func (m Model) withSettings(km keyMap) keyMap {
	if m.settingsEnv.Path == "" {
		return km
	}
	out := make(keyMap, 0, len(km)+1)
	for _, b := range km {
		if b.action == actHelp {
			out = append(out, settingsBinding)
		}
		out = append(out, b)
	}
	return out
}
