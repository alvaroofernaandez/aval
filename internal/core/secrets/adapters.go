package secrets

import (
	"errors"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// runner runs one external tool without a shell and returns its stdout. The
// secret value, when there is one, travels in stdin and never in the arguments.
type runner func(name string, args []string, stdin string) (string, error)

type exitError struct{ code int }

func (e *exitError) Error() string { return "exit status " + strconv.Itoa(e.code) }

func exitCodeOf(err error) (int, bool) {
	var e *exitError
	if errors.As(err, &e) {
		return e.code, true
	}
	return 0, false
}

// execRunner is the real runner. A missing binary is ErrUnavailable.
func execRunner(name string, args []string, stdin string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		return "", &exitError{code: ee.ExitCode()}
	case errors.Is(err, exec.ErrNotFound):
		return "", ErrUnavailable
	case err != nil:
		return "", ErrUnavailable
	}
	return string(out), nil
}

const service = "aval-secrets"

// macStore keeps secrets in the login keychain through `security`. The value
// is sent to `security -i` on stdin, so it never shows in the process list.
type macStore struct{ run runner }

func (s *macStore) Get(name string) (string, error) {
	out, err := s.run("security", []string{"find-generic-password", "-a", service, "-s", name, "-w"}, "")
	if code, ok := exitCodeOf(err); ok && code == 44 {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(out, "\n"), nil
}

func (s *macStore) Set(name, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return errors.New("a secret value cannot contain a line break on macOS")
	}
	q := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	_, err := s.run("security", []string{"-i"}, `add-generic-password -U -a `+service+` -s `+name+` -w "`+q+`"`+"\n")
	return err
}

func (s *macStore) Delete(name string) error {
	_, err := s.run("security", []string{"delete-generic-password", "-a", service, "-s", name}, "")
	if code, ok := exitCodeOf(err); ok && code == 44 {
		return ErrNotFound
	}
	return err
}

func (s *macStore) List() ([]string, error) { return nil, errors.New("use Indexed") }

// linuxStore keeps secrets in the Secret Service through `secret-tool`, which
// reads the value from stdin.
type linuxStore struct{ run runner }

func linuxAttrs(name string) []string { return []string{"service", service, "name", name} }

func (s *linuxStore) Get(name string) (string, error) {
	out, err := s.run("secret-tool", append([]string{"lookup"}, linuxAttrs(name)...), "")
	if code, ok := exitCodeOf(err); ok && code == 1 {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", ErrNotFound
	}
	return out, nil
}

func (s *linuxStore) Set(name, value string) error {
	args := append([]string{"store", "--label=aval:" + name}, linuxAttrs(name)...)
	_, err := s.run("secret-tool", args, value)
	return err
}

func (s *linuxStore) Delete(name string) error {
	if _, err := s.Get(name); err != nil {
		return err
	}
	_, err := s.run("secret-tool", append([]string{"clear"}, linuxAttrs(name)...), "")
	return err
}

func (s *linuxStore) List() ([]string, error) { return nil, errors.New("use Indexed") }

// indexName is the one entry that lists the names, since neither keychain
// tool lists by attribute without prompting. It holds names, never values.
const indexName = "aval.index"

type indexed struct{ Store }

// Indexed adds List to a Store that cannot enumerate: the names live in one
// reserved entry of the same store.
func Indexed(inner Store) Store { return &indexed{inner} }

func (s *indexed) names() ([]string, error) {
	v, err := s.Store.Get(indexName)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Fields(v), nil
}

func (s *indexed) save(names []string) error {
	slices.Sort(names)
	return s.Store.Set(indexName, strings.Join(slices.Compact(names), " "))
}

func (s *indexed) Set(name, value string) error {
	if name == indexName {
		return ErrInvalidName
	}
	if err := s.Store.Set(name, value); err != nil {
		return err
	}
	names, err := s.names()
	if err != nil {
		return err
	}
	return s.save(append(names, name))
}

func (s *indexed) Delete(name string) error {
	if name == indexName {
		return ErrInvalidName
	}
	if err := s.Store.Delete(name); err != nil {
		return err
	}
	names, err := s.names()
	if err != nil {
		return err
	}
	return s.save(slices.DeleteFunc(names, func(n string) bool { return n == name }))
}

func (s *indexed) Get(name string) (string, error) {
	if name == indexName {
		return "", ErrInvalidName
	}
	return s.Store.Get(name)
}

func (s *indexed) List() ([]string, error) { return s.names() }
