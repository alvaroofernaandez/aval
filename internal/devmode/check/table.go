package check

// The pinned tools. This is the one place a version lives: the table below,
// the install hints and --list read it, and tests/workflows_test.go requires
// ci.yml's setup and install steps to carry the same values. Nothing is
// "latest".
const (
	GoVersion         = "1.25"
	NodeVersion       = "22"
	ActionlintVersion = "v1.7.12"
	GitleaksVersion   = "v8.30.1"
	AgnixVersion      = "0.61.0"
)

// Steps is the table: the source of truth for what "green" means, run in this
// order by `aval dev check` locally and in CI.
var Steps = []Step{
	{
		Name: "gofmt", Tool: "gofmt", Args: []string{"-l", "."},
		Fast: true, FailOnOutput: true,
	},
	{
		Name: "vet", Tool: "go", Args: []string{"vet", "./..."},
		Fast: true,
	},
	{
		// -race needs cgo and a C toolchain, which a Windows runner does not
		// reliably provide. --fast drops it and passes -short, the repo's switch for
		// its slow end-to-end cases, to stay an edit-loop command.
		Name: "test", Tool: "go", Args: []string{"test", "./...", "-race"},
		Fast: true, FastArgs: []string{"test", "./...", "-short"},
		OSArgs: map[string][]string{"windows": {"test", "./..."}},
	},
	{
		// node expands the glob itself (Node 21 and later), so no shell is needed.
		Name: "npm", Tool: "node", Args: []string{"--test", "npm/cli/lib/*.test.js"},
		Fast: true,
	},
	{
		Name: "actionlint", Tool: "actionlint",
		Fast: true,
	},
	{
		// Fetches the pinned agnix through npx, so it needs the network; the
		// agnix test in tests/ skips on Windows for the same reason (.cmd shim).
		Name: "agents", Tool: "npx", Args: []string{"-y", "agnix@" + AgnixVersion, "validate", "."},
		SkipOS: []string{"windows"},
	},
	{
		// Scans every commit, so it needs the full history CI checks out.
		Name: "gitleaks", Tool: "gitleaks", Args: []string{"detect", "--source", ".", "--redact", "--no-banner"},
	},
}
