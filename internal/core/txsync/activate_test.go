package txsync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
	"github.com/alvaroofernaandez/aval/internal/core/agentcfg/claude"
	"github.com/alvaroofernaandez/aval/internal/core/backup"
	"github.com/alvaroofernaandez/aval/internal/core/generation"
)

// genWith builds a generation whose Claude Code settings are the repo's
// with env AVAL_GEN=tag, and with an extra Stop hook command when hook != "".
func genWith(t *testing.T, opt Options, tag, hook string) string {
	t.Helper()
	cfg, err := agentcfg.LoadSettings(filepath.Join(opt.Repo, "config/claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	native := cfg.Native[agentcfg.ClaudeCode]
	native["env"] = map[string]any{"AVAL_GEN": tag}
	cfg = agentcfg.FromSettings(native)
	if hook != "" {
		cfg.Hooks = append(cfg.Hooks, agentcfg.HookGroup{Event: "Stop", Handlers: []agentcfg.HookHandler{{Type: "command", Command: hook}}})
	}
	g, _, err := generation.Build(generation.Options{
		Home: opt.Home, Config: cfg, Now: opt.Now,
		Adapters: []agentcfg.Adapter{claude.New(claude.Options{Home: opt.Home, GOOS: opt.GOOS})},
	})
	if err != nil {
		t.Fatal(err)
	}
	return g.ID
}

func activate(t *testing.T, opt Options, id string) {
	t.Helper()
	opt.Generation = id
	if _, err := Run(opt); err != nil {
		t.Fatalf("activate %s: %v", id, err)
	}
}

func current(t *testing.T, opt Options) string {
	t.Helper()
	id, err := generation.Current(opt.Home)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Activation goes through the sync transaction with the generation's
// settings (owner: G5 = A), and a rollback to a generation is byte-identical
// to its first activation.
func TestActivateMakesAGenerationLiveAndRollbackIsByteIdentical(t *testing.T) {
	opt := env(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	put(t, live, `{"model":"sonnet"}`, 0o600)
	a, b := genWith(t, opt, "a", ""), genWith(t, opt, "b", "")

	activate(t, opt, a)
	first := read(t, live)
	if !strings.Contains(first, `"AVAL_GEN": "a"`) || !strings.Contains(first, `"model": "sonnet"`) || current(t, opt) != a {
		t.Fatalf("after activating a: current %s, live %s", current(t, opt), first)
	}
	activate(t, opt, b)
	if !strings.Contains(read(t, live), `"AVAL_GEN": "b"`) || current(t, opt) != b {
		t.Fatalf("after activating b: current %s, live %s", current(t, opt), read(t, live))
	}
	activate(t, opt, a)
	if got := read(t, live); got != first || current(t, opt) != a {
		t.Errorf("rollback to a: current %s, live differs from its first activation:\n%s\nwant\n%s", current(t, opt), got, first)
	}
}

func TestAFailingCheckAfterActivationKeepsThePreviousGeneration(t *testing.T) {
	opt := env(t)
	live := filepath.Join(opt.Home, ".claude/settings.json")
	put(t, live, `{"model":"sonnet"}`, 0o600)
	a := genWith(t, opt, "a", "")
	bad := genWith(t, opt, "bad", filepath.ToSlash(filepath.Join(opt.Home, "nope", "missing.sh")))
	activate(t, opt, a)
	before := read(t, live)

	opt.Generation = bad
	out, err := Run(opt)
	if err == nil || !failedCheck(out, "hooks-resolve") || !strings.Contains(err.Error(), "hooks-resolve") {
		t.Fatalf("activating a generation that fails hooks-resolve: %v, %+v", err, out)
	}
	if got := read(t, live); got != before || current(t, opt) != a {
		t.Errorf("after the failed activation: current %s, live %s; want %s and %s", current(t, opt), got, a, before)
	}
}

func TestActivatingAnUnknownGenerationWritesNothing(t *testing.T) {
	opt := env(t)
	opt.Generation = "gen-0000000000000000"
	if _, err := Run(opt); !errors.Is(err, generation.ErrUnknown) {
		t.Errorf("err = %v, want ErrUnknown", err)
	}
	if _, err := os.Stat(filepath.Join(opt.Home, ".claude")); err == nil {
		t.Error("an unknown generation wrote ~/.claude")
	}
}

// A crash at any point of an activation (the process dies: no rollback, no
// commit) is undone by the next transaction's recovery: the files and the
// current pointer are the old generation's again, never a mix.
func TestAnInterruptedActivationRecoversTheOldGeneration(t *testing.T) {
	for _, stage := range []string{stageApplied, stageSwitched} {
		t.Run(stage, func(t *testing.T) {
			opt := env(t)
			live := filepath.Join(opt.Home, ".claude/settings.json")
			put(t, live, `{"model":"sonnet"}`, 0o600)
			a, b := genWith(t, opt, "a", ""), genWith(t, opt, "b", "")
			activate(t, opt, a)
			before := read(t, live)

			crashAt(t, stage)
			func() {
				defer func() { _ = recover() }()
				opt.Generation = b
				_, _ = Run(opt)
				t.Fatal("the activation did not reach the crash")
			}()
			crash = func(string) {}

			pend, _ := backup.NewStore(opt.Home).Pending()
			if len(pend) != 1 {
				t.Fatalf("%d pending runs after the crash, want 1", len(pend))
			}
			recovered, err := Recover(opt)
			if err != nil || len(recovered) != 1 {
				t.Fatalf("Recover = %v, %v", recovered, err)
			}
			if got := read(t, live); got != before || current(t, opt) != a {
				t.Errorf("after recovery: current %s, live %s; want %s and %s", current(t, opt), got, a, before)
			}
		})
	}
}

func crashAt(t *testing.T, stage string) {
	t.Helper()
	crash = func(s string) {
		if s == stage {
			panic("crash at " + s)
		}
	}
	t.Cleanup(func() { crash = func(string) {} })
}
