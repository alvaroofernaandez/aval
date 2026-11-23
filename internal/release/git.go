package release

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// Git runs git commands in a repository.
type Git interface {
	Run(args ...string) (string, error)
}

type execGit struct{ dir string }

// NewGit returns a Git bound to a directory.
func NewGit(dir string) Git { return execGit{dir: dir} }

func (g execGit) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, text)
	}
	return text, nil
}

// State is what the repository says about releases.
type State struct {
	Head       string    // full sha of HEAD
	Tags       []Version // every v* SemVer tag
	HeadTags   []string  // v* tags pointing at HEAD
	Unreleased []Commit  // since the last stable tag
	SinceTag   []Commit  // since the last tag of any kind
	LastTag    Version
	HasLastTag bool
}

// Inspect reads tags and commits.
func Inspect(g Git) (State, error) {
	var st State
	head, err := g.Run("rev-parse", "HEAD")
	if err != nil {
		return st, fmt.Errorf("no commits to release: %w", err)
	}
	st.Head = head

	out, err := g.Run("tag", "--list", "v*")
	if err != nil {
		return st, err
	}
	for _, name := range strings.Fields(out) {
		if v, err := Parse(name); err == nil && v.Tag() == name {
			st.Tags = append(st.Tags, v)
		}
	}
	for _, v := range st.Tags {
		if !st.HasLastTag || v.Compare(st.LastTag) > 0 {
			st.LastTag, st.HasLastTag = v, true
		}
	}

	if out, err := g.Run("tag", "--points-at", "HEAD", "v*"); err == nil {
		st.HeadTags = strings.Fields(out)
	}

	stable := LastStable(st.Tags)
	if stable == (Version{}) {
		if st.Unreleased, err = commitsIn(g, "HEAD"); err != nil {
			return st, err
		}
	} else if st.Unreleased, err = commitsIn(g, stable.Tag()+"..HEAD"); err != nil {
		return st, err
	}
	if st.HasLastTag {
		if st.SinceTag, err = commitsIn(g, st.LastTag.Tag()+"..HEAD"); err != nil {
			return st, err
		}
	} else {
		st.SinceTag = st.Unreleased
	}
	return st, nil
}

func commitsIn(g Git, rng string) ([]Commit, error) {
	out, err := g.Run("log", "--format=%H%x1f%s%x1f%b%x1e", rng)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		f := strings.SplitN(rec, "\x1f", 3)
		for len(f) < 3 {
			f = append(f, "")
		}
		commits = append(commits, ParseCommit(f[0], f[1], f[2]))
	}
	return commits, nil
}

// Proposal is a release about to be cut.
type Proposal struct {
	Version Version
	Kind    string
	Commits []Commit // what justifies the version
	Notes   string
	Target  string // commit to tag
}

// Resolve turns a kind (rc, beta, patch, minor, major, promote) into a
// concrete proposal.
func Resolve(g Git, st State, kind string) (Proposal, error) {
	p := Proposal{Kind: kind, Target: st.Head, Commits: st.Unreleased}
	var err error
	switch kind {
	case "rc", "beta":
		p.Version, err = Next(st.Tags, st.Unreleased, kind)
	case "patch", "minor", "major":
		p.Version, err = Bump(st.Tags, kind)
	case "promote":
		return resolvePromote(g, st)
	default:
		return p, fmt.Errorf("unknown release kind %q", kind)
	}
	if err != nil {
		return p, err
	}
	if len(st.HeadTags) > 0 {
		return p, fmt.Errorf("HEAD is already tagged %s; commit something first", strings.Join(st.HeadTags, ", "))
	}
	for _, t := range st.Tags {
		if t == p.Version {
			return p, fmt.Errorf("%s already exists", p.Version.Tag())
		}
	}
	notes := st.Unreleased
	if p.Version.IsPre() {
		notes = st.SinceTag
	}
	p.Notes = Notes(notes)
	return p, nil
}

func resolvePromote(g Git, st State) (Proposal, error) {
	p := Proposal{Kind: "promote"}
	v, err := Promote(st.Tags)
	if err != nil {
		return p, err
	}
	var rc Version
	for _, t := range st.Tags {
		if t.IsPre() && t.Major == v.Major && t.Minor == v.Minor && t.Patch == v.Patch && t.Compare(rc) > 0 {
			rc = t
		}
	}
	target, err := g.Run("rev-list", "-n1", rc.Tag())
	if err != nil {
		return p, err
	}
	rng := rc.Tag()
	if stable := LastStable(st.Tags); stable != (Version{}) {
		rng = stable.Tag() + ".." + rc.Tag()
	}
	commits, err := commitsIn(g, rng)
	if err != nil {
		return p, err
	}
	p.Version, p.Target, p.Commits, p.Notes = v, target, commits, Notes(commits)
	return p, nil
}

// ErrNoCI means CI status cannot be queried here (no gh, no network).
var ErrNoCI = errors.New("CI status unavailable")

// Gate holds the checks a release must pass.
type Gate struct {
	Git       Git
	Remote    string
	Branch    string
	RunTests  func() error
	CI        func(sha string) (string, error) // "" when green, else a reason
	SkipTests bool
	// Promote checks the rc commit, not HEAD: the branch and up-to-date
	// checks do not apply, since promote tags a commit already on the branch.
	Promote bool
}

// Run executes every check, printing one line each, and fails if any failed.
func (gt Gate) Run(target string, w io.Writer) error {
	failed := 0
	report := func(ok bool, format string, a ...any) {
		mark := "ok  "
		if !ok {
			mark, failed = "FAIL", failed+1
		}
		fmt.Fprintf(w, "  [%s] %s\n", mark, fmt.Sprintf(format, a...))
	}
	note := func(format string, a ...any) { fmt.Fprintf(w, "  [skip] %s\n", fmt.Sprintf(format, a...)) }

	if out, err := gt.Git.Run("status", "--porcelain"); err != nil || out != "" {
		report(false, "working tree has uncommitted changes")
	} else {
		report(true, "working tree clean")
	}

	if gt.Promote {
		note("branch and up-to-date checks (promote tags the rc commit, already on %s)", gt.Branch)
	} else if b, _ := gt.Git.Run("rev-parse", "--abbrev-ref", "HEAD"); b != gt.Branch {
		report(false, "on branch %q; releases are cut from %s", b, gt.Branch)
	} else {
		report(true, "on %s", gt.Branch)
	}

	upstream := gt.Remote + "/" + gt.Branch
	if gt.Promote {
		// Checked by promoteCheck before the gate runs.
	} else if _, err := gt.Git.Run("fetch", "--quiet", gt.Remote); err != nil {
		report(false, "cannot fetch %s: %v", gt.Remote, err)
	} else if counts, err := gt.Git.Run("rev-list", "--left-right", "--count", "HEAD..."+upstream); err != nil {
		report(false, "cannot compare with %s: %v", upstream, err)
	} else {
		var ahead, behind int
		fmt.Sscanf(counts, "%d %d", &ahead, &behind)
		if ahead == 0 && behind == 0 {
			report(true, "up to date with %s", upstream)
		} else {
			report(false, "HEAD differs from %s (%d ahead, %d behind): push or pull first", upstream, ahead, behind)
		}
	}

	switch {
	case gt.SkipTests:
		note("go test ./... (dry run)")
	case gt.RunTests == nil:
		note("go test ./... (no runner)")
	default:
		if err := gt.RunTests(); err != nil {
			report(false, "go test ./... failed: %v", err)
		} else {
			report(true, "go test ./... passes")
		}
	}

	if gt.CI == nil {
		note("CI status (gh not available)")
	} else if reason, err := gt.CI(target); errors.Is(err, ErrNoCI) {
		note("CI status (%v)", err)
	} else if err != nil {
		report(false, "CI status for %s: %v", short(target), err)
	} else if reason != "" {
		report(false, "CI on %s is not green: %s", short(target), reason)
	} else {
		report(true, "CI green on %s", short(target))
	}

	if failed > 0 {
		return gateFailed(failed)
	}
	return nil
}

func short(sha string) string { return shortHash(sha) }

// CutOptions drives Cut.
type CutOptions struct {
	Kind   string
	Yes    bool
	DryRun bool
	In     io.Reader
	Out    io.Writer
	Gate   Gate
	Remote string
}

// ErrAborted means the user declined.
var ErrAborted = errors.New("aborted: nothing was tagged")

// Cut checks, summarises, confirms, tags and pushes.
func Cut(g Git, o CutOptions) error {
	w := o.Out
	if o.Remote == "" {
		o.Remote = "origin"
	}

	// Tags created elsewhere change what "next" means.
	if _, err := g.Run("fetch", "--quiet", "--tags", o.Remote); err != nil {
		return fmt.Errorf("cannot fetch tags from %s: %w", o.Remote, err)
	}
	st, err := Inspect(g)
	if err != nil {
		return err
	}
	p, err := Resolve(g, st, o.Kind)
	if err != nil {
		return err
	}
	var after []Commit
	if o.Kind == "promote" {
		if after, err = promoteCheck(g, p.Target, o.Remote, o.Gate.Branch); err != nil {
			return err
		}
		o.Gate.Promote = true
	}

	channel := ChannelStable
	if p.Version.IsPre() {
		channel = ChannelPrerelease
	}
	fmt.Fprintf(w, "Release %s (%s)\n", p.Version.Tag(), channel)
	fmt.Fprintf(w, "  commit %s, %d commit(s) since the last %s\n\n", short(p.Target), len(p.Commits), "stable tag")
	if p.Notes != "" {
		fmt.Fprintln(w, p.Notes)
	} else {
		fmt.Fprintln(w, "(no user-facing changes to list)")
		fmt.Fprintln(w)
	}
	if err := reportAfterRC(g, after, w); err != nil {
		return err
	}

	fmt.Fprintln(w, "Checks")
	if err := o.Gate.Run(p.Target, w); err != nil {
		return err
	}
	if o.DryRun {
		fmt.Fprintln(w, "\nDry run: nothing was tagged or pushed.")
		return nil
	}

	if !o.Yes {
		fmt.Fprintf(w, "\nCreate and push annotated tag %s? [y/N] ", p.Version.Tag())
		line, _ := bufio.NewReader(o.In).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(w)
			return ErrAborted
		}
	}

	tag := p.Version.Tag()
	msg := "aval " + p.Version.String()
	if p.Notes != "" {
		msg += "\n\n" + p.Notes
	}
	args := []string{"tag", "-a", tag, "-m", msg}
	if p.Target != "" {
		args = append(args, p.Target)
	}
	if _, err := g.Run(args...); err != nil {
		return err
	}
	fmt.Fprintf(w, "\nTagged %s (annotated).\n", tag)

	if _, err := g.Run("push", o.Remote, tag); err != nil {
		fmt.Fprintf(w, "Push failed; the tag exists locally. Retry with `git push %s %s` or remove it with `git tag -d %s`.\n", o.Remote, tag, tag)
		return err
	}
	fmt.Fprintf(w, "Pushed %s to %s.\n", tag, o.Remote)

	url := ""
	if remote, err := g.Run("remote", "get-url", o.Remote); err == nil {
		url = WorkflowURL(remote)
	}
	fmt.Fprintln(w, "\nThe release workflow is now waiting for approval of the \"release\" environment.")
	if url != "" {
		fmt.Fprintf(w, "Approve it here: %s\n", url)
	}
	return nil
}

var remoteRe = regexp.MustCompile(`github\.com[:/]([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

// WorkflowURL is the release workflow page for a remote URL, or "".
func WorkflowURL(remote string) string {
	m := remoteRe.FindStringSubmatch(strings.TrimSpace(remote))
	if m == nil {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s/actions/workflows/release.yml", m[1], m[2])
}

// gateFailed is the number of release checks that failed; each one is a
// [FAIL] line in the gate's output.
type gateFailed int

func (n gateFailed) Error() string {
	return fmt.Sprintf("%d check(s) failed; nothing was tagged", int(n))
}

// errRCNotOnMain refuses a promote whose rc commit is not on the release branch.
var errRCNotOnMain = errors.New("the release candidate is not on the release branch")

// promoteCheck requires the rc commit to be on remote/branch and returns the
// commits the branch gained after it, which the stable tag will not contain.
func promoteCheck(g Git, target, remote, branch string) ([]Commit, error) {
	if branch == "" {
		branch = "main"
	}
	upstream := remote + "/" + branch
	if _, err := g.Run("merge-base", "--is-ancestor", target, upstream); err != nil {
		return nil, fmt.Errorf("%w: %s is not an ancestor of %s; cut a new rc from %s", errRCNotOnMain, short(target), upstream, branch)
	}
	return commitsIn(g, target+".."+upstream)
}

// reportAfterRC lists the commits made after the rc and warns when any of them
// changes shipped code. It never refuses: the owner decides.
func reportAfterRC(g Git, after []Commit, w io.Writer) error {
	if len(after) == 0 {
		return nil
	}
	fmt.Fprintf(w, "Not in this release: %d commit(s) after the rc\n", len(after))
	code := false
	for _, c := range after {
		out, err := g.Run("diff-tree", "--no-commit-id", "--name-only", "-r", c.Hash)
		if err != nil {
			return err
		}
		mark := ""
		if shipsCode(strings.Fields(out)) {
			code, mark = true, "  (shipped code)"
		}
		fmt.Fprintf(w, "  %s %s%s\n", short(c.Hash), c.Subject, mark)
	}
	if code {
		fmt.Fprintln(w, "Warning: commits after the rc change shipped code; the stable will not include them. Consider cutting a new rc.")
	}
	fmt.Fprintln(w)
	return nil
}

// shipsCode reports whether any path is part of what a release builds, that
// is, outside docs, tests, plans, agent skills and Markdown files.
func shipsCode(paths []string) bool {
	for _, p := range paths {
		if strings.HasSuffix(p, ".md") {
			continue
		}
		skip := false
		for _, dir := range []string{"docs/", "tests/", "odd/", ".claude/"} {
			if strings.HasPrefix(p, dir) {
				skip = true
				break
			}
		}
		if !skip {
			return true
		}
	}
	return false
}
