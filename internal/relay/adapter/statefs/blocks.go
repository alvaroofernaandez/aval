package statefs

import (
	"encoding/json"
	"os"
	"time"
)

// block is the prompt a hook last refused in a session.
type block struct {
	Key string    `json:"key"`
	At  time.Time `json:"at"`
}

// NoteBlock records that the hook refused the prompt named by key in
// session. One record per session: a later block replaces it.
func (s Store) NoteBlock(session, key string, at time.Time) error {
	path, err := s.path("blocks", session+".json")
	if err != nil {
		return err
	}
	raw, err := json.Marshal(block{Key: key, At: at})
	if err != nil {
		return err
	}
	return writeAtomic(path, append(raw, '\n'))
}

// TakeBlock reports whether the prompt named by key was refused in session
// no more than within before now, and forgets the record when it was: the
// person resending the same prompt gets through once.
func (s Store) TakeBlock(session, key string, within time.Duration, now time.Time) bool {
	path, err := s.path("blocks", session+".json")
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var b block
	if json.Unmarshal(raw, &b) != nil || b.Key != key || now.Sub(b.At) > within {
		return false
	}
	return os.Remove(path) == nil
}
