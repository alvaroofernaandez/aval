package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEndpointForChannel(t *testing.T) {
	// Stable builds follow `latest`; prerelease builds follow `next` (and
	// `latest`, so an rc learns when its stable ships); dev builds (go run, a
	// checkout, a git-describe build) never ask.
	cases := map[string][]string{
		"1.2.3":           {PackageURL + "/latest"},
		"0.1.0-rc.1":      {PackageURL + "/next", PackageURL + "/latest"},
		"0.1.0-beta.2":    {PackageURL + "/next", PackageURL + "/latest"},
		"dev":             nil,
		"v0.1.0-3-gabc12": nil,
	}
	for version, want := range cases {
		got := endpointsFor(PackageURL, version)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("endpointsFor(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestNoticeOnlyForNewerVersions(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"1.3.0", "1.2.0", false}, // a stale dist-tag is not an update
		{"0.1.0-rc.2", "0.1.0-rc.1", false},
		{"0.1.0-rc.1", "0.1.0-rc.2", true},
		{"0.1.0-rc.2", "0.1.0", true}, // the stable of the rc you run
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "weird", true}, // unparsable: fall back to "differs"
	}
	for _, c := range cases {
		got := Notice{Current: c.current, Latest: c.latest, Command: "x"}.String() != ""
		if got != c.want {
			t.Errorf("Notice(%s -> %s) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestCheckForPrereleaseTakesTheHighestOfNextAndLatest(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		switch r.URL.Path {
		case "/pkg/next":
			w.Write([]byte(`{"version":"0.1.0-rc.2"}`))
		case "/pkg/latest":
			w.Write([]byte(`{"version":"0.1.0"}`))
		}
	}))
	defer srv.Close()

	if n := checkFor(context.Background(), srv.URL+"/pkg", "0.1.0-rc.1"); n.Latest != "0.1.0" {
		t.Errorf("prerelease Latest = %q, want the stable 0.1.0 (asked %v)", n.Latest, asked)
	}

	asked = nil
	if n := checkFor(context.Background(), srv.URL+"/pkg", "0.0.9"); n.Latest != "0.1.0" || len(asked) != 1 || asked[0] != "/pkg/latest" {
		t.Errorf("stable: Latest = %q, asked %v; want only /pkg/latest", n.Latest, asked)
	}

	asked = nil
	if n := checkFor(context.Background(), srv.URL+"/pkg", "dev"); n.String() != "" || len(asked) != 0 {
		t.Errorf("dev builds must not touch the network (asked %v)", asked)
	}
}
