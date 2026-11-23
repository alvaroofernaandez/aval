package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alvaroofernaandez/aval/internal/core/manifest"
	"github.com/alvaroofernaandez/aval/internal/testenv"
)

func cacheEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	testenv.SetHome(t, dir)
	t.Setenv("AVAL_NO_UPDATE_CHECK", "")
	return dir
}

func registry(t *testing.T, version string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"version":"` + version + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		s := ""
		if !info.IsDir() {
			b, _ := os.ReadFile(p)
			s = string(b) + info.ModTime().String()
		}
		got[p] = s
		return nil
	})
	return got
}

func TestCachedReturnsWhatTheLastCheckSaw(t *testing.T) {
	cacheEnv(t)
	checkForAndStore(context.Background(), registry(t, "2.0.0"), "1.0.0")

	n := Cached("1.0.0")
	if n.Latest != "2.0.0" || n.Current != "1.0.0" || n.String() == "" {
		t.Errorf("Cached = %+v, want an available 1.0.0 -> 2.0.0 notice", n)
	}
}

func TestCachedWithNoCacheIsEmpty(t *testing.T) {
	cacheEnv(t)
	if n := Cached("1.0.0"); n.Latest != "" || n.String() != "" {
		t.Errorf("Cached = %+v, want empty", n)
	}
}

func TestCachedIsAvailableOnlyWhenStrictlyNewer(t *testing.T) {
	cacheEnv(t)
	checkForAndStore(context.Background(), registry(t, "2.0.0"), "1.0.0")
	for _, cur := range []string{"2.0.0", "2.1.0"} {
		if n := Cached(cur); n.String() != "" {
			t.Errorf("running %s: Cached reports an update: %+v", cur, n)
		}
	}
}

func TestCachedHonoursTheOffSwitches(t *testing.T) {
	cacheEnv(t)
	checkForAndStore(context.Background(), registry(t, "2.0.0"), "1.0.0")
	if n := CachedWith("1.0.0", false); n.Latest != "" {
		t.Errorf("update_check = off ignored: %+v", n)
	}
	t.Setenv("AVAL_NO_UPDATE_CHECK", "1")
	if n := Cached("1.0.0"); n.Latest != "" {
		t.Errorf("AVAL_NO_UPDATE_CHECK ignored: %+v", n)
	}
}

func TestCachedNeverTouchesTheNetworkOrWrites(t *testing.T) {
	dir := cacheEnv(t)
	checkForAndStore(context.Background(), registry(t, "2.0.0"), "1.0.0")

	// From here on any request is a failure, whatever host it targets.
	orig := http.DefaultTransport
	http.DefaultTransport = failingTransport{t}
	defer func() { http.DefaultTransport = orig }()

	time.Sleep(10 * time.Millisecond)
	before := tree(t, dir)
	Cached("1.0.0")
	CachedWith("1.0.0", true)
	if after := tree(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("Cached wrote: before %v after %v", before, after)
	}

	empty := cacheEnv(t)
	before = tree(t, empty)
	Cached("1.0.0")
	if after := tree(t, empty); !reflect.DeepEqual(before, after) {
		t.Errorf("Cached wrote with no cache: %v -> %v", before, after)
	}
}

func TestManagedPathsListsTheManifestAndEverySourceOnce(t *testing.T) {
	m := &manifest.Manifest{Groups: []manifest.Group{
		{FanoutSource: "config/skills", Entries: []manifest.Entry{
			{Source: "config/claude/agents", Dest: ".claude/agents"},
			{Source: "config/claude/agents", Dest: ".other"},
		}},
		{Entries: []manifest.Entry{{Source: "install.manifest"}, {Source: "config/x", Dest: ".x"}}},
	}}
	want := []string{"install.manifest", "config/skills", "config/claude/agents", "config/x"}
	if got := ManagedPaths(m); !reflect.DeepEqual(got, want) {
		t.Errorf("ManagedPaths = %v, want %v", got, want)
	}
	if got := ManagedPaths(nil); !reflect.DeepEqual(got, []string{"install.manifest"}) {
		t.Errorf("ManagedPaths(nil) = %v", got)
	}
}

type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("Cached made an HTTP request to %s", r.URL)
	return nil, http.ErrNotSupported
}
