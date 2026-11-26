package statefs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// handoff is the capsule a session left for the next one in the same cwd.
type handoff struct {
	Cwd  string    `json:"cwd"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

func (s Store) handoffPath(cwd string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(cwd)))
	return filepath.Join(s.Dir, "handoff", hex.EncodeToString(sum[:8])+".json")
}

// PutHandoff stores text for the next session started in cwd, replacing
// any earlier one.
func (s Store) PutHandoff(cwd, text string, at time.Time) error {
	raw, err := json.Marshal(handoff{Cwd: filepath.Clean(cwd), Text: text, At: at})
	if err != nil {
		return err
	}
	return writeAtomic(s.handoffPath(cwd), append(raw, '\n'))
}

// TakeHandoff returns the text stored for cwd and deletes it, once. One
// older than maxAge is deleted and not returned.
func (s Store) TakeHandoff(cwd string, now time.Time, maxAge time.Duration) (string, bool) {
	path := s.handoffPath(cwd)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var h handoff
	err = json.Unmarshal(raw, &h)
	if rerr := os.Remove(path); rerr != nil || err != nil || now.Sub(h.At) > maxAge {
		return "", false
	}
	return h.Text, true
}
