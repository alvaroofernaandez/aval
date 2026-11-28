package statusline

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// RateLimitsFile is the name, inside Options.StateDir, of the file that keeps
// the last rate limits the runtime reported. It has the shape of the status
// line's own JSON, so `aval usage` reads it with the parser it uses for
// --rate-limits; the file's modification time says when it was written.
const RateLimitsFile = "rate-limits.json"

// rateLimitsRefresh is how long an unchanged reading is left alone. The status
// line refreshes constantly; rewriting the same two numbers each time would be
// pure I/O on the hot path. Newer than this, the file still reads as current.
const rateLimitsRefresh = time.Minute

// persistRateLimits keeps the reported plan percentages for `aval usage`. It
// writes a temp file and renames it over the target, mode 0600, so a reader
// never sees half a file. Any failure is dropped: the status line must always
// draw, and a missing file only means the share reads n/a.
func persistRateLimits(o Options, in input) {
	rl := in.RateLimits
	if o.StateDir == "" || rl == nil || (rl.FiveHour == nil && rl.SevenDay == nil) {
		return
	}
	body := []byte(`{"rate_limits":{`)
	sep := ""
	for _, w := range []struct {
		name string
		used *rateWindow
	}{{"five_hour", rl.FiveHour}, {"seven_day", rl.SevenDay}} {
		if w.used == nil {
			continue
		}
		body = append(body, sep+`"`+w.name+`":{"used_percentage":`+strconv.FormatFloat(w.used.UsedPercentage, 'f', -1, 64)+`}`...)
		sep = ","
	}
	body = append(body, "}}\n"...)

	target := filepath.Join(o.StateDir, RateLimitsFile)
	if old, err := os.ReadFile(target); err == nil && string(old) == string(body) {
		if fi, err := os.Stat(target); err == nil && o.Now.Sub(fi.ModTime()) < rateLimitsRefresh {
			return
		}
	}
	if os.MkdirAll(o.StateDir, 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(o.StateDir, RateLimitsFile+".tmp*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(body)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(tmp.Name(), 0o600) != nil || os.Rename(tmp.Name(), target) != nil {
		_ = os.Remove(tmp.Name())
	}
}
