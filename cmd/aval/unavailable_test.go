package main

import (
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/manifest"
)

func TestUnavailableGroupsNameTheRealPlatform(t *testing.T) {
	m := &manifest.Manifest{Groups: []manifest.Group{
		{ID: "term", Label: "Terminal environment", Platforms: manifest.PlatformUnix},
		{ID: "git", Label: "Git", Platforms: manifest.PlatformAll},
	}}

	got := unavailableGroups(m, "windows")
	if len(got) != 1 || got[0].Label != "Terminal environment" {
		t.Fatalf("windows: got %+v, want only the unix group", got)
	}
	if want := "needs WSL on Windows"; got[0].Reason != want {
		t.Errorf("windows reason = %q, want %q", got[0].Reason, want)
	}

	// Elsewhere every group applies, so nothing may mention Windows.
	for _, goos := range []string{"darwin", "linux"} {
		for _, u := range unavailableGroups(m, goos) {
			if strings.Contains(u.Reason, "Windows") {
				t.Errorf("%s: reason %q mentions Windows", goos, u.Reason)
			}
		}
	}
	for _, u := range got {
		if strings.Contains(u.Reason, "—") {
			t.Errorf("em dash in reason %q", u.Reason)
		}
	}
}
