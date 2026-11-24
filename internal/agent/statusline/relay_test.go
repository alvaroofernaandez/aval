package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC)

// relayState writes metrics.jsonl lines, each a repo and an age.
func relayState(t testing.TB, rows ...any) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < len(rows); i += 2 {
		fmt.Fprintf(&b, `{"at":%q,"trigger":"auto","action":"replace","tokens_before":200000,"tokens_after":40000,"repo":%q}`+"\n",
			now.Add(-rows[i+1].(time.Duration)).Format(time.RFC3339Nano), rows[i].(string))
	}
	if err := os.WriteFile(filepath.Join(dir, "metrics.jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func statusInput(dir string) string {
	return `{"model":{"display_name":"Opus"},"workspace":{"current_dir":"` + filepath.ToSlash(dir) + `","project_dir":"` + filepath.ToSlash(dir) + `"}}`
}

func TestRender_RelaySegment(t *testing.T) {
	d := repo(t, "main")
	state := relayState(t, d, time.Hour, d, 2*time.Hour, "/other/repo", time.Hour, d, 13*time.Hour, d, 11*time.Hour)
	got := plain(RenderWith(strings.NewReader(statusInput(d)), Options{RelayDir: state, Now: now}))
	if !strings.HasSuffix(got, " · relay 3") {
		t.Fatalf("got %q, want it to end with the segment relay 3", got)
	}
}

func TestRender_NoRelayStateIsByteIdentical(t *testing.T) {
	d := repo(t, "main")
	in := statusInput(d)
	want := Render(strings.NewReader(in))
	for name, o := range map[string]Options{
		"no dir":          {Now: now},
		"missing file":    {RelayDir: t.TempDir(), Now: now},
		"other repo only": {RelayDir: relayState(t, "/other/repo", time.Hour), Now: now},
		"outside window":  {RelayDir: relayState(t, d, 13*time.Hour), Now: now},
	} {
		if got := RenderWith(strings.NewReader(in), o); got != want {
			t.Errorf("%s: got %q, want the line without a segment %q", name, got, want)
		}
	}
}

func TestRender_RelaySegmentSurvivesATornLine(t *testing.T) {
	d := repo(t, "main")
	state := relayState(t, d, time.Hour)
	f, _ := os.OpenFile(filepath.Join(state, "metrics.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"at":"2026-11`)
	f.Close()
	if got := plain(RenderWith(strings.NewReader(statusInput(d)), Options{RelayDir: state, Now: now})); !strings.HasSuffix(got, " · relay 1") {
		t.Fatalf("got %q, want relay 1", got)
	}
}

func BenchmarkRenderWithRelay(b *testing.B) {
	d := b.TempDir()
	rows := make([]any, 0, 4000)
	for i := 0; i < 2000; i++ {
		rows = append(rows, d, time.Duration(i)*time.Minute)
	}
	o := Options{RelayDir: relayState(b, rows...), Now: now}
	in := statusInput(d)
	b.ResetTimer()
	for b.Loop() {
		RenderWith(strings.NewReader(in), o)
	}
}
