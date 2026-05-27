---
name: bitbucket
version: 0.1.0
description: "Use Bitbucket as a code-hosting backend for coding agents. Browse repositories, drive pull request review and merge workflows, post inline review comments, and query commits and branches. Supports Bitbucket Cloud and Data Center / Server. Use when the user mentions Bitbucket, a PR or pull-request URL or ID, repository browsing, code review, approve/decline/merge a PR, or asks to read a diff."
metadata:
  requires:
    bins: ["bitbucket-cli"]
---

# Bitbucket

`bitbucket-cli` drives Bitbucket from the terminal. It reads repositories,
walks the full pull-request lifecycle (list → get → diff → comment → approve →
merge), and posts inline review comments. It supports Bitbucket Cloud and
Data Center / Server behind one flavor-agnostic command tree.

## When to use

Trigger this skill when the user:

- Pastes a Bitbucket URL (`https://bitbucket.org/<workspace>/<repo>` or a PR
  permalink), or names a `<workspace>/<repo>[#<id>]` reference.
- Asks to look up, review, comment on, approve, decline, or merge a pull
  request.
- Wants to browse a repository, list branches, query commits, or compare refs.

## Getting started

```sh
bitbucket-cli config init --pretty       # interactive setup
bitbucket-cli doctor                     # verify connectivity + auth
bitbucket-cli whoami
```

See `references/getting-started.md` for auth schemes, env vars, and config
contexts.

## Core workflows

- **Read a PR** — `bitbucket-cli pr get <ws>/<repo>/<id> --scope summary`
  then `--scope diff` for the unified patch, `--scope commits` for the
  contained commits, `--scope activity` for the timeline.
- **Comment** — `bitbucket-cli comment add --pr <ws>/<repo>/<id> --content "<text>"`,
  add `--inline <path>:<line>` for inline review comments.
- **Review & merge** — `bitbucket-cli pr approve <ws>/<repo>/<id>`,
  `bitbucket-cli pr merge <ws>/<repo>/<id> --strategy squash`.
- **Repository / branches / commits** — see `references/reading-repos.md`.

See the topic references in `references/` for details and decision trees.
