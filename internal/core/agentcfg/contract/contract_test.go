package contract_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/contract"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/fake"
)

// spy records what the suite reports instead of failing the real test.
type spy struct {
	t      *testing.T
	errors []string
}

func (s *spy) Helper()                        {}
func (s *spy) TempDir() string                { return s.t.TempDir() }
func (s *spy) Errorf(format string, a ...any) { s.errors = append(s.errors, fmt.Sprintf(format, a...)) }

var cfg = agentcfg.Config{
	Rules:  []agentcfg.Rule{{ID: "agent-rules", Body: "Strict TDD."}},
	Skills: []agentcfg.Skill{{Name: "strict-tdd", Hash: "sha256:00"}},
}

// broken wraps the fake and breaks one promise.
type broken struct {
	*fake.Adapter
	render func(agentcfg.Rendered) agentcfg.Rendered
	apply  func(target string) error
	calls  int
}

func (b *broken) Render(c agentcfg.Config) (agentcfg.Rendered, error) {
	r, err := b.Adapter.Render(c)
	b.calls++
	if b.render != nil {
		r = b.render(r)
	}
	return r, err
}

func (b *broken) Apply(target string, r agentcfg.Rendered, o agentcfg.ApplyOptions) ([]agentcfg.Change, error) {
	if b.apply != nil {
		if err := b.apply(target); err != nil {
			return nil, err
		}
	}
	return b.Adapter.Apply(target, r, o)
}

func TestSuiteCatchesEachBrokenPromise(t *testing.T) {
	var b *broken
	cases := map[string]*broken{
		"nondeterministic": {render: func(r agentcfg.Rendered) agentcfg.Rendered {
			r.Files[0].Data = []byte(fmt.Sprint(b.calls))
			return r
		}},
		"silent drop": {render: func(r agentcfg.Rendered) agentcfg.Rendered {
			r.Projected = r.Projected[1:]
			return r
		}},
		"escapes the target": {render: func(r agentcfg.Rendered) agentcfg.Rendered {
			r.Files[0].Path = "../outside/x"
			return r
		}},
		"writes beside the target": {apply: func(target string) error {
			return os.WriteFile(filepath.Join(filepath.Dir(target), "outside", "leak"), []byte("x"), 0o644)
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			c.Adapter = fake.New()
			b = c
			s := &spy{t: t}
			contract.Run(s, c, cfg)
			if len(s.errors) == 0 {
				t.Errorf("the suite passed an adapter that is %s", name)
			}
		})
	}
}

func TestSuiteReportsUnsupportedYetProjected(t *testing.T) {
	a := &fake.Adapter{Caps: agentcfg.Capabilities{agentcfg.CapRules: agentcfg.Full}}
	lying := &broken{Adapter: a, render: func(r agentcfg.Rendered) agentcfg.Rendered {
		r.Projected = append(r.Projected, "skill.strict-tdd")
		return r
	}}
	s := &spy{t: t}
	contract.Run(s, lying, cfg)
	if len(s.errors) == 0 {
		t.Error("the suite passed an adapter that projects an unsupported capability")
	}
}

func TestSuitePassesTheFake(t *testing.T) {
	s := &spy{t: t}
	contract.Run(s, fake.New(), cfg)
	if len(s.errors) != 0 {
		t.Errorf("the reference adapter failed the suite: %v", s.errors)
	}
}
