package deck

import (
	"errors"
	"slices"
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
		slide  int // the slide every problem is on; 0 for the deck itself
	}{
		{"no title", func(d *Deck) { d.Title = "" }, 0},
		{"no slides", func(d *Deck) { d.Slides = nil }, 0},
		{"first slide not a cover", func(d *Deck) { d.Slides[0].Layout = "cards"; d.Slides[0].Items = items(2) }, 1},
		{"unknown layout", func(d *Deck) { d.Slides[1].Layout = "bullets" }, 2},
		{"missing slide title", func(d *Deck) { d.Slides[1].Title = "" }, 2},
		{"too many cards", func(d *Deck) { d.Slides[1].Items = items(6) }, 2},
		{"title too long", func(d *Deck) { d.Slides[1].Title = strings.Repeat("x", 81) }, 2},
		{"item text too long", func(d *Deck) { d.Slides[1].Items[0].Text = strings.Repeat("x", 161) }, 2},
		{"bad tone", func(d *Deck) { d.Slides[1].Items[0].Tone = "red" }, 2},
		{"timeline needs tags", func(d *Deck) {
			d.Slides[1] = Slide{Layout: "timeline", Title: "A day", Items: []Item{{Title: "a"}, {Title: "b"}}}
		}, 2},
		{"table ragged row", func(d *Deck) {
			d.Slides[1] = Slide{Layout: "table", Title: "Decisions", Columns: []string{"a", "b"}, Rows: [][]string{{"1", "2"}, {"3"}}}
		}, 2},
		{"too many slides", func(d *Deck) {
			for len(d.Slides) < 41 {
				d.Slides = append(d.Slides, Slide{Layout: "section", Title: "More"})
			}
		}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := valid()
			c.mutate(&d)
			var se *SpecError
			if err := d.Validate(); !errors.As(err, &se) || len(se.Problems) == 0 {
				t.Fatalf("err = %v, want a *SpecError", err)
			}
			for _, p := range se.Problems {
				if p.Slide != c.slide {
					t.Errorf("problem %q is on slide %d, want %d", p.Text, p.Slide, c.slide)
				}
			}
		})
	}
}

// Every problem is reported at once, so a fix is one round trip, not one per error.
func TestValidateReportsAllProblems(t *testing.T) {
	d := valid()
	d.Slides[1].Title = ""
	d.Slides[2].Layout = "nope"
	var se *SpecError
	if err := d.Validate(); !errors.As(err, &se) {
		t.Fatalf("err = %v, want a *SpecError", err)
	}
	var got []int
	for _, p := range se.Problems {
		got = append(got, p.Slide)
	}
	if !slices.Equal(got, []int{2, 3}) {
		t.Errorf("problems on slides %v, want [2 3]", got)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	// Valid but for the unknown field: the decoder refused it, not Validate.
	_, err := Parse(strings.NewReader(`{"title":"x","slides":[{"layout":"cover","title":"t","subtitle":"oops"}]}`))
	var se *SpecError
	if err == nil || errors.As(err, &se) {
		t.Fatalf("err = %v, want the decoder to refuse the unknown field", err)
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
