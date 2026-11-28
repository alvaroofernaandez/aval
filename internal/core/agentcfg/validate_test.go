package agentcfg_test

import (
	"errors"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

func valid() agentcfg.Config {
	return agentcfg.Config{
		Skills: []agentcfg.Skill{{Name: "strict-tdd", Description: "RED first"}},
		Agents: []agentcfg.Agent{{Name: "planner", Description: "Plans", Skills: []string{"strict-tdd"}}},
		Hooks:  []agentcfg.HookGroup{{Event: "Stop", Handlers: []agentcfg.HookHandler{{Type: "command", Command: "aval hook done"}}}},
	}
}

func TestValidateAcceptsAConsistentModel(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateRejectsBrokenReferences(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(*agentcfg.Config)
		want   error
		id     string
	}{
		{"agent preloads a missing skill", func(c *agentcfg.Config) { c.Agents[0].Skills = []string{"ghost"} }, agentcfg.ErrUnknownSkill, "agent.planner"},
		{"hook without a command", func(c *agentcfg.Config) { c.Hooks[0].Handlers[0].Command = " " }, agentcfg.ErrHookWithoutCommand, "hook.Stop.0"},
		{"skill defined twice", func(c *agentcfg.Config) { c.Skills = append(c.Skills, c.Skills[0]) }, agentcfg.ErrDuplicate, "skill.strict-tdd"},
		{"skill without description", func(c *agentcfg.Config) { c.Skills[0].Description = "" }, agentcfg.ErrNoDescription, "skill.strict-tdd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.break_(&c)
			err := c.Validate()
			if !errors.Is(err, tc.want) {
				t.Fatalf("Validate = %v, want %v", err, tc.want)
			}
			var ve *agentcfg.ValidationError
			if !errors.As(err, &ve) || ve.ID != tc.id {
				t.Errorf("the error names %+v, want element %s", ve, tc.id)
			}
		})
	}
}
