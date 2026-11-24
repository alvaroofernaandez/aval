// Package claudehook is the relay adapter for Claude Code's classic hooks,
// ladder level 2 of #65: when the relay mod does not run, PreCompact steers
// the native summary and SessionStart(compact) adds the capsule. The facts
// come from the session transcript, whose format Claude Code does not
// document; the shape read here is the one recorded on 2.1.331.
package claudehook

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// maxLine bounds one transcript line; a longer one is skipped.
const maxLine = 16 << 20

type entry struct {
	Type        string `json:"type"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	Summary     bool   `json:"isCompactSummary"` // seen on 2.1.331 on the native summary
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
	var f facts
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && !e.IsSidechain {
			f.add(e)
		}
	}
	return capsule(f.first, f.prompts, f.files)
}

// facts accumulates what the transcript says, in order.
type facts struct {
	first          string // from an earlier capsule
	prompts, files []string
}

func (f *facts) add(e entry) {
	text, blocks := content(e.Message.Content)
	switch {
	case e.Type == "assistant":
		f.files = append(f.files, edited(blocks)...)
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
	Repo          func(dir string) *relay.RepoFacts // repo facts of the session's cwd; nil for none
	CapsuleTokens int                               // budget of the SessionStart capsule
	Relayed       bool                              // the mod just replaced the context (level 0)
	Paused        func(dir string) bool             // aval relay pause holds for dir; nil for never
}

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
	c, cwd, ok := read(in, "compact", opt)
	if !ok {
		return
	}
	var o struct {
		H struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	o.H.Event, o.H.Context = "SessionStart", relay.Render(opt.withRepo(c, cwd), opt.CapsuleTokens)
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

// withRepo adds the repo facts of cwd when the caller gives a source.
func (o Options) withRepo(c relay.Capsule, cwd string) relay.Capsule {
	if o.Repo != nil && cwd != "" {
		c.Repo = o.Repo(cwd)
	}
	return c
}
