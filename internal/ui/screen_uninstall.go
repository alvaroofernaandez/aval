package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/core/uninstall"
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
	uninstallSafety = "Everything is recorded in ~/.aval-backup first · undo: aval restore latest"
	undoLatest      = "aval restore latest"
	// uninstallStateW is the widest state cell ("! remove link").
	uninstallStateW = 13
)

func newUninstallView(svc Services) screen {
	o, err := newOperation(operationSpec[[]uninstall.Item]{
		Kind: opAtomic,
		Mode: confirmDestructive,
		Confirm: confirmSpec{
			Action:     "Uninstall",
			ActionNote: "remove the links, put back what was there",
			Safety:     uninstallSafety,
		},
		Ready: func(items []uninstall.Item) bool { return len(items) > 0 },
		Preview: func(context.Context) ([]uninstall.Item, error) {
			if svc.UninstallPreview == nil {
				return nil, errors.New("uninstall is not available here")
			}
			return svc.UninstallPreview()
		},
		Run: func(_ context.Context, _ []uninstall.Item) (opResult, error) {
			if svc.Uninstall == nil {
				return opResult{}, errors.New("uninstall is not available here")
			}
			done, err := svc.Uninstall()
			if err != nil {
				return opResult{}, err
			}
			counts := uninstallCounts(done)
			return opResult{
				Summary: "Uninstalled. Put everything back with " + undoLatest + ".",
				Counts:  counts,
				Undo:    undoLatest,
			}, nil
		},
		PreviewRows: func(c chrome, l layout, items []uninstall.Item) ([]string, []string) {
			return uninstallBody(c, l, svc.Home, items)
		},
		PreviewStatus: "Nothing has changed yet",
		Loading:       "Reading what aval linked",
		Running:       "Uninstalling",
	})
	if err != nil {
		panic(err)
	}
	return glyphSepScreen{screen: o, sep: " · "}
}

// uninstallCounts is the result's counts, with zero counts left out.
func uninstallCounts(items []uninstall.Item) []opCount {
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
	add := func(n int, label string) {
		if n > 0 {
			out = append(out, opCount{n, label})
		}
	}
	add(links, uninstallNoun(links, "link removed", "links removed"))
	add(copies, uninstallNoun(copies, "copy removed", "copies removed"))
	add(dirs, uninstallNoun(dirs, "empty directory removed", "empty directories removed"))
	add(restored, uninstallNoun(restored, "previous file restored", "previous files restored"))
	add(blocks, uninstallNoun(blocks, "rules block cut", "rules blocks cut"))
	return out
}

func uninstallNoun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// uninstallWord names what an uninstall does to one item.
func uninstallWord(k uninstall.ItemKind) string {
	switch k {
	case uninstall.KindRemoveLink:
		return "remove link"
	case uninstall.KindRemoveCopy:
		return "remove copy"
	case uninstall.KindCutBlock:
		return "cut block"
	case uninstall.KindRemoveDir:
		return "remove dir"
	}
	return string(k)
}

// uninstallBody is the head (heading and counts line) and one row per item;
// an empty preview says why there is nothing to do. The rows scroll.
func uninstallBody(c chrome, l layout, home string, items []uninstall.Item) (head, rows []string) {
	out := c.headingRow(l, "Uninstall", c.theme.Strong, "", c.theme.Muted)
	out = append(out, "")
	if len(items) == 0 {
		return append(out, c.theme.State(StateOK, c.fit("nothing to uninstall: no link points into this repo", l.width-2))), nil
	}
	var parts []string
	for _, n := range uninstallCounts(items) {
		parts = append(parts, fmt.Sprintf("%d %s", n.N, n.Label))
	}
	for _, s := range c.wrap(strings.Join(parts, c.sep()), l.width) {
		out = append(out, c.theme.Text.Render(s))
	}
	out = append(out, "")
	head = out

	const note = "restores the previous file"
	lead := uninstallStateW + 2 + 2 // glyph and space are in the state cell
	room := max(l.width-lead, 1)
	paths := make([]string, len(items))
	pathW := 0
	for i, it := range items {
		paths[i] = tildeUnder(home, it.Path)
		pathW = max(pathW, lipgloss.Width(paths[i]))
	}
	pathW = min(pathW, room)
	withNote := room-pathW >= 2+len(note)
	for i, it := range items {
		state := c.theme.State(StateWarn, uninstallWord(it.Kind))
		cell := state + strings.Repeat(" ", max(uninstallStateW-lipgloss.Width(state), 0)) + "  "
		line := cell + c.theme.Text.Render(c.pad(c.fitMiddle(paths[i], pathW), pathW))
		if it.Restores && withNote {
			line += "  " + c.theme.Muted.Render(note)
		}
		rows = append(rows, strings.TrimRight(line, " "))
	}
	return head, rows
}

func init() { registerView(viewUninstall, newUninstallView) }
