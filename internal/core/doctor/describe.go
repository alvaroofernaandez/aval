package doctor

type description struct{ title, why, how string }

// descriptions explain every check id for the hub's lower pane. howToFix is
// the exact command where one exists, otherwise the manual step.
var descriptions = map[string]description{
	"links": {"Manifest links",
		"Every destination in install.manifest should be a symlink into the repo. A missing or replaced link means that agent is not running the repo's config.",
		"aval install"},
	"broken-links": {"Dangling links",
		"Dangling links in the agent directories point at files that no longer exist, usually a skill removed from the repo.",
		"aval doctor --fix"},
	"hooks-resolve": {"Hook commands resolve",
		"Every command named by a hook in ~/.claude/settings.json must exist, or the hook fails silently on each event.",
		"aval sync, or install the missing program and re-run aval doctor"},
	"hooks-retired": {"No retired hooks",
		"Hooks that call retired SDD or skill-registry commands run nothing useful and slow every event.",
		"aval sync"},
	"hooks-drift": {"Hooks match the repo",
		"The live hooks should equal the ones the repo defines. Drift means a hook was added, removed or edited by hand.",
		"aval sync"},
	"agents": {"Agent models",
		"Agents must pin a model alias (opus, sonnet, haiku or inherit). A dated model id stops working when it is retired.",
		"Edit the model: line in the listed ~/.claude/agents file to an alias"},
	"skills": {"Skills are complete",
		"Each skill needs a SKILL.md with name and description in its frontmatter, or the agent cannot discover it.",
		"Add SKILL.md with name and description to the listed skill, or remove the directory"},
	"plugins": {"Plugins installed",
		"A plugin enabled in settings.json but missing from installed_plugins.json cannot load.",
		"claude plugin install <name>, or disable it in ~/.claude/settings.json"},
	"settings-drift": {"Settings match the repo",
		"The keys aval manages in ~/.claude/settings.json should equal the repo's. Drift means a hand edit or a stale sync.",
		"aval sync"},
	"overlay": {"Private overlay",
		"The optional private overlay holds settings and links that stay out of the repo. This reports what it contains.",
		"Nothing to fix; create the directory to use it (aval config shows the path)"},
	"overlay-links": {"Overlay links",
		"Each link in the overlay's links.manifest should point at an existing source and be in place.",
		"aval sync, or fix links.manifest in the private overlay"},
	"config": {"aval config file",
		"Bad values and unknown keys in aval's config file are ignored, so the setting you meant does not apply.",
		"Edit the file at the line reported; aval config lists valid keys"},
	"deps": {"External tools",
		"aval, its hooks and its skills call tools it does not ship. A missing blocking tool (git) stops aval; a missing degrading one turns a feature off.",
		"Install the missing tool (brew bundle --file=Brewfile on macOS); aval deps lists what uses each one"},
	"sec-permissions-broad": {"Broad permissions",
		"Allow rules such as a bare Bash or a wildcard let the agent run anything without asking.",
		"Narrow the rule in ~/.claude/settings.json, or aval doctor --security --allow sec-permissions-broad"},
	"sec-hooks-remote-exec": {"Hooks fetch and run code",
		"A hook that pipes a download into a shell runs whatever the server sends, on every event.",
		"Vendor the script into the repo, or aval doctor --security --allow sec-hooks-remote-exec"},
	"sec-hooks-npx-latest": {"Hooks run unpinned npx",
		"npx with no version, or @latest, runs a different package whenever upstream publishes.",
		"Pin a version in the hook command, or aval doctor --security --allow sec-hooks-npx-latest"},
	"sec-hooks-outside": {"Hooks outside known locations",
		"A hook script outside the repo and ~/.claude/hooks is not versioned or reviewed with the rest.",
		"Move the script under ~/.claude/hooks, or aval doctor --security --allow sec-hooks-outside"},
	"sec-mcp-inline-secrets": {"Inline MCP secrets",
		"A token written into an MCP server definition is stored in plain text and can leak with the file.",
		"Move the value to an environment variable, or aval doctor --security --allow sec-mcp-inline-secrets"},
}

// Describe explains a check id for the doctor view. An unknown id returns
// empty strings.
func Describe(id string) (title, why, howToFix string) {
	d := descriptions[id]
	return d.title, d.why, d.how
}
