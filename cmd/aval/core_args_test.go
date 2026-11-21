package main

import (
	"reflect"
	"testing"
)

// --repo is a global flag, but people write it after the subcommand too
// (`aval doctor --repo X`). Go's flag package stops at the first non-flag, so
// it used to be silently ignored there and the command failed to find the repo.
func TestHoistRepo(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{[]string{"doctor", "--repo", "/r"}, []string{"--repo", "/r", "doctor"}},
		{[]string{"doctor", "--repo=/r", "--json"}, []string{"--repo=/r", "doctor", "--json"}},
		{[]string{"sync", "-repo", "/r", "--check"}, []string{"-repo", "/r", "sync", "--check"}},
		{[]string{"--repo", "/r", "doctor"}, []string{"--repo", "/r", "doctor"}},
		{[]string{"doctor", "--fix"}, []string{"doctor", "--fix"}},
		// After "--" everything belongs to the wrapped command.
		{[]string{"dev", "run", "--", "aval", "--repo", "/x"}, []string{"dev", "run", "--", "aval", "--repo", "/x"}},
		// A trailing --repo with no value is left for flag to report.
		{[]string{"doctor", "--repo"}, []string{"doctor", "--repo"}},
	}
	for _, c := range cases {
		if got := hoistRepo(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("hoistRepo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
