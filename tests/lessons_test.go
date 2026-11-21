package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The lessons hook wiring and the lessons themselves.
//
// Rules that only matter near certain files (Docker, UI, commits) used to ride
// in the always-on instructions: ~6.6K chars paid on every turn of every
// session. `aval hook lessons` injects a short lesson only when a PreToolUse
// call touches a matching path or command. These tests pin what can silently
// rot: the wiring in settings.json, the shape of each lesson, and the latency
// of a hook that runs before every edit.

func lessonsDir() string { return p("config", "claude", "lessons") }

func lessonBody(text string) string {
	lines := strings.Split(text, "\n")
	c := 0
	var body []string
	for _, l := range lines {
		if c >= 2 {
			body = append(body, l)
		}
		if l == "---" {
			c++
		}
	}
	return strings.TrimRight(strings.Join(body, "\n"), "\n")
}

func TestLessonsFilesAreWellFormed(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(lessonsDir(), "*.md"))
	isTrue(t, "there are lessons (>= 4)", len(files) >= 4)
	for _, f := range files {
		name := filepath.Base(f)
		text := readAbs(t, f)
		lines := strings.Split(text, "\n")
		if lines[0] != "---" {
			t.Errorf("%s: no frontmatter", name)
			continue
		}
		var fm []string
		for _, l := range lines[1:] {
			if l == "---" {
				break
			}
			fm = append(fm, l)
		}
		if !has(`^(triggers|commands):`, strings.Join(fm, "\n")) {
			t.Errorf("%s: no triggers/commands", name)
		}
		if n := utf8.RuneCountInString(lessonBody(text)); n > 601 {
			t.Errorf("%s: body is %d chars (max 600)", name, n)
		}
	}
}

func lessonsHook(t *testing.T) (hookCmd, bool) {
	t.Helper()
	for _, g := range repoSettings(t).Hooks["PreToolUse"] {
		if g.Matcher != "Edit|Write|NotebookEdit|Bash" {
			continue
		}
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "hook lessons") {
				return h, true
			}
		}
	}
	return hookCmd{}, false
}

func TestLessonsWiring(t *testing.T) {
	h, ok := lessonsHook(t)
	isTrue(t, "PreToolUse runs the lessons hook for Edit|Write|NotebookEdit|Bash", ok)
	isTrue(t, "the lessons hook is synchronous (context must arrive before the call)", !h.Async)
	isTrue(t, "the lessons hook has a short timeout (<= 5 s)", h.Timeout > 0 && h.Timeout <= 5)
	mustHave(t, `(?m)^config/claude/lessons\s+\.claude/lessons$`, read(t, "install.manifest"), "install.manifest links the lessons directory")
}

// The binary may be absent (fresh machine, npm not installed): the hook must
// then be invisible, not an error on every single edit.
func TestLessonsHookWithNoBinaryIsASilentNoOp(t *testing.T) {
	needTool(t, "sh")
	h, ok := lessonsHook(t)
	if !ok {
		t.Fatal("lessons hook is not wired")
	}
	home := t.TempDir()
	r := run(home, withEnv(isoEnv(home), "PATH=/usr/bin:/bin"), "", "sh", "-c", h.Command)
	eq(t, "exit code with no aval binary", 0, r.Code)
	eq(t, "output with no aval binary", "", r.Out())
}

// Behaviour against the real lessons, with the built binary.
func TestLessonsInjectionAgainstTheRealLessons(t *testing.T) {
	aval := avalBin(t)
	env := isoEnv(t.TempDir(), "AVAL_LESSONS_DIR="+lessonsDir())
	fire := func(payload string) string {
		r := run(repo, env, payload, aval, "hook", "lessons")
		var v struct {
			H struct {
				C string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		_ = json.Unmarshal([]byte(r.Stdout), &v)
		return v.H.C
	}
	mustHave(t, `screenshot|shot.mjs`,
		fire(`{"session_id":"u1","tool_name":"Edit","tool_input":{"file_path":"/p/web/src/Card.tsx"}}`),
		"editing a .tsx file injects the screenshot lesson")
	mustHave(t, `native`,
		fire(`{"session_id":"u2","tool_name":"Write","tool_input":{"file_path":"/p/web/src/Card.tsx"}}`),
		"editing a .tsx file injects the native-elements lesson")
	mustHave(t, `Kiri`,
		fire(`{"session_id":"d1","tool_name":"Edit","tool_input":{"file_path":"/p/api/Dockerfile"}}`),
		"editing a Dockerfile injects the Kiri lesson")
	mustHave(t, `Kiri`,
		fire(`{"session_id":"d2","tool_name":"Bash","tool_input":{"command":"docker compose up -d"}}`),
		"running docker injects the Kiri lesson")
	mustHave(t, `Co-Authored-By`,
		fire(`{"session_id":"c1","tool_name":"Bash","tool_input":{"command":"git commit -m \"feat: x\""}}`),
		"committing injects the attribution lesson")
	eq(t, "editing plain Go code injects nothing", "",
		fire(`{"session_id":"n1","tool_name":"Edit","tool_input":{"file_path":"/p/main.go"}}`))
	eq(t, "a lesson shows once per session", "",
		fire(`{"session_id":"d1","tool_name":"Edit","tool_input":{"file_path":"/p/api/Dockerfile"}}`))
}

// The hook runs before every Edit/Write/Bash call. Measured, not assumed.
func TestLessonsHookLatency(t *testing.T) {
	needTool(t, "hyperfine")
	slow(t)
	aval := avalBin(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	write(t, in, `{"session_id":"lat","tool_name":"Edit","tool_input":{"file_path":"/p/main.go"}}`)
	out := filepath.Join(dir, "hf.json")
	run(repo, isoEnv(dir, "AVAL_LESSONS_DIR="+lessonsDir()), "", "hyperfine", "-N", "--warmup", "10", "--runs", "50",
		"--input", in, "--export-json", out, aval+" hook lessons")
	var hf struct {
		Results []struct {
			Mean    *float64 `json:"mean"`
			Summary *struct {
				Wall struct {
					Mean float64 `json:"mean"`
				} `json:"time_wall_clock"`
			} `json:"summary"`
		} `json:"results"`
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("hyperfine wrote no report: %v", err)
	}
	_ = json.Unmarshal(b, &hf)
	if len(hf.Results) == 0 {
		t.Fatal("hyperfine report has no results")
	}
	mean := 0.0
	if hf.Results[0].Summary != nil { // newer hyperfine nests it under summary
		mean = hf.Results[0].Summary.Wall.Mean
	} else if hf.Results[0].Mean != nil {
		mean = *hf.Results[0].Mean
	}
	ms := mean * 1000
	isTrue(t, "the lessons hook runs in under 10 ms (is "+strconv.FormatFloat(ms, 'f', 1, 64)+" ms)", ms > 0 && ms < 10)
}

// The always-on text got smaller.
func TestLessonsTookTheScopedRulesOutOfTheAlwaysOnBlock(t *testing.T) {
	blk := sharedBlock(read(t, "config/claude/CLAUDE.md"))
	const maxChars = 3000
	isTrue(t, "the shared always-on block stays under 3000 chars (is "+itoa(utf8.RuneCountInString(blk))+")",
		utf8.RuneCountInString(blk) <= maxChars)
	for _, needle := range []string{"design gate native", "shot.mjs", "BUILDX_BUILDER", "OrbStack"} {
		if strings.Contains(blk, needle) {
			t.Errorf("path-scoped rule %q is still in the always-on block", needle)
		}
	}
}
