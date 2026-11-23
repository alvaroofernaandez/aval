package tests

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The layering of internal/ is a mechanism, not a review comment. Every
// internal-to-internal import edge, read from `go list` for each GOOS that CI
// runs on (so build-tagged files count), must be allowed by archRules or be a
// counted entry in archExceptions. The exceptions are a ratchet: a new one
// fails, and one that no longer exists fails too, so the list can only shrink.
//
// docs/architecture/model.md is generated from the same rules and graph.

var archUpdate = flag.Bool("update", false, "rewrite generated files: docs/architecture/model.md and tests/testdata/quality-allowlist.txt")

// archGOOS are the platforms CI tests on (.github/workflows/ci.yml matrix).
var archGOOS = []string{"linux", "darwin", "windows"}

// archRule says what one package (or a family, with a trailing /*) may import
// from inside the module. allowTest is allowed only from _test.go files.
type archRule struct {
	pkg       string
	allow     []string
	allowTest []string
	why       string
}

// archRules is the allowlist. The first rule whose pkg matches wins; a package
// no rule matches is itself a violation, so a new package forces a decision.
// internal/testenv is allowed from any test import and from no production one;
// nothing may import cmd/aval. Changing this table needs an ADR (docs/adr).
var archRules = []archRule{
	{pkg: "cmd/aval", allow: []string{"internal/*"}, why: "composition root: wires every package"},
	{pkg: "internal/ui", allow: []string{"internal/core/*", "internal/devmode", "internal/i18n", "internal/release", "internal/ui/caps"},
		allowTest: []string{"internal/design/gate"}, why: "the hub drives core use cases; tests check the design gate"},
	{pkg: "internal/ui/cli", allow: []string{"internal/i18n", "internal/ui", "internal/ui/caps"}, why: "plain-terminal front end over the ui models"},
	{pkg: "internal/ui/caps", why: "leaf: terminal capabilities"},
	{pkg: "internal/core/*", allow: []string{"internal/core/*", "internal/release"}, why: "domain: install, sync, doctor, update"},
	{pkg: "internal/design/hook", allow: []string{"internal/design/ack"}, why: "the PreToolUse gate reads the ack it checks"},
	{pkg: "internal/design/ack", why: "leaf"},
	{pkg: "internal/design/gate", why: "leaf"},
	{pkg: "internal/agent/*", why: "leaf: agent hooks and helpers"},
	{pkg: "internal/devops/*", why: "leaf: docker and runtime helpers"},
	{pkg: "internal/term/*", why: "leaf: terminal integrations"},
	{pkg: "internal/docs/*", why: "leaf: document renderers"},
	{pkg: "internal/herdr/*", why: "leaf: herdr mods"},
	{pkg: "internal/deck", why: "leaf"},
	{pkg: "internal/release", why: "leaf"},
	{pkg: "internal/devmode", why: "leaf"},
	{pkg: "internal/i18n", why: "leaf: message catalog and language selection (ADR 0006)"},
	{pkg: "internal/testenv", why: "leaf: test-only helpers"},
	{pkg: "tools/*", why: "leaf: build-time tools"},
	{pkg: "tests", why: "repository checks: drive the binary, import nothing internal"},
}

// archExceptions are the edges that break archRules today, keyed
// "from -> to" (module-relative packages), each with its reason. It may
// shrink, never grow. Empty: the graph matched the rules when this guard
// landed.
var archExceptions = map[string]string{}

// archMaxExceptions caps archExceptions, like hagaharness's MAX_CROSS_LAYER:
// an exception cannot ride in with its violation without raising this number
// in the same diff. Lower it whenever an exception is deleted; never raise it.
const archMaxExceptions = 0

const (
	archCmd     = "cmd/aval"
	archTestenv = "internal/testenv"
)

// --- the graph ------------------------------------------------------------------

type archPkg struct {
	Path       string          // module-relative
	Prod, Test map[string]bool // module-relative internal imports
}

type archGraph struct {
	Module, Root string
	Pkgs         map[string]*archPkg
}

var (
	archCacheMu sync.Mutex
	archCache   = map[string]*archGraph{}
)

// loadArchGraph runs `go list` once per GOOS for the module at root and merges
// the results. Cached per root for the whole test run.
func loadArchGraph(t testing.TB, root string) *archGraph {
	t.Helper()
	needTool(t, "go")
	archCacheMu.Lock()
	defer archCacheMu.Unlock()
	if g, ok := archCache[root]; ok {
		return g
	}
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mod := regexpFind(`(?m)^module\s+(\S+)`, string(gomod))
	if mod == "" {
		t.Fatalf("%s/go.mod: no module line", root)
	}
	g := &archGraph{Module: mod, Root: root, Pkgs: map[string]*archPkg{}}
	rel := func(ip string) (string, bool) {
		if ip == mod {
			return ".", true
		}
		r, ok := strings.CutPrefix(ip, mod+"/")
		return r, ok
	}
	for _, goos := range archGOOS {
		cmd := exec.Command("go", "list", "-e", "-f",
			`{{.ImportPath}}|{{join .Imports ","}}|{{join .TestImports ","}}|{{join .XTestImports ","}}`, "./...")
		cmd.Dir = root
		cmd.Env = withEnv(os.Environ(), "GOOS="+goos, "CGO_ENABLED=0")
		out, err := cmd.Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				t.Fatalf("GOOS=%s go list: %v\n%s", goos, err, ee.Stderr)
			}
			t.Fatalf("GOOS=%s go list: %v", goos, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Split(line, "|")
			if len(f) != 4 {
				continue
			}
			from, ok := rel(f[0])
			if !ok {
				continue
			}
			pk := g.Pkgs[from]
			if pk == nil {
				pk = &archPkg{Path: from, Prod: map[string]bool{}, Test: map[string]bool{}}
				g.Pkgs[from] = pk
			}
			for i, list := range f[1:] {
				for _, imp := range strings.Split(list, ",") {
					to, ok := rel(imp)
					if !ok || to == from {
						continue
					}
					if i == 0 {
						pk.Prod[to] = true
					} else {
						pk.Test[to] = true
					}
				}
			}
		}
	}
	archCache[root] = g
	return g
}

// --- the check ------------------------------------------------------------------

type archViolation struct {
	From, To string
	Test     bool   // the edge exists only in test files
	Detail   string // why it is not allowed
}

func (v archViolation) key() string { return v.From + " -> " + v.To }

func archMatch(pattern, pkg string) bool {
	if base, ok := strings.CutSuffix(pattern, "/*"); ok {
		return strings.HasPrefix(pkg, base+"/")
	}
	return pattern == pkg
}

func archMatchAny(patterns []string, pkg string) bool {
	for _, p := range patterns {
		if archMatch(p, pkg) {
			return true
		}
	}
	return false
}

func archRuleFor(pkg string) *archRule {
	for i := range archRules {
		if archMatch(archRules[i].pkg, pkg) {
			return &archRules[i]
		}
	}
	return nil
}

// archCheck returns every edge of g that archRules does not allow, sorted by
// package and import. A package with no rule is one violation with no To.
func archCheck(g *archGraph) []archViolation {
	var out []archViolation
	for _, from := range archSorted(g.Pkgs) {
		pk := g.Pkgs[from]
		rule := archRuleFor(from)
		if rule == nil {
			out = append(out, archViolation{From: from, Detail: "no rule in archRules"})
			continue
		}
		tos := map[string]bool{}
		for to := range pk.Prod {
			tos[to] = true
		}
		for to := range pk.Test {
			tos[to] = true
		}
		for _, to := range archSorted(tos) {
			prod := pk.Prod[to]
			var ok bool
			switch {
			case to == archCmd:
				ok = false
			case to == archTestenv:
				ok = !prod
			default:
				ok = archMatchAny(rule.allow, to) || (!prod && archMatchAny(rule.allowTest, to))
			}
			if !ok {
				out = append(out, archViolation{From: from, To: to, Test: !prod})
			}
		}
	}
	return out
}

func archSorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// archFiles names the files of the importing package that carry the edge:
// production files for a production edge, _test.go files for a test-only one.
func archFiles(g *archGraph, v archViolation) []string {
	dir := filepath.Join(g.Root, filepath.FromSlash(v.From))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	quoted := `"` + g.Module + "/" + v.To + `"`
	var out []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") != v.Test {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil && strings.Contains(string(b), quoted) {
			out = append(out, v.From+"/"+name)
		}
	}
	return out
}

// archReport renders a violation as "file -> import" lines.
func archReport(g *archGraph, v archViolation) []string {
	if v.To == "" {
		return []string{v.From + ": " + v.Detail}
	}
	files := archFiles(g, v)
	if len(files) == 0 {
		files = []string{v.From}
	}
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f + " -> " + v.To
	}
	return out
}

// archRatchet compares the violations with the exceptions: fresh are
// violations no exception covers, stale are exceptions no violation needs.
func archRatchet(vs []archViolation, exc map[string]string) (fresh, stale []string) {
	seen := map[string]bool{}
	for _, v := range vs {
		k := v.key()
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, ok := exc[k]; !ok {
			fresh = append(fresh, k)
		}
	}
	for _, k := range archSorted(exc) {
		if !seen[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(fresh)
	return fresh, stale
}

// archGrew fails when the exceptions outnumber the cap.
func archGrew(exc map[string]string, max int) error {
	if len(exc) > max {
		return fmt.Errorf("archExceptions has %d entries, over the cap of %d: fix the import instead of excusing it "+
			"(the ratchet may shrink, never grow)", len(exc), max)
	}
	return nil
}

// --- the model ------------------------------------------------------------------

// archComponent folds a package into its diagram node: families collapse to
// one node (core/*, agent/*), single packages keep their path under internal/.
// "" leaves the package out of the diagram (the repository checks in tests/).
func archComponent(pkg string) string {
	switch {
	case pkg == "tests":
		return ""
	case pkg == archCmd:
		return pkg
	case strings.HasPrefix(pkg, "tools/"):
		return "tools/*"
	}
	rest, ok := strings.CutPrefix(pkg, "internal/")
	if !ok {
		return pkg
	}
	for _, fam := range []string{"core", "agent", "devops", "term", "docs", "herdr"} {
		if strings.HasPrefix(rest, fam+"/") {
			return fam + "/*"
		}
	}
	return rest
}

func archNodeID(c string) string {
	return strings.NewReplacer("/*", "", "/", "_", "-", "_").Replace(c)
}

// archLayered are the components drawn outside the leaves box.
var archLayered = []string{"cmd/aval", "ui/cli", "ui", "core/*", "design/hook"}

// archModel renders docs/architecture/model.md from the rules and the graph.
func archModel(g *archGraph) string {
	type edge struct{ from, to string }
	prod, test := map[edge]bool{}, map[edge]bool{}
	comps := map[string]bool{}
	for _, from := range archSorted(g.Pkgs) {
		cf := archComponent(from)
		if cf == "" {
			continue
		}
		comps[cf] = true
		pk := g.Pkgs[from]
		for to := range pk.Prod {
			if ct := archComponent(to); ct != "" && ct != cf {
				prod[edge{cf, ct}] = true
			}
		}
		for to := range pk.Test {
			if ct := archComponent(to); ct != "" && ct != cf {
				test[edge{cf, ct}] = true
			}
		}
	}
	layered := map[string]bool{}
	for _, c := range archLayered {
		layered[c] = true
	}
	node := func(c string) string { return fmt.Sprintf("  %s[\"%s\"]\n", archNodeID(c), c) }

	var b strings.Builder
	b.WriteString("# Architecture model\n\n")
	b.WriteString("<!-- Generated by tests/architecture_test.go. Do not edit by hand. -->\n")
	b.WriteString("<!-- Regenerate: go test ./tests/ -run TestArchitectureModel -update -->\n\n")
	b.WriteString("The component graph of the Go packages, read from `go list` for linux, darwin and windows.\n")
	b.WriteString("Families of packages fold into one node (`core/*`, `agent/*`). A solid arrow is a production\n")
	b.WriteString("import; a dotted one exists only in tests. `tests/` (the repository checks) is left out.\n\n")
	b.WriteString("`TestArchitectureLayeringHolds` enforces the rules below; changing them needs an ADR in `docs/adr/`.\n\n")
	b.WriteString("```mermaid\nflowchart TD\n")
	for _, c := range archLayered {
		if comps[c] {
			b.WriteString(node(c))
		}
	}
	b.WriteString("  subgraph leaves[\"leaves: no internal imports outside tests\"]\n")
	for _, c := range archSorted(comps) {
		if !layered[c] {
			b.WriteString("  " + node(c))
		}
	}
	b.WriteString("  end\n")
	var lines []string
	for e := range prod {
		lines = append(lines, fmt.Sprintf("  %s --> %s", archNodeID(e.from), archNodeID(e.to)))
	}
	for e := range test {
		if !prod[e] {
			lines = append(lines, fmt.Sprintf("  %s -.->|test| %s", archNodeID(e.from), archNodeID(e.to)))
		}
	}
	sort.Strings(lines)
	b.WriteString(strings.Join(lines, "\n") + "\n```\n\n")

	b.WriteString("## Rules\n\n| Package | May import | In tests only | Why |\n| :--- | :--- | :--- | :--- |\n")
	code := func(ps []string) string {
		if len(ps) == 0 {
			return "nothing internal"
		}
		q := make([]string, len(ps))
		for i, p := range ps {
			q[i] = "`" + p + "`"
		}
		return strings.Join(q, ", ")
	}
	for _, r := range archRules {
		tests := "-"
		if len(r.allowTest) > 0 {
			tests = code(r.allowTest)
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", r.pkg, code(r.allow), tests, r.why)
	}
	b.WriteString("\nEverywhere: `internal/testenv` only from test files, and nothing imports `cmd/aval`.\n\n")
	fmt.Fprintf(&b, "## Exceptions\n\n%d counted exceptions. The list may shrink, never grow.\n", len(archExceptions))
	for _, k := range archSorted(archExceptions) {
		fmt.Fprintf(&b, "\n- `%s`: %s\n", k, archExceptions[k])
	}
	return b.String()
}

// archModelDrift reports whether the committed model differs from the one the
// graph generates today.
func archModelDrift(committed, generated string) error {
	if committed != generated {
		return errors.New("docs/architecture/model.md is stale: the import graph or the rules changed")
	}
	return nil
}

// --- tests ----------------------------------------------------------------------

func archPlant(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/plant\n\ngo 1.25\n")
	for rel, src := range files {
		write(t, filepath.Join(dir, rel), src)
	}
	return dir
}

func archReportAll(g *archGraph) []string {
	var out []string
	for _, v := range archCheck(g) {
		out = append(out, archReport(g, v)...)
	}
	sort.Strings(out)
	return out
}

func TestArchitectureGuardCatchesAPlantedEdge(t *testing.T) {
	dir := archPlant(t, map[string]string{
		"internal/ui/ui.go":             "package ui\n\nconst X = 1\n",
		"internal/core/foo/foo.go":      "package foo\n\nimport \"example.com/plant/internal/ui\"\n\nvar _ = ui.X\n",
		"internal/core/foo/ok.go":       "package foo\n",
		"internal/core/bar/bar.go":      "package bar\n\nimport _ \"example.com/plant/internal/core/foo\"\n",
		"internal/release/release.go":   "package release\n",
		"internal/core/bar/bar_test.go": "package bar\n\nimport _ \"example.com/plant/internal/release\"\n",
	})
	got := archReportAll(loadArchGraph(t, dir))
	want := []string{"internal/core/foo/foo.go -> internal/ui"}
	eq(t, "violations in the planted module", strings.Join(want, "\n"), strings.Join(got, "\n"))
}

func TestArchitectureGuardCatchesBuildTaggedAndTestOnlyEdges(t *testing.T) {
	dir := archPlant(t, map[string]string{
		"internal/testenv/env.go":      "package testenv\n",
		"internal/deck/deck.go":        "package deck\n",
		"internal/deck/win_windows.go": "package deck\n\nimport _ \"example.com/plant/internal/testenv\"\n",
		"internal/ui/ui.go":            "package ui\n",
		"internal/ui/ui_test.go":       "package ui\n\nimport _ \"example.com/plant/internal/testenv\"\n",
		"internal/ui/cli/cli.go":       "package cli\n\nimport _ \"example.com/plant/cmd/aval\"\n",
		"cmd/aval/main.go":             "package main\n\nfunc main() {}\n",
		"internal/brandnew/x.go":       "package brandnew\n",
	})
	got := archReportAll(loadArchGraph(t, dir))
	want := []string{
		"internal/brandnew: no rule in archRules",
		"internal/deck/win_windows.go -> internal/testenv",
		"internal/ui/cli/cli.go -> cmd/aval",
	}
	eq(t, "violations in the planted module", strings.Join(want, "\n"), strings.Join(got, "\n"))
}

func TestArchitectureRatchetRejectsANewException(t *testing.T) {
	vs := []archViolation{{From: "internal/core/x", To: "internal/ui"}, {From: "internal/deck", To: "internal/release"}}
	fresh, stale := archRatchet(vs, map[string]string{"internal/deck -> internal/release": "legacy"})
	eq(t, "fresh", "internal/core/x -> internal/ui", strings.Join(fresh, ","))
	eq(t, "stale", 0, len(stale))
}

func TestArchitectureRatchetRejectsAStaleException(t *testing.T) {
	fresh, stale := archRatchet(nil, map[string]string{"internal/deck -> internal/release": "fixed long ago"})
	eq(t, "fresh", 0, len(fresh))
	eq(t, "stale", "internal/deck -> internal/release", strings.Join(stale, ","))
}

func TestArchitectureRatchetCapsTheExceptions(t *testing.T) {
	two := map[string]string{"a -> b": "x", "c -> d": "y"}
	isTrue(t, "two exceptions under a cap of 1 are rejected", archGrew(two, 1) != nil)
	isTrue(t, "two exceptions under a cap of 2 pass", archGrew(two, 2) == nil)
}

func TestArchitectureModelDriftIsDetected(t *testing.T) {
	gen := "# model\n\nA --> B\n"
	isTrue(t, "an identical file is not stale", archModelDrift(gen, gen) == nil)
	isTrue(t, "a modified file is stale", archModelDrift(gen+"C --> D\n", gen) != nil)
	isTrue(t, "a missing file is stale", archModelDrift("", gen) != nil)
}

func TestArchitectureModelRendersTheComponents(t *testing.T) {
	dir := archPlant(t, map[string]string{
		"cmd/aval/main.go":           "package main\n\nimport _ \"example.com/plant/internal/ui/cli\"\n\nfunc main() {}\n",
		"internal/ui/cli/cli.go":     "package cli\n\nimport _ \"example.com/plant/internal/ui\"\n",
		"internal/ui/ui.go":          "package ui\n\nimport _ \"example.com/plant/internal/core/a\"\n",
		"internal/core/a/a.go":       "package a\n\nimport _ \"example.com/plant/internal/release\"\n",
		"internal/release/r.go":      "package release\n",
		"internal/agent/x/x.go":      "package x\n",
		"internal/agent/x/x_test.go": "package x\n\nimport _ \"example.com/plant/internal/testenv\"\n",
		"internal/testenv/t.go":      "package testenv\n",
	})
	m := archModel(loadArchGraph(t, dir))
	for _, want := range []string{"```mermaid", "flowchart", `cmd_aval["cmd/aval"]`, `ui_cli["ui/cli"]`,
		"cmd_aval --> ui_cli", "ui_cli --> ui", "ui --> core", "core --> release", "agent -.->|test| testenv"} {
		mustContain(t, m, want, "model.md")
	}
}

func TestArchitectureLayeringHolds(t *testing.T) {
	g := loadArchGraph(t, repo)
	vs := archCheck(g)
	fresh, stale := archRatchet(vs, archExceptions)
	if len(fresh) > 0 {
		var lines []string
		for _, v := range vs {
			if _, ok := archExceptions[v.key()]; !ok {
				lines = append(lines, archReport(g, v)...)
			}
		}
		t.Errorf("imports that break the internal/ layering (fix them, or change archRules with an ADR):\n  %s",
			strings.Join(lines, "\n  "))
	}
	if err := archGrew(archExceptions, archMaxExceptions); err != nil {
		t.Error(err)
	}
	if len(stale) > 0 {
		t.Errorf("archExceptions lists edges that no longer exist; delete them (the ratchet only shrinks):\n  %s",
			strings.Join(stale, "\n  "))
	}
}

func TestArchitectureModel(t *testing.T) {
	gen := archModel(loadArchGraph(t, repo))
	path := p("docs", "architecture", "model.md")
	if *archUpdate {
		write(t, path, gen)
		return
	}
	b, _ := os.ReadFile(path)
	if err := archModelDrift(string(b), gen); err != nil {
		t.Errorf("%v\n  regenerate it: go test ./tests/ -run TestArchitectureModel -update", err)
	}
}
