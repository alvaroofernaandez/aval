package relay

// Role is who wrote a message.
type Role int

const (
	RoleUser Role = iota
	RoleAssistant
)

// Message is one message of the conversation as the relay sees it: a stable
// handle the runtime gave it (S1 of spike #63), its size and its tool ids.
type Message struct {
	Handle      string
	Role        Role
	Tokens      int
	ToolUses    []string // ids of the tool calls it makes
	ToolResults []string // ids of the tool calls it answers
}
