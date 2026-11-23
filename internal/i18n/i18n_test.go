package i18n

import "testing"

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// planted is a catalog the formatter tests own, so they never depend on the
// real keys.
var planted = map[Lang]catalog{
	English: {
		"greet":      {other: "hello {name}, bye {name}"},
		"files":      {one: "{count} file in {dir}", other: "{count} files in {dir}"},
		"only.en":    {other: "english only"},
		"no.holders": {other: "plain"},
	},
	Spanish: {
		"greet": {other: "hola {name}, adiós {name}"},
		"files": {one: "{count} archivo en {dir}", other: "{count} archivos en {dir}"},
	},
}

func TestTFillsNamedPlaceholders(t *testing.T) {
	tr := Translator{lang: English, cats: planted}
	if got := tr.T("greet", Args{"name": "Ada"}); got != "hello Ada, bye Ada" {
		t.Errorf("T = %q", got)
	}
	// A placeholder with no value stays visible rather than vanishing.
	if got := tr.T("greet", nil); got != "hello {name}, bye {name}" {
		t.Errorf("T without args = %q", got)
	}
	if got := (Translator{lang: Spanish, cats: planted}).T("greet", Args{"name": "Ada"}); got != "hola Ada, adiós Ada" {
		t.Errorf("Spanish T = %q", got)
	}
}

func TestNPicksOneOrOther(t *testing.T) {
	cases := []struct {
		lang  Lang
		count int
		want  string
	}{
		{English, 1, "1 file in /r"},
		{English, 0, "0 files in /r"},
		{English, 2, "2 files in /r"},
		{Spanish, 1, "1 archivo en /r"},
		{Spanish, 5, "5 archivos en /r"},
	}
	for _, c := range cases {
		tr := Translator{lang: c.lang, cats: planted}
		if got := tr.N("files", c.count, Args{"dir": "/r"}); got != c.want {
			t.Errorf("%s N(%d) = %q, want %q", c.lang, c.count, got, c.want)
		}
	}
}

func TestLookupFallsBackToEnglish(t *testing.T) {
	es := Translator{lang: Spanish, cats: planted}
	if got := es.T("only.en", nil); got != "english only" {
		t.Errorf("missing Spanish key = %q, want the English text", got)
	}
	if got := es.T("nowhere", nil); got != "nowhere" {
		t.Errorf("unknown key = %q, want the key itself", got)
	}
}

func TestNewAndLang(t *testing.T) {
	if got := New(Spanish).Lang(); got != Spanish {
		t.Errorf("New(es).Lang() = %q", got)
	}
	if got := New("fr").Lang(); got != English {
		t.Errorf("New(fr).Lang() = %q, want en", got)
	}
	if got := (Translator{}).Lang(); got != English {
		t.Errorf("zero Translator speaks %q, want en", got)
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Lang
	}{
		{"es_ES.UTF-8", map[string]string{"LANG": "es_ES.UTF-8"}, Spanish},
		{"es", map[string]string{"LANG": "es"}, Spanish},
		{"es_MX", map[string]string{"LANG": "es_MX"}, Spanish},
		{"es.UTF-8", map[string]string{"LANG": "es.UTF-8"}, Spanish},
		{"C", map[string]string{"LANG": "C"}, English},
		{"POSIX", map[string]string{"LANG": "POSIX"}, English},
		{"en_US.UTF-8", map[string]string{"LANG": "en_US.UTF-8"}, English},
		{"estonian is not spanish", map[string]string{"LANG": "et_EE.UTF-8"}, English},
		{"nothing set", nil, English},
		{"LC_ALL beats LANG", map[string]string{"LC_ALL": "C", "LANG": "es_ES.UTF-8"}, English},
		{"LC_ALL es beats LANG en", map[string]string{"LC_ALL": "es_ES.UTF-8", "LANG": "en_US.UTF-8"}, Spanish},
		{"LC_MESSAGES beats LANG", map[string]string{"LC_MESSAGES": "es_AR.UTF-8", "LANG": "en_GB.UTF-8"}, Spanish},
		{"LC_ALL beats LC_MESSAGES", map[string]string{"LC_ALL": "en_US.UTF-8", "LC_MESSAGES": "es_ES.UTF-8"}, English},
		{"empty LC_ALL is skipped", map[string]string{"LC_ALL": "", "LANG": "es_ES.UTF-8"}, Spanish},
	}
	for _, c := range cases {
		if got := Detect(envOf(c.env)); got != c.want {
			t.Errorf("%s: Detect = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSelect(t *testing.T) {
	es, c := envOf(map[string]string{"LANG": "es_ES.UTF-8"}), envOf(map[string]string{"LANG": "C"})
	cases := []struct {
		setting string
		env     func(string) string
		want    Lang
	}{
		{"en", es, English},
		{"es", c, Spanish},
		{"auto", es, Spanish},
		{"auto", c, English},
		{"", es, Spanish},
	}
	for _, x := range cases {
		if got := Select(x.setting, x.env); got != x.want {
			t.Errorf("Select(%q) = %q, want %q", x.setting, got, x.want)
		}
	}
}
