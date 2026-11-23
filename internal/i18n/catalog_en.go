package i18n

// english is the source of truth: every key is defined here first, in one
// part per surface (catalog_ui_en.go, catalog_cli_en.go).
var english = join(englishCore, englishUI, englishCLI)

var englishCore = catalog{
	"config.invalid_language": {other: "language = {value} is not a valid language. Valid: {valid}."},
	"config.did_you_mean":     {other: "Did you mean {suggestion}?"},
}
