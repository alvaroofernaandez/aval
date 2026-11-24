package statefs

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// metricsFile is the append-only log of committed relays, one JSON object a line.
const metricsFile = "metrics.jsonl"

// Metric is one committed relay, a line of metrics.jsonl.
type Metric struct {
	At      time.Time `json:"at"`
	Session string    `json:"session_id"`
	Trigger string    `json:"trigger"`
	Action  string    `json:"action"` // the answer: replace, steer, new_session
	Source  string    `json:"source"` // where the token reading came from
	Before  int       `json:"tokens_before"`
	After   int       `json:"tokens_after"`
	Floor   int       `json:"floor"`
	Repo    string    `json:"repo,omitempty"` // the repository root the relay ran in
}

// AppendMetric adds m to metrics.jsonl, creating it with mode 0600. One
// write per line, so concurrent appenders never interleave inside a line.
func (s Store) AppendMetric(m Metric) error {
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, metricsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	if err := f.Close(); werr == nil {
		werr = err
	}
	return werr
}

// ReadMetrics returns every metric, oldest first; no file is no metrics. A
// line that does not parse (a torn last write) is skipped.
func (s Store) ReadMetrics() ([]Metric, error) {
	f, err := os.Open(filepath.Join(s.Dir, metricsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Metric
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m Metric
		if json.Unmarshal(sc.Bytes(), &m) == nil {
			out = append(out, m)
		}
	}
	return out, sc.Err()
}
