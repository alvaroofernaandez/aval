package main

import (
	"fmt"
	"strings"
)

// The stack image tells the roadmap: each lane is a job some tool does today
// and the native aval piece that takes it over, with the version that ships it.
// The tools aval keeps driving (languages, git, CLI, platforms) sit below.

type lane struct {
	name    string
	today   []tool
	target  string // the aval piece that replaces them
	version string // the milestone that ships it
	note    string
	icon    func(ink string) string
}

type group struct {
	name  string
	tools []tool
}

var lanes = []lane{
	{"terminal window", []tool{{"Ghostty", "ghostty.png", ""}},
		"aval desktop", "v0.9.0", "native window on aval core", desktopGroup},
	{"workspace", []tool{{"tmux", "tmux.svg", ""}, {"Herdr", "herdr.svg", ""}, {"yazi", "yazi.png", ""}},
		"aval core + TUI", "v0.7.0", "sessions, panes, agents, files", markGroup},
	{"memory and context", []tool{{"Engram", "engram.png", ""}, {"RTK", "rtk.png", ""}},
		"aval memory + context", "v0.3.0", "native ports, tools as adapters", markGroup},
	{"agent runtimes", []tool{{"Claude Code", "claude.svg", ""}, {"OpenCode", "opencode.svg", ""}},
		"aval agents", "v0.4.0", "one config, one adapter per agent", markGroup},
}

var kept = []group{
	{"languages and packages", []tool{
		{"Go", "go.svg", ""}, {"Node.js", "nodejs.svg", ""}, {"npm", "npm.svg", ""}, {"Bun", "bun.svg", ""},
		{"Python", "python.svg", ""}, {"Bash", "bash.svg", ""}, {"Zsh", "zsh.svg", ""}, {"Homebrew", "homebrew.svg", ""},
	}},
	{"runtime and git", []tool{
		{"Docker", "docker.svg", ""}, {"Kiri", "kiri.svg", ""}, {"Lightpanda", "lightpanda.png", ""},
		{"Git", "git.svg", ""}, {"GitHub CLI", "github.svg", ""}, {"gitleaks", "gitleaks.png", ""},
	}},
	{"cli", []tool{
		{"ripgrep", "", "rg"}, {"fd", "fd.svg", ""}, {"bat", "bat.png", ""}, {"eza", "eza.png", ""},
		{"fzf", "fzf.png", ""}, {"zoxide", "zoxide.svg", ""}, {"atuin", "atuin.png", ""}, {"delta", "", "Δ"},
	}},
	{"platforms", []tool{
		{"macOS", "apple.svg", ""}, {"Linux", "linux.svg", ""}, {"Windows", "windows.svg", ""},
	}},
}

// allTools lists every tool in the image, lanes first, in drawing order.
func allTools() []tool {
	var out []tool
	for _, l := range lanes {
		out = append(out, l.today...)
	}
	for _, g := range kept {
		out = append(out, g.tools...)
	}
	return out
}

const (
	bandH    = 44
	laneH    = head + card + 52
	keptRowH = head + card + 52
	targetX  = 3*tileW + 104
	targetW  = stackW - targetX - 20
)

func label(x float64, y int, fill, text string) string {
	return fmt.Sprintf(`<text x="%s" y="%d" font-family="%s" font-size="12" letter-spacing="3" fill="%s">%s</text>`,
		pyFloat(x), y, monoFont, fill, esc(strings.ToUpper(text)))
}

func (g gen) lane(y int, l lane, t theme, th string) ([]string, error) {
	left := float64(tileW)/2 - float64(card)/2
	out := []string{fmt.Sprintf(`<g data-part="lane" data-target="%s" data-version="%s">`, esc(l.target), l.version),
		label(left, y+16, t.muted, l.name)}
	for col, tl := range l.today {
		p, err := g.tile(col*tileW, y, tl, t, th)
		if err != nil {
			return nil, err
		}
		out = append(out, p...)
	}
	top := y + head
	mid := top + card/2
	ax0, ax1 := 3*tileW+12, targetX-16
	out = append(out,
		fmt.Sprintf(`<path d="M %d %d H %d" stroke="%s" stroke-width="2" stroke-dasharray="4 6" fill="none"/>`, ax0, mid, ax1-6, t.muted),
		fmt.Sprintf(`<path d="M %d %d L %d %d L %d %d" stroke="%s" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>`,
			ax1-14, mid-8, ax1-4, mid, ax1-14, mid+8, t.muted),
		fmt.Sprintf(`<rect class="target" x="%d" y="%d" width="%d" height="%d" rx="20" fill="none" `+
			`stroke="%s" stroke-width="1.5" stroke-dasharray="6 5"/>`, targetX, top, targetW, card, t.fg),
		fmt.Sprintf(`<g transform="translate(%d %d) scale(%.4f)">%s</g>`, targetX+18, top+(card-logoSz)/2,
			float64(logoSz)/float64(markM), l.icon(t.fg)),
		fmt.Sprintf(`<text x="%d" y="%d" font-family="%s" font-size="20" font-weight="600" fill="%s">%s</text>`,
			targetX+18+logoSz+18, top+38, sansFont, t.fg, esc(l.target)),
		fmt.Sprintf(`<text x="%d" y="%d" font-family="%s" font-size="12" fill="%s">%s · %s</text>`,
			targetX+18+logoSz+18, top+62, monoFont, t.muted, l.version, esc(l.note)),
		"</g>")
	return out, nil
}

func (g gen) stack(th string) (string, error) {
	t := themes[th]
	left := float64(tileW)/2 - float64(card)/2
	keptTop := bandH + laneH*len(lanes) + 28
	height := keptTop + 36 + keptRowH*len(kept)
	parts := []string{fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
		`width="%d" height="%d" role="img" `+
		`aria-label="The tools aval runs on today and the native aval pieces that replace them on the roadmap">`,
		stackW, height, stackW, height),
		label(left, 18, t.fg, "today"),
		label(float64(targetX), 18, t.fg, "on the roadmap"),
		fmt.Sprintf(`<path d="M %s 30 H %s" stroke="%s" stroke-width="1"/>`, pyFloat(left), pyFloat(float64(stackW)-left), t.edge),
	}
	for i, l := range lanes {
		p, err := g.lane(bandH+i*laneH, l, t, th)
		if err != nil {
			return "", err
		}
		parts = append(parts, p...)
	}
	parts = append(parts, `<g data-part="kept">`,
		fmt.Sprintf(`<path d="M %s %d H %s" stroke="%s" stroke-width="1"/>`, pyFloat(left), keptTop, pyFloat(float64(stackW)-left), t.edge),
		label(left, keptTop+24, t.fg, "kept · aval drives these, it does not replace them"))
	for row, grp := range kept {
		y := keptTop + 36 + row*keptRowH
		parts = append(parts, label(left, y+16, t.muted, grp.name))
		for col, tl := range grp.tools {
			p, err := g.tile(col*tileW, y, tl, t, th)
			if err != nil {
				return "", err
			}
			parts = append(parts, p...)
		}
	}
	parts = append(parts, "</g>", "</svg>")
	return strings.Join(parts, "\n") + "\n", nil
}
