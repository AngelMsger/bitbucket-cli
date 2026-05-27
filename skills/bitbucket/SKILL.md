---
name: bitbucket
version: 0.3.1
description: "Use Bitbucket as a code-hosting backend for coding agents. Browse repositories and source files at any ref, drive pull request review and merge workflows, see per-file diffs and diffstats, check mergeability and CI build status, fetch a PR into a local git checkout, post inline review comments, and preview every write with --dry-run or lock the session with read-only mode. Supports Bitbucket Cloud and Data Center / Server. Use when the user mentions Bitbucket, a PR or pull-request URL or ID, repository browsing, file content at a ref, code review, approve/decline/merge a PR, asks to read a diff, or wants a dry-run / read-only / safe-mode session."
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

- **Review a PR (with local codebase)** — start with `pr status` (mergeable + CI),
  then `pr files` (diffstat) to budget context, then `pr diff --path <p>` per
  file (or `pr fetch --exec` to bring the PR into your local clone and read
  files directly). Finish with `pr threads` to see inline discussions,
  `comment add --inline` to reply, and `pr approve` / `pr merge`. See
  `references/reviewing-locally.md` for the full decision tree.
- **Browse source at any ref** — `bitbucket-cli file list/get/tree` reads
  directories and files at a branch, tag or commit. See `references/files.md`.
- **Comment** — `bitbucket-cli comment add --pr <ws>/<repo>/<id> --content "<text>"`,
  add `--inline <path>:<line>` for inline review comments.
- **Repository / branches / commits** — see `references/reading-repos.md`.

See the topic references in `references/` for details and decision trees.
