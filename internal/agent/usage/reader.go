package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Root is a directory of transcripts and the runtime that wrote them.
type Root struct {
	Dir    string
	Source string // "claude" or "pi"
}

// Corpus is what Read found.
type Corpus struct {
	Entries []Entry
	// Files is the number of transcript files read.
	Files int
	// Corrupt is the number of lines that could not be read and were skipped.
	Corrupt int
}

// Read reads every *.jsonl under the roots. A root that does not exist is
// empty, not an error. Transcripts are only read, never written. A response
// that appears in more than one file counts once (see dedupeKey).
func Read(roots []Root) (Corpus, error) {
	var c Corpus
	seen := map[uint64]struct{}{}
	for _, r := range roots {
		var files []string
		err := filepath.WalkDir(r.Dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".jsonl") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return c, err
		}
		sort.Strings(files)
		for _, p := range files {
			f, err := os.Open(p)
			if err != nil {
				return c, err
			}
			c.Files++
			err = readFile(f, r, p, seen, &c)
			f.Close()
			if err != nil {
				return c, err
			}
		}
	}
	return c, nil
}

func readFile(f io.Reader, r Root, path string, seen map[uint64]struct{}, c *Corpus) error {
	br := bufio.NewReaderSize(f, 256<<10)
	ctx := fileContext{root: r, path: path}
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			ctx.line(bytes.TrimSpace(line), seen, c)
		}
		if err != nil {
			ctx.finish(seen, c)
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// fileContext is the state of the file being read: Pi writes the session's
// cwd, id and consumer once, in a header line, and the entries follow.
type fileContext struct {
	root Root
	path string
	pi   piHeader
	// piUsage holds standalone {"type":"usage"} entries; they count only when
	// the file has no assistant message carrying usage, so one response is
	// never counted twice.
	piUsage    []Entry
	piMessages int
}

func (x *fileContext) finish(seen map[uint64]struct{}, c *Corpus) {
	if x.piMessages == 0 {
		for _, e := range x.piUsage {
			c.add(e, piKey(e), seen)
		}
	}
}

func (c *Corpus) add(e Entry, key uint64, seen map[uint64]struct{}) {
	if key != 0 {
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
	}
	c.Entries = append(c.Entries, e)
}

var usageMarker = []byte(`"usage"`)

func (x *fileContext) line(b []byte, seen map[uint64]struct{}, c *Corpus) {
	if x.root.Source == "pi" {
		x.piLine(b, seen, c)
		return
	}
	// A line without usage costs one scan, not a parse. It is still corrupt
	// when it is not a whole JSON object (a truncated write).
	if !bytes.Contains(b, usageMarker) {
		if b[0] != '{' || b[len(b)-1] != '}' {
			c.Corrupt++
		}
		return
	}
	e, key, ok, err := parseClaude(b, x)
	if err != nil {
		c.Corrupt++
		return
	}
	if !ok {
		return
	}
	c.add(e, key, seen)
}

type claudeLine struct {
	Timestamp  string `json:"timestamp"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	Entrypoint string `json:"entrypoint"`
	RequestID  string `json:"requestId"`
	Message    *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// parseClaude reads one Claude Code line. ok is false for a line that is valid
// but carries no usage. key is the dedupe hash, 0 when the line has none.
func parseClaude(b []byte, x *fileContext) (e Entry, key uint64, ok bool, err error) {
	var l claudeLine
	if err = json.Unmarshal(b, &l); err != nil {
		return e, 0, false, err
	}
	if l.Message == nil || l.Message.Usage == nil || l.Message.Model == "<synthetic>" {
		return e, 0, false, nil
	}
	t, terr := time.Parse(time.RFC3339Nano, l.Timestamp)
	if terr != nil {
		return e, 0, false, terr
	}
	u := l.Message.Usage
	e = Entry{
		Time: t, Source: "claude", Project: l.Cwd, Session: l.SessionID, Model: l.Message.Model,
		Consumer: claudeConsumer(l.Entrypoint),
		Tokens:   Tokens{Input: u.Input, Output: u.Output, CacheWrite: u.CacheWrite, CacheRead: u.CacheRead},
	}
	if e.Project == "" {
		e.Project = filepath.Base(filepath.Dir(x.path))
	}
	if e.Session == "" {
		e.Session = strings.TrimSuffix(filepath.Base(x.path), ".jsonl")
	}
	// ccusage deduplicates on message id plus request id and only when both are present.
	if l.Message.ID != "" && l.RequestID != "" {
		h := fnv.New64a()
		h.Write([]byte(l.Message.ID + ":" + l.RequestID))
		key = h.Sum64()
	}
	return e, key, true, nil
}

// Roots are the transcript directories on this machine, resolved like ccusage
// does for Claude Code: CLAUDE_CONFIG_DIR (comma separated) when set, else
// $XDG_CONFIG_HOME/claude (default ~/.config/claude) and ~/.claude; each holds
// projects/. Pi: $PI_CODING_AGENT_DIR/sessions, default ~/.pi/agent/sessions.
func Roots(getenv func(string) string, home string) []Root {
	var out []Root
	if v := getenv("CLAUDE_CONFIG_DIR"); strings.TrimSpace(v) != "" {
		for _, d := range strings.Split(v, ",") {
			if d = strings.TrimSpace(d); d != "" {
				out = append(out, Root{filepath.Join(d, "projects"), "claude"})
			}
		}
	} else {
		xdg := getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		out = append(out,
			Root{filepath.Join(xdg, "claude", "projects"), "claude"},
			Root{filepath.Join(home, ".claude", "projects"), "claude"})
	}
	pi := getenv("PI_CODING_AGENT_DIR")
	if pi == "" {
		pi = filepath.Join(home, ".pi", "agent")
	}
	return append(out, Root{filepath.Join(pi, "sessions"), "pi"})
}

// claudeConsumer is the rule behind the consumer tag of a Claude Code entry:
// the transcript's entrypoint field is "cli" for the interactive terminal and
// "sdk-cli" for `claude -p` (and "sdk-ts" for the TypeScript SDK), so any
// entrypoint starting with "sdk" is headless and everything else interactive.
func claudeConsumer(entrypoint string) Consumer {
	if strings.HasPrefix(entrypoint, "sdk") {
		return Headless
	}
	return Interactive
}
