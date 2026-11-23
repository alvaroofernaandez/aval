package release

// The GitHub environments a release tag publishes from. EnvRelease requires
// the owner's approval; EnvPatch does not, so a patch publishes on its own.
const (
	EnvRelease = "release"
	EnvPatch   = "release-patch"
)

// Environment picks the environment a tag publishes from, given every tag in
// the repository. Only a stable patch (Z > 0) whose minor X.Y.0 is already
// tagged publishes without approval; a prerelease, a minor, a major, or a
// patch over an unreleased minor waits for the owner.
func Environment(v Version, tags []Version) string {
	if v.IsPre() || v.Patch == 0 {
		return EnvRelease
	}
	base := Version{Major: v.Major, Minor: v.Minor}
	for _, t := range tags {
		if t == base {
			return EnvPatch
		}
	}
	return EnvRelease
}
