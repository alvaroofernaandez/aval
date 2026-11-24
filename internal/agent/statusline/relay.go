package statusline

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Options are what the status line reads besides its input. The zero value
// shows no relay segment.
type Options struct {
	RelayDir string    // the relay state directory; "" when there is none
	Now      time.Time // the clock for the relay window
}

// relayWindow is how far back the relay segment counts. The mods API gives
// the relay no session id the status line also sees, so the segment counts
// the relays of the repository in this window, not of the session.
const relayWindow = 12 * time.Hour

// relayTail bounds how much of metrics.jsonl one refresh reads: the newest
// lines only, so the cost stays flat however long the log grows.
const relayTail = 64 << 10

// RenderWith is Render plus the relay segment, `relay N`, when the
// repository has relays in the window.
func RenderWith(r io.Reader, o Options) string {
	in := decode(r)
	line := render(in)
	proj := in.Workspace.ProjectDir
	if proj == "" {
		proj = in.Workspace.CurrentDir
	}
	if n := relaysSince(o, proj); n > 0 {
		line += separator + "relay " + strconv.Itoa(n)
	}
	return line
}

// relaysSince counts the relays of repo in metrics.jsonl within the window.
// The line format is pinned by cmd/aval/relay_contract_test.go; this package
// reads it itself so it imports nothing internal.
func relaysSince(o Options, repo string) int {
	if o.RelayDir == "" || repo == "" {
		return 0
	}
	f, err := os.Open(filepath.Join(o.RelayDir, "metrics.jsonl"))
	if err != nil {
		return 0
	}
	defer f.Close()
	var buf []byte
	if fi, err := f.Stat(); err == nil && fi.Size() > relayTail {
		buf = make([]byte, relayTail)
		n, _ := f.ReadAt(buf, fi.Size()-relayTail)
		buf = buf[:n]
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:] // the first line may be cut
		}
	} else if buf, err = io.ReadAll(f); err != nil {
		return 0
	}
	repoField, err := json.Marshal(filepath.Clean(repo))
	if err != nil {
		return 0
	}
	repoField = append([]byte(`"repo":`), repoField...)
	since := o.Now.Add(-relayWindow)
	lines := bytes.Split(buf, []byte{'\n'})
	count := 0
	for i := range lines {
		at, ok := metricTime(lines[i])
		if !ok {
			continue // a torn or foreign line
		}
		if !at.Before(since) && bytes.Contains(lines[i], repoField) {
			count++
		}
	}
	return count
}

// metricPrefix starts every metrics.jsonl line: statefs.Metric marshals its
// time first.
var metricPrefix = []byte(`{"at":"`)

// metricTime reads the time of a metrics.jsonl line without decoding it all.
func metricTime(line []byte) (time.Time, bool) {
	rest, ok := bytes.CutPrefix(line, metricPrefix)
	if !ok {
		return time.Time{}, false
	}
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, string(rest[:end]))
	return at, err == nil
}
