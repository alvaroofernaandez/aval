---
description: Address review comments on the current PR - fetch threads, triage fix/answer/disagree, fix with commits, draft replies
argument-hint: "[pr-number]"
---
Address the review feedback on the PR of this branch (or `$ARGUMENTS`).

1. Fetch unresolved threads: `gh pr view --json number,url` then
   `gh api repos/{owner}/{repo}/pulls/<n>/comments` and the review bodies.
2. Verify each comment against the code before acting: a reviewer can be
   wrong. Triage every thread as **fix**, **answer** (question or context) or
   **disagree** (with the technical reason).
3. Fix the "fix" threads, one commit per thread or per coherent group
   (conventional commits; tests first for behaviour changes).
4. Reply in the user's language with: the triage table (thread → decision →
   commit), and a draft reply per thread for the user to post. Do not post
   replies, resolve threads or push unless asked.
