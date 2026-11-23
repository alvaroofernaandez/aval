package ui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// confirm is the confirm step of the operation pattern (DESIGN.md section 10):
// two rows, the action and Cancel, chosen with the arrows and enter. There is
// no y key and no box, and the safety net is always named in the status line.

// confirmMode says where the rows sit and which row the cursor opens on.
type confirmMode int

const (
	// confirmDestructive is a step of its own, opening on Cancel: restore,
	// uninstall, a doctor fix, sandbox reset and remove. esc is Cancel.
	confirmDestructive confirmMode = iota
	// confirmInline sits under the preview, opening on the action: update
	// and sync. Its cancel row is Back.
	confirmInline
)

// confirmSpec is the text of a confirm step.
type confirmSpec struct {
	// Action is the action row, for example "Remove 2 links". Required.
	Action, ActionNote string
	// Cancel defaults to "Cancel" (destructive) or "Back" (inline);
	// CancelNote to "change nothing". Both default when the step renders,
	// so they follow the language.
	Cancel, CancelNote string
	// Prompt is an optional line above the rows on a destructive step.
	Prompt string
	// Safety names the safety net, or says there is none. Required: the
	// confirm step always says what catches a mistake.
	Safety string
}

// confirmChoice is what a key did to a confirm.
type confirmChoice int

const (
	choiceNone   confirmChoice = iota // moved, or nothing
	choiceAction                      // enter on the action row
	choiceCancel                      // enter on the cancel row, or esc on a destructive step
)

const (
	confirmRowAction = 0
	confirmRowCancel = 1
)

// Confirm and operation actions, negative like the other screen-local ones.
const (
	actConfirmChoose action = -90 // enter on a row
	actConfirmCancel action = -91 // esc on a destructive step
	actOpContinue    action = -92 // enter on a destructive preview
	actOpClose       action = -93 // enter on the result
)

var (
	errNoSafety = errors.New("confirm: the safety line is required; name the safety net or say there is none")
	errNoAction = errors.New("confirm: the action row needs a label")
)

type confirm struct {
	mode   confirmMode
	spec   confirmSpec
	cursor int
}

// newConfirm builds a confirm step. An empty safety line or action label is
// an error, so no view can ship a confirm that hides its safety net.
func newConfirm(mode confirmMode, spec confirmSpec) (confirm, error) {
	if strings.TrimSpace(spec.Safety) == "" {
		return confirm{}, errNoSafety
	}
	if strings.TrimSpace(spec.Action) == "" {
		return confirm{}, errNoAction
	}
	c := confirm{mode: mode, spec: spec, cursor: confirmRowAction}
	if mode == confirmDestructive {
		c.cursor = confirmRowCancel
	}
	return c, nil
}

func (c confirm) onCancel() bool { return c.cursor == confirmRowCancel }

// update handles one key from the confirm's own map.
func (c confirm) update(msg tea.KeyMsg) (confirm, confirmChoice) {
	act, ok := c.keys().lookup(msg.String())
	if !ok {
		return c, choiceNone
	}
	switch act {
	case actUp:
		c.cursor = confirmRowAction
	case actDown:
		c.cursor = confirmRowCancel
	case actConfirmCancel:
		return c, choiceCancel
	case actConfirmChoose:
		if c.onCancel() {
			return c, choiceCancel
		}
		return c, choiceAction
	}
	return c, choiceNone
}

var confirmKeys = keyMap{
	{keys: []string{"up", "k"}, action: actUp, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{"down", "j"}, action: actDown, label: labelUpDown, desc: i18n.Key("key.move")},
	{keys: []string{"enter"}, action: actConfirmChoose, label: "enter", desc: i18n.Key("key.choose"), core: true},
}

// confirmEsc is esc on a destructive step. It has no hint: the router's
// `esc back` already says it, and back from the step is the preview.
var confirmEsc = binding{keys: []string{"esc"}, action: actConfirmCancel, desc: i18n.Key("key.cancel")}

// keys is the confirm's map. Inline leaves esc to the router, which pops the
// view, the same as Back.
func (c confirm) keys() keyMap {
	if c.mode == confirmDestructive {
		return append(append(keyMap{}, confirmKeys...), confirmEsc)
	}
	return confirmKeys
}

// lines renders the optional prompt and the two rows: `› Cancel` with the
// cursor, `  Remove 2 links` without. Notes sit beside the labels when they
// fit, and wrap under them otherwise.
func (c confirm) lines(ch chrome, l layout) []string {
	var out []string
	if c.mode == confirmDestructive && c.spec.Prompt != "" {
		for _, p := range ch.wrap(c.spec.Prompt, l.width) {
			out = append(out, ch.theme.Text.Render(p))
		}
		out = append(out, "")
	}
	cancel, note := c.spec.Cancel, c.spec.CancelNote
	if cancel == "" {
		cancel = ch.tr.T("confirm.cancel", nil)
		if c.mode == confirmInline {
			cancel = ch.tr.T("confirm.back", nil)
		}
	}
	if note == "" {
		note = ch.tr.T("confirm.change_nothing", nil)
	}
	rows := [2][2]string{{c.spec.Action, c.spec.ActionNote}, {cancel, note}}
	const lead = 2 // the cursor glyph and its space
	labelW := max(lipgloss.Width(rows[0][0]), lipgloss.Width(rows[1][0]))
	labelW = min(labelW, max(l.width-lead, 1))
	indent := strings.Repeat(" ", min(lead, max(l.width-1, 0)))
	for i, r := range rows {
		marker := "  "
		cell := ch.theme.Text.Render(ch.pad(r[0], labelW))
		if i == c.cursor {
			marker = ch.theme.Marker.Render(ch.theme.Glyphs.Cursor + " ")
			cell = ch.theme.Selected.Render(ch.pad(r[0], labelW))
		}
		line := marker + cell
		noteW := l.width - lead - labelW - 2
		switch {
		case r[1] == "":
			out = append(out, strings.TrimRight(line, " "))
		case lipgloss.Width(r[1]) <= noteW:
			out = append(out, line+"  "+ch.theme.Muted.Render(r[1]))
		default:
			out = append(out, strings.TrimRight(line, " "))
			for _, nl := range ch.wrap(r[1], max(l.width-len(indent), 1)) {
				out = append(out, indent+ch.theme.Muted.Render(nl))
			}
		}
	}
	return out
}

// status is the safety line, for the frame's status area.
func (c confirm) status() string { return c.spec.Safety }
