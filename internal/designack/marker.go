package designack

import (
	"os"
	"path/filepath"
	"strings"
)

// Marker is what the gate needs from an ack file.
type Marker struct {
	Epoch      string // unparsed: the gate reports a non-number as "unreadable"
	DesignSHA1 string
}

// Sha1Hex is the hex sha1 of b, the key and hash format of the marker.
func Sha1Hex(b []byte) string { return sha1hex(b) }

// MarkerPath is <cache>/design-gate/<sha1 of project root>.
func MarkerPath(cache, root string) string {
	return filepath.Join(cache, "design-gate", sha1hex([]byte(root)))
}

// ReadMarker parses a marker file: key=value lines, the last occurrence of a
// key wins, anything else is ignored. ok is false when the file cannot be read.
func ReadMarker(path string) (m Marker, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Marker{}, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch k {
		case "epoch":
			m.Epoch = v
		case "design_sha1":
			m.DesignSHA1 = v
		}
	}
	return m, true
}
