package generation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/backup"
)

// ErrConsumerName means a consumer name is empty or not a plain name.
var ErrConsumerName = errors.New("generation: bad consumer name")

// consumersRoot is the home of every consumer: Pen, the factories.
func consumersRoot(home string) string { return filepath.Join(home, ".aval", "consumers") }

// ConsumerDir is where consumer name keeps its adapter trees and its pin.
func ConsumerDir(home, name string) string { return filepath.Join(consumersRoot(home), name) }

// pinFile names the generation a consumer boots from, in its dir.
const pinFile = "generation"

func validName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// ApplyConsumer applies adapter a's tree of generation id into the
// consumer's own writable dir and pins the consumer to id, so a runtime that
// keeps state in its config dir does not write into the immutable store. It
// returns that dir. Files the consumer's previous generation rendered and
// this one does not are removed; anything else in the dir is left alone.
func ApplyConsumer(home, name, id string, a agentcfg.Adapter) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("%w: %q", ErrConsumerName, name)
	}
	if err := Verify(home, id); err != nil {
		return "", err
	}
	g, _ := Read(home, id)
	var modes map[string]string
	for _, info := range g.Adapters {
		if info.Name == a.Name() {
			modes = info.Modes
		}
	}
	r, err := storedRendering(Dir(home, id, a.Name()), modes)
	if err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	target := filepath.Join(ConsumerDir(home, name), a.Name())
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	if err := dropStale(home, name, a.Name(), target, r); err != nil {
		return "", err
	}
	if _, err := a.Apply(target, r, agentcfg.ApplyOptions{}); err != nil {
		return "", err
	}
	if err := backup.WriteFileAtomic(filepath.Join(ConsumerDir(home, name), pinFile), []byte(id+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	return target, nil
}

// dropStale removes what the consumer's pinned generation rendered for
// adapter and r does not.
func dropStale(home, name, adapter, target string, r agentcfg.Rendered) error {
	old, err := ConsumerPin(home, name)
	if err != nil || old == "" {
		return nil
	}
	prev, err := storedRendering(Dir(home, old, adapter), nil)
	if err != nil {
		return nil // the old generation is gone: nothing to compare with
	}
	keep := map[string]bool{}
	for _, f := range r.Files {
		keep[f.Path] = true
	}
	for _, f := range prev.Files {
		if !keep[f.Path] {
			if err := os.Remove(filepath.Join(target, filepath.FromSlash(f.Path))); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("generation: %w", err)
			}
		}
	}
	return nil
}

// ConsumerPin is the generation consumer name is pinned to, "" when none.
func ConsumerPin(home, name string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("%w: %q", ErrConsumerName, name)
	}
	b, err := os.ReadFile(filepath.Join(ConsumerDir(home, name), pinFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	if id := strings.TrimSpace(string(b)); validID(id) {
		return id, nil
	}
	return "", nil
}

// consumerPins is every generation some consumer is pinned to.
func consumerPins(home string) []string {
	ents, _ := os.ReadDir(consumersRoot(home))
	var out []string
	for _, e := range ents {
		if id, err := ConsumerPin(home, e.Name()); err == nil && id != "" {
			out = append(out, id)
		}
	}
	return out
}
