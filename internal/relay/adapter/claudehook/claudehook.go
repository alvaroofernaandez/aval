// Package claudehook is the relay adapter for Claude Code's classic hooks,
// ladder level 2 of #65: when the relay mod does not run, PreCompact steers
// the native summary and SessionStart(compact) adds the capsule. The facts
// come from the session transcript, whose format Claude Code does not
// document; the shape read here is the one recorded on 2.1.331.
package claudehook

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// maxLine bounds one transcript line; a longer one is skipped.
const maxLine = 16 << 20

type entry struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	IsMeta      bool      `json:"isMeta"`
	IsSidechain bool      `json:"isSidechain"`
	Summary     bool      `json:"isCompactSummary"` // seen on 2.1.331 on the native summary
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Name  string `json:"name"`
	Input struct {
		FilePath string `json:"file_path"`
	} `json:"input"`
}

// editTools are the tools whose file_path is an edited file.
var editTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// FromTranscript reads a transcript and returns the capsule facts: the
// person's requests and the files edited. Lines it cannot read are skipped.
func FromTranscript(r io.Reader) relay.Capsule {
	t := fromTranscript(r)
	return t.capsule
}

// transcript is what a transcript says beyond the capsule: the last assistant
// reply and when the session began.
type transcript struct {
	capsule relay.Capsule
	reply   string    // text of the last assistant reply
	since   time.Time // timestamp of the first entry; zero when none
}

func fromTranscript(r io.Reader) transcript {
	var f facts
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && !e.IsSidechain {
			f.add(e)
		}
	}
	return transcript{capsule: capsule(f.first, f.prompts, f.files), reply: f.reply, since: f.since}
}

// facts accumulates what the transcript says, in order.
type facts struct {
	first          string // from an earlier capsule
	prompts, files []string
	reply          string // text of the last assistant entry that has any
	since          time.Time
}

func (f *facts) add(e entry) {
	if f.since.IsZero() && !e.Timestamp.IsZero() {
		f.since = e.Timestamp
	}
	text, blocks := content(e.Message.Content)
	switch {
	case e.Type == "assistant":
		f.files = append(f.files, edited(blocks)...)
		if text != "" {
			f.reply = text
		}
	case e.Type != "user" || e.IsMeta || e.Summary:
	case isCapsule(text):
		if first, _ := relay.CapsuleFirst(text); f.first == "" {
			f.first = first
		}
	case relay.IsRequest(text):
		f.prompts = append(f.prompts, text)
	}
}

func isCapsule(text string) bool {
	_, ok := relay.CapsuleFirst(text)
	return ok
}

func edited(blocks []block) []string {
	var files []string
	for _, b := range blocks {
		if b.Type == "tool_use" && editTools[b.Name] && b.Input.FilePath != "" {
			files = append(files, b.Input.FilePath)
		}
	}
	return files
}

// content is a message's text (a string, or its text blocks; empty when it
// holds a tool result) and its blocks.
func content(raw json.RawMessage) (string, []block) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}
	var texts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "tool_result":
			return "", blocks
		}
	}
	return strings.Join(texts, "\n"), blocks
}

func capsule(first string, prompts, files []string) relay.Capsule {
	if first == "" && len(prompts) > 0 {
		first, prompts = prompts[0], prompts[1:]
	}
	c := relay.Capsule{FirstPrompt: first}
	if n := len(prompts); n > 0 {
		c.LastPrompt, c.Prompts = prompts[n-1], prompts[:n-1]
	}
	slices.Reverse(files)
	for _, f := range files {
		if !slices.Contains(c.Files, f) {
			c.Files = append(c.Files, f)
		}
	}
	return c
}

// Options are what the hooks need from their caller.
type Options struct {
	Repo          func(dir string) *relay.RepoFacts                                 // repo facts of the session's cwd; nil for none
	CapsuleTokens int                                                               // budget of the SessionStart capsule
	Relayed       bool                                                              // the mod just replaced the context (level 0)
	Paused        func(dir string) bool                                             // aval relay pause holds for dir; nil for never
	Handoff       Handoffs                                                          // capsule kept across /clear; nil for none
	Plans         func(cwd string, files []string, branch string) []relay.PlanFacts // open plan tasks; nil for none
	Tree          func(cwd string, since time.Time) *relay.TreeFacts                // working tree facts since the session began; nil for none
	Now           time.Time                                                         // the clock of SessionEnd and SessionStart(clear)
	Owner         func() string                                                     // id of the Claude Code process, the handoff key beside cwd; nil or "" for cwd only
}

func (o Options) owner() string {
	if o.Owner == nil {
		return ""
	}
	return o.Owner()
}

// Handoffs keeps the capsule a cleared session leaves for the next one.
type Handoffs interface {
	PutHandoff(cwd, owner, text string, at time.Time) error
	TakeHandoff(cwd, owner string, now time.Time, maxAge time.Duration) (string, bool)
}

// handoffMaxAge is how long a stored capsule stays valid for the next session:
// until consumed, or a day.
const handoffMaxAge = 24 * time.Hour

// SessionEnd keeps the capsule, with the last exchange, for the session that
// follows a /clear. It stores nothing for other reasons or unusable input.
// It reads the transcript once and does no other work: SessionEnd has 1.5 s.
func SessionEnd(in io.Reader, opt Options) {
	var h input
	if opt.Handoff == nil || json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&h) != nil ||
		h.Reason != "clear" || h.TranscriptPath == "" || h.CWD == "" || (opt.Paused != nil && opt.Paused(h.CWD)) {
		return
	}
	f, err := os.Open(h.TranscriptPath)
	if err != nil {
		return
	}
	defer f.Close()
	t := fromTranscript(f)
	if t.capsule.FirstPrompt == "" {
		return
	}
	text := relay.Render(opt.withWork(opt.withRepo(t.capsule, h.CWD), h.CWD, t.since), opt.CapsuleTokens)
	if t.reply != "" {
		text += "\n\nLast reply before /clear:\n" + relay.Redact(relay.TruncateBytes(t.reply, maxReply))
	}
	_ = opt.Handoff.PutHandoff(h.CWD, opt.owner(), text, opt.Now)
}

// maxReply bounds the last assistant reply the handoff carries, in bytes.
const maxReply = relay.BoundaryReplyMax

// PreCompact prints the summariser instructions built from the transcript
// (plain stdout is the instruction channel of PreCompact). Input it cannot
// use prints nothing: the native compaction runs as it would.
func PreCompact(in io.Reader, out io.Writer, opt Options) {
	c, cwd, ok := read(in, "", opt)
	if !ok {
		return
	}
	_, _ = io.WriteString(out, relay.SteerText(opt.withRepo(c, cwd)))
}

// SessionStart prints the capsule as additionalContext after a compaction
// the mod did not answer. It prints nothing for other sources, after a mod
// relay, or on input it cannot use.
func SessionStart(in io.Reader, out io.Writer, opt Options) {
	if opt.Relayed {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(in, 1<<20))
	if err != nil {
		return
	}
	var h input
	if json.Unmarshal(raw, &h) == nil && h.Source == "clear" {
		if opt.Handoff == nil || h.CWD == "" {
			return
		}
		if text, ok := opt.Handoff.TakeHandoff(h.CWD, opt.owner(), opt.Now, handoffMaxAge); ok {
			context(out, text)
		}
		return
	}
	c, cwd, ok := read(bytes.NewReader(raw), "compact", opt)
	if !ok {
		return
	}
	context(out, relay.Render(opt.withRepo(c, cwd), opt.CapsuleTokens))
}

// context prints text as the additionalContext of SessionStart.
func context(out io.Writer, text string) {
	var o struct {
		H struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	o.H.Event, o.H.Context = "SessionStart", text
	b, err := json.Marshal(o)
	if err != nil {
		return
	}
	_, _ = out.Write(append(b, '\n'))
}

type input struct {
	CWD            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
	Source         string `json:"source"`
	Reason         string `json:"reason"`
}

// read decodes the hook input, checks its source when one is wanted and that
// the repo is not paused, and reads the transcript. A capsule with no request is not usable.
func read(in io.Reader, source string, opt Options) (relay.Capsule, string, bool) {
	var h input
	if json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&h) != nil || h.TranscriptPath == "" {
		return relay.Capsule{}, "", false
	}
	if source != "" && h.Source != source {
		return relay.Capsule{}, "", false
	}
	if opt.Paused != nil && opt.Paused(h.CWD) {
		return relay.Capsule{}, "", false
	}
	f, err := os.Open(h.TranscriptPath)
	if err != nil {
		return relay.Capsule{}, "", false
	}
	defer f.Close()
	c := FromTranscript(f)
	if c.FirstPrompt == "" {
		return relay.Capsule{}, "", false
	}
	return c, h.CWD, true
}

// withWork adds the open plan tasks and the working tree when the caller
// gives a source for them.
func (o Options) withWork(c relay.Capsule, cwd string, since time.Time) relay.Capsule {
	if o.Plans != nil && cwd != "" {
		branch := ""
		if c.Repo != nil {
			branch = c.Repo.Branch
		}
		c.Plans = o.Plans(cwd, c.Files, branch)
	}
	if o.Tree != nil && cwd != "" {
		c.Tree = o.Tree(cwd, since)
	}
	return c
}

// withRepo adds the repo facts of cwd when the caller gives a source.
func (o Options) withRepo(c relay.Capsule, cwd string) relay.Capsule {
	if o.Repo != nil && cwd != "" {
		c.Repo = o.Repo(cwd)
	}
	return c
}
