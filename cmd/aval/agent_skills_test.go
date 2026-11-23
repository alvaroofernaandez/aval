package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/docs/pdfreport"
)

func TestParsePDFAssetFlags(t *testing.T) {
	o, pos, err := parsePDFAssetFlags([]string{"proj", "--brand-dir", "b", "--people", "ana, luis,", "--logo-file", "l.png", "--ink", "1,2,3", "--accent", "4,5,6"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pos) != 1 || pos[0] != "proj" || o.BrandDir != "b" || o.LogoFile != "l.png" ||
		strings.Join(o.People, "|") != "ana|luis" || o.Ink != [3]uint8{1, 2, 3} || o.Accent != [3]uint8{4, 5, 6} {
		t.Errorf("parsed %+v %v", o, pos)
	}
	o, _, err = parsePDFAssetFlags(nil)
	if err != nil || o.BrandDir != "brand" || o.LogoFile != "logo.png" || o.Ink != [3]uint8{23, 20, 31} {
		t.Errorf("defaults %+v %v", o, err)
	}
	if _, _, err := parsePDFAssetFlags([]string{"--ink", "300,1,1"}); err == nil {
		t.Error("a bad --ink must fail")
	}
}

func TestRunPDFUsage(t *testing.T) {
	var out bytes.Buffer
	for _, args := range [][]string{nil, {"nope"}, {"init"}, {"init", "a", "b"}, {"assets", "a", "b"}} {
		if err := runPDF(args, &out); !isUsage(err) {
			t.Errorf("runPDF(%v) = %v, want usage", args, err)
		}
	}
	err := runPDF([]string{"assets", t.TempDir(), "--brand-dir", filepath.Join(t.TempDir(), "gone")}, &out)
	if !errors.Is(err, pdfreport.ErrBrandDirMissing) {
		t.Errorf("assets with a missing brand dir: %v", err)
	}
}

func TestRunKiriAndDockerUsage(t *testing.T) {
	if err := runKiri(nil); !isUsage(err) {
		t.Errorf("runKiri(nil) = %v", err)
	}
	if err := runKiri([]string{"down"}); !isUsage(err) {
		t.Errorf("runKiri(down) = %v", err)
	}
	if err := runDocker(nil); !isUsage(err) {
		t.Errorf("runDocker(nil) = %v", err)
	}
	if err := runDocker([]string{"prune"}); !isUsage(err) {
		t.Errorf("runDocker(prune) = %v", err)
	}
}
