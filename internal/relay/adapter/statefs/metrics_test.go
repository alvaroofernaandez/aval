package statefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMetrics_AppendAndReadBack(t *testing.T) {
	s := store(t)
	first := Metric{At: at, Session: "s1", Trigger: "auto", Action: "replace", Source: "exact", Before: 212_000, After: 61_000, Floor: 58_000}
	second := first
	second.Session, second.Action = "s2", "steer"
	for _, m := range []Metric{first, second} {
		if err := s.AppendMetric(m); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ReadMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("read back %+v, want [%+v %+v]", got, first, second)
	}
}

func TestMetrics_NoFileIsNoMetrics(t *testing.T) {
	got, err := store(t).ReadMetrics()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no metrics and no error", got, err)
	}
}

func TestMetrics_TornLineIsSkipped(t *testing.T) {
	s := store(t)
	if err := s.AppendMetric(Metric{At: at, Session: "s1", Before: 1}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, "metrics.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"at":"2026-11`)
	f.Close()
	if got, err := s.ReadMetrics(); err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v; want the one whole line", got, err)
	}
}

func TestMetrics_FileIs0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	s := store(t)
	if err := s.AppendMetric(Metric{At: at, Session: "s1"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(s.Dir, "metrics.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", fi.Mode().Perm())
	}
}
