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
