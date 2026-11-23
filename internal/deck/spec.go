// Package deck turns a small JSON spec into a slide deck rendered as a PDF.
//
// The agent writes only content: a title, a layout per slide and short texts.
// The look (dark aval brand, type scale, spacing) lives in the embedded template,
// so every deck looks the same and no slide can drift from it. The limits below
// keep every slide inside its margins before anything is rendered; the overflow
// check in the browser is the second guard.
package deck

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// Deck is the whole spec.
type Deck struct {
	Title  string  `json:"title"`
	Lang   string  `json:"lang,omitempty"` // BCP 47 tag of the text, e.g. "es"; default "en"
	Slides []Slide `json:"slides"`
}

// Slide is one page. Which fields it uses depends on its layout.
type Slide struct {
	Layout  string     `json:"layout"`
	Eyebrow string     `json:"eyebrow,omitempty"` // small label above the title: the section it belongs to
	Title   string     `json:"title"`
	Lede    string     `json:"lede,omitempty"` // one or two sentences under the title
	Items   []Item     `json:"items,omitempty"`
	Columns []string   `json:"columns,omitempty"` // table header
	Rows    [][]string `json:"rows,omitempty"`    // table body
	Note    string     `json:"note,omitempty"`    // what to say; kept in the spec, not printed
}

// Item is a card, a step, a moment, a box or a row, depending on the layout.
type Item struct {
	Title string   `json:"title"`
	Text  string   `json:"text,omitempty"`
	Tag   string   `json:"tag,omitempty"`  // time on a timeline, the answer in a tree, a small label elsewhere
	Tone  string   `json:"tone,omitempty"` // "", "good", "warn" or "muted"
	Chips []string `json:"chips,omitempty"`
}

// rule is what a layout accepts.
type rule struct {
	minItems, maxItems int
	lede               bool // lede required
	tag                bool // every item needs a tag
	chips              bool // every item needs chips
	table              bool
	help               string
}

var rules = map[string]rule{
	"cover":     {help: "first slide: title and an optional lede"},
	"section":   {help: "divider between parts: title and an optional lede"},
	"statement": {lede: true, help: "one big idea: title and lede"},
	"cards":     {minItems: 2, maxItems: 4, help: "2-4 side-by-side cards: title and text"},
	"steps":     {minItems: 2, maxItems: 5, help: "2-5 numbered steps in order"},
	"timeline":  {minItems: 2, maxItems: 6, tag: true, help: "2-6 moments: tag is the time or label"},
	"checklist": {minItems: 2, maxItems: 6, help: "2-6 things that are guaranteed or included"},
	"table":     {table: true, help: "2-4 columns and 1-8 rows of short cells"},
	"flow":      {minItems: 2, maxItems: 5, help: "2-5 boxes joined by arrows, left to right"},
	"tree":      {minItems: 2, maxItems: 5, tag: true, help: "2-5 questions (title) leading to an answer (tag)"},
	"groups":    {minItems: 1, maxItems: 4, chips: true, help: "1-4 groups: title, text and chips"},
	"compare":   {minItems: 2, maxItems: 2, help: "before and after: exactly 2 items"},
	"closing":   {help: "last slide: title and an optional lede"},
}

// Text limits, in characters, sized for the 1920x1080 template.
const (
	maxSlides  = 40
	maxTitle   = 80
	maxEyebrow = 40
	maxLede    = 200
	maxItemT   = 48
	maxItemTx  = 160
	maxTag     = 40
	maxCell    = 90
	maxChip    = 24
	maxChips   = 12
	maxRows    = 8
	maxCols    = 4
	minCols    = 2
)

var tones = map[string]bool{"": true, "good": true, "warn": true, "muted": true}

// Layouts lists every layout name, sorted.
func Layouts() []string {
	out := make([]string, 0, len(rules))
	for k := range rules {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Help describes each layout in one line, for `aval deck example` and errors.
func Help() string {
	var b strings.Builder
	for _, l := range Layouts() {
		fmt.Fprintf(&b, "  %-10s %s\n", l, rules[l].help)
	}
	return b.String()
}

// Parse reads a spec strictly (unknown fields are errors, so typos surface) and
// validates it.
func Parse(r io.Reader) (Deck, error) {
	var d Deck
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Deck{}, fmt.Errorf("reading the deck spec: %w", err)
	}
	if err := d.Validate(); err != nil {
		return Deck{}, err
	}
	return d, nil
}

// Validate reports every problem at once, each with the slide it is on.
func (d Deck) Validate() error {
	var errs []Problem
	slide := 0 // the slide add attributes a problem to; 0 is the deck itself
	add := func(format string, a ...any) { errs = append(errs, Problem{slide, fmt.Sprintf(format, a...)}) }
	limit := func(where, field, s string, max int) {
		if n := utf8.RuneCountInString(s); n > max {
			add("%s: %s is %d characters, the limit is %d", where, field, n, max)
		}
	}

	if strings.TrimSpace(d.Title) == "" {
		add("the deck title is required")
	}
	switch {
	case len(d.Slides) == 0:
		add("a deck needs at least one slide")
	case len(d.Slides) > maxSlides:
		add("a deck has at most 40 slides, this one has %d: split it", len(d.Slides))
	case d.Slides[0].Layout != "cover":
		slide = 1
		add("slide 1 must be a cover, got %q", d.Slides[0].Layout)
	}

	for i, s := range d.Slides {
		n := i + 1
		slide = n
		r, ok := rules[s.Layout]
		if !ok {
			add("slide %d: unknown layout %q (one of: %s)", n, s.Layout, strings.Join(Layouts(), ", "))
			continue
		}
		where := fmt.Sprintf("slide %d (%s)", n, s.Layout)
		if strings.TrimSpace(s.Title) == "" {
			add("%s: title is required", where)
		}
		limit(where, "title", s.Title, maxTitle)
		limit(where, "eyebrow", s.Eyebrow, maxEyebrow)
		limit(where, "lede", s.Lede, maxLede)
		if r.lede && strings.TrimSpace(s.Lede) == "" {
			add("%s: lede is required", where)
		}

		if r.table {
			if c := len(s.Columns); c < minCols || c > maxCols {
				add("%s: needs %d to %d columns, got %d", where, minCols, maxCols, c)
			}
			if len(s.Rows) == 0 || len(s.Rows) > maxRows {
				add("%s: needs 1 to %d rows, got %d", where, maxRows, len(s.Rows))
			}
			for _, c := range s.Columns {
				limit(where, "a column name", c, maxCell)
			}
			for ri, row := range s.Rows {
				if len(row) != len(s.Columns) {
					add("%s row %d: has %d cells, the table has %d columns", where, ri+1, len(row), len(s.Columns))
				}
				for _, c := range row {
					limit(fmt.Sprintf("%s row %d", where, ri+1), "a cell", c, maxCell)
				}
			}
		} else if len(s.Columns) > 0 || len(s.Rows) > 0 {
			add("%s: columns and rows belong to the table layout", where)
		}

		if r.maxItems == 0 {
			if len(s.Items) > 0 {
				add("%s: takes no items", where)
			}
			continue
		}
		if c := len(s.Items); c < r.minItems || c > r.maxItems {
			if r.minItems == r.maxItems {
				add("%s: needs exactly %d items, got %d", where, r.minItems, c)
			} else {
				add("%s: needs %d to %d items, got %d", where, r.minItems, r.maxItems, c)
			}
		}
		for ii, it := range s.Items {
			iw := fmt.Sprintf("%s item %d", where, ii+1)
			if strings.TrimSpace(it.Title) == "" {
				add("%s: title is required", iw)
			}
			limit(iw, "title", it.Title, maxItemT)
			limit(iw, "text", it.Text, maxItemTx)
			limit(iw, "tag", it.Tag, maxTag)
			if !tones[it.Tone] {
				add(`%s: tone %q is not one of "", "good", "warn", "muted"`, iw, it.Tone)
			}
			if r.tag && strings.TrimSpace(it.Tag) == "" {
				add("%s: tag is required", iw)
			}
			if r.chips && (len(it.Chips) == 0 || len(it.Chips) > maxChips) {
				add("%s: needs 1 to %d chips, got %d", iw, maxChips, len(it.Chips))
			}
			if !r.chips && len(it.Chips) > 0 {
				add("%s: chips belong to the groups layout", iw)
			}
			for _, c := range it.Chips {
				limit(iw, "a chip", c, maxChip)
			}
		}
	}

	if len(errs) > 0 {
		return &SpecError{Problems: errs}
	}
	return nil
}

// SpecError is every problem Validate found, in order, so a fix is one round
// trip. Its message lists them under one heading.
type SpecError struct{ Problems []Problem }

// Problem is one broken rule. Slide is 1-based, 0 for the deck as a whole.
type Problem struct {
	Slide int
	Text  string
}

func (e *SpecError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.Text
	}
	return "the deck spec has problems:\n  " + strings.Join(lines, "\n  ")
}
