package ui

// Completed reports whether the run applied every selected action and reached
// the done screen. It is the single question main asks before it writes
// anything outside the install itself (settings seed, sync, repo record, git
// hooks): quitting on the select screen, or mid-run, must leave all of that
// untouched.
//
// Seam for T9 (graceful stop): a stopped run must make Completed false. T9
// either never moves a stopped run to ScreenDone, or adds a `stopped` field to
// Model and extends the condition below with `&& !m.stopped`.
func (m Model) Completed() bool {
	return m.screen == ScreenDone
}
