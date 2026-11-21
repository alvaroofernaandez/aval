package release

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		want    Version
		wantErr bool
	}{
		{"1.2.3", Version{1, 2, 3, ""}, false},
		{"v1.2.3", Version{1, 2, 3, ""}, false},
		{"v0.1.0-rc.1", Version{0, 1, 0, "rc.1"}, false},
		{"v2.0.0-beta.12", Version{2, 0, 0, "beta.12"}, false},
		{"v1.2", Version{}, true},
		{"v1.2.3.4", Version{}, true},
		{"vx.y.z", Version{}, true},
		{"v01.2.3", Version{}, true},
		{"latest", Version{}, true},
		{"dev", Version{}, true},
		{"", Version{}, true},
		{"v1.2.3-", Version{}, true},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("Parse(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestStringAndTag(t *testing.T) {
	v := Version{0, 3, 1, "rc.2"}
	if v.String() != "0.3.1-rc.2" {
		t.Errorf("String() = %q", v.String())
	}
	if v.Tag() != "v0.3.1-rc.2" {
		t.Errorf("Tag() = %q", v.Tag())
	}
	if (Version{1, 0, 0, ""}).Tag() != "v1.0.0" {
		t.Error("stable Tag() wrong")
	}
}

func TestCompare(t *testing.T) {
	ordered := []string{
		"0.1.0-beta.1", "0.1.0-beta.2", "0.1.0-beta.10", "0.1.0-rc.1", "0.1.0-rc.2",
		"0.1.0", "0.1.1", "0.2.0", "1.0.0-rc.1", "1.0.0", "1.10.0", "2.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			a, _ := Parse(ordered[i])
			b, _ := Parse(ordered[j])
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := a.Compare(b); got != want {
				t.Errorf("%s.Compare(%s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
}

func TestChannel(t *testing.T) {
	cases := map[string]string{
		"1.2.3":           ChannelStable,
		"v1.2.3":          ChannelStable,
		"0.1.0-rc.1":      ChannelPrerelease,
		"v0.1.0-beta.3":   ChannelPrerelease,
		"dev":             ChannelDev,
		"":                ChannelDev,
		"v0.1.0-3-gabc12": ChannelDev, // `git describe` output between tags
		"abc1234":         ChannelDev,
		"latest":          ChannelDev,
	}
	for in, want := range cases {
		if got := Channel(in); got != want {
			t.Errorf("Channel(%q) = %q, want %q", in, got, want)
		}
	}
}
