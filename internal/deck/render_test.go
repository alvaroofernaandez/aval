package deck

import (
	"strings"
	"testing"
)

func everyLayout() Deck {
	two := []Item{{Title: "Uno", Text: "Primero", Tag: "10:00", Tone: "good"}, {Title: "Dos", Text: "Segundo", Tag: "11:00", Tone: "warn"}}
	return Deck{Title: "Todo", Lang: "es", Slides: []Slide{
		{Layout: "cover", Eyebrow: "aval", Title: "Portada", Lede: "Entradilla"},
		{Layout: "section", Title: "Parte dos"},
		{Layout: "statement", Title: "Idea", Lede: "La idea en una frase"},
		{Layout: "cards", Title: "Tarjetas", Items: two},
		{Layout: "steps", Title: "Pasos", Items: two},
		{Layout: "timeline", Title: "Un día", Items: two},
		{Layout: "checklist", Title: "Incluye", Items: two},
		{Layout: "table", Title: "Decisiones", Columns: []string{"Decidimos", "Porque"}, Rows: [][]string{{"A", "B"}}},
		{Layout: "flow", Title: "Piezas", Items: two},
		{Layout: "tree", Title: "Cómo elige", Items: two},
		{Layout: "groups", Title: "Agentes", Items: []Item{{Title: "Ni lo notas", Chips: []string{"Claude Code", "Pi"}}}},
		{Layout: "compare", Title: "Antes y después", Items: two},
		{Layout: "closing", Title: "Fin"},
	}}
}

func render(t *testing.T, d Deck) string {
	t.Helper()
	html, err := Render(d)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return html
}

func TestRenderOnePagePerSlide(t *testing.T) {
	d := everyLayout()
	html := render(t, d)
	if got := strings.Count(html, `<section class="slide`); got != len(d.Slides) {
		t.Errorf("sections = %d, want %d", got, len(d.Slides))
	}
	for _, want := range []string{"@page", "1920px 1080px", `lang="es"`, "<title>Todo</title>", "data-overflow"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestRenderEveryLayoutShowsItsContent(t *testing.T) {
	html := render(t, everyLayout())
	for _, l := range Layouts() {
		if !strings.Contains(html, "layout-"+l) {
			t.Errorf("no section for layout %q", l)
		}
	}
	for _, want := range []string{"Portada", "Entradilla", "Primero", "10:00", "Decidimos", "Claude Code", "Antes y después", "Fin", "tone-good", "tone-warn"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// Steps are numbered by the renderer, never by the author.
func TestRenderNumbersSteps(t *testing.T) {
	html := render(t, everyLayout())
	if !strings.Contains(html, `class="num">1<`) || !strings.Contains(html, `class="num">2<`) {
		t.Error("steps are not numbered 1 and 2")
	}
}

func TestRenderEscapesText(t *testing.T) {
	d := valid()
	d.Slides[0].Title = `<script>alert(1)</script>`
	html := render(t, d)
	if strings.Contains(html, "<script>alert(1)") {
		t.Fatal("slide text is not escaped")
	}
}

func TestRenderPageNumbers(t *testing.T) {
	html := render(t, valid())
	if !strings.Contains(html, ">2 / 3<") {
		t.Error("missing the page counter")
	}
}

func TestRenderDefaultsLangToEnglish(t *testing.T) {
	d := valid()
	d.Lang = ""
	if !strings.Contains(render(t, d), `lang="en"`) {
		t.Error(`want lang="en" by default`)
	}
}
