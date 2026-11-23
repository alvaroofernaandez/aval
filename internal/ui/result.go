package ui

// Completed reports whether the run applied every selected action and reached
// the done screen. It is the single question main asks before it writes
// anything outside the install itself (settings seed, sync, repo record, git
// hooks): quitting on the select screen, or stopping mid-run, must leave all
// of that untouched. The backup run is not part of that list: main commits it
// whatever happened, so a stopped run can still be undone.
func (m Model) Completed() bool {
	return m.screen == ScreenDone && !m.stopped
}

// Stopped reports whether the run was stopped with actions left: ctrl+c or q
// during install let the step in flight finish and skipped the rest. A stop
// pressed during the last step skips nothing and is a completed run.
func (m Model) Stopped() bool { return m.stopped }

// Progress is how many queued actions have run (failures included) out of the
// total, so a stopped run reads "Stopped after done of total".
func (m Model) Progress() (done, total int) { return m.index, len(m.queue) }
