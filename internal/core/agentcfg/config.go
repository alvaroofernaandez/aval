// Package agentcfg is aval's single configuration model: the rules, skills,
// commands, agents, hooks, permissions and MCP servers the repo declares, read
// once, plus the Adapter interface that turns the model into the files of one
// agent runtime (Claude Code, Pi, ...).
//
// The model holds what is portable across runtimes. Settings only one runtime
// understands (Claude Code's status line, plugins, themes) stay in Native,
// keyed by adapter name, and every other adapter ignores them by definition.
//
// Every portable item is an Element with an HCS-style dotted id
// ("skill.strict-tdd", "hook.PreToolUse.0") and a content hash. An adapter
// must account for each one: it is either projected into the runtime's files
// or reported in a warning, never dropped in silence.
package agentcfg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

// Capability is one kind of thing the model describes.
type Capability string

// The capabilities of the model, in rendering order.
const (
	CapRules       Capability = "rules"
	CapSkills      Capability = "skills"
	CapCommands    Capability = "commands"
	CapAgents      Capability = "agents"
	CapHooks       Capability = "hooks"
	CapPermissions Capability = "permissions"
	CapMCP         Capability = "mcp"
)

// AllCapabilities lists every capability of the model.
var AllCapabilities = []Capability{CapRules, CapSkills, CapCommands, CapAgents, CapHooks, CapPermissions, CapMCP}

// Config is the whole model.
type Config struct {
	Rules       []Rule
	Skills      []Skill
	Commands    []Command
	Agents      []Agent
	Hooks       []HookGroup
	Permissions Permissions
	MCP         MCP
	// Native holds runtime-specific settings by adapter name. It is not part
	// of the portable model and carries no elements.
	Native map[string]map[string]any
}

// Rule is a block of always-on instructions.
type Rule struct {
	ID   string // short name: "agent-rules"
	Body string
}

// Skill is an Agent Skills directory (SKILL.md plus its files).
type Skill struct {
	Name        string
	Description string
	// Dir is the absolute source directory; Files lists its regular files,
	// slash-separated and relative to Dir, sorted.
	Dir   string
	Files []string
	// Hash is the content hash of every file in the directory.
	Hash string
}

// Command is a prompt template invoked by name (a slash command).
type Command struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string
	// Raw is the whole source file, front matter included.
	Raw string
}

// Agent is a sub-agent definition.
type Agent struct {
	Name        string
	Description string
	Model       string
	// Skills are skills the agent preloads; each must exist in Config.Skills.
	Skills []string
	Body   string
	Raw    string
}

// HookGroup is one matcher of one lifecycle event and the handlers it runs.
type HookGroup struct {
	Event    string
	Matcher  string
	Handlers []HookHandler
}

// HookHandler is one handler of a hook group.
type HookHandler struct {
	Type    string // "command"
	Command string
	Timeout int // seconds, 0 when unset
	// Extra keeps the handler fields the model does not name.
	Extra map[string]any
}

// Permissions is the tool permission policy. Deny is a safety floor.
type Permissions struct {
	Allow []string
	Deny  []string
	Ask   []string
	// DefaultMode is the runtime's default permission mode, "" when unset.
	DefaultMode string
}

// MCP lists the MCP servers to configure and the ones to keep off.
type MCP struct {
	Servers []MCPServer
	Denied  []string
}

// MCPServer is one MCP server.
type MCPServer struct {
	Name    string
	Command string
	Args    []string
	URL     string
}

// Element is one portable item of the model.
type Element struct {
	ID         string
	Capability Capability
	Hash       string
}

// Elements lists every portable item with its id and content hash, in a
// stable order.
func (c Config) Elements() []Element {
	var out []Element
	add := func(capability Capability, id string, v any) {
		out = append(out, Element{ID: id, Capability: capability, Hash: hashOf(v)})
	}
	for _, r := range c.Rules {
		add(CapRules, "rule."+r.ID, r)
	}
	for _, s := range c.Skills {
		add(CapSkills, "skill."+s.Name, s.Hash)
	}
	for _, cm := range c.Commands {
		add(CapCommands, "command."+cm.Name, cm)
	}
	for _, a := range c.Agents {
		add(CapAgents, "agent."+a.Name, a)
	}
	perEvent := map[string]int{}
	for _, h := range c.Hooks {
		add(CapHooks, HookID(h.Event, perEvent[h.Event]), h)
		perEvent[h.Event]++
	}
	for _, p := range c.Permissions.Deny {
		add(CapPermissions, "permission.deny."+p, p)
	}
	for _, p := range c.Permissions.Allow {
		add(CapPermissions, "permission.allow."+p, p)
	}
	for _, p := range c.Permissions.Ask {
		add(CapPermissions, "permission.ask."+p, p)
	}
	if c.Permissions.DefaultMode != "" {
		add(CapPermissions, "permission.mode", c.Permissions.DefaultMode)
	}
	for _, s := range c.MCP.Servers {
		add(CapMCP, "mcp."+s.Name, s)
	}
	for _, d := range c.MCP.Denied {
		add(CapMCP, "mcp.denied."+d, d)
	}
	return out
}

// HookID is the element id of the i-th hook group, counted per event.
func HookID(event string, i int) string {
	return "hook." + event + "." + strconv.Itoa(i)
}

// Hash is the content hash of the whole model, Native included: two configs
// with the same hash render the same files.
func (c Config) Hash() string {
	type nativeEntry struct {
		Adapter  string
		Settings map[string]any
	}
	var native []nativeEntry
	for k, v := range c.Native {
		native = append(native, nativeEntry{k, v})
	}
	sort.Slice(native, func(i, j int) bool { return native[i].Adapter < native[j].Adapter })
	return hashOf(struct {
		Elements []Element
		Native   []nativeEntry
	}{c.Elements(), native})
}

// hashOf is the sha256 of v's JSON encoding. encoding/json sorts map keys, so
// the encoding is canonical for the types the model uses.
func hashOf(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	sum := sha256.Sum256(buf.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:])
}
