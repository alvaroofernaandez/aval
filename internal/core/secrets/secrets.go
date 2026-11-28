// Package secrets keeps tokens in the operating system keychain behind a Store
// port. A value is never printed, logged or put in an error: callers name a
// secret by NAME only.
package secrets

import (
	"errors"
	"maps"
	"slices"
	"sync"
)

var (
	// ErrNotFound: no secret with that name.
	ErrNotFound = errors.New("secret not found")
	// ErrInvalidName: the name is empty or has characters outside [A-Za-z0-9_.-].
	ErrInvalidName = errors.New("invalid secret name")
	// ErrUnavailable: the keychain cannot be reached; there is no plaintext fallback.
	ErrUnavailable = errors.New("keychain unavailable")
)

// Store is the port every adapter implements.
type Store interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
	List() ([]string, error)
}

// ValidName reports whether name is a legal secret name: letters, digits, '_',
// '.' and '-', not starting with '-'. It is also a legal environment variable
// name for the names aval itself generates.
func ValidName(name string) bool {
	if name == "" || name[0] == '-' {
		return false
	}
	for _, r := range name {
		ok := r == '_' || r == '.' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !ok {
			return false
		}
	}
	return true
}

// Memory is an in-process Store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
}

// NewMemory returns an empty Memory store.
func NewMemory() *Memory { return &Memory{m: map[string]string{}} }

func (s *Memory) Get(name string) (string, error) {
	if !ValidName(name) {
		return "", ErrInvalidName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *Memory) Set(name, value string) error {
	if !ValidName(name) {
		return ErrInvalidName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[name] = value
	return nil
}

func (s *Memory) Delete(name string) error {
	if !ValidName(name) {
		return ErrInvalidName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[name]; !ok {
		return ErrNotFound
	}
	delete(s.m, name)
	return nil
}

func (s *Memory) List() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Sorted(maps.Keys(s.m)), nil
}

// ChildEnv returns base plus NAME=value for every name, read from s. base is
// not modified; the value lives only in the returned slice.
func ChildEnv(s Store, names []string, base []string) ([]string, error) {
	env := slices.Clone(base)
	for _, n := range names {
		v, err := s.Get(n)
		if err != nil {
			return nil, err
		}
		env = append(env, n+"="+v)
	}
	return env, nil
}
