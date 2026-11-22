package deck

import (
	"bytes"
	_ "embed"
	"html/template"
)

//go:embed assets/deck.css
var deckCSS string

//go:embed assets/deck.html.tmpl
var deckTmpl string

var page = template.Must(template.New("deck").Funcs(template.FuncMap{
	"inc": func(i int) int { return i + 1 },
	"tone": func(t string) string {
		if t == "" {
			return ""
		}
		return "tone-" + t
	},
	// bg alternates the plain background so consecutive slides read as distinct
	// pages; covers, dividers and closings sit on the deepest tone.
	"bg": func(layout string, i int) string {
		switch layout {
		case "cover", "section", "closing":
			return "deep"
		}
		if i%2 == 0 {
			return "alt"
		}
		return ""
	},
}).Parse(deckTmpl))

// Render returns the deck as one self-contained HTML page, one 1920x1080 page
// per slide. The stylesheet is trusted (embedded); every text is escaped.
func Render(d Deck) (string, error) {
	lang := d.Lang
	if lang == "" {
		lang = "en"
	}
	var b bytes.Buffer
	err := page.Execute(&b, struct {
		Deck
		Lang string
		CSS  template.CSS
	}{d, lang, template.CSS(deckCSS)})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

//go:embed assets/example.deck.json
var exampleSpec string

// Example is a complete, valid spec that uses every layout and the full story
// arc; agents copy its shape.
func Example() string { return exampleSpec }
