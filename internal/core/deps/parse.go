package deps

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

//go:embed deps.manifest
var embedded []byte

// personalSection holds bare names, one per line, instead of keys.
const personalSection = "personal"

var substitute = regexp.MustCompile(`^(none|#[0-9]+)$`)

// Embedded is the inventory compiled into the binary.
func Embedded() (Manifest, error) { return Parse(bytes.NewReader(embedded)) }

// ParseLayer reads a --layer value.
func ParseLayer(s string) (Layer, error) {
	if l := Layer(s); slices.Contains(Layers, l) {
		return l, nil
	}
	return "", fmt.Errorf("%w %q", ErrUnknownLayer, s)
}

// Parse reads a manifest: a [name] section per tool with layer, used_by,
// if_missing, native_substitute and probe, and a [personal] section of bare
// names. Lines starting with # are comments.
func Parse(r io.Reader) (Manifest, error) {
	p := parser{seen: map[string]bool{}}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := p.line(n, line); err != nil {
			return Manifest{}, err
		}
	}
	if err := sc.Err(); err != nil {
		return Manifest{}, err
	}
	if err := p.finish(); err != nil {
		return Manifest{}, err
	}
	return p.m, nil
}

// parser is the state of Parse between lines.
type parser struct {
	m       Manifest
	cur     *Entry
	curLine int
	section string
	seen    map[string]bool
}

// line reads one non-blank, non-comment line numbered n.
func (p *parser) line(n int, line string) error {
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		return p.header(n, strings.TrimSpace(line[1:len(line)-1]))
	}
	switch p.section {
	case "":
		return fmt.Errorf("deps.manifest:%d: %w", n, errOutsideSection)
	case personalSection:
		if p.seen[line] {
			return fmt.Errorf("deps.manifest:%d: %w %q", n, ErrDuplicate, line)
		}
		p.seen[line] = true
		p.m.Personal = append(p.m.Personal, line)
		return nil
	}
	if err := setField(p.cur, line); err != nil {
		return fmt.Errorf("deps.manifest:%d: [%s]: %w", n, p.cur.Name, err)
	}
	return nil
}

// header closes the open entry and starts the section name.
func (p *parser) header(n int, name string) error {
	if err := p.finish(); err != nil {
		return err
	}
	p.section = name
	if name == personalSection {
		return nil
	}
	if p.seen[name] {
		return fmt.Errorf("deps.manifest:%d: %w %q", n, ErrDuplicate, name)
	}
	p.seen[name] = true
	p.cur, p.curLine = &Entry{Name: name}, n
	return nil
}

// finish validates the open entry and adds it.
func (p *parser) finish() error {
	if p.cur == nil {
		return nil
	}
	if err := validate(*p.cur); err != nil {
		return fmt.Errorf("deps.manifest:%d: [%s]: %w", p.curLine, p.cur.Name, err)
	}
	p.m.Entries = append(p.m.Entries, *p.cur)
	p.cur = nil
	return nil
}

func setField(e *Entry, line string) error {
	k, v, ok := strings.Cut(line, "=")
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownKey, line)
	}
	k, v = strings.TrimSpace(k), strings.TrimSpace(v)
	switch k {
	case "layer":
		e.Layer = Layer(v)
	case "used_by":
		e.UsedBy = nil
		for _, u := range strings.Split(v, ",") {
			if u = strings.TrimSpace(u); u != "" {
				e.UsedBy = append(e.UsedBy, u)
			}
		}
	case "if_missing":
		e.IfMissing = IfMissing(v)
	case "native_substitute":
		e.NativeSubstitute = v
	case "probe":
		p, err := parseProbe(v)
		if err != nil {
			return err
		}
		e.Probe = p
	default:
		return fmt.Errorf("%w %q", ErrUnknownKey, k)
	}
	return nil
}

// parseProbe reads "bin <name> [version args]", "app <bundle path>" or
// "share <path under share/>". An app path may hold spaces.
func parseProbe(v string) (Probe, error) {
	kind, rest, _ := strings.Cut(v, " ")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return Probe{}, fmt.Errorf("%w: %q", ErrBadProbe, v)
	}
	switch ProbeKind(kind) {
	case ProbeBin:
		f := strings.Fields(rest)
		return Probe{Kind: ProbeBin, Target: f[0], Args: f[1:]}, nil
	case ProbeApp, ProbeShare:
		return Probe{Kind: ProbeKind(kind), Target: rest}, nil
	}
	return Probe{}, fmt.Errorf("%w: %q", ErrBadProbe, v)
}

func validate(e Entry) error {
	switch {
	case e.Layer == "":
		return fmt.Errorf("%w layer", ErrMissingField)
	case !slices.Contains(Layers, e.Layer):
		return fmt.Errorf("%w %q", ErrUnknownLayer, e.Layer)
	case len(e.UsedBy) == 0:
		return fmt.Errorf("%w used_by", ErrMissingField)
	case e.IfMissing == "":
		return fmt.Errorf("%w if_missing", ErrMissingField)
	case e.IfMissing != Blocks && e.IfMissing != Degrades && e.IfMissing != Ignored:
		return fmt.Errorf("%w %q", ErrUnknownIfMissing, e.IfMissing)
	case !substitute.MatchString(e.NativeSubstitute):
		return fmt.Errorf("%w, got %q", ErrNativeSubstitute, e.NativeSubstitute)
	case e.Probe.Target == "":
		return fmt.Errorf("%w probe", ErrMissingField)
	}
	return nil
}

// Filter keeps the entries of one layer, in order.
func Filter(es []Entry, l Layer) []Entry {
	var out []Entry
	for _, e := range es {
		if e.Layer == l {
			out = append(out, e)
		}
	}
	return out
}
