// Package pi is the Pi coding agent adapter of the agentcfg model. It renders
// a whole Pi agent directory (the PI_CODING_AGENT_DIR of a run), so a run reads
// only what aval rendered and never the ambient ~/.pi/agent:
//
//	AGENTS.md               the shared agent-rules block, nothing Claude-specific
//	settings.json           Native["pi"], or {}
//	mcp.json                {"mcpServers": ...}, denied servers left out
//	skills/<name>/...       each skill directory, byte for byte
//	prompts/<command>.md    each command, with the front matter Pi reads
//	auth.json               Apply only: a link to the owner's subscription
//	                        login, never a copy, never in Files
//
// Pi has no subagents and no permission model, and hooks exist only as
// TypeScript extensions: those elements are reported, never rendered.
package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

// Name is the adapter's id and the key of its native settings.
const Name = "pi"

// ErrAPIKey means a rendering would carry an API key or an API-billed auth
// setting: every Pi run aval prepares bills the owner's Claude subscription.
var ErrAPIKey = errors.New("pi: an API key in the rendering; Pi runs on the Claude subscription only")

// apiEnv are the variables through which Pi would bill an API account or use a
// credential other than the subscription login.
var apiEnv = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_OAUTH_TOKEN"}

// promptKeys are the front-matter keys a Pi prompt template reads.
var promptKeys = []string{"description", "argument-hint"}

// Adapter renders a Pi agent directory.
type Adapter struct {
	// Login is the owner's Pi subscription login that Apply links into the
	// target; empty links nothing.
	Login string
}

// ForHome returns the adapter that links the login of the Pi install in home.
func ForHome(home string) *Adapter {
	return &Adapter{Login: filepath.Join(home, ".pi", "agent", "auth.json")}
}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

// Name implements agentcfg.Adapter.
func (a *Adapter) Name() string { return Name }

// Capabilities implements agentcfg.Adapter.
func (a *Adapter) Capabilities() agentcfg.Capabilities {
	return agentcfg.Capabilities{
		agentcfg.CapRules:       agentcfg.Full,
		agentcfg.CapSkills:      agentcfg.Full,
		agentcfg.CapCommands:    agentcfg.Partial,
		agentcfg.CapAgents:      agentcfg.Unsupported,
		agentcfg.CapHooks:       agentcfg.Unsupported,
		agentcfg.CapPermissions: agentcfg.Unsupported,
		agentcfg.CapMCP:         agentcfg.Full,
	}
}

// Render implements agentcfg.Adapter.
func (a *Adapter) Render(cfg agentcfg.Config) (agentcfg.Rendered, error) {
	var r agentcfg.Rendered
	if err := noAPIKey(cfg); err != nil {
		return r, err
	}
	settings, err := encode(nativeSettings(cfg))
	if err != nil {
		return r, err
	}
	mcp, err := encode(map[string]any{"mcpServers": servers(cfg.MCP)})
	if err != nil {
		return r, err
	}
	r.Files = append(r.Files, rules(cfg, &r), agentcfg.File{Path: "mcp.json", Data: mcp})
	r.Files = append(r.Files, prompts(cfg, &r)...)
	r.Files = append(r.Files, agentcfg.File{Path: "settings.json", Data: settings})
	skills, err := skillFiles(cfg, &r)
	if err != nil {
		return r, err
	}
	r.Files = append(r.Files, skills...)
	mcpOutcomes(cfg, &r)
	lost(cfg, &r)
	return r, nil
}

// Diff implements agentcfg.Adapter.
func (a *Adapter) Diff(target string, r agentcfg.Rendered) ([]agentcfg.Change, error) {
	return agentcfg.DiffFiles(target, r)
}

// Apply implements agentcfg.Adapter.
// After the files, it links <target>/auth.json to the login, when there is one.
func (a *Adapter) Apply(target string, r agentcfg.Rendered, opt agentcfg.ApplyOptions) ([]agentcfg.Change, error) {
	changes, err := agentcfg.ApplyFiles(target, r, opt)
	if err != nil || opt.DryRun {
		return changes, err
	}
	return changes, a.linkLogin(target)
}

// linkLogin points <target>/auth.json at the owner's login, so the isolated dir
// bills the subscription without a copy of the credential. A missing login
// links nothing (Pi asks for /login), and a real file already there is kept.
func (a *Adapter) linkLogin(target string) error {
	if a.Login == "" {
		return nil
	}
	if _, err := os.Stat(a.Login); err != nil {
		return nil
	}
	dst := filepath.Join(target, "auth.json")
	if cur, err := os.Readlink(dst); err == nil {
		if cur == a.Login {
			return nil
		}
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("pi: login link: %w", err)
		}
	} else if _, err := os.Lstat(dst); err == nil {
		return nil
	}
	if err := os.Symlink(a.Login, dst); err != nil {
		return fmt.Errorf("pi: login link: %w", err)
	}
	return nil
}

// Env returns base prepared for a Pi run on agentDir: PI_CODING_AGENT_DIR
// points at the rendered directory, and every API-billed credential is
// removed, so Pi falls back to the subscription login.
func Env(base []string, agentDir string) []string {
	out := make([]string, 0, len(base)+1)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if k == "PI_CODING_AGENT_DIR" || slices.Contains(apiEnv, k) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "PI_CODING_AGENT_DIR="+agentDir)
}

func rules(cfg agentcfg.Config, r *agentcfg.Rendered) agentcfg.File {
	var b strings.Builder
	for i, rule := range cfg.Rules {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(rule.Body))
		r.Projected = append(r.Projected, "rule."+rule.ID)
	}
	b.WriteString("\n")
	return agentcfg.File{Path: "AGENTS.md", Data: []byte(b.String())}
}

func prompts(cfg agentcfg.Config, r *agentcfg.Rendered) []agentcfg.File {
	var out []agentcfg.File
	for _, c := range cfg.Commands {
		id := "command." + c.Name
		var head strings.Builder
		if c.Description != "" {
			fmt.Fprintf(&head, "description: %s\n", c.Description)
		}
		if c.ArgumentHint != "" {
			fmt.Fprintf(&head, "argument-hint: %s\n", c.ArgumentHint)
		}
		data := c.Body
		if head.Len() > 0 {
			data = "---\n" + head.String() + "---\n" + c.Body
		}
		out = append(out, agentcfg.File{Path: "prompts/" + c.Name + ".md", Data: []byte(data)})
		r.Projected = append(r.Projected, id)
		if dropped := droppedKeys(c.Raw); len(dropped) > 0 {
			r.Warnings = append(r.Warnings, agentcfg.Warning{ID: id, Capability: agentcfg.CapCommands,
				Reason: "Pi prompts read only description and argument-hint; dropped " + strings.Join(dropped, ", ")})
		}
	}
	return out
}

// droppedKeys lists the top-level front-matter keys of raw a Pi prompt ignores.
func droppedKeys(raw string) []string {
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		return nil
	}
	head, _, _ := strings.Cut(rest, "\n---")
	var out []string
	for _, ln := range strings.Split(head, "\n") {
		if ln == "" || ln[0] == ' ' || ln[0] == '\t' || ln[0] == '-' {
			continue
		}
		k, _, _ := strings.Cut(ln, ":")
		if k = strings.TrimSpace(k); k != "" && !slices.Contains(promptKeys, k) {
			out = append(out, k)
		}
	}
	return out
}

func skillFiles(cfg agentcfg.Config, r *agentcfg.Rendered) ([]agentcfg.File, error) {
	var out []agentcfg.File
	for _, s := range cfg.Skills {
		for _, f := range s.Files {
			b, err := os.ReadFile(filepath.Join(s.Dir, filepath.FromSlash(f)))
			if err != nil {
				return nil, fmt.Errorf("pi: skill %s: %w", s.Name, err)
			}
			out = append(out, agentcfg.File{Path: "skills/" + s.Name + "/" + f, Data: b})
		}
		r.Projected = append(r.Projected, "skill."+s.Name)
	}
	return out, nil
}

func servers(m agentcfg.MCP) map[string]any {
	out := map[string]any{}
	for _, s := range m.Servers {
		if slices.Contains(m.Denied, s.Name) {
			continue
		}
		e := map[string]any{}
		if s.Command != "" {
			e["command"] = s.Command
		}
		if len(s.Args) > 0 {
			e["args"] = s.Args
		}
		if s.URL != "" {
			e["url"] = s.URL
		}
		out[s.Name] = e
	}
	return out
}

func mcpOutcomes(cfg agentcfg.Config, r *agentcfg.Rendered) {
	for _, s := range cfg.MCP.Servers {
		if slices.Contains(cfg.MCP.Denied, s.Name) {
			r.Warnings = append(r.Warnings, agentcfg.Warning{ID: "mcp." + s.Name, Capability: agentcfg.CapMCP, Reason: "denied, left out of mcp.json"})
			continue
		}
		r.Projected = append(r.Projected, "mcp."+s.Name)
	}
	for _, d := range cfg.MCP.Denied {
		r.Projected = append(r.Projected, "mcp.denied."+d)
	}
}

// lost reports every element of a capability Pi does not have.
func lost(cfg agentcfg.Config, r *agentcfg.Rendered) {
	reasons := map[agentcfg.Capability]string{
		agentcfg.CapAgents:      "Pi has no subagent definitions",
		agentcfg.CapHooks:       "Pi hooks exist only as TypeScript extensions; aval does not generate them",
		agentcfg.CapPermissions: "Pi has no permission model; a tool_call extension would be needed",
	}
	for _, e := range cfg.Elements() {
		if reason, ok := reasons[e.Capability]; ok {
			r.Warnings = append(r.Warnings, agentcfg.Warning{ID: e.ID, Capability: e.Capability, Reason: reason})
		}
	}
}

func nativeSettings(cfg agentcfg.Config) map[string]any {
	if s := cfg.Native[Name]; s != nil {
		return s
	}
	return map[string]any{}
}

// noAPIKey refuses a config whose Pi settings or MCP servers carry an API key.
func noAPIKey(cfg agentcfg.Config) error {
	if hasKey(cfg.Native[Name]) {
		return ErrAPIKey
	}
	for _, s := range cfg.MCP.Servers {
		if hasKey(s.Command) || hasKey(s.URL) || hasKey(s.Args) {
			return ErrAPIKey
		}
	}
	return nil
}

func hasKey(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			lk := strings.ToLower(strings.ReplaceAll(k, "_", ""))
			if lk == "apikey" || slices.Contains(apiEnv, k) || hasKey(e) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(x, hasKey)
	case []string:
		return slices.ContainsFunc(x, func(s string) bool { return hasKey(s) })
	case string:
		return strings.Contains(x, "sk-ant-")
	}
	return false
}

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("pi: %w", err)
	}
	return buf.Bytes(), nil
}
