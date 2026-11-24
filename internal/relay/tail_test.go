package relay

import (
	"slices"
	"testing"
)

func msg(h string, tokens int) Message { return Message{Handle: h, Tokens: tokens} }

func use(h string, tokens int, id string) Message {
	return Message{Handle: h, Role: RoleAssistant, Tokens: tokens, ToolUses: []string{id}}
}

func result(h string, tokens int, id string) Message {
	return Message{Handle: h, Tokens: tokens, ToolResults: []string{id}}
}

func TestSelectTail(t *testing.T) {
	cases := []struct {
		name   string
		msgs   []Message
		budget int
		want   []string
	}{
		{"NeverSplitsToolPair",
			[]Message{msg("u1", 10), use("a1", 10, "x"), result("r1", 10, "x"), msg("a2", 10)}, 25,
			[]string{"a2"}},
		{"RespectsBudget",
			[]Message{msg("m1", 10), msg("m2", 10), msg("m3", 10), msg("m4", 10)}, 35,
			[]string{"m2", "m3", "m4"}},
		{"MidTurnKeepsInflightPair",
			[]Message{msg("u1", 10), use("a1", 10, "y"), result("r1", 10, "y"), use("a2", 10, "z")}, 15,
			[]string{"a2"}},
		{"OversizedLastPairKeptAlone",
			[]Message{msg("u1", 10), use("a1", 50, "z"), result("r1", 50, "z")}, 30,
			[]string{"a1", "r1"}},
		{"Empty", nil, 100, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SelectTail(c.msgs, c.budget); !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
