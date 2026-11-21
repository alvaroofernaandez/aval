package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveBuild(t *testing.T) {
	vcs := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.time", Value: "2026-01-02T03:04:05Z"},
		},
	}
	installed := &debug.BuildInfo{Main: debug.Module{Version: "v0.3.0"}}

	cases := []struct {
		name                  string
		version, commit, date string
		bi                    *debug.BuildInfo
		want                  buildInfo
	}{
		{"ldflags win", "0.1.0-rc.1", "abc1234", "2026-05-06T00:00:00Z", vcs, buildInfo{"0.1.0-rc.1", "abc1234", "2026-05-06T00:00:00Z"}},
		{"plain go build uses vcs", "dev", "", "", vcs, buildInfo{"dev", "0123456", "2026-01-02T03:04:05Z"}},
		{"go install of a tag", "dev", "", "", installed, buildInfo{"0.3.0", "unknown", "unknown"}},
		{"nothing known", "dev", "", "", nil, buildInfo{"dev", "unknown", "unknown"}},
		{"ldflags version survives a module version", "0.2.0", "", "", installed, buildInfo{"0.2.0", "unknown", "unknown"}},
	}
	for _, c := range cases {
		if got := resolveBuild(c.version, c.commit, c.date, c.bi); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestVersionText(t *testing.T) {
	cases := []struct {
		version, channel string
	}{
		{"0.1.0", "stable"},
		{"0.1.0-rc.1", "prerelease"},
		{"dev", "dev"},
	}
	for _, c := range cases {
		got := buildInfo{c.version, "abc1234", "2026-05-06T00:00:00Z"}.text("darwin", "arm64")
		if !strings.HasPrefix(got, "aval "+c.version+" (darwin/arm64)\n") {
			t.Errorf("first line must stay `aval <version> (<os>/<arch>)`:\n%s", got)
		}
		for _, want := range []string{"abc1234", "2026-05-06T00:00:00Z", "channel: " + c.channel} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: missing %q in:\n%s", c.version, want, got)
			}
		}
	}
}
