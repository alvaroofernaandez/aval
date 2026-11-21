package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alvaroofernaandez/aval/internal/designack"
	"github.com/alvaroofernaandez/aval/internal/designgate"
)

const designUsage = `usage:
  aval design ack <surface>
  aval design gate contrast <fg> <bg> [--large]
  aval design gate tokens <file|dir>...
  aval design gate pairs <colors.json> [--aaa]
  aval design gate hardcodes [--ext .css,.tsx] [--no-tw] <path>...
  aval design gate theme-refs --theme <theme.css>... <path>...
  aval design gate component [--root <dir>]      (checks <dir>/components/*.md)
  aval design gate emoji <path>...
  aval design gate native <path>...              (native select/date/dialog/confirm)`

// runDesign dispatches `aval design`. It needs no repository: the gates work
// on whatever project the current directory is in.
func runDesign(args []string, cwd string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(designUsage)
	}
	switch args[0] {
	case "ack":
		if len(args) != 2 {
			return errors.New(designUsage)
		}
		cache, err := designack.CacheDir()
		if err != nil {
			return err
		}
		return designack.Ack(out, designack.Options{
			Surface: args[1], Dir: cwd, Cache: cache, Now: time.Now(), TTL: designack.TTLFromEnv(),
		})
	case "gate":
		return runDesignGate(args[1:], out)
	}
	return errors.New(designUsage)
}

// splitFlags separates positionals from flags. valued flags take the next
// argument; the rest are booleans. Flags may appear anywhere.
func splitFlags(args []string, valued ...string) (pos []string, flags map[string][]string) {
	flags = map[string][]string{}
	isValued := map[string]bool{}
	for _, v := range valued {
		isValued[v] = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case isValued[a] && i+1 < len(args):
			flags[a] = append(flags[a], args[i+1])
			i++
		case strings.HasPrefix(a, "--"):
			flags[a] = append(flags[a], "")
		default:
			pos = append(pos, a)
		}
	}
	return pos, flags
}

func runDesignGate(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(designUsage)
	}
	pos, flags := splitFlags(args[1:], "--ext", "--theme", "--root")
	has := func(f string) bool { _, ok := flags[f]; return ok }

	var code int
	switch args[0] {
	case "contrast":
		if len(pos) != 2 {
			return errors.New(designUsage)
		}
		code = designgate.Contrast(out, pos[0], pos[1], has("--large"))
	case "tokens":
		code = designgate.Tokens(out, pos)
	case "pairs":
		if len(pos) != 1 {
			return errors.New(designUsage)
		}
		code = designgate.Pairs(out, pos[0], has("--aaa"))
	case "hardcodes":
		if len(pos) == 0 {
			return errors.New(designUsage)
		}
		o := designgate.HardcodeOpts{Tailwind: !has("--no-tw") && !has("--no-tailwind")}
		if exts := flags["--ext"]; len(exts) > 0 {
			o.Exts = map[string]bool{}
			for _, e := range strings.Split(exts[len(exts)-1], ",") {
				if e = strings.TrimSpace(e); e != "" {
					o.Exts["."+strings.TrimPrefix(e, ".")] = true
				}
			}
		}
		code = designgate.Hardcodes(out, pos, o)
	case "theme-refs":
		themes := flags["--theme"]
		if len(themes) == 0 && len(pos) >= 2 {
			// The two-argument form: the first path is the theme.
			themes, pos = pos[:1], pos[1:]
		}
		code = designgate.ThemeRefs(out, themes, pos)
	case "component":
		root := "."
		if r := flags["--root"]; len(r) > 0 {
			root = r[len(r)-1]
		}
		code = designgate.Component(out, root)
	case "emoji":
		code = designgate.Emoji(out, pos)
	case "native":
		code = designgate.Native(out, pos)
	default:
		return fmt.Errorf("unknown gate %q\n%s", args[0], designUsage)
	}
	switch code {
	case 0:
		return nil
	case 2:
		return errors.New(designUsage)
	}
	return errChecksFailed
}
