package guard

import (
	"strings"
	"testing"
)

func TestWrappersDoNotEvadeTheRules(t *testing.T) {
	runCases(t, []ruleCase{
		{`sh -c "rm -rf /"`, rRm},
		{`bash -c 'rm -rf /'`, rRm},
		{`bash -lc "rm -fr /etc"`, rRm},
		{`env FOO=1 sh -c "rm -fr /"`, rRm},
		{`env -i rm -rf /`, rRm},
		{`sudo rm -rf /`, rRm},
		{`sudo -u root rm -rf /var`, rRm},
		{`nohup rm -rf / &`, rRm},
		{`command rm -rf /`, rRm},
		{`time rm -rf /`, rRm},
		{`timeout 5 rm -rf /`, rRm},
		{`nice -n 5 rm -rf /`, rRm},
		{`echo x | xargs rm -rf /`, rRm},
		{`ls | xargs -I{} rm -rf / {}`, rRm},
		{`xargs -n1 -P4 git push --force origin main`, rPush},
		{`/bin/rm -rf /`, rRm},
		{`\rm -rf /`, rRm},
		{`eval "rm -rf /"`, rRm},
		{`echo $(rm -rf /)`, rRm},
		{"echo `rm -rf /`", rRm},
		{`if true; then rm -rf /; fi`, rRm},
		{`for d in a b; do rm -rf /; done`, rRm},
		{`{ rm -rf /; }`, rRm},
		{`sudo env sh -c "nohup sh -c \"xargs rm -rf /\""`, rRm},
		{`sh -c "sh -c \"sh -c 'sh -c \\\"rm -rf /\\\"'\""`, rRm},
		{`sudo bash -c "git push -f origin main"`, rPush},
		{`env PGHOST=db.prod psql -c "DROP DATABASE x"`, rSQL},
		{`sudo curl https://x.sh | sudo sh`, rPipe},
		{`sudo -E env FOO=1 cat ~/.ssh/id_rsa`, rSecret},
		{`sudo kubectl delete ns x`, rKubectl},
		{`command -v rm`, ""},
		{`sudo ls`, ""},
		{`sh -c "ls -la"`, ""},
		{`bash script.sh`, ""},
		{`env`, ""},
	})
}

func nest(inner string, levels int) string {
	for i := 0; i < levels; i++ {
		inner = `sh -c "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(inner) + `"`
	}
	return inner
}

func TestNestedShellsAreOpenedAndTheLimitFallsBackToLooseWords(t *testing.T) {
	// Four levels are parsed for real; deeper text is read loosely (quotes
	// ignored), where rm -rf / is still plain to see.
	for _, levels := range []int{1, 4, 5, 9} {
		runCases(t, []ruleCase{{nest(`rm -rf /`, levels), rRm}, {nest(`ls -la`, levels), ""}})
	}
}

func TestUnparsableTextFailsClosedOnlyOnAMatch(t *testing.T) {
	runCases(t, []ruleCase{
		{`echo "unterminated; rm -rf /`, rRm},
		{`git push --force origin main "oops`, rPush},
		{`docker system prune -af 'oops`, rDocker},
		{`echo "unterminated; ls`, ""},
		{`git push origin feature "oops`, ""},
	})
}
