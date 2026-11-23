package i18n

// englishCLI holds the keys of the human CLI output (cmd/aval, internal/ui/cli).
var englishCLI = catalog{
	// usage
	"cli.usage.tagline":      {other: "Nothing passes without evidence."},
	"cli.usage.about":        {other: "aval installs and maintains this machine's dotfiles and agent configs."},
	"cli.usage.heading":      {other: "Usage:"},
	"cli.usage.invocation":   {other: "aval <command> [flags]"},
	"cli.usage.group.setup":  {other: "Setup"},
	"cli.usage.group.health": {other: "Health"},
	"cli.usage.group.undo":   {other: "Undo"},
	"cli.usage.group.agent":  {other: "Agent tooling"},
	"cli.usage.group.plumb":  {other: "Plumbing"},
	"cli.usage.plumbing":     {other: "Called by hooks and scripts, rarely by hand."},

	"cli.usage.cmd.install":    {other: "link the repo's configs into HOME (TUI; --yes for none)"},
	"cli.usage.cmd.update":     {other: "pull the repo, then re-apply"},
	"cli.usage.cmd.doctor":     {other: "check this machine against the repo"},
	"cli.usage.cmd.sync":       {other: "merge repo-managed settings into the live config"},
	"cli.usage.cmd.rules":      {other: "mirror CLAUDE.md's shared rules into OpenCode's AGENTS.md"},
	"cli.usage.cmd.budget":     {other: "always-on context per agent and hook latency"},
	"cli.usage.cmd.review":     {other: "git-only risk tier and reviewer lenses"},
	"cli.usage.cmd.design":     {other: "design gates for the project in the current directory"},
	"cli.usage.cmd.config":     {other: "show or change aval settings (theme, glyphs, motion)"},
	"cli.usage.cmd.restore":    {other: "list or restore backup runs"},
	"cli.usage.cmd.uninstall":  {other: "remove aval's links and restore what they displaced"},
	"cli.usage.cmd.dev":        {other: "sandboxed HOME for developing aval"},
	"cli.usage.cmd.hook":       {other: "agent hooks and the git pre-push gate"},
	"cli.usage.cmd.statusline": {other: "Claude Code status line"},
	"cli.usage.cmd.tmux":       {other: "tmux, Ghostty and zsh helpers"},
	"cli.usage.cmd.macos":      {other: "macOS defaults and LaunchAgents"},
	"cli.usage.cmd.reap":       {other: "end orphaned test processes"},
	"cli.usage.cmd.kiri":       {other: "make Docker ready on Kiri"},
	"cli.usage.cmd.docker":     {other: "safe Docker prunes on the kiri context"},
	"cli.usage.cmd.deck":       {other: "turn a plan into a PDF slide deck, rendered locally"},
	"cli.usage.cmd.pdf":        {other: "scaffold PDF report projects and their brand assets"},
	"cli.usage.cmd.release":    {other: "cut a release"},
	"cli.usage.cmd.version":    {other: "version, commit, build date and channel"},
	"cli.usage.cmd.help":       {other: "show this help"},

	"cli.usage.flags":      {other: "Global flags:"},
	"cli.usage.flag.repo":  {other: "the aval repository to use"},
	"cli.usage.flag.theme": {other: "colour theme for this run (aval, terminal, mono, catppuccin, ...)"},
	"cli.usage.flag.plain": {other: "monochrome, ASCII glyphs and no motion"},
	"cli.usage.flag.lang":  {other: "language for human output: auto, en or es"},
	"cli.usage.settings": {other: "Settings: theme, background, glyphs, motion, update_check, language. Precedence\n" +
		"is flag, then AVAL_* environment, then ~/.config/aval/config; language then\n" +
		"follows the locale (LC_ALL, LC_MESSAGES, LANG). NO_COLOR turns colour off."},
	"cli.usage.repo": {other: "The repository is found from, in order: --repo, $AVAL_REPO, the location\n" +
		"'aval install' recorded in ~/.config/aval/config, then the current directory.\n" +
		"First time: clone the repo, cd into it and run 'aval install'."},
	"cli.usage.first_run": {other: "Not set up yet. Run: aval install"},

	// doctor
	"cli.doctor.col.status":  {other: "STATUS"},
	"cli.doctor.col.check":   {other: "CHECK"},
	"cli.doctor.col.message": {other: "MESSAGE"},
	"cli.doctor.status.ok":   {other: "OK"},
	"cli.doctor.status.warn": {other: "WARN"},
	"cli.doctor.status.fail": {other: "FAIL"},
	"cli.doctor.fixable":     {other: "(fixable: aval doctor --fix)"},
	"cli.doctor.count.ok":    {one: "{count} ok", other: "{count} ok"},
	"cli.doctor.count.warn":  {one: "{count} warn", other: "{count} warn"},
	"cli.doctor.count.fail":  {one: "{count} fail", other: "{count} fail"},
	"cli.doctor.removed":     {other: "removed broken link {path}"},
	"cli.doctor.synced":      {other: "synced settings and agent rules"},
}
