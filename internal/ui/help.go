package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The help overlay replaces the body of the current screen inside the same
// frame. Its rows come from the screen's own key map, hidden bindings
// included, so the list can never drift from what Update handles.

// helpKeysFor is the map while help is open: it closes or scrolls, and
// nothing else acts. ctrl+c keeps its stop meaning on the install screen.
func helpKeysFor(s Screen) keyMap {
	km := keyMap{
		{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: "scroll"},
		{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: "scroll"},
		{keys: []string{"esc", "?"}, action: actHelp, label: "esc", desc: "close", core: true},
	}
	if s == ScreenInstall {
		km = append(km, binding{keys: []string{"ctrl+c"}, action: actStop})
	}
	return km
}

// helpAction handles the keys of the open overlay.
func (m Model) helpAction(act action) (tea.Model, tea.Cmd) {
	switch act {
	case actHelp:
		m.help = false
	case actUp:
		m.helpTop = max(m.helpTop-1, 0)
	case actDown:
		m.helpTop = min(m.helpTop+1, max(len(m.screenKeys())-1, 0))
	}
	return m, nil
}

func (m Model) helpView() view {
	return view{body: m.helpBody, status: "help"}
}

// keyName is how one tea key string reads in the overlay. Arrows come from
// the glyph tier, so ASCII terminals see "up" and "down".
func (g Glyphs) keyName(k string) string {
	switch k {
	case " ":
		return "space"
	case "up":
		return splitPair(g.UpDown, 0)
	case "down":
		return splitPair(g.UpDown, 1)
	case "left":
		return splitPair(g.LeftRight, 0)
	case "right":
		return splitPair(g.LeftRight, 1)
	}
	return k
}

// splitPair returns half i of a glyph pair: "↑↓" or "up/down".
func splitPair(pair string, i int) string {
	if parts := strings.Split(pair, "/"); len(parts) == 2 {
		return parts[i]
	}
	r := []rune(pair)
	if len(r) == 2 {
		return string(r[i])
	}
	return pair
}

// helpRows is "keys, description" for every binding of the current screen.
func (m Model) helpRows() [][2]string {
	var rows [][2]string
	for _, b := range m.screenKeys() {
		names := make([]string, len(b.keys))
		for i, k := range b.keys {
			names[i] = m.theme.Glyphs.keyName(k)
		}
		rows = append(rows, [2]string{strings.Join(names, ", "), b.desc})
	}
	return rows
}

func (m Model) helpBody(l layout, h int) []string {
	rows := m.helpRows()
	keyW := 0
	for _, r := range rows {
		keyW = max(keyW, lipgloss.Width(r[0]))
	}
	// Keep room for at least a few columns of description.
	keyW = min(keyW, max(l.width/2, 1))

	lines := []string{m.theme.Strong.Render("Keys")}
	room := max(h-len(lines), 0)
	start, end := scrollWindow(len(rows), m.helpTop, room)
	for _, r := range rows[start:end] {
		keys := m.pad(r[0], keyW)
		desc := m.fit(r[1], max(l.width-keyW-2, 1))
		lines = append(lines, m.theme.Key.Render(keys)+"  "+m.theme.Text.Render(desc))
	}
	return lines
}
