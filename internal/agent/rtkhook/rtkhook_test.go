package rtkhook

import (
	"bytes"
	"strings"
	"testing"
)

type fake struct {
	missing   bool
	version   string
	rewritten string
	code      int
	asked     []string
}

func (f *fake) Available() bool { return !f.missing }
func (f *fake) Version() string { return f.version }
func (f *fake) Rewrite(cmd string) (string, int) {
	f.asked = append(f.asked, cmd)
	return f.rewritten, f.code
}

const payload = `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"git status","description":"d","timeout":5}}`

const allow = `{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "RTK auto-rewrite",
    "updatedInput": {
      "command": "rtk git status",
      "description": "d",
      "timeout": 5
    }
  }
}
`

const ask = `{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "updatedInput": {
      "command": "rtk git status",
      "description": "d",
      "timeout": 5
    }
  }
}
`

func TestRun(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		f          fake
		wantOut    string
		wantErr    string
		wantAsked  bool
		wantNoCall bool
	}{
		{name: "rewrite found: auto-allow", in: payload, f: fake{version: "rtk 0.35.0", rewritten: "rtk git status\n", code: 0}, wantOut: allow, wantAsked: true},
		{name: "ask rule: rewrite without a decision", in: payload, f: fake{version: "rtk 0.35.0", rewritten: "rtk git status", code: 3}, wantOut: ask, wantAsked: true},
		{name: "already rtk: silent", in: payload, f: fake{version: "rtk 0.35.0", rewritten: "git status", code: 0}, wantAsked: true},
		{name: "no equivalent: pass through", in: payload, f: fake{version: "rtk 0.35.0", rewritten: "", code: 1}, wantAsked: true},
		{name: "deny rule: left to Claude Code", in: payload, f: fake{version: "rtk 0.35.0", rewritten: "x", code: 2}, wantAsked: true},
		{name: "unknown exit code: pass through", in: payload, f: fake{version: "rtk 0.35.0", rewritten: "x", code: 9}, wantAsked: true},
		{name: "rtk missing: warn and pass", in: payload, f: fake{missing: true}, wantErr: "[rtk] WARNING: rtk is not installed", wantNoCall: true},
		{name: "rtk too old: warn and pass", in: payload, f: fake{version: "rtk 0.22.9", rewritten: "rtk git status", code: 0}, wantErr: "[rtk] WARNING: rtk 0.22.9 is too old (need >= 0.23.0)", wantNoCall: true},
		{name: "0.23.0 is new enough", in: payload, f: fake{version: "rtk 0.23.0", rewritten: "rtk git status", code: 0}, wantOut: allow, wantAsked: true},
		{name: "unparseable version is trusted", in: payload, f: fake{version: "weird", rewritten: "rtk git status", code: 0}, wantOut: allow, wantAsked: true},
		{name: "no command: silent", in: `{"tool_input":{}}`, f: fake{version: "rtk 0.35.0"}, wantNoCall: true},
		{name: "non-string command: silent", in: `{"tool_input":{"command":5}}`, f: fake{version: "rtk 0.35.0"}, wantNoCall: true},
		{name: "malformed stdin: silent", in: `not json`, f: fake{version: "rtk 0.35.0"}, wantNoCall: true},
		{name: "empty stdin: silent", in: ``, f: fake{version: "rtk 0.35.0"}, wantNoCall: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errw bytes.Buffer
			f := c.f
			Run(strings.NewReader(c.in), &out, &errw, &f)
			if out.String() != c.wantOut {
				t.Errorf("stdout =\n%q\nwant\n%q", out.String(), c.wantOut)
			}
			if c.wantErr == "" && errw.Len() != 0 || !strings.Contains(errw.String(), c.wantErr) {
				t.Errorf("stderr = %q, want to contain %q", errw.String(), c.wantErr)
			}
			if c.wantNoCall && len(f.asked) != 0 {
				t.Errorf("rtk rewrite must not run, asked %v", f.asked)
			}
			if c.wantAsked && (len(f.asked) != 1 || f.asked[0] != "git status") {
				t.Errorf("rtk rewrite asked %v, want [git status]", f.asked)
			}
		})
	}
}

func TestRunKeepsToolInputKeyOrderAndEscapes(t *testing.T) {
	in := `{"tool_input":{"zeta":1,"command":"echo <a> & \"b\"","alpha":{"k":[1,2]}}}`
	f := &fake{version: "rtk 0.35.0", rewritten: "rtk echo <a>", code: 0}
	var out bytes.Buffer
	Run(strings.NewReader(in), &out, &bytes.Buffer{}, f)
	want := `    "updatedInput": {
      "zeta": 1,
      "command": "rtk echo <a>",
      "alpha": {
        "k": [
          1,
          2
        ]
      }
    }`
	if !strings.Contains(out.String(), want) {
		t.Errorf("updatedInput must keep original key order and not HTML-escape:\n%s", out.String())
	}
}
