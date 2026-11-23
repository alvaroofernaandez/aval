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
		{"v0.2.2", EnvPatch, "a patch over a released minor publishes on its own"},
		{"v0.2.9", EnvPatch, "a patch may skip patch numbers"},
		{"v1.0.1", EnvPatch, "the rule is the same after 1.0"},
		{"v0.3.0", EnvRelease, "a minor needs the owner"},
		{"v2.0.0", EnvRelease, "a major needs the owner"},
		{"v0.2.2-rc.1", EnvRelease, "a prerelease needs the owner"},
		{"v0.3.1", EnvRelease, "a patch over an unreleased minor would skip the owner"},
		{"v0.4.0-rc.1", EnvRelease, "an rc of a minor needs the owner"},
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
