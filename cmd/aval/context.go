package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/i18n"
	"github.com/alvaroofernaandez/aval/internal/relay"
)

const contextUsage = "usage: aval context floor [--session <id>] [--project-dir <dir>]"

// runContext is `aval context <sub>`. Only floor exists: the context a fresh
// session and a fresh sub-agent start with, read from the transcript.
func runContext(sub []string, wd string, c cmdIO) error {
	if len(sub) == 0 || sub[0] != "floor" {
		return newUsage(contextUsage)
	}
	fs := newFlagSet("context floor", c.stderr)
	session := fs.String("session", "", "session id (default: the newest of the project)")
	projectDir := fs.String("project-dir", "", "transcript directory (default: ~/.claude/projects/<cwd>)")
	if err := fs.Parse(sub[1:]); err != nil {
		return newUsage(contextUsage)
	}
	if *projectDir == "" {
		home, err := userHomeDir(c.env)
		if err != nil {
			return err
		}
		*projectDir = filepath.Join(home, ".claude", "projects", claudeProjectDirName(wd))
	}
	transcript, err := pickSession(*projectDir, *session)
	if err != nil {
		return err
	}
	tr := i18n.New(usageLang(c.env, ""))
	main, ok := firstTurnOf(transcript, false)
	if !ok {
		return errors.New(tr.T("cli.context.floor.empty", i18n.Args{"path": transcript}))
	}
	fmt.Fprintln(c.stdout, tr.T("cli.context.floor.main", i18n.Args{"tokens": main.Tokens, "short": relay.Tokens(main.Tokens)}))
	if sub, ok := firstSubagentTurn(transcript); ok {
		fmt.Fprintln(c.stdout, tr.T("cli.context.floor.sub", i18n.Args{"tokens": sub.Tokens, "short": relay.Tokens(sub.Tokens)}))
	}
	return nil
}

// pickSession is the transcript of session in dir, or the newest one.
func pickSession(dir, session string) (string, error) {
	if session != "" {
		path := filepath.Join(dir, session+".jsonl")
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return path, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var best string
	var bestAt time.Time
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		if info, err := e.Info(); err == nil && (best == "" || info.ModTime().After(bestAt)) {
			best, bestAt = filepath.Join(dir, e.Name()), info.ModTime()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no session transcript in %s", dir)
	}
	return best, nil
}

func firstTurnOf(path string, sidechain bool) (relay.Turn, bool) {
	f, err := os.Open(path)
	if err != nil {
		return relay.Turn{}, false
	}
	defer f.Close()
	return relay.FirstTurn(relay.ReadUsage(f), sidechain)
}

// firstSubagentTurn is the earliest sub-agent turn of a session: its own
// sidechain entries and the files under <session>/subagents.
func firstSubagentTurn(transcript string) (relay.Turn, bool) {
	files, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(transcript, ".jsonl"), "subagents", "*.jsonl"))
	var best relay.Turn
	found := false
	for _, f := range append([]string{transcript}, files...) {
		if t, ok := firstTurnOf(f, true); ok && (!found || (!t.At.IsZero() && t.At.Before(best.At))) {
			best, found = t, true
		}
	}
	return best, found
}

// claudeProjectDirName is the directory Claude Code keeps a cwd's transcripts
// in: every character that is not an ASCII letter or digit becomes "-".
func claudeProjectDirName(wd string) string {
	b := []byte(wd)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			b[i] = '-'
		}
	}
	return string(b)
}
