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

// handoffPath is the file for a cwd and the owner, the id of the Claude Code
// process that survives /clear. An empty owner is the old cwd-only key.
func (s Store) handoffPath(cwd, owner string) string {
	key := filepath.Clean(cwd)
	if owner != "" {
		key += "\x00" + owner
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.Dir, "handoff", hex.EncodeToString(sum[:8])+".json")
}

// PutHandoff stores text for the next session of owner started in cwd,
// replacing any earlier one.
func (s Store) PutHandoff(cwd, owner, text string, at time.Time) error {
	raw, err := json.Marshal(handoff{Cwd: filepath.Clean(cwd), Text: text, At: at})
	if err != nil {
		return err
	}
	return writeAtomic(s.handoffPath(cwd, owner), append(raw, '\n'))
}

// TakeHandoff returns the text stored for cwd and owner and deletes it, once.
// A non-empty owner falls back to the old cwd-only key. One older than maxAge
// is deleted and not returned.
func (s Store) TakeHandoff(cwd, owner string, now time.Time, maxAge time.Duration) (string, bool) {
	path := s.handoffPath(cwd, owner)
	raw, err := os.ReadFile(path)
	if err != nil && owner != "" {
		path = s.handoffPath(cwd, "")
		raw, err = os.ReadFile(path)
	}
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
