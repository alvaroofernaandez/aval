// Package claude is the Claude Code adapter of the agentcfg model. It renders
// ~/.claude/settings.json: the repo-owned keys (hooks, permissions, the denied
// MCP servers and the Claude-only keys kept in Native), adapted to the OS, and
// on Apply merges them into the live file with the personal keys and the
// private overlay, as `aval sync` always has.
package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/agentcfg"
)

// SettingsFile is the one file the adapter renders, relative to ~/.claude.
const SettingsFile = "settings.json"

// ErrNoSettings means a rendering handed to Diff or Apply has no settings.json.
var ErrNoSettings = errors.New("claude: the rendering has no settings.json")

// Options places the rendering: the OS flavour of the hooks and the private
// overlay merged on Apply.
type Options struct {
	GOOS       string // empty means runtime.GOOS
	AvalBin    string // the aval the Windows hooks run; empty means ResolveAvalBin
	Home       string // where ResolveAvalBin looks
	PrivateDir string // the private overlay; empty means none
}

// Adapter renders ~/.claude/settings.json.
type Adapter struct{ opt Options }

// New returns the adapter.
func New(opt Options) *Adapter {
	if opt.GOOS == "" {
		opt.GOOS = runtime.GOOS
	}
	return &Adapter{opt: opt}
}

// Name is agentcfg.ClaudeCode.
func (a *Adapter) Name() string { return agentcfg.ClaudeCode }

// Capabilities of Claude Code. Rules, skills, commands and agents reach it
// through the `aval install` links (Rendered.Linked); MCP is partial: the
// denylist is rendered, servers live in ~/.claude.json.
func (a *Adapter) Capabilities() agentcfg.Capabilities {
	return agentcfg.Capabilities{
		agentcfg.CapRules:       agentcfg.Full,
		agentcfg.CapSkills:      agentcfg.Full,
		agentcfg.CapCommands:    agentcfg.Full,
		agentcfg.CapAgents:      agentcfg.Full,
		agentcfg.CapHooks:       agentcfg.Full,
		agentcfg.CapPermissions: agentcfg.Full,
		agentcfg.CapMCP:         agentcfg.Partial,
	}
}

// Render renders the managed settings.
func (a *Adapter) Render(cfg agentcfg.Config) (agentcfg.Rendered, error) {
	data, err := encode(a.Settings(cfg))
	if err != nil {
		return agentcfg.Rendered{}, fmt.Errorf("claude: %w", err)
	}
	r := agentcfg.Rendered{Files: []agentcfg.File{{Path: SettingsFile, Data: data, Mode: 0o600}}}
	for _, e := range cfg.Elements() {
		switch {
		case e.Capability == agentcfg.CapHooks, e.Capability == agentcfg.CapPermissions,
			e.Capability == agentcfg.CapMCP && strings.HasPrefix(e.ID, "mcp.denied."):
			r.Projected = append(r.Projected, e.ID)
		case linkOf(cfg, e) != nil:
			r.Linked = append(r.Linked, *linkOf(cfg, e))
		case e.Capability == agentcfg.CapMCP:
			r.Warnings = append(r.Warnings, agentcfg.Warning{ID: e.ID, Capability: e.Capability,
				Reason: "Claude Code reads MCP servers from ~/.claude.json, which aval does not write"})
		}
	}
	return r, nil
}

// Settings is the managed settings map for cfg: the native spelling of the
// repo, except where the model says otherwise, adapted to the OS. Native keeps
// the bytes of the source (key order aside); the model wins when it differs,
// so a Config edited in memory renders what it says.
func (a *Adapter) Settings(cfg agentcfg.Config) map[string]any {
	out := cloneMap(cfg.Native[agentcfg.ClaudeCode])
	if out == nil {
		out = map[string]any{}
	}
	derived := agentcfg.FromSettings(cloneMap(out))
	if !sameJSON(cfg.Hooks, derived.Hooks) {
		setOrDelete(out, "hooks", hooksJSON(cfg.Hooks), len(cfg.Hooks) > 0)
	}
	if !sameJSON(cfg.Permissions, derived.Permissions) {
		perms := permissionsJSON(out["permissions"], cfg.Permissions)
		setOrDelete(out, "permissions", perms, len(perms) > 0)
	}
	if !sameJSON(cfg.MCP.Denied, derived.MCP.Denied) {
		setOrDelete(out, "deniedMcpServers", deniedJSON(out["deniedMcpServers"], cfg.MCP.Denied), len(cfg.MCP.Denied) > 0)
	}
	return ForOS(out, a.opt.GOOS, a.opt.AvalBin, a.opt.Home)
}

// Diff compares the rendering, merged as Apply would, with the live settings
// under target.
func (a *Adapter) Diff(target string, r agentcfg.Rendered) ([]agentcfg.Change, error) {
	p, err := a.plan(target, r)
	if err != nil || (p.exists && len(p.changed) == 0) {
		return nil, err
	}
	kind := agentcfg.Update
	if !p.exists {
		kind = agentcfg.Create
	}
	return []agentcfg.Change{{Path: SettingsFile, Kind: kind}}, nil
}

// Apply merges the rendering into the live settings under target.
func (a *Adapter) Apply(target string, r agentcfg.Rendered, opt agentcfg.ApplyOptions) ([]agentcfg.Change, error) {
	res, err := a.ApplySettings(target, r, opt)
	if err != nil || !res.Written {
		return nil, err
	}
	kind := agentcfg.Update
	if res.Created {
		kind = agentcfg.Create
	}
	return []agentcfg.Change{{Path: SettingsFile, Kind: kind}}, nil
}

// Live reads the live settings under target; a missing file is empty.
func (a *Adapter) Live(target string) (live map[string]any, exists bool, err error) {
	m, _, err := readJSONFile(filepath.Join(target, SettingsFile))
	switch {
	case err == nil:
		return m, true, nil
	case errors.Is(err, os.ErrNotExist):
		return map[string]any{}, false, nil
	}
	return nil, false, fmt.Errorf("live settings: %w", err)
}

// Overlay reads the private settings fragment, nil when there is none.
func (a *Adapter) Overlay() (map[string]any, error) {
	if a.opt.PrivateDir == "" {
		return nil, nil
	}
	m, _, err := readJSONFile(filepath.Join(a.opt.PrivateDir, SettingsFile))
	switch {
	case err == nil:
		return m, nil
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	}
	return nil, fmt.Errorf("private settings: %w", err)
}

func sameJSON(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func setOrDelete(m map[string]any, key string, v any, set bool) {
	if set {
		m[key] = v
	} else {
		delete(m, key)
	}
}

func hooksJSON(groups []agentcfg.HookGroup) map[string]any {
	out := map[string]any{}
	for _, g := range groups {
		hs := []any{}
		for _, h := range g.Handlers {
			hm := cloneMap(h.Extra)
			if hm == nil {
				hm = map[string]any{}
			}
			if h.Type != "" {
				hm["type"] = h.Type
			}
			if h.Command != "" {
				hm["command"] = h.Command
			}
			if h.Timeout > 0 {
				hm["timeout"] = json.Number(strconv.Itoa(h.Timeout))
			}
			hs = append(hs, hm)
		}
		gm := map[string]any{"hooks": hs}
		if g.Matcher != "" {
			gm["matcher"] = g.Matcher
		}
		list, _ := out[g.Event].([]any)
		out[g.Event] = append(list, gm)
	}
	return out
}

func permissionsJSON(native any, p agentcfg.Permissions) map[string]any {
	m, _ := clone(native).(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	for key, list := range map[string][]string{"allow": p.Allow, "deny": p.Deny, "ask": p.Ask} {
		_, had := m[key]
		setOrDelete(m, key, toAny(list), len(list) > 0 || had)
	}
	setOrDelete(m, "defaultMode", p.DefaultMode, p.DefaultMode != "")
	return m
}

func deniedJSON(native any, names []string) []any {
	byName := map[string]map[string]any{}
	l, _ := native.([]any)
	for _, x := range l {
		if m, ok := x.(map[string]any); ok {
			if n, ok := m["serverName"].(string); ok {
				byName[n] = m
			}
		}
	}
	out := make([]any, 0, len(names))
	for _, n := range names {
		e := cloneMap(byName[n])
		if e == nil {
			e = map[string]any{}
		}
		e["serverName"] = n
		out = append(out, e)
	}
	return out
}

// linkOf is the install.manifest link that delivers e to Claude Code, nil for
// an element the settings carry.
func linkOf(cfg agentcfg.Config, e agentcfg.Element) *agentcfg.Link {
	l := agentcfg.Link{ID: e.ID, Capability: e.Capability}
	name := e.ID[strings.IndexByte(e.ID, '.')+1:]
	switch e.Capability {
	case agentcfg.CapRules:
		l.Source, l.Dest = "config/claude/CLAUDE.md", ".claude/CLAUDE.md"
	case agentcfg.CapSkills:
		l.Source, l.Dest = "skills/"+name, ".claude/skills/"+name
	case agentcfg.CapCommands:
		l.Source, l.Dest = "config/claude/commands/"+name+".md", ".claude/commands/"+name+".md"
	case agentcfg.CapAgents:
		file := name + ".md"
		for _, a := range cfg.Agents {
			if a.Name == name && a.File != "" {
				file = a.File
			}
		}
		l.Source, l.Dest = "config/claude/agents/"+file, ".claude/agents/"+file
	default:
		return nil
	}
	return &l
}
