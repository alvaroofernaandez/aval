package generation

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HistoryFile is the append-only record of activations, inside Root: one
// "<id> <RFC3339>" line per activation, oldest first.
const HistoryFile = "history"

// ErrNoPrevious means no generation was active before the current one.
var ErrNoPrevious = errors.New("generation: no earlier activation to roll back to")

// AppendHistory records that id was activated at.
func AppendHistory(home, id string, at time.Time) error {
	if !validID(id) {
		return fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	f, err := os.OpenFile(filepath.Join(Root(home), HistoryFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	_, err = fmt.Fprintf(f, "%s %s\n", id, at.UTC().Format(time.RFC3339))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	return nil
}

// Previous is the generation active before the last activation: the latest
// history entry that differs from the last one.
func Previous(home string) (string, error) {
	f, err := os.Open(filepath.Join(Root(home), HistoryFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoPrevious
	}
	if err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	defer f.Close()
	var ids []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if id, _, _ := strings.Cut(sc.Text(), " "); validID(id) {
			ids = append(ids, id)
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	for i := len(ids) - 2; i >= 0; i-- {
		if ids[i] != ids[len(ids)-1] {
			return ids[i], nil
		}
	}
	return "", ErrNoPrevious
}
