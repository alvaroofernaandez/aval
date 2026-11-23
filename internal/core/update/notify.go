package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/release"
)

// LatestURL is the npm registry endpoint for the CLI package. The registry is
// the right source even for a curl-installed binary: it is the same version
// stream, it is CDN-backed, and it needs no authentication or rate-limit token
// the way the GitHub API does.
const LatestURL = PackageURL + "/latest"

// PackageURL is the registry document of the CLI package; a dist-tag is
// appended to it.
const PackageURL = "https://registry.npmjs.org/@alvaroofernaandez/aval"

// endpointsFor lists the dist-tags worth asking for, by the running build's
// channel. Stable builds follow `latest`. Prerelease builds follow `next`, and
// also `latest` so a release candidate learns when its stable ships. Dev builds
// never ask: a checkout or `go run` is not something to nag.
func endpointsFor(base, current string) []string {
	switch release.Channel(current) {
	case release.ChannelStable:
		return []string{base + "/latest"}
	case release.ChannelPrerelease:
		return []string{base + "/next", base + "/latest"}
	}
	return nil
}

// checkFor asks every endpoint of the build's channel and keeps the highest
// version it hears about.
func checkFor(ctx context.Context, base, current string) Notice {
	n := Notice{Current: current, Command: updateCommand()}
	var best release.Version
	for _, url := range endpointsFor(base, current) {
		got := checkAt(ctx, url, current)
		v, err := release.Parse(got.Latest)
		if err != nil {
			continue
		}
		if n.Latest == "" || v.Compare(best) > 0 {
			best, n.Latest = v, got.Latest
		}
	}
	return n
}

// checkInterval bounds how often the network is touched. An installer that
// phones home on every invocation is an installer people stop running.
const checkInterval = 24 * time.Hour

// Notice is what to tell the user, empty when there is nothing to say.
type Notice struct {
	Current string
	Latest  string
	Command string
}

// String renders the notice, or "" when no update is available.
func (n Notice) String() string {
	if n.Latest == "" || n.Latest == n.Current {
		return ""
	}
	// A dist-tag can lag behind what is installed (a stale `next`, or a build
	// newer than the registry): only a strictly newer version is an update.
	if l, lerr := release.Parse(n.Latest); lerr == nil {
		if c, cerr := release.Parse(n.Current); cerr == nil && l.Compare(c) <= 0 {
			return ""
		}
	}
	return fmt.Sprintf("A newer version is available: %s → %s\nUpdate with: %s",
		n.Current, n.Latest, n.Command)
}

// CheckLatest asks the registry for the published version.
//
// Every failure path returns an empty Notice rather than an error the caller
// has to handle: being offline, behind a proxy, or on a machine with no DNS
// must never stop someone installing their dotfiles.
func CheckLatest(ctx context.Context, current string) Notice {
	return checkForAndStore(ctx, PackageURL, current)
}

// checkAt is CheckLatest with the endpoint injected, so the failure paths can
// be exercised against a test server instead of the real registry.
func checkAt(ctx context.Context, url, current string) Notice {
	n := Notice{Current: current, Command: updateCommand()}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return n
	}
	// Ask for the abbreviated document: the full one is megabytes of history.
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return n
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return n
	}

	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return n
	}
	n.Latest = strings.TrimSpace(body.Version)
	return n
}

// updateCommand names the command that actually updates THIS installation.
// Telling someone to run npm when they installed with curl is worse than
// saying nothing, so the install method is inferred from where the binary sits.
func updateCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return "aval update"
	}
	return commandFor(exe)
}

// commandFor picks the update command from where the binary lives.
func commandFor(exe string) string {
	if strings.Contains(filepath.ToSlash(exe), "/node_modules/") {
		return "npm update -g @alvaroofernaandez/aval"
	}
	return "aval update"
}

// ShouldCheck reports whether enough time has passed since the last check, and
// records the attempt. The stamp lives in the user's cache directory, so a
// read-only or unavailable cache simply means checking every run rather than
// failing.
func ShouldCheck(now time.Time) bool {
	// An explicit opt-out, honoured before anything else. Some people do not
	// want a tool touching the network, and that is a legitimate position.
	if os.Getenv("AVAL_NO_UPDATE_CHECK") != "" {
		return false
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return true
	}
	stamp := filepath.Join(cache, "aval", "last-update-check")

	if info, err := os.Stat(stamp); err == nil {
		if now.Sub(info.ModTime()) < checkInterval {
			return false
		}
	}

	if err := os.MkdirAll(filepath.Dir(stamp), 0o755); err != nil {
		return true
	}
	// Touch before checking, not after: if the network call hangs and the user
	// interrupts, the next run should still back off rather than hang again.
	_ = os.WriteFile(stamp, []byte(now.Format(time.RFC3339)), 0o644)
	return true
}

// ShouldCheckWith is ShouldCheck with the update_check setting applied:
// enabled=false (update_check = off) silences the notice like the env opt-out.
func ShouldCheckWith(now time.Time, enabled bool) bool {
	return enabled && ShouldCheck(now)
}
