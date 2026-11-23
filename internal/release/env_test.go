package release

import "testing"

func TestEnvironment(t *testing.T) {
	tags := []Version{}
	for _, s := range []string{"v0.1.0", "v0.2.0-rc.1", "v0.2.0", "v0.2.1", "v1.0.0"} {
		v, err := Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		tags = append(tags, v)
	}
	cases := []struct{ tag, want, why string }{
		{"v0.2.2", EnvAuto, "a patch over a released line publishes on its own"},
		{"v0.2.9", EnvAuto, "a patch may skip patch numbers"},
		{"v1.0.1", EnvAuto, "the rule is the same after 1.0"},
		{"v1.1.0", EnvAuto, "a minor after 1.0 changes nothing that breaks"},
		{"v0.2.2-rc.1", EnvAuto, "a prerelease publishes on its own"},
		{"v0.4.0-rc.1", EnvAuto, "an rc of a new 0.x line publishes on its own"},
		{"v2.0.0-beta.1", EnvAuto, "a beta of a major publishes on its own"},
		{"v0.3.0", EnvRelease, "before 1.0 a minor is a breaking line and needs the owner"},
		{"v1.0.0", EnvRelease, "1.0 needs the owner"},
		{"v2.0.0", EnvRelease, "a major needs the owner"},
		{"v0.3.1", EnvRelease, "a patch over an unreleased 0.x line would skip the owner"},
		{"v2.1.0", EnvRelease, "a minor over an unreleased major would skip the owner"},
	}
	for _, c := range cases {
		v, err := Parse(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := Environment(v, tags); got != c.want {
			t.Errorf("Environment(%s) = %q, want %q: %s", c.tag, got, c.want, c.why)
		}
	}
}

// A release moves `latest` or `next` only when it is the newest of its kind:
// publishing an older rc after a newer one (or re-running an old release) must
// not send users back to it.
func TestNpmTag(t *testing.T) {
	tags := []Version{}
	for _, s := range []string{"v0.2.3", "v0.2.4", "v0.3.0-rc.1", "v0.3.0-rc.2", "v0.3.0-rc.4"} {
		v, err := Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		tags = append(tags, v)
	}
	cases := []struct{ tag, want, why string }{
		{"v0.3.0-rc.4", NpmNext, "the newest prerelease moves next"},
		{"v0.3.0-rc.5", NpmNext, "a prerelease newer than every tag moves next"},
		{"v0.3.0-rc.1", NpmSuperseded, "an older rc published late must not move next back"},
		{"v0.2.4", NpmLatest, "the newest stable moves latest"},
		{"v0.2.3", NpmSuperseded, "an older stable re-run must not move latest back"},
		{"v0.3.0", NpmLatest, "a stable newer than every stable moves latest"},
		{"v0.2.4-rc.1", NpmSuperseded, "a prerelease older than a released stable moves nothing"},
	}
	for _, c := range cases {
		v, err := Parse(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := NpmTag(v, tags); got != c.want {
			t.Errorf("NpmTag(%s) = %q, want %q: %s", c.tag, got, c.want, c.why)
		}
	}
}
