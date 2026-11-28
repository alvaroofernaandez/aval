//go:build !windows

package secrets

import "runtime"

// Default is the keychain of this machine, with List available. Linux without
// secret-tool reports ErrUnavailable on first use; nothing falls back to a file.
func Default() Store {
	if runtime.GOOS == "darwin" {
		return Indexed(&macStore{run: execRunner})
	}
	return Indexed(&linuxStore{run: execRunner})
}
