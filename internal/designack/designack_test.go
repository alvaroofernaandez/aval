package designack

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sum(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestAckWritesMarkerCompatibleWithGate(t *testing.T) {
	proj := t.TempDir()
	if err := os.Mkdir(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	design := "# DESIGN\n"
	if err := os.WriteFile(filepath.Join(proj, ".agents", "DESIGN.md"), []byte(design), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(proj, "src", "ui")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	now := time.Unix(1700000000, 0)

	var out bytes.Buffer
	if err := Ack(&out, Options{Surface: "pricing", Dir: sub, Cache: cache, Now: now}); err != nil {
		t.Fatal(err)
	}

	root, _ := filepath.EvalSymlinks(proj)
	got, err := os.ReadFile(filepath.Join(cache, "design-gate", sum(root)))
	if err != nil {
		t.Fatalf("marker not at design-gate/<sha1 of root>: %v", err)
	}
	want := "surface=pricing\nepoch=1700000000\ndesign_sha1=" + sum(design) + "\n"
	if string(got) != want {
		t.Errorf("marker =\n%q\nwant\n%q", got, want)
	}
	for _, s := range []string{"design-ack: pricing recorded for " + root, "valid 240 min", "hash locked in", "aval design gate"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("output lacks %q:\n%s", s, out.String())
		}
	}
}

func TestAckNoDesignFileAndNoGit(t *testing.T) {
	dir := t.TempDir()
	cache := t.TempDir()
	var out bytes.Buffer
	if err := Ack(&out, Options{Surface: "x", Dir: dir, Cache: cache, Now: time.Unix(5, 0), TTL: 60}); err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.EvalSymlinks(dir)
	got, err := os.ReadFile(filepath.Join(cache, "design-gate", sum(root)))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "surface=x\nepoch=5\ndesign_sha1=\n" {
		t.Errorf("marker = %q", got)
	}
	if !strings.Contains(out.String(), "valid 1 min") || !strings.Contains(out.String(), "no .agents/DESIGN.md yet") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestAckRequiresSurface(t *testing.T) {
	if err := Ack(&bytes.Buffer{}, Options{Dir: t.TempDir(), Cache: t.TempDir()}); err == nil {
		t.Error("an empty surface must be rejected")
	}
}

func TestMarkerPathAndRead(t *testing.T) {
	cache := t.TempDir()
	p := MarkerPath(cache, "/p/root")
	if want := filepath.Join(cache, "design-gate", sum("/p/root")); p != want {
		t.Fatalf("MarkerPath = %q, want %q", p, want)
	}
	if _, ok := ReadMarker(p); ok {
		t.Error("an absent marker must report ok=false")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("surface=a=b\nepoch=12\nepoch=34\ndesign_sha1=abc\njunk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, ok := ReadMarker(p)
	if !ok || m.Epoch != "34" || m.DesignSHA1 != "abc" {
		t.Errorf("ReadMarker = %+v ok=%v, want the last epoch=34 and design_sha1=abc", m, ok)
	}
}

func TestSha1Hex(t *testing.T) {
	if got := Sha1Hex([]byte("x")); got != sum("x") {
		t.Errorf("Sha1Hex = %q", got)
	}
}
