# diff-review

A Herdr panel for reviewing the space's uncommitted diff GitHub-style: click a
line to comment on it, then send every comment to the agent in one message.

- **Open:** `cmd-shift-r` (keys.json) opens it as a tab of the focused space.
  `tab.new {mod: "diff-review", props: {folder: "/path"}}` reviews another folder.
- **Left:** files changed against `HEAD` (staged, unstaged and untracked).
  **Right:** the selected file's unified diff with old/new line numbers.
- **Keys:** `j`/`k` move, `c` or `Enter` comment, `Enter` save, `Esc` cancel,
  `⌘Enter` send. Also bindable as `diff-review.next|prev|comment|cancel|send|clear`.
- **Comments** live in shared state per machine + repo + branch, so they survive
  closing the panel. A successful send or **Clear** (click twice) removes them.
- **Send to agent** pastes one message with `hc send` (then Enter) to an agent of
  the panel's tab, else of its space; with several, pick one in the header. It
  refuses a `working` agent and a `blocked` one (typing would answer its prompt).

Pure logic is in `logic.js`; test it with
`node --test config/herdr-app/mods/diff-review/logic.test.js`.
