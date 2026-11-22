package deck

import (
	"strings"
	"testing"
)

func items(n int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{Title: "Item", Text: "Some text", Tag: "10:00"}
	}
	return out
}

func valid() Deck {
	return Deck{Title: "Plan", Lang: "es", Slides: []Slide{
		{Layout: "cover", Title: "A clear plan", Lede: "What we will do"},
		{Layout: "cards", Title: "Three problems", Items: items(3)},
		{Layout: "closing", Title: "Done"},
	}}
}

func TestValidateAcceptsAGoodDeck(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Deck)
		want   string
	}{
		{"no title", func(d *Deck) { d.Title = "" }, "deck title"},
		{"no slides", func(d *Deck) { d.Slides = nil }, "at least one slide"},
		{"first slide not a cover", func(d *Deck) { d.Slides[0].Layout = "cards"; d.Slides[0].Items = items(2) }, "slide 1 must be a cover"},
		{"unknown layout", func(d *Deck) { d.Slides[1].Layout = "bullets" }, `slide 2: unknown layout "bullets"`},
		{"missing slide title", func(d *Deck) { d.Slides[1].Title = "" }, "slide 2 (cards): title is required"},
		{"too many cards", func(d *Deck) { d.Slides[1].Items = items(6) }, "slide 2 (cards): needs 2 to 4 items, got 6"},
		{"title too long", func(d *Deck) { d.Slides[1].Title = strings.Repeat("x", 81) }, "slide 2 (cards): title is 81 characters, the limit is 80"},
		{"item text too long", func(d *Deck) { d.Slides[1].Items[0].Text = strings.Repeat("x", 161) }, "slide 2 (cards) item 1: text is 161 characters, the limit is 160"},
		{"bad tone", func(d *Deck) { d.Slides[1].Items[0].Tone = "red" }, `slide 2 (cards) item 1: tone "red"`},
		{"timeline needs tags", func(d *Deck) {
			d.Slides[1] = Slide{Layout: "timeline", Title: "A day", Items: []Item{{Title: "a"}, {Title: "b"}}}
		}, "slide 2 (timeline) item 1: tag is required"},
		{"table ragged row", func(d *Deck) {
			d.Slides[1] = Slide{Layout: "table", Title: "Decisions", Columns: []string{"a", "b"}, Rows: [][]string{{"1", "2"}, {"3"}}}
		}, "slide 2 (table) row 2: has 1 cells, the table has 2 columns"},
		{"too many slides", func(d *Deck) {
			for len(d.Slides) < 41 {
				d.Slides = append(d.Slides, Slide{Layout: "section", Title: "More"})
			}
		}, "at most 40 slides"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := valid()
			c.mutate(&d)
			err := d.Validate()
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
		})
	}
}

// Every problem is reported at once, so a fix is one round trip, not one per error.
func TestValidateReportsAllProblems(t *testing.T) {
	d := valid()
	d.Slides[1].Title = ""
	d.Slides[2].Layout = "nope"
	err := d.Validate()
	if err == nil || !strings.Contains(err.Error(), "slide 2") || !strings.Contains(err.Error(), "slide 3") {
		t.Fatalf("want both slides reported, got %v", err)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse(strings.NewReader(`{"title":"x","slides":[{"layout":"cover","title":"t","subtitle":"oops"}]}`))
	if err == nil || !strings.Contains(err.Error(), "subtitle") {
		t.Fatalf("want an unknown-field error naming subtitle, got %v", err)
	}
}

func TestParseValidates(t *testing.T) {
	d, err := Parse(strings.NewReader(`{"title":"x","slides":[{"layout":"cover","title":"Hello"}]}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Slides[0].Title != "Hello" {
		t.Errorf("got %+v", d)
	}
	if _, err := Parse(strings.NewReader(`{"title":"x","slides":[]}`)); err == nil {
		t.Error("Parse must validate")
	}
}

func TestEveryLayoutHasRules(t *testing.T) {
	for _, l := range Layouts() {
		if _, ok := rules[l]; !ok {
			t.Errorf("layout %q has no rules", l)
		}
	}
	if len(Layouts()) < 12 {
		t.Errorf("want the full layout catalogue, got %v", Layouts())
	}
}
