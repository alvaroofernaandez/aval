package usage

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// piHeader is the {"type":"session"} line that opens a Pi transcript.
type piHeader struct {
	ID       string `json:"id"`
	Cwd      string `json:"cwd"`
	Consumer string `json:"consumer"`
}

type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Cost       *struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

type piLine struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp json.RawMessage `json:"timestamp"`
	Model     string          `json:"model"`
	Usage     *piUsage        `json:"usage"`
	Message   *struct {
		Role      string          `json:"role"`
		Model     string          `json:"model"`
		Timestamp json.RawMessage `json:"timestamp"`
		Usage     *piUsage        `json:"usage"`
	} `json:"message"`
	// A standalone usage entry may also be flat.
	piUsage
}

var piHeaderMarker = []byte(`"type":"session"`)

func (x *fileContext) piLine(b []byte, seen map[uint64]struct{}, c *Corpus) {
	if bytes.Contains(b, piHeaderMarker) {
		var h struct {
			Type string `json:"type"`
			piHeader
		}
		if err := json.Unmarshal(b, &h); err != nil {
			c.Corrupt++
			return
		}
		if h.Type == "session" {
			x.pi = h.piHeader
		}
		return
	}
	if !bytes.Contains(b, usageMarker) {
		if b[0] != '{' || b[len(b)-1] != '}' {
			c.Corrupt++
		}
		return
	}
	var l piLine
	if err := json.Unmarshal(b, &l); err != nil {
		c.Corrupt++
		return
	}
	var u *piUsage
	var model string
	ts := l.Timestamp
	standalone := false
	switch {
	case l.Type == "message" && l.Message != nil && l.Message.Role == "assistant" && l.Message.Usage != nil:
		u, model = l.Message.Usage, l.Message.Model
		if len(ts) == 0 {
			ts = l.Message.Timestamp
		}
	case l.Type == "usage":
		standalone = true
		u, model = l.Usage, l.Model
		if u == nil {
			u = &l.piUsage
		}
	default:
		return
	}
	t, ok := piTime(ts)
	if !ok {
		c.Corrupt++
		return
	}
	e := Entry{
		Time: t, Source: "pi", Project: x.pi.Cwd, Session: x.pi.ID, Model: model, Consumer: Daemon,
		Tokens: Tokens{Input: u.Input, Output: u.Output, CacheWrite: u.CacheWrite, CacheRead: u.CacheRead},
	}
	switch x.pi.Consumer {
	case string(Interactive), string(Headless):
		e.Consumer = Consumer(x.pi.Consumer)
	}
	if u.Cost != nil {
		e.HasReported, e.Reported = true, Cost(math.Round(u.Cost.Total*1e8))
	}
	if e.Project == "" {
		e.Project = filepath.Base(filepath.Dir(x.path))
	}
	if e.Session == "" {
		e.Session = strings.TrimSuffix(filepath.Base(x.path), ".jsonl")
	}
	if standalone {
		x.piUsage = append(x.piUsage, e)
		return
	}
	x.piMessages++
	c.add(e, piKey(e), seen)
}

// piKey identifies a Pi response in case a forked session repeats it.
func piKey(e Entry) uint64 {
	h := fnv.New64a()
	h.Write([]byte(e.Session + "|" + e.Time.Format(time.RFC3339Nano) + "|" + e.Model + "|" + strconv.FormatInt(e.Tokens.Total(), 10)))
	return h.Sum64() | 1
}

// piTime reads an RFC 3339 string or Unix milliseconds.
func piTime(raw json.RawMessage) (time.Time, bool) {
	if len(raw) == 0 {
		return time.Time{}, false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		t, err := time.Parse(time.RFC3339Nano, s)
		return t, err == nil
	}
	var ms int64
	if json.Unmarshal(raw, &ms) == nil {
		return time.UnixMilli(ms).UTC(), true
	}
	return time.Time{}, false
}
