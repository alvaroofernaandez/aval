# Security policy

## Supported versions

Only the latest release of aval receives security fixes.

## Reporting a vulnerability

Do **not** open a public issue. Use GitHub's
[private vulnerability reporting](https://github.com/alvaroofernaandez/aval/security/advisories/new)
with:

- affected version (`aval --version`) and platform,
- steps to reproduce,
- impact as you understand it.

You will get an acknowledgement within 7 days. Fixes are released as soon as
they are verified, and the advisory is published afterwards with credit unless
you prefer otherwise.

## Scope

aval writes into `$HOME` (symlinks, settings, hooks). Of particular interest:
path traversal or writes outside the declared destinations, secrets leaking
into tracked files or logs, and hook commands that execute untrusted input.
