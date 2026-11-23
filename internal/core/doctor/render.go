package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Render prints the aligned table and the summary line.
func Render(w io.Writer, rs []Result) { RenderStyled(w, rs, nil) }

// Styler decorates a status word for a terminal. The doctor package owns the
// interface so internal/core never imports the UI layer.
type Styler interface {
	OK(word string) string
	Warn(word string) string
	Fail(word string) string
	// Width is the visible width of a styled cell (escape codes excluded).
	Width(s string) int
}

// Text is the wording of the human table: the column headings, the word for
// each status, the hint on a fixable row and the summary line. The package
// holds English only; a caller that speaks another language passes its own,
// so internal/core never imports the catalog. The message of each check is
// data from the checks and is printed as it is.
type Text struct {
	Status, Check, Message string
	// Word is the status cell before styling.
	Word func(Status) string
	// Fixable is appended to a fixable row that is not OK, with its space.
	Fixable string
	// Summary renders the line under the table.
	Summary func(ok, warn, fail int) string
}

// EnglishText is the table's wording when no language is given.
func EnglishText() Text {
	return Text{
		Status: "STATUS", Check: "CHECK", Message: "MESSAGE",
		Word:    func(s Status) string { return strings.ToUpper(string(s)) },
		Fixable: " (fixable: aval doctor --fix)",
		Summary: func(ok, warn, fail int) string { return fmt.Sprintf("%d ok, %d warn, %d fail", ok, warn, fail) },
	}
}

// RenderStyled is Render with an optional Styler. A nil Styler prints the
// plain, byte-stable table scripts rely on.
func RenderStyled(w io.Writer, rs []Result, st Styler) {
	RenderText(w, rs, st, EnglishText())
}

// RenderText is RenderStyled with the table's wording given.
func RenderText(w io.Writer, rs []Result, st Styler, tx Text) {
	width := utf8.RuneCountInString(tx.Check)
	for _, r := range rs {
		width = max(width, utf8.RuneCountInString(r.ID))
	}
	cells := make([]string, len(rs))
	statusWidth := utf8.RuneCountInString(tx.Status)
	for i, r := range rs {
		cells[i] = statusCell(r.Status, st, tx)
		if st != nil {
			statusWidth = max(statusWidth, st.Width(cells[i]))
		} else {
			statusWidth = max(statusWidth, utf8.RuneCountInString(cells[i]))
		}
	}
	pad := func(cell string) string {
		n := statusWidth - utf8.RuneCountInString(cell)
		if st != nil {
			n = statusWidth - st.Width(cell)
		}
		return cell + strings.Repeat(" ", max(n, 0))
	}
	fmt.Fprintf(w, "%s  %-*s  %s\n", pad(tx.Status), width, tx.Check, tx.Message)
	var ok, warn, fail int
	for i, r := range rs {
		switch r.Status {
		case OK:
			ok++
		case Warn:
			warn++
		case Fail:
			fail++
		}
		msg := r.Message
		if r.Fixable && r.Status != OK {
			msg += tx.Fixable
		}
		fmt.Fprintf(w, "%s  %-*s  %s\n", pad(cells[i]), width, r.ID, msg)
	}
	fmt.Fprintf(w, "\n%s\n", tx.Summary(ok, warn, fail))
}

// statusCell is the status word, decorated when a Styler is given.
func statusCell(s Status, st Styler, tx Text) string {
	word := tx.Word(s)
	if st == nil {
		return word
	}
	switch s {
	case OK:
		return st.OK(word)
	case Warn:
		return st.Warn(word)
	case Fail:
		return st.Fail(word)
	}
	return word
}

// RenderJSON prints the results as a JSON array.
func RenderJSON(w io.Writer, rs []Result) error {
	if rs == nil {
		rs = []Result{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rs)
}
