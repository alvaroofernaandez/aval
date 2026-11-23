package tests

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// GitHub Actions runs in a public repo: every workflow is attack surface.
// These invariants are the reason Actions could be switched on at all:
//   - third-party code only by immutable commit SHA, and only GitHub-owned actions;
//   - read-only token by default, write only where a job needs it;
//   - nothing that runs untrusted PR code with secrets (pull_request_target);
//   - publishing never runs on a plain push: tags or manual dispatch, behind the
//     "release" environment, which requires the owner's approval.

func workflows(t *testing.T) map[string]string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(repo, ".github", "workflows", "*.yml"))
	if len(files) == 0 {
		t.Fatal("no workflows found")
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(f)] = string(b)
	}
	return out
}

var (
	usesRe    = regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*([^\s#]+)`)
	pinnedRe  = regexp.MustCompile(`^actions/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)
	topPermRe = regexp.MustCompile(`(?m)^permissions:[ \t]*\n((?:[ \t]+.+\n)+)`)
)

func TestWorkflowsAreHardened(t *testing.T) {
	for name, wf := range workflows(t) {
		for _, m := range usesRe.FindAllStringSubmatch(wf, -1) {
			if !pinnedRe.MatchString(m[1]) {
				t.Errorf("%s: %q must be a GitHub-owned action pinned to a full commit SHA", name, m[1])
			}
		}
		if strings.Contains(wf, "pull_request_target") {
			t.Errorf("%s: pull_request_target runs fork code with secrets", name)
		}
		top := topPermRe.FindStringSubmatch(wf)
		if top == nil {
			t.Errorf("%s: needs a top-level permissions block", name)
		} else if strings.Contains(top[1], "write") {
			t.Errorf("%s: top-level permissions must be read-only; grant write per job", name)
		}
		checkouts := strings.Count(wf, "actions/checkout@")
		if got := strings.Count(wf, "persist-credentials: false"); got < checkouts {
			t.Errorf("%s: every checkout needs persist-credentials: false (%d of %d)", name, got, checkouts)
		}
		for _, bad := range []string{"@latest", "| sh", "| bash", "releases/latest/download"} {
			if strings.Contains(wf, bad) {
				t.Errorf("%s: unpinned download or piped install (%q)", name, bad)
			}
		}
	}
}

var (
	jobHeadRe = regexp.MustCompile(`(?m)^  ([a-z][a-z0-9_-]*):[ \t]*$`)
	// A step that publishes: to npm, to a GitHub release, or GoReleaser's real
	// (non --snapshot) release.
	publishRe = regexp.MustCompile(`npm publish|gh release (create|upload|edit)|goreleaser release`)
)

// jobs splits the `jobs:` section into name -> body.
func jobs(wf string) map[string]string {
	_, section, ok := strings.Cut(wf, "\njobs:\n")
	if !ok {
		return nil
	}
	heads := jobHeadRe.FindAllStringSubmatchIndex(section, -1)
	out := map[string]string{}
	for i, h := range heads {
		end := len(section)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		out[section[h[2]:h[3]]] = section[h[1]:end]
	}
	return out
}

func publishes(job string) bool {
	for _, line := range strings.Split(job, "\n") {
		if publishRe.MatchString(line) && !strings.Contains(line, "--snapshot") {
			return true
		}
	}
	return false
}

func TestPublishingNeedsATagOrApproval(t *testing.T) {
	for name, wf := range workflows(t) {
		var publishing []string
		for jobName, body := range jobs(wf) {
			if !publishes(body) {
				continue
			}
			publishing = append(publishing, jobName)
			if !strings.Contains(body, "environment: release") {
				t.Errorf("%s: job %q publishes, so it must run in the approval-gated \"release\" environment", name, jobName)
			}
		}
		if len(publishing) > 0 && regexp.MustCompile(`(?m)^\s+branches:`).MatchString(wf) {
			t.Errorf("%s: publishes on a branch push; use tags or workflow_dispatch", name)
		}
	}
}

// GoReleaser is downloaded, not installed from a third-party action: the URL
// must name an exact version, and the tarball's sha256 is checked before it runs.
func TestGoReleaserIsPinnedAndVerified(t *testing.T) {
	urlRe := regexp.MustCompile(`https://github\.com/goreleaser/goreleaser/releases/download/\$\{GORELEASER_VERSION\}/\S+`)
	versionRe := regexp.MustCompile(`(?m)^  GORELEASER_VERSION: v\d+\.\d+\.\d+$`)
	shaRe := regexp.MustCompile(`(?m)^  GORELEASER_SHA256: [0-9a-f]{64}$`)

	found := false
	for name, wf := range workflows(t) {
		for _, line := range strings.Split(wf, "\n") {
			if !strings.Contains(line, "curl") && !strings.Contains(line, "wget") {
				continue
			}
			if !urlRe.MatchString(line) {
				t.Errorf("%s: download %q must be the version-pinned GoReleaser URL", name, strings.TrimSpace(line))
			}
		}
		if !strings.Contains(wf, "goreleaser") {
			continue
		}
		found = true
		if !versionRe.MatchString(wf) || !shaRe.MatchString(wf) {
			t.Errorf("%s: GORELEASER_VERSION (exact vX.Y.Z) and GORELEASER_SHA256 (64 hex) must be set at the top level", name)
		}
		if !strings.Contains(wf, "sha256sum -c") {
			t.Errorf("%s: the GoReleaser tarball must be verified with sha256sum -c before it is extracted", name)
		}
	}
	if !found {
		t.Error("no workflow runs goreleaser")
	}
}

// `aval release promote` puts the stable tag on the rc's commit, so two tags
// point at it. Left alone, GoReleaser picks one by itself (it picked the rc and
// re-uploaded the rc's assets instead of creating v0.1.0). The workflow must name
// the tag it is releasing.
func TestGoReleaserBuildsTheTagBeingReleased(t *testing.T) {
	re := regexp.MustCompile(`(?m)^  GORELEASER_CURRENT_TAG: \$\{\{ inputs\.tag \|\| github\.ref_name \}\}$`)
	for name, wf := range workflows(t) {
		if strings.Contains(wf, "goreleaser release") && !re.MatchString(wf) {
			t.Errorf("%s: set GORELEASER_CURRENT_TAG to the released tag at the top level", name)
		}
	}
}

// Versions are annotated, immutable vX.Y.Z tags. The moving `latest` tag and its
// "Latest build" release were removed at v0.1.0 and must not come back.
func TestNoMovingLatestTag(t *testing.T) {
	re := regexp.MustCompile(`git tag\b[^\n]*\blatest\b|refs/tags/latest|releases/download/latest`)
	for name, wf := range workflows(t) {
		if m := re.FindString(wf); m != "" {
			t.Errorf("%s: %q reintroduces the moving latest tag", name, m)
		}
	}
}

// The binaries npm ships and the ones GoReleaser builds are the same six.
func TestGoReleaserTargetsMatchNpm(t *testing.T) {
	cfg, err := os.ReadFile(filepath.Join(repo, ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	list := func(key string) []string {
		m := regexp.MustCompile(`(?m)^\s+` + key + `: \[([^\]]+)\]`).FindStringSubmatch(string(cfg))
		if m == nil {
			t.Fatalf(".goreleaser.yaml has no %s list", key)
		}
		return strings.Split(strings.ReplaceAll(m[1], " ", ""), ",")
	}
	want := map[string]bool{}
	for _, goos := range list("goos") {
		for _, goarch := range list("goarch") {
			want[goos+"/"+goarch] = true
		}
	}

	resolve, err := os.ReadFile(filepath.Join(repo, "npm", "cli", "lib", "resolve.js"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range regexp.MustCompile(`goos: "(\w+)", goarch: "(\w+)"`).FindAllStringSubmatch(string(resolve), -1) {
		got[m[1]+"/"+m[2]] = true
	}
	for target := range want {
		if !got[target] {
			t.Errorf("GoReleaser builds %s but npm/cli/lib/resolve.js does not package it", target)
		}
	}
	for target := range got {
		if !want[target] {
			t.Errorf("npm packages %s but .goreleaser.yaml does not build it", target)
		}
	}
	if len(got) != 6 {
		t.Errorf("expected 6 targets in resolve.js, found %d", len(got))
	}
}

func TestCIRunsOnPullRequestsWithoutSecrets(t *testing.T) {
	ci, ok := workflows(t)["ci.yml"]
	if !ok {
		t.Fatal("missing .github/workflows/ci.yml")
	}
	if !strings.Contains(ci, "pull_request:") {
		t.Error("ci.yml must run on pull_request")
	}
	if strings.Contains(ci, "secrets.") {
		t.Error("ci.yml must not use secrets: it runs on pull requests")
	}
}

// A test that exists but never runs in CI protects nothing. ciGaps lists every
// way a test can escape the ci.yml workflow: a Go package outside `go test
// ./...`, a file gated by a build tag no CI step sets, a non-Go runner nothing
// invokes, a tool whose absence makes a test skip. Each escape is either fixed
// in ci.yml or named in an exemption map below with the reason it is fine. An
// exemption that no longer applies fails too, so the maps cannot rot.

// ciExemptTags: custom build tags that gate test files and that CI does not set.
// Empty today: the only gate is `!windows` (tmux e2e), which runs on Linux and
// macOS. Add `"e2e": "needs a real X; run locally with go test -tags e2e ./pkg"`.
var ciExemptTags = map[string]string{}

// ciExemptRunners: path globs (slash form, relative to the repo root) of
// non-Go test files that CI does not run.
var ciExemptRunners = map[string]string{
	"skills/readme-portal/assets/*.test.mjs": "templates the readme-portal skill copies into other repos; they assume that repo's layout (git-tracked Markdown, units manifest), so they cannot run here",
}

// ciExemptTools: external tools a Go test needs that CI neither installs nor
// finds preinstalled. The tests skip cleanly without them.
var ciExemptTools = map[string]string{
	"atuin":     "optional shell-history tool; its integration test skips when absent and CI does not install it",
	"delta":     "optional diff pager; its test skips when absent and CI does not install it",
	"fzf":       "optional fuzzy finder; its test skips when absent and CI does not install it",
	"ghostty":   "GUI terminal, not available on CI runners; the test skips when absent",
	"hyperfine": "optional benchmark tool; its test skips when absent and CI does not install it",
	"yazi":      "optional file manager; the tmux e2e case that needs it skips when absent",
	"zoxide":    "optional directory jumper; its test skips when absent and CI does not install it",
}

// ciPreinstalled: tools every GitHub-hosted runner image ships (or, with go and
// node, that ci.yml sets up), so ci.yml need not install them.
var ciPreinstalled = map[string]bool{"git": true, "go": true, "sh": true, "bash": true, "node": true, "npx": true, "ps": true, "plutil": true}

type ciExemptions struct{ tags, runners, tools map[string]string }

var realCIExemptions = ciExemptions{ciExemptTags, ciExemptRunners, ciExemptTools}

var (
	knownOS   = map[string]bool{"linux": true, "darwin": true, "windows": true, "freebsd": true, "netbsd": true, "openbsd": true, "plan9": true, "solaris": true, "aix": true, "dragonfly": true, "illumos": true, "js": true, "wasip1": true, "android": true, "ios": true}
	knownArch = map[string]bool{"amd64": true, "arm64": true, "386": true, "arm": true, "riscv64": true, "ppc64": true, "ppc64le": true, "s390x": true, "mips": true, "mipsle": true, "mips64": true, "mips64le": true, "loong64": true, "wasm": true}
	// CI matrix: runner image -> runner.os value -> GOOS.
	ciOSes = map[string][2]string{"ubuntu-latest": {"Linux", "linux"}, "macos-latest": {"macOS", "darwin"}, "windows-latest": {"Windows", "windows"}}

	buildTokRe   = regexp.MustCompile(`[A-Za-z0-9_.]+|&&|\|\||[!()]`)
	runnerFileRe = regexp.MustCompile(`(\.(test|spec)\.(js|mjs|cjs|ts|tsx)|\.bats|_test\.py)$|(^|/)test_[^/]*\.py$`)
	matrixOSRe   = regexp.MustCompile(`(?m)^\s+os:\s*\[([^\]]+)\]`)
	needToolRe   = regexp.MustCompile(`needTool\(\w+((?:,\s*"[^"]+")+)`)
	lookPathRe   = regexp.MustCompile(`LookPath\("([^"]+)"\)`)
	quotedRe     = regexp.MustCompile(`"([^"]+)"`)
	runnerOSIf   = regexp.MustCompile(`^runner\.os\s*(==|!=)\s*'(\w+)'$`)
)

// evalBuild evaluates a //go:build expression; has says whether a tag is set.
func evalBuild(expr string, has func(string) bool) bool {
	toks := buildTokRe.FindAllString(expr, -1)
	pos := 0
	var or func() bool
	atom := func() bool {
		if pos >= len(toks) {
			return false
		}
		tk := toks[pos]
		pos++
		switch tk {
		case "(":
			v := or()
			pos++ // ")"
			return v
		}
		return has(tk)
	}
	var unary func() bool
	unary = func() bool {
		if pos < len(toks) && toks[pos] == "!" {
			pos++
			return !unary()
		}
		return atom()
	}
	and := func() bool {
		v := unary()
		for pos < len(toks) && toks[pos] == "&&" {
			pos++
			r := unary()
			v = v && r
		}
		return v
	}
	or = func() bool {
		v := and()
		for pos < len(toks) && toks[pos] == "||" {
			pos++
			r := and()
			v = v || r
		}
		return v
	}
	return or()
}

func buildConstraint(src string) string {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//go:build ") {
			return strings.TrimPrefix(line, "//go:build ")
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
	}
	return ""
}

// fileNameOS returns the GOOS a *_GOOS_test.go (or _GOOS_GOARCH) name pins, "" if none.
func fileNameOS(name string) string {
	parts := strings.Split(strings.TrimSuffix(name, "_test.go"), "_")
	n := len(parts)
	if n >= 3 && knownArch[parts[n-1]] && knownOS[parts[n-2]] {
		return parts[n-2]
	}
	if n >= 2 && knownOS[parts[n-1]] {
		return parts[n-1]
	}
	return ""
}

func walkRepo(root string, visit func(rel string, d os.DirEntry)) {
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			n := d.Name()
			if rel != "." && (n == "node_modules" || n == "vendor" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		visit(rel, d)
		return nil
	})
}

type ciStep struct{ cond, run, workdir string }

// ciSteps splits the test job into steps, reading name/if/run/working-directory.
func ciSteps(job string) []ciStep {
	var steps []ciStep
	for _, raw := range regexp.MustCompile(`(?m)^      - `).Split(job, -1)[1:] {
		var s ciStep
		lines := strings.Split(raw, "\n")
		for i := 0; i < len(lines); i++ {
			l := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "- "))
			switch {
			case strings.HasPrefix(l, "if: "):
				s.cond = strings.TrimSpace(strings.TrimPrefix(l, "if: "))
			case strings.HasPrefix(l, "working-directory:"):
				s.workdir = strings.TrimSpace(strings.TrimPrefix(l, "working-directory:"))
			case l == "run: |" || l == "run: >":
				for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "          ") {
					i++
					s.run += strings.TrimSpace(lines[i]) + "\n"
				}
			case strings.HasPrefix(l, "run: "):
				s.run += strings.TrimSpace(strings.TrimPrefix(l, "run: ")) + "\n"
			}
		}
		steps = append(steps, s)
	}
	return steps
}

func (s ciStep) runsOn(runnerOS string) (bool, bool) {
	if s.cond == "" {
		return true, true
	}
	m := runnerOSIf.FindStringSubmatch(s.cond)
	if m == nil {
		return false, false
	}
	return (m[1] == "==") == (m[2] == runnerOS), true
}

func ciWithoutComments(ci string) string {
	var keep []string
	for _, l := range strings.Split(ci, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

func ciGaps(root string, ex ciExemptions) []string {
	var gaps []string
	gap := func(f string, a ...any) { gaps = append(gaps, fmt.Sprintf(f, a...)) }

	ciBytes, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		return []string{"ci.yml is unreadable: " + err.Error()}
	}
	ci := string(ciBytes)
	testJob, ok := jobs(ci)["test"]
	if !ok {
		return []string{"ci.yml has no `test` job"}
	}

	// Which OSes the job runs on.
	var osList []string
	if m := matrixOSRe.FindStringSubmatch(testJob); m != nil {
		for _, o := range strings.Split(m[1], ",") {
			osList = append(osList, strings.TrimSpace(o))
		}
	}
	for img := range ciOSes {
		found := false
		for _, o := range osList {
			found = found || o == img
		}
		if !found {
			gap("ci.yml: the test job matrix must include %s (found %v)", img, osList)
		}
	}

	// `go test ./...` from the module root, on every OS; collect -tags.
	steps := ciSteps(testJob)
	ciTags := map[string]bool{}
	for _, img := range osList {
		pair, known := ciOSes[img]
		if !known {
			continue
		}
		covered := false
		for _, s := range steps {
			on, evaluable := s.runsOn(pair[0])
			if !evaluable {
				gap("ci.yml: cannot evaluate step condition %q; use runner.os == 'X' or != 'X'", s.cond)
				continue
			}
			if !on || s.workdir != "" && s.workdir != "." {
				continue
			}
			for _, line := range strings.Split(s.run, "\n") {
				f := strings.Fields(line)
				if len(f) < 3 || f[0] != "go" || f[1] != "test" {
					continue
				}
				all, narrowed := false, false
				for i, a := range f[2:] {
					switch {
					case a == "./...":
						all = true
					case strings.HasPrefix(a, "-run"), strings.HasPrefix(a, "-skip"), strings.HasPrefix(a, "-short"), strings.HasPrefix(a, "-list"):
						narrowed = true
					case strings.HasPrefix(a, "-tags="):
						for _, tg := range strings.Split(strings.TrimPrefix(a, "-tags="), ",") {
							ciTags[tg] = true
						}
					case a == "-tags" && i+3 < len(f):
						for _, tg := range strings.Split(f[i+3], ",") {
							ciTags[tg] = true
						}
					}
				}
				if all && !narrowed {
					covered = true
				}
			}
		}
		if !covered {
			gap("ci.yml: no step runs `go test ./...` unnarrowed from the module root on %s", pair[0])
		}
	}

	// Walk the tree once.
	var testFiles, goTestSrc, runnerFiles []string // rel paths
	testDirs := map[string]bool{}
	modRoots := map[string]bool{}
	walkRepo(root, func(rel string, d os.DirEntry) {
		switch {
		case d.Name() == "go.mod":
			modRoots[path.Dir(rel)] = true
		case strings.HasSuffix(d.Name(), "_test.go"):
			testFiles = append(testFiles, rel)
			testDirs[path.Dir(rel)] = true
		}
		if runnerFileRe.MatchString(rel) {
			runnerFiles = append(runnerFiles, rel)
		}
	})
	if !modRoots["."] {
		gap("no go.mod at the module root; `go test ./...` has nothing to run")
	}
	for dir := range testDirs {
		for d := dir; ; d = path.Dir(d) {
			if modRoots[d] {
				if d != "." {
					gap("%s has Go tests but sits in the nested module %s, which `go test ./...` from the root skips", dir, d)
				}
				break
			}
			if d == "." {
				break
			}
		}
	}

	// Build constraints: every test file must run on some CI OS with CI's tags.
	usedTags := map[string]bool{}
	srcs := map[string]string{}
	for _, rel := range testFiles {
		b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		srcs[rel] = string(b)
		goTestSrc = append(goTestSrc, rel)
		expr := buildConstraint(srcs[rel])
		var custom []string
		for _, id := range buildTokRe.FindAllString(expr, -1) {
			if strings.ContainsAny(id[:1], "!&|()") || knownOS[id] || knownArch[id] || id == "unix" || id == "gc" || id == "cgo" || id == "race" || strings.HasPrefix(id, "go1.") {
				continue
			}
			custom = append(custom, id)
			usedTags[id] = true
		}
		pinned := fileNameOS(path.Base(rel))
		runs := false
		for _, pair := range ciOSes {
			goos := pair[1]
			if pinned != "" && pinned != goos {
				continue
			}
			has := func(tg string) bool {
				switch {
				case tg == goos, tg == "gc", ciTags[tg], strings.HasPrefix(tg, "go1."), knownArch[tg] && !knownOS[tg]:
					return true
				case tg == "unix", tg == "race":
					return goos != "windows"
				case tg == "cgo":
					return goos != "windows"
				}
				return false
			}
			if expr == "" || evalBuild(expr, has) {
				runs = true
			}
		}
		if runs {
			continue
		}
		exempt := false
		for _, tg := range custom {
			_, e := ex.tags[tg]
			exempt = exempt || e
		}
		if !exempt {
			gap("%s (//go:build %s) never runs in CI: no CI step sets its tag; set it in ci.yml or add the tag to ciExemptTags with a reason", rel, expr)
		}
	}
	for tg := range ex.tags {
		switch {
		case !usedTags[tg]:
			gap("stale ciExemptTags entry %q: no test file is gated by it", tg)
		case ciTags[tg]:
			gap("stale ciExemptTags entry %q: ci.yml sets it now", tg)
		}
	}

	// Non-Go runners: a CI `node --test` glob, a Go test that shells out to
	// `node --test` on the file, or an exemption.
	inCI := map[string]bool{}
	for _, s := range steps {
		for _, line := range strings.Split(s.run, "\n") {
			f := strings.Fields(line)
			for i := 0; i+1 < len(f); i++ {
				if f[i] == "node" && f[i+1] == "--test" {
					for _, g := range f[i+2:] {
						matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(g)))
						for _, m := range matches {
							r, _ := filepath.Rel(root, m)
							inCI[filepath.ToSlash(r)] = true
						}
					}
				}
			}
		}
	}
	viaGo := func(rel string) bool {
		for _, g := range goTestSrc {
			if strings.Contains(srcs[g], `"--test"`) && strings.Contains(srcs[g], `"`+path.Base(rel)+`"`) {
				return true
			}
		}
		return false
	}
	matched := map[string]bool{}
	allCovered := map[string]bool{}
	for _, rel := range runnerFiles {
		covered := inCI[rel] || viaGo(rel)
		exempted := false
		for pat := range ex.runners {
			if ok, _ := path.Match(pat, rel); ok {
				matched[pat] = true
				exempted = true
				if !covered {
					allCovered[pat] = false
				} else if _, seen := allCovered[pat]; !seen {
					allCovered[pat] = true
				}
			}
		}
		if !covered && !exempted {
			gap("%s is a test runner file no CI step invokes (add `node --test` for it to ci.yml or an entry to ciExemptRunners)", rel)
		}
	}
	for pat := range ex.runners {
		switch {
		case !matched[pat]:
			gap("stale ciExemptRunners entry %q: it matches no file", pat)
		case allCovered[pat]:
			gap("stale ciExemptRunners entry %q: CI runs every file it matches", pat)
		}
	}

	// External tools Go tests skip without.
	needed := map[string]bool{}
	for _, rel := range goTestSrc {
		for _, m := range needToolRe.FindAllStringSubmatch(srcs[rel], -1) {
			for _, q := range quotedRe.FindAllStringSubmatch(m[1], -1) {
				needed[q[1]] = true
			}
		}
		for _, m := range lookPathRe.FindAllStringSubmatch(srcs[rel], -1) {
			needed[m[1]] = true
		}
	}
	installed := func(tool string) bool {
		return ciPreinstalled[tool] || regexp.MustCompile(`\b`+regexp.QuoteMeta(tool)+`\b`).MatchString(ciWithoutComments(ci))
	}
	for tool := range needed {
		_, exempt := ex.tools[tool]
		if !installed(tool) && !exempt {
			gap("tests need %q but ci.yml never installs it, so they skip in CI; install it (pinned) or add it to ciExemptTools", tool)
		}
	}
	for tool := range ex.tools {
		switch {
		case !needed[tool]:
			gap("stale ciExemptTools entry %q: no test needs it", tool)
		case installed(tool):
			gap("stale ciExemptTools entry %q: ci.yml installs it now", tool)
		}
	}
	return gaps
}

// fakeCIRepo is a one-package module whose ci.yml is the real one, optionally
// mutated, so each guard is shown to fail on a planted gap.
func fakeCIRepo(t *testing.T, mutate func(string) string, files map[string]string) string {
	t.Helper()
	real, err := os.ReadFile(filepath.Join(repo, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	ci := string(real)
	if mutate != nil {
		ci = mutate(ci)
	}
	dir := t.TempDir()
	base := map[string]string{
		"go.mod":                   "module fake\n\ngo 1.25\n",
		"a/a_test.go":              "package a\n",
		".github/workflows/ci.yml": ci,
	}
	for k, v := range files {
		base[k] = v
	}
	for name, body := range base {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func wantGap(t *testing.T, gaps []string, substr string) {
	t.Helper()
	for _, g := range gaps {
		if strings.Contains(g, substr) {
			return
		}
	}
	t.Errorf("expected a gap mentioning %q, got %q", substr, gaps)
}

func TestWorkflowCIRunsEveryTest(t *testing.T) {
	for _, g := range ciGaps(repo, realCIExemptions) {
		t.Error(g)
	}
}

func TestWorkflowCIGapsAreDetected(t *testing.T) {
	none := ciExemptions{}
	t.Run("baseline passes", func(t *testing.T) {
		if g := ciGaps(fakeCIRepo(t, nil, nil), none); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
	})
	t.Run("narrowed go test", func(t *testing.T) {
		d := fakeCIRepo(t, func(s string) string { return strings.ReplaceAll(s, "go test ./...", "go test ./internal/...") }, nil)
		wantGap(t, ciGaps(d, none), "go test ./...")
	})
	t.Run("one OS without go test", func(t *testing.T) {
		d := fakeCIRepo(t, func(s string) string {
			return strings.Replace(s, "if: runner.os == 'Windows'", "if: runner.os == 'Plan9'", 1)
		}, nil)
		wantGap(t, ciGaps(d, none), "Windows")
	})
	t.Run("nested module is skipped", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{"sub/go.mod": "module sub\n", "sub/s_test.go": "package s\n"})
		wantGap(t, ciGaps(d, none), "nested module")
	})
	t.Run("unset build tag", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{"e/e_test.go": "//go:build e2e\n\npackage e\n"})
		wantGap(t, ciGaps(d, none), "e/e_test.go")
	})
	t.Run("not-windows tag runs on linux and macos", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{"e/e_test.go": "//go:build !windows\n\npackage e\n"})
		if g := ciGaps(d, none); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
	})
	t.Run("windows-only file still runs", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{"w/w_windows_test.go": "package w\n"})
		if g := ciGaps(d, none); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
	})
	t.Run("freebsd-only file never runs", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{"w/w_freebsd_test.go": "package w\n"})
		wantGap(t, ciGaps(d, none), "w_freebsd_test.go")
	})
	t.Run("exempt tag is accepted, then stale", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{"e/e_test.go": "//go:build e2e\n\npackage e\n"})
		ex := ciExemptions{tags: map[string]string{"e2e": "needs a real tmux"}}
		if g := ciGaps(d, ex); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
		wantGap(t, ciGaps(fakeCIRepo(t, nil, nil), ex), "stale ciExemptTags")
	})
	t.Run("tag set by ci needs no exemption, exemption goes stale", func(t *testing.T) {
		d := fakeCIRepo(t, func(s string) string { return strings.ReplaceAll(s, "go test ./...", "go test -tags e2e ./...") },
			map[string]string{"e/e_test.go": "//go:build e2e\n\npackage e\n"})
		if g := ciGaps(d, none); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
		wantGap(t, ciGaps(d, ciExemptions{tags: map[string]string{"e2e": "x"}}), "ci.yml sets it now")
	})
	t.Run("node test nobody runs", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{"web/x.test.mjs": "// t\n", "npm/cli/lib/a.test.js": "// t\n"})
		g := ciGaps(d, none)
		wantGap(t, g, "web/x.test.mjs")
		for _, m := range g {
			if strings.Contains(m, "npm/cli/lib/a.test.js") {
				t.Errorf("npm/cli test is run by CI, got %q", m)
			}
		}
		ex := ciExemptions{runners: map[string]string{"web/*.test.mjs": "template"}}
		if g := ciGaps(d, ex); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
		wantGap(t, ciGaps(d, ciExemptions{runners: map[string]string{"gone/*.test.mjs": "x", "web/*.test.mjs": "y"}}), `stale ciExemptRunners entry "gone/*.test.mjs"`)
	})
	t.Run("runner invoked by a Go test is covered", func(t *testing.T) {
		d := fakeCIRepo(t, nil, map[string]string{
			"mod/logic.test.js": "// t\n",
			"mod/m_test.go":     "package m\n\nvar _ = []string{\"--test\", \"logic.test.js\"}\n",
		})
		if g := ciGaps(d, none); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
	})
	t.Run("tool tests skip without", func(t *testing.T) {
		files := map[string]string{"b/b_test.go": "package b\n\nfunc f() { needTool(t, \"bun\") }\n"}
		noBun := func(s string) string { return regexp.MustCompile(`\bbun\b`).ReplaceAllString(s, "xxx") }
		wantGap(t, ciGaps(fakeCIRepo(t, noBun, files), none), `"bun"`)
		if g := ciGaps(fakeCIRepo(t, nil, files), none); len(g) != 0 {
			t.Errorf("ci.yml installs bun, got %q", g)
		}
		ex := ciExemptions{tools: map[string]string{"bun": "optional"}}
		if g := ciGaps(fakeCIRepo(t, noBun, files), ex); len(g) != 0 {
			t.Errorf("unexpected gaps: %q", g)
		}
		wantGap(t, ciGaps(fakeCIRepo(t, nil, files), ex), "stale ciExemptTools")
	})
}
