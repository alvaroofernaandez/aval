package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/alvaroofernaandez/aval/internal/core/update"
)

// TestPreviewUpdateOutputIsPinned freezes the bytes `aval update --dry-run`
// prints, so moving the managed-paths logic into core cannot change them.
func TestPreviewUpdateOutputIsPinned(t *testing.T) {
	// Fixed dates make the commit hashes, and so the output, deterministic.
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z")
	repo, _ := upstreamPair(t)

	var out bytes.Buffer
	if err := previewUpdate(context.Background(), update.NewGit(repo), repo, &out, englishTr); err != nil {
		t.Fatal(err)
	}
	const want = "Would update c62bccb → 33554b2 (1 commit(s))\n\nManaged paths that change:\nconfig/claude/agents/a.md | 2 +-\n install.manifest          | 1 +\n 2 files changed, 2 insertions(+), 1 deletion(-)\n  + manifest entry config/claude/hooks -> .claude/hooks\n\nDry run: nothing was changed.\n"
	if out.String() != want {
		t.Errorf("previewUpdate output changed:\n%q", out.String())
	}
}
