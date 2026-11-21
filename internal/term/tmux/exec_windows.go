//go:build windows

package tmux

import (
	"errors"
	"math/rand/v2"
	"runtime"
)

func goos() string { return runtime.GOOS }

func randInt(n int) int { return rand.IntN(n) }

// tmux helpers are unix tooling; on Windows they report so instead of failing
// to build the binary.
func execReplace(string, []string) error             { return errors.New("not supported on windows") }
func spawnDetached(string, []string, []string) error { return errors.New("not supported on windows") }
