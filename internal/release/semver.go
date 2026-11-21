// Package release computes versions and release notes from git history.
package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a SemVer 2.0.0 version. Pre is the prerelease part without the
// leading dash ("rc.1"), empty for a stable version.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

// Channels a build can belong to.
const (
	ChannelStable     = "stable"
	ChannelPrerelease = "prerelease"
	ChannelDev        = "dev"
)

var (
	semverRe = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
	// `git describe` between tags yields v0.1.0-3-gabc1234: a dev build.
	describeRe = regexp.MustCompile(`-\d+-g[0-9a-f]+$`)
)

// Parse reads "1.2.3", "v1.2.3" or "v1.2.3-rc.1".
func Parse(s string) (Version, error) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%q is not a SemVer version", s)
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	return Version{Major: n(1), Minor: n(2), Patch: n(3), Pre: m[4]}, nil
}

// String renders the version without the "v" prefix.
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Tag renders the git tag name, with the "v" prefix.
func (v Version) Tag() string { return "v" + v.String() }

// IsPre reports whether this is a prerelease.
func (v Version) IsPre() bool { return v.Pre != "" }

// Compare orders versions by SemVer precedence: -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for _, p := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	return comparePre(v.Pre, o.Pre)
}

func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		switch {
		case xerr == nil && yerr == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case xerr == nil: // numeric identifiers sort before alphanumeric
			return -1
		case yerr == nil:
			return 1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

// Channel classifies a version string stamped into a binary: stable,
// prerelease, or dev for anything that is not a clean release version.
func Channel(s string) string {
	if describeRe.MatchString(s) {
		return ChannelDev
	}
	v, err := Parse(s)
	switch {
	case err != nil:
		return ChannelDev
	case v.IsPre():
		return ChannelPrerelease
	}
	return ChannelStable
}
