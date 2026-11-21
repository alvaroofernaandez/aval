package main

import "strings"

// hoistRepo moves a --repo flag written after the subcommand to the front, so
// `aval doctor --repo X` means the same as `aval --repo X doctor`. Only --repo
// moves: other global flags share names with subcommand flags. Arguments after
// "--" belong to a wrapped command and are never touched.
func hoistRepo(args []string) []string {
	var lead, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		switch {
		case a == "--repo" || a == "-repo":
			if i+1 < len(args) {
				lead = append(lead, a, args[i+1])
				i++
				continue
			}
		case strings.HasPrefix(a, "--repo=") || strings.HasPrefix(a, "-repo="):
			lead = append(lead, a)
			continue
		}
		rest = append(rest, a)
	}
	return append(lead, rest...)
}
