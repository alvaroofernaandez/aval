package agentcfg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ClaudeCode is the adapter name of Claude Code, the key of its native settings.
const ClaudeCode = "claude-code"

const (
	rulesBegin = "<!-- shared:agent-rules -->"
	rulesEnd   = "<!-- /shared:agent-rules -->"
)

// ErrNoSharedRules means config/claude/CLAUDE.md has no shared rules block.
var ErrNoSharedRules = errors.New("agentcfg: no shared agent-rules block")

// Load reads the model from the aval repository at repo, in the format the
// repository already uses:
//
//   - rules: the shared block of config/claude/CLAUDE.md (between the
//     shared:agent-rules markers), as rule "agent-rules";
//   - skills: skills/<name>/SKILL.md and the files beside it;
//   - commands and agents: config/claude/commands/*.md, config/claude/agents/*.md;
//   - hooks, permissions and denied MCP servers: config/claude/settings.json,
//     which is also kept whole as Native["claude-code"].
func Load(repo string) (Config, error) {
	var c Config
	var err error
	if c.Rules, err = loadRules(filepath.Join(repo, "config", "claude", "CLAUDE.md")); err != nil {
		return c, err
	}
	if c.Skills, err = loadSkills(filepath.Join(repo, "skills")); err != nil {
		return c, err
	}
	if c.Commands, err = loadCommands(filepath.Join(repo, "config", "claude", "commands")); err != nil {
		return c, err
	}
	if c.Agents, err = loadAgents(filepath.Join(repo, "config", "claude", "agents")); err != nil {
		return c, err
	}
	s, err := LoadSettings(filepath.Join(repo, "config", "claude", "settings.json"))
	c.Hooks, c.Permissions, c.MCP.Denied, c.Native = s.Hooks, s.Permissions, s.MCP.Denied, s.Native
	return c, err
}

// LoadSettings reads only the settings part of the model from a Claude Code
// settings.json: hooks, permissions, denied MCP servers, and the whole file as
// Native["claude-code"].
func LoadSettings(path string) (Config, error) {
	settings, err := readSettings(path)
	if err != nil {
		return Config{}, err
	}
	return FromSettings(settings), nil
}

// FromSettings is LoadSettings for settings already decoded. The map is kept
// as it is, not copied.
func FromSettings(settings map[string]any) Config {
	return Config{
		Hooks:       hooksFrom(settings["hooks"]),
		Permissions: permissionsFrom(settings["permissions"]),
		MCP:         MCP{Denied: deniedFrom(settings["deniedMcpServers"])},
		Native:      map[string]map[string]any{ClaudeCode: settings},
	}
}

// RulesBlock is the shared agent-rules block of a CLAUDE.md, the text every
// runtime receives as its rules.
func RulesBlock(path string) (string, error) {
	rules, err := loadRules(path)
	if err != nil {
		return "", err
	}
	return rules[0].Body, nil
}

func loadRules(path string) ([]Rule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agentcfg: %w", err)
	}
	var block []string
	in, found := false, false
	for _, l := range strings.Split(string(b), "\n") {
		switch strings.TrimSpace(l) {
		case rulesBegin:
			in, found = true, true
			continue
		case rulesEnd:
			in = false
			continue
		}
		if in {
			block = append(block, l)
		}
	}
	if !found || len(block) == 0 {
		return nil, fmt.Errorf("%w in %s", ErrNoSharedRules, path)
	}
	return []Rule{{ID: "agent-rules", Body: strings.Join(block, "\n")}}, nil
}

func loadSkills(dir string) ([]Skill, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("agentcfg: %w", err)
	}
	var out []Skill
	for _, e := range ents {
		sdir := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(filepath.Join(sdir, "SKILL.md"))
		if err != nil {
			continue // not a skill: _shared, PROVENANCE.md
		}
		fm, _ := splitFrontMatter(string(b))
		s := Skill{Name: fm.values["name"], Description: fm.values["description"], Dir: sdir}
		if s.Name == "" {
			s.Name = e.Name()
		}
		if s.Files, s.Hash, err = hashDir(sdir); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// hashDir lists the regular files under dir and hashes their paths and bytes.
func hashDir(dir string) ([]string, string, error) {
	var files []string
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files = append(files, rel)
		sum := sha256.Sum256(b)
		fmt.Fprintf(h, "%s %x\n", rel, sum)
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("agentcfg: %w", err)
	}
	sort.Strings(files)
	return files, "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// markdownFiles reads the *.md files of dir, sorted by name. A missing dir is
// empty; an entry that cannot be read (a dangling link) is skipped.
func markdownFiles(dir string) (names, contents []string, err error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("agentcfg: %w", err)
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		contents = append(contents, string(b))
	}
	return names, contents, nil
}

func loadCommands(dir string) ([]Command, error) {
	names, contents, err := markdownFiles(dir)
	var out []Command
	for i, raw := range contents {
		fm, body := splitFrontMatter(raw)
		out = append(out, Command{Name: names[i], Description: fm.values["description"], ArgumentHint: fm.values["argument-hint"], Body: body, Raw: raw})
	}
	return out, err
}

func loadAgents(dir string) ([]Agent, error) {
	names, contents, err := markdownFiles(dir)
	var out []Agent
	for i, raw := range contents {
		fm, body := splitFrontMatter(raw)
		name := fm.values["name"]
		if name == "" {
			name = names[i]
		}
		out = append(out, Agent{Name: name, File: names[i] + ".md", Description: fm.values["description"], Model: fm.values["model"], Skills: fm.lists["skills"], Body: body, Raw: raw})
	}
	return out, err
}

// readSettings reads a settings file; its errors carry the path and no
// prefix, so callers word them as before (sync: repo settings: <path>: ...).
func readSettings(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := decodeJSON(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// DecodeJSON decodes a JSON object keeping numbers as json.Number, so a
// re-encoding writes them back unchanged.
func DecodeJSON(b []byte) (map[string]any, error) {
	m, err := decodeJSON(b)
	if err != nil {
		return nil, fmt.Errorf("agentcfg: %w", err)
	}
	return m, nil
}

func decodeJSON(b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func hooksFrom(v any) []HookGroup {
	byEvent, _ := v.(map[string]any)
	events := make([]string, 0, len(byEvent))
	for e := range byEvent {
		events = append(events, e)
	}
	sort.Strings(events)
	var out []HookGroup
	for _, e := range events {
		groups, _ := byEvent[e].([]any)
		for _, g := range groups {
			gm, _ := g.(map[string]any)
			hg := HookGroup{Event: e}
			hg.Matcher, _ = gm["matcher"].(string)
			hs, _ := gm["hooks"].([]any)
			for _, h := range hs {
				hm, _ := h.(map[string]any)
				hg.Handlers = append(hg.Handlers, handlerFrom(hm))
			}
			out = append(out, hg)
		}
	}
	return out
}

func handlerFrom(m map[string]any) HookHandler {
	h := HookHandler{}
	for k, v := range m {
		switch k {
		case "type":
			h.Type, _ = v.(string)
		case "command":
			h.Command, _ = v.(string)
		case "timeout":
			if n, ok := v.(json.Number); ok {
				i, _ := n.Int64()
				h.Timeout = int(i)
			}
		default:
			if h.Extra == nil {
				h.Extra = map[string]any{}
			}
			h.Extra[k] = v
		}
	}
	return h
}

func permissionsFrom(v any) Permissions {
	m, _ := v.(map[string]any)
	p := Permissions{Allow: stringList(m["allow"]), Deny: stringList(m["deny"]), Ask: stringList(m["ask"])}
	p.DefaultMode, _ = m["defaultMode"].(string)
	return p
}

func deniedFrom(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, x := range l {
		if m, ok := x.(map[string]any); ok {
			if s, ok := m["serverName"].(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func stringList(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
