package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/uninstall"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// glyphSepScreen hosts an operation whose static texts (the safety line) were
// written with the Unicode " · " separator and shows them with the tier's own.
type glyphSepScreen struct {
	screen
	sep string
}

func (g glyphSepScreen) update(msg tea.Msg) (screen, tea.Cmd) {
	if cm, ok := msg.(chromeMsg); ok {
		g.sep = cm.c.sep()
	}
	next, cmd := g.screen.update(msg)
	g.screen = next
	return g, cmd
}

func (g glyphSepScreen) view() view {
	v := g.screen.view()
	if g.sep != "" && g.sep != " · " {
		v.status = strings.ReplaceAll(v.status, " · ", g.sep)
	}
	return v
}

// The uninstall view is the operation pattern on uninstall.Preview and
// uninstall.Run: a dry run lists what would go (nothing is written), enter
// leads to a destructive confirm on Cancel, and the action runs atomically.

const (
	undoLatest = "aval restore latest"
)

func newUninstallView(svc Services) screen {
	tr := svc.tr
	o, err := newOperation(operationSpec[[]uninstall.Item]{
		Kind: opAtomic,
		Mode: confirmDestructive,
		Confirm: confirmSpec{
			Action:     tr.T("uninstall.action", nil),
			ActionNote: tr.T("uninstall.action_note", nil),
			Safety:     tr.T("uninstall.safety", nil),
		},
		Ready: func(items []uninstall.Item) bool { return len(items) > 0 },
		Preview: func(context.Context) ([]uninstall.Item, error) {
			if svc.UninstallPreview == nil {
				return nil, errors.New(tr.T("uninstall.unavailable", nil))
			}
			return svc.UninstallPreview()
		},
		Run: func(_ context.Context, _ []uninstall.Item) (opResult, error) {
			if svc.Uninstall == nil {
				return opResult{}, errors.New(tr.T("uninstall.unavailable", nil))
			}
			done, err := svc.Uninstall()
			if err != nil {
				return opResult{}, err
			}
			counts := uninstallCounts(tr, done)
			return opResult{
				Summary: tr.T("uninstall.summary", i18n.Args{"command": undoLatest}),
				Counts:  counts,
				Undo:    undoLatest,
			}, nil
		},
		PreviewRows: func(c chrome, l layout, items []uninstall.Item) ([]string, []string) {
			return uninstallBody(c, l, svc.Home, items)
		},
		PreviewStatus: tr.T("uninstall.preview_status", nil),
		Loading:       tr.T("uninstall.loading", nil),
		Running:       tr.T("uninstall.running", nil),
	})
	if err != nil {
		panic(err)
	}
	return glyphSepScreen{screen: o, sep: " · "}
}

// uninstallCounts is the result's counts, with zero counts left out.
func uninstallCounts(tr i18n.Translator, items []uninstall.Item) []opCount {
	var links, copies, blocks, dirs, restored int
	for _, it := range items {
		switch it.Kind {
		case uninstall.KindRemoveLink:
			links++
		case uninstall.KindRemoveCopy:
			copies++
		case uninstall.KindCutBlock:
			blocks++
		case uninstall.KindRemoveDir:
			dirs++
		}
		if it.Restores {
			restored++
		}
	}
	var out []opCount
	// Each label is a plural message that leaves the number out.
	add := func(n int, key string) {
		if n > 0 {
			out = append(out, opCount{n, tr.N(key, n, nil)})
		}
	}
	add(links, i18n.Key("uninstall.count.links"))
	add(copies, i18n.Key("uninstall.count.copies"))
	add(dirs, i18n.Key("uninstall.count.dirs"))
	add(restored, i18n.Key("uninstall.count.restored"))
	add(blocks, i18n.Key("uninstall.count.blocks"))
	return out
}

// uninstallWord names what an uninstall does to one item.
func uninstallWord(tr i18n.Translator, k uninstall.ItemKind) string {
	switch k {
	case uninstall.KindRemoveLink:
		return tr.T("uninstall.word.link", nil)
	case uninstall.KindRemoveCopy:
		return tr.T("uninstall.word.copy", nil)
	case uninstall.KindCutBlock:
		return tr.T("uninstall.word.block", nil)
	case uninstall.KindRemoveDir:
		return tr.T("uninstall.word.dir", nil)
	}
	return string(k)
}

// uninstallBody is the head (heading and counts line) and one row per item;
// an empty preview says why there is nothing to do. The rows scroll.
func uninstallBody(c chrome, l layout, home string, items []uninstall.Item) (head, rows []string) {
	out := c.headingRow(l, c.tr.T("uninstall.title", nil), c.theme.Strong, "", c.theme.Muted)
	out = append(out, "")
	if len(items) == 0 {
		return append(out, c.theme.State(StateOK, c.fit(c.tr.T("uninstall.nothing", nil), l.width-2))), nil
	}
	var parts []string
	for _, n := range uninstallCounts(c.tr, items) {
		parts = append(parts, fmt.Sprintf("%d %s", n.N, n.Label))
	}
	for _, s := range c.wrap(strings.Join(parts, c.sep()), l.width) {
		out = append(out, c.theme.Text.Render(s))
	}
	out = append(out, "")
	head = out

	note := c.tr.T("uninstall.note", nil)
	// The state cell is the glyph, a space and the widest word.
	stateW := lipgloss.Width(c.theme.Glyphs.Warn) + 1
	words := 0
	for _, k := range []uninstall.ItemKind{uninstall.KindRemoveLink, uninstall.KindRemoveCopy, uninstall.KindCutBlock, uninstall.KindRemoveDir} {
		words = max(words, lipgloss.Width(uninstallWord(c.tr, k)))
	}
	stateW += words
	lead := stateW + 2 + 2
	room := max(l.width-lead, 1)
	paths := make([]string, len(items))
	pathW := 0
	for i, it := range items {
		paths[i] = tildeUnder(home, it.Path)
		pathW = max(pathW, lipgloss.Width(paths[i]))
	}
	pathW = min(pathW, room)
	withNote := room-pathW >= 2+lipgloss.Width(note)
	for i, it := range items {
		state := c.theme.State(StateWarn, uninstallWord(c.tr, it.Kind))
		cell := state + strings.Repeat(" ", max(stateW-lipgloss.Width(state), 0)) + "  "
		line := cell + c.theme.Text.Render(c.pad(c.fitMiddle(paths[i], pathW), pathW))
		if it.Restores && withNote {
			line += "  " + c.theme.Muted.Render(note)
		}
		rows = append(rows, strings.TrimRight(line, " "))
	}
	return head, rows
}

func init() { registerView(viewUninstall, newUninstallView) }
