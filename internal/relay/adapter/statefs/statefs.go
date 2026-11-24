// Package statefs keeps the relay's state on disk: the journal of each relay,
// the per-session lock and the one-line session summary the status line reads.
// Every file is written atomically with mode 0600.
package statefs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/relay"
)

// Errors a caller tells apart with errors.Is.
var (
	ErrLocked       = errors.New("statefs: another relay holds the session lock")
	ErrBadSessionID = errors.New("statefs: session id is not a safe file name")
	ErrNoEntry      = errors.New("statefs: no journal entry")
)

// State of a journal entry.
type State string

const (
	Prepared  State = "prepared"  // the answer was given, not yet seen applied
	Committed State = "committed" // the next measure saw the new context
	Void      State = "void"      // the session resumed without it
)

// Entry is the journal of one relay.
type Entry struct {
	State        State     `json:"state"`
	Trigger      string    `json:"trigger"`
	TokensBefore int       `json:"tokens_before"`
	TokensAfter  int       `json:"tokens_after,omitempty"`
	At           time.Time `json:"at"`
}

// Summary is what the status line shows for a session.
type Summary struct {
	Relays  int
	Before  int
	After   int
	Floor   int
	Trigger string
}

// Store is the relay state under Dir. Alive reports whether a process with
// that pid and start time still runs; it is injected so tests need no process.
type Store struct {
	Dir   string
	PID   int
	Start int64
	Alive func(pid int, start int64) bool
}

// Prepare records a relay answer that the runtime has not applied yet.
func (s Store) Prepare(session string, e Entry) error {
	e.State = Prepared
	return s.writeEntry(session, e)
}

// Commit marks the prepared relay of session as applied.
func (s Store) Commit(session string, tokensAfter int) error {
	return s.update(session, func(e *Entry) { e.State, e.TokensAfter = Committed, tokensAfter })
}

// VoidPrepared voids a relay that was prepared but never applied: the
// session resumed from before it.
func (s Store) VoidPrepared(session string) error {
	return s.update(session, func(e *Entry) {
		if e.State == Prepared {
			e.State = Void
		}
	})
}

// Read returns the journal entry of session.
func (s Store) Read(session string) (Entry, error) {
	path, err := s.path("journal", session+".json")
	if err != nil {
		return Entry{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{}, ErrNoEntry
	}
	if err != nil {
		return Entry{}, err
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Entry{}, fmt.Errorf("statefs: journal of %s: %w", session, err)
	}
	return e, nil
}

func (s Store) update(session string, change func(*Entry)) error {
	e, err := s.Read(session)
	if err != nil {
		return err
	}
	change(&e)
	return s.writeEntry(session, e)
}

func (s Store) writeEntry(session string, e Entry) error {
	path, err := s.path("journal", session+".json")
	if err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return writeAtomic(path, append(raw, '\n'))
}

// Lock takes the relay lock of session; the returned func releases it. A
// lock whose holder no longer runs is reclaimed; a live one gives ErrLocked.
func (s Store) Lock(session string) (func(), error) {
	path, err := s.path("locks", session+".lock")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	owner := fmt.Sprintf("%d %d\n", s.PID, s.Start)
	for range 2 {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, werr := f.WriteString(owner)
			if cerr := f.Close(); werr == nil {
				werr = cerr
			}
			if werr != nil {
				os.Remove(path)
				return nil, werr
			}
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if s.holderAlive(path) {
			return nil, ErrLocked
		}
		os.Remove(path)
	}
	return nil, ErrLocked
}

func (s Store) holderAlive(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pid int
	var start int64
	if _, err := fmt.Sscan(string(raw), &pid, &start); err != nil {
		return false
	}
	return s.Alive == nil || s.Alive(pid, start) // unknown liveness never steals a lock
}

// summaryFormat is the one line the status line reads (pinned by tests).
const summaryFormat = "relays=%d before=%d after=%d floor=%d trigger=%s\n"

// WriteSummary replaces the summary of session.
func (s Store) WriteSummary(session string, sum Summary) error {
	path, err := s.path("sessions", session)
	if err != nil {
		return err
	}
	line := fmt.Sprintf(summaryFormat, sum.Relays, sum.Before, sum.After, sum.Floor, sum.Trigger)
	return writeAtomic(path, []byte(line))
}

// ReadSummary returns the summary of session.
func (s Store) ReadSummary(session string) (Summary, error) {
	path, err := s.path("sessions", session)
	if err != nil {
		return Summary{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	var sum Summary
	if _, err := fmt.Sscanf(string(raw), summaryFormat, &sum.Relays, &sum.Before, &sum.After, &sum.Floor, &sum.Trigger); err != nil {
		return Summary{}, fmt.Errorf("statefs: summary of %s: %w", session, err)
	}
	return sum, nil
}

// path joins dir and name under Dir after checking the session part of name
// is a plain file name.
func (s Store) path(dir, name string) (string, error) {
	base := strings.TrimSuffix(strings.TrimSuffix(name, ".json"), ".lock")
	if base == "" || base == "." || base == ".." || strings.ContainsAny(base, `/\`) {
		return "", ErrBadSessionID
	}
	return filepath.Join(s.Dir, dir, name), nil
}

// writeAtomic writes data to a temp file beside path and renames it, so a
// reader never sees a partial file.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if err := f.Chmod(0o600); werr == nil {
		werr = err
	}
	if err := f.Close(); werr == nil {
		werr = err
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		os.Remove(tmp)
	}
	return werr
}

// ReadChain returns the relay chain of session; none yet is an empty chain.
func (s Store) ReadChain(session string) (relay.Chain, error) {
	path, err := s.path("chains", session+".json")
	if err != nil {
		return relay.Chain{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return relay.Chain{}, nil
	}
	if err != nil {
		return relay.Chain{}, err
	}
	var c relay.Chain
	if err := json.Unmarshal(raw, &c.Links); err != nil {
		return relay.Chain{}, fmt.Errorf("statefs: chain of %s: %w", session, err)
	}
	return c, nil
}

// AppendLink adds one relay to the chain of session.
func (s Store) AppendLink(session string, l relay.Link) error {
	c, err := s.ReadChain(session)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(c.Record(l).Links)
	if err != nil {
		return err
	}
	path, err := s.path("chains", session+".json")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(raw, '\n'))
}

// Paused reports whether the relay is paused for the repository at root.
func (s Store) Paused(root string) bool {
	_, err := os.Stat(s.pausePath(root))
	return err == nil
}

// SetPaused pauses or resumes the relay for the repository at root. The flag
// file is named by a hash of root and holds root, for a person reading it.
func (s Store) SetPaused(root string, on bool) error {
	path := s.pausePath(root)
	if on {
		return writeAtomic(path, []byte(root+"\n"))
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s Store) pausePath(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return filepath.Join(s.Dir, "paused", hex.EncodeToString(sum[:8]))
}

// RelayedSince reports whether any session's journal holds a relay that was
// not voided and is no older than since. The classic SessionStart hook uses
// it: it cannot know the mod's session id, and a mod relay just happened
// when one is this recent.
func (s Store) RelayedSince(since time.Time) bool {
	ents, err := os.ReadDir(filepath.Join(s.Dir, "journal"))
	if err != nil {
		return false
	}
	for _, d := range ents {
		session, ok := strings.CutSuffix(d.Name(), ".json")
		if !ok {
			continue
		}
		if e, err := s.Read(session); err == nil && e.State != Void && !e.At.Before(since) {
			return true
		}
	}
	return false
}
