// Package review sizes a code review from git alone: a deterministic risk tier
// and the reviewer lenses worth running for it.
//
// The tier decides how much review a change earns, so it has to be the same
// answer every time for the same diff: no model, no judgement, only paths and
// line counts.
//
//	high    touches a high-risk path (hooks, settings, install.manifest,
//	        installers, auth, migrations, payments, CI workflows,
//	        permissions), or more than 400 production lines
//	medium  source changed without test changes, or more than 100 production
//	        lines even with them
//	low     docs and tests only, or a small source change that carries tests
package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// HighLines is the production-line count above which a change is high risk.
	HighLines = 400
	// MediumLines is the count above which even a tested change is medium.
	MediumLines = 100
)

// Result is the outcome of Assess.
type Result struct {
	Base            string   `json:"base"`
	Tier            string   `json:"tier"`
	Reasons         []string `json:"reasons"`
	Lenses          []string `json:"lenses"`
	Files           int      `json:"files"`
	Added           int      `json:"added"`
	Removed         int      `json:"removed"`
	ProductionLines int      `json:"production_lines"`
	TestsChanged    bool     `json:"tests_changed"`
	HighRiskPaths   []string `json:"high_risk_paths"`
}

var lensesByTier = map[string][]string{
	"low":    nil,
	"medium": {"review-reliability"},
	"high":   {"review-risk", "review-resilience", "review-reliability", "review-readability"},
}

// --- path classification ---------------------------------------------------

var (
	testRe = regexp.MustCompile(`(^|/)(tests?|__tests__|spec|specs)/|_test\.|\.test\.|\.spec\.|(^|/)test_[^/]*\.py$`)
	docRe  = regexp.MustCompile(`\.(md|mdx|txt|rst|adoc)$|(^|/)docs?/|(^|/)(license|changelog|notice)[^/]*$`)
)

type category struct {
	label string
	re    *regexp.Regexp
}

// highRisk is ordered: reasons are reported in this order.
var highRisk = []category{
	{"hooks", regexp.MustCompile(`(^|/)hooks?/|[-_]hooks?\.[a-z]+$`)},
	{"settings", regexp.MustCompile(`(^|/)settings(\.local)?\.json$`)},
	{"install.manifest", regexp.MustCompile(`(^|/)install\.manifest$`)},
	{"installers", regexp.MustCompile(`(^|/)install(er)?[^/]*\.(sh|ps1|js|ts|go|py)$|(^|/)installers?/|(^|/)install/`)},
	{"auth", regexp.MustCompile(`(^|[/._-])(auth|authn|authz|oauth|authentication|authorization)([/._-]|$)`)},
	{"migrations", regexp.MustCompile(`(^|[/._-])migrations?([/._-]|$)`)},
	{"payments", regexp.MustCompile(`(^|[/._-])(payments?|billing|stripe)([/._-]|$)`)},
	{".github/workflows", regexp.MustCompile(`^\.github/workflows/`)},
	{"permissions", regexp.MustCompile(`(^|[/._-])permissions?([/._-]|$)`)},
}

type kind int

const (
	kindSource kind = iota
	kindTest
	kindDoc
)

func classify(p string) kind {
	switch {
	case testRe.MatchString(p):
		return kindTest
	case docRe.MatchString(p):
		return kindDoc
	}
	return kindSource
}

// --- git -------------------------------------------------------------------

func gitOut(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func verify(dir, ref string) bool {
	_, err := gitOut(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// resolveBase picks the ref to compare against: the explicit one, or the
// first of main/master/origin/main/origin/master that exists, or HEAD (only
// uncommitted work then counts).
func resolveBase(dir, base string) (string, error) {
	if base != "" {
		if !verify(dir, base) {
			return "", fmt.Errorf("unknown base %q", base)
		}
		return base, nil
	}
	for _, c := range []string{"main", "master", "origin/main", "origin/master"} {
		if verify(dir, c) {
			return c, nil
		}
	}
	return "HEAD", nil
}

type stat struct{ added, removed int }

// Assess compares the working tree of the repository at dir (committed,
// staged, unstaged and untracked changes) with the merge-base of base and HEAD.
func Assess(dir, base string) (*Result, error) {
	if _, err := gitOut(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("not a git repository: %s", dir)
	}
	if !verify(dir, "HEAD") {
		return nil, fmt.Errorf("repository has no commits yet")
	}
	name, err := resolveBase(dir, base)
	if err != nil {
		return nil, err
	}
	from := name
	if mb, err := gitOut(dir, "merge-base", name, "HEAD"); err == nil {
		from = strings.TrimSpace(string(mb))
	}

	files := map[string]stat{}
	num, err := gitOut(dir, "diff", "--numstat", "--no-renames", "-z", from)
	if err != nil {
		return nil, err
	}
	for _, rec := range strings.Split(string(num), "\x00") {
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		a, _ := strconv.Atoi(parts[0]) // "-" (binary) counts as 0
		d, _ := strconv.Atoi(parts[1])
		files[filepath.ToSlash(parts[2])] = stat{a, d}
	}
	untracked, err := gitOut(dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(string(untracked), "\x00") {
		if p != "" {
			files[filepath.ToSlash(p)] = stat{added: countLines(filepath.Join(dir, p))}
		}
	}

	return tier(name, files), nil
}

func countLines(path string) int {
	b, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(b, 0) >= 0 {
		return 0
	}
	n := bytes.Count(b, []byte{'\n'})
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// --- the decision ----------------------------------------------------------

func tier(base string, files map[string]stat) *Result {
	r := &Result{Base: base, Files: len(files)}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var nSource, nTest, nDoc int
	byCat := map[string][]string{}
	for _, p := range paths {
		s := files[p]
		r.Added += s.added
		r.Removed += s.removed
		lower := strings.ToLower(p)
		switch classify(lower) {
		case kindTest:
			nTest++
		case kindDoc:
			nDoc++
		default:
			nSource++
			r.ProductionLines += s.added + s.removed
			for _, c := range highRisk {
				if c.re.MatchString(lower) {
					byCat[c.label] = append(byCat[c.label], p)
					r.HighRiskPaths = append(r.HighRiskPaths, p)
				}
			}
		}
	}
	r.TestsChanged = nTest > 0
	sort.Strings(r.HighRiskPaths)
	r.HighRiskPaths = dedupe(r.HighRiskPaths)

	switch {
	case len(files) == 0:
		r.Tier = "low"
		r.Reasons = []string{"no changes against " + base}
	case len(r.HighRiskPaths) > 0 || r.ProductionLines > HighLines:
		r.Tier = "high"
		for _, c := range highRisk {
			if ps := byCat[c.label]; len(ps) > 0 {
				r.Reasons = append(r.Reasons, fmt.Sprintf("touches %s (%d file(s)): %s", c.label, len(ps), strings.Join(ps, ", ")))
			}
		}
		if r.ProductionLines > HighLines {
			r.Reasons = append(r.Reasons, fmt.Sprintf("%d production lines exceed %d", r.ProductionLines, HighLines))
		}
	case nSource > 0 && !r.TestsChanged:
		r.Tier = "medium"
		r.Reasons = []string{fmt.Sprintf("source changed without test changes (%d production lines)", r.ProductionLines)}
	case nSource > 0 && r.ProductionLines > MediumLines:
		r.Tier = "medium"
		r.Reasons = []string{fmt.Sprintf("%d production lines exceed %d even though tests changed", r.ProductionLines, MediumLines)}
	case nSource > 0:
		r.Tier = "low"
		r.Reasons = []string{fmt.Sprintf("small source change (%d production lines) and tests changed", r.ProductionLines)}
	case nTest > 0 && nDoc > 0:
		r.Tier = "low"
		r.Reasons = []string{"only docs and tests changed"}
	case nTest > 0:
		r.Tier = "low"
		r.Reasons = []string{"only tests changed"}
	default:
		r.Tier = "low"
		r.Reasons = []string{"only docs changed"}
	}
	r.Lenses = lensesByTier[r.Tier]
	return r
}

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// --- output ----------------------------------------------------------------

// Render prints the result for a person.
func Render(w io.Writer, r *Result) {
	fmt.Fprintf(w, "risk      %s\n", r.Tier)
	fmt.Fprintf(w, "base      %s\n", r.Base)
	tests := "no"
	if r.TestsChanged {
		tests = "yes"
	}
	fmt.Fprintf(w, "changes   %d file(s), +%d -%d, %d production lines, tests changed: %s\n",
		r.Files, r.Added, r.Removed, r.ProductionLines, tests)
	fmt.Fprintln(w, "reasons")
	for _, why := range r.Reasons {
		fmt.Fprintf(w, "  - %s\n", why)
	}
	if len(r.Lenses) == 0 {
		fmt.Fprintln(w, "reviewers none (self-review is enough)")
		return
	}
	fmt.Fprintf(w, "reviewers %s\n", strings.Join(r.Lenses, ", "))
}

// RenderJSON prints the result as indented JSON. Empty lists are arrays, not
// null, so a consumer can iterate without a check.
func RenderJSON(w io.Writer, r *Result) error {
	c := *r
	if c.Lenses == nil {
		c.Lenses = []string{}
	}
	if c.Reasons == nil {
		c.Reasons = []string{}
	}
	if c.HighRiskPaths == nil {
		c.HighRiskPaths = []string{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(c)
}
