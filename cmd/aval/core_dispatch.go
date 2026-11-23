package main

// runStandalone runs the commands that need no repository and no update
// check: the skill tooling and dev. handled is false for any other command,
// which dispatch then resolves as before.
func runStandalone(name string, sub []string, repoFlag string, c cmdIO) (handled bool, err error) {
	switch name {
	case "kiri":
		return true, runKiri(sub)
	case "docker":
		return true, runDocker(sub)
	case "pdf":
		return true, runPDF(sub, c.stdout)
	case "deck":
		return true, runDeck(sub, c.stdout, realDeckDeps())
	case "dev":
		return true, runDev(repoFlag, sub)
	}
	return false, nil
}
