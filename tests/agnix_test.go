package tests

import (
	"regexp"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/testenv"
)

// Lint every agent config in the repo (skills, CLAUDE.md, hooks, MCP, plugins)
// with agnix, pinned. Errors fail the test; warnings and info are reported by
// `npx agnix validate .` on demand. Skipped when node is not installed or the
// package cannot be fetched (it needs the network).

const agnixVersion = "0.61.0"

func TestAgnixReportsNoErrorsInAnyAgentConfig(t *testing.T) {
	testenv.SkipWindows(t, "npx is a .cmd shim and the check needs the network; covered on Linux and macOS")
	t.Parallel()
	needTool(t, "npx")
	slow(t)
	// A throwaway HOME: the suite never touches the real one, so npx re-fetches
	// the pinned package into a fresh cache. agnix itself only reads the repo.
	r := run(repo, isoEnv(t.TempDir()), "", "npx", "-y", "agnix@"+agnixVersion, "validate", ".")
	out := r.Out()
	errs := regexp.MustCompile(`(?m) error: `).FindAllString(out, -1)
	if len(errs) == 0 && r.Code != 0 && regexp.MustCompile(`(?i)ENOTFOUND|ETIMEDOUT|ECONNREFUSED|EAI_AGAIN|npm error`).MatchString(out) {
		t.Skipf("agnix could not be fetched (offline?): %s", strings.TrimSpace(out))
	}
	if len(errs) > 0 {
		var lines []string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, " error: ") && len(lines) < 10 {
				lines = append(lines, l)
			}
		}
		t.Errorf("agnix reports %d error(s):\n%s", len(errs), strings.Join(lines, "\n"))
	}
}
