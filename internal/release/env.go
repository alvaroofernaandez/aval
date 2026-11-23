package release

// The GitHub environments a release tag publishes from. EnvRelease requires
// the owner's approval; EnvAuto does not, so the tag publishes on its own.
// EnvAuto keeps its first name, release-patch, which the workflow and the
// repository settings use.
const (
	EnvRelease = "release"
	EnvAuto    = "release-patch"
)

// Environment picks the environment a tag publishes from, given every tag in
// the repository. Only the first stable release of a breaking line waits for
// the owner: a new major (v1.0.0, v2.0.0) or, before 1.0, a new minor
// (v0.3.0). Prereleases, patches and minors after 1.0 publish on their own,
// unless the line's first stable release is not tagged yet: then the tag would
// skip the owner's approval of that line, so it waits too.
func Environment(v Version, tags []Version) string {
	if v.IsPre() {
		return EnvAuto
	}
	base := lineBase(v)
	if v == base {
		return EnvRelease
	}
	for _, t := range tags {
		if t == base {
			return EnvAuto
		}
	}
	return EnvRelease
}

// lineBase is the first stable release of v's breaking line: vX.0.0 from 1.0
// on, v0.Y.0 before.
func lineBase(v Version) Version {
	if v.Major == 0 {
		return Version{Minor: v.Minor}
	}
	return Version{Major: v.Major}
}
