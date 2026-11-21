---
commands:
  - 'git( -C \S+)? commit'
  - 'gh pr (create|edit)'
---
Conventional commits only, in English. Never add a Co-Authored-By trailer, a
"Generated with" line or any other AI attribution to a commit message or PR
body (the harness `attribution` setting already blanks them; do not re-add by
hand). Stage per work unit, never `git add -A` blindly; never push unasked.
