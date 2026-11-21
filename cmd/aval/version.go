package main

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/release"
)

// buildInfo is what a binary knows about itself.
type buildInfo struct {
	Version, Commit, Date string
}

var releaseTagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+(-(rc|beta)\.\d+)?$`)

// resolveBuild merges the ldflags-stamped values with what the Go toolchain
// recorded, so a plain `go install` or `go build` still reports something
// useful. Stamped values always win.
func resolveBuild(version, commit, date string, bi *debug.BuildInfo) buildInfo {
	b := buildInfo{Version: version, Commit: commit, Date: date}
	if bi != nil {
		if b.Version == "dev" && releaseTagRe.MatchString(bi.Main.Version) {
			b.Version = strings.TrimPrefix(bi.Main.Version, "v")
		}
		for _, s := range bi.Settings {
			switch {
			case s.Key == "vcs.revision" && b.Commit == "":
				b.Commit = s.Value
				if len(b.Commit) > 7 {
					b.Commit = b.Commit[:7]
				}
			case s.Key == "vcs.time" && b.Date == "":
				b.Date = s.Value
			}
		}
	}
	if b.Commit == "" {
		b.Commit = "unknown"
	}
	if b.Date == "" {
		b.Date = "unknown"
	}
	return b
}

// text is the multi-line `aval version` output. The first line is what
// `--version` has always printed.
func (b buildInfo) text(goos, goarch string) string {
	return fmt.Sprintf("aval %s (%s/%s)\n  commit:  %s\n  built:   %s\n  channel: %s\n",
		b.Version, goos, goarch, b.Commit, b.Date, release.Channel(b.Version))
}

// currentBuild is this binary's identity, preferring the stamped values.
func currentBuild() buildInfo {
	bi, _ := debug.ReadBuildInfo()
	return resolveBuild(version, commit, date, bi)
}
