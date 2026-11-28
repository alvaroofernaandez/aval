package usage

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Errorf("%s differs from the output:\n%s", name, got)
	}
}

func TestRenderGoldens(t *testing.T) {
	five := 42.4
	budgets := []Budget{{Headless, "7d", BudgetTokens, 20000}, {Interactive, "7d", BudgetShare, 90}}
	for _, c := range []struct {
		name string
		o    Options
	}{
		{"day_api", Options{Billing: BillingAPI, By: ByDay, Budgets: budgets}},
		{"consumer_subscription", Options{Billing: BillingSubscription, By: ByConsumer, Rates: RateLimits{FiveHour: &five}}},
		{"project_unset_window5h", Options{By: ByProject, Window: "5h"}},
	} {
		r := buildFixture(t, c.o)
		var txt, js bytes.Buffer
		if err := Render(&txt, r); err != nil {
			t.Fatal(err)
		}
		if err := RenderJSON(&js, r); err != nil {
			t.Fatal(err)
		}
		golden(t, c.name+".txt", txt.Bytes())
		golden(t, c.name+".json", js.Bytes())
	}
}

func TestNoDollarsOutsideTheAPIMode(t *testing.T) {
	for _, billing := range []string{BillingSubscription, BillingNone} {
		r := buildFixture(t, Options{Billing: billing})
		var txt, js bytes.Buffer
		_ = Render(&txt, r)
		_ = RenderJSON(&js, r)
		if txt.Len() == 0 || js.Len() == 0 {
			t.Fatal("empty output")
		}
		if bytes.ContainsAny(txt.Bytes(), "$") || bytes.Contains(js.Bytes(), []byte("cost")) {
			t.Errorf("billing %q prints a cost:\n%s\n%s", billing, txt.Bytes(), js.Bytes())
		}
	}
}
