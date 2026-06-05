---
name: bitbucket
version: 0.7.0
description: "Use Bitbucket as a code-hosting backend for coding agents. Browse repositories and source files at any ref, drive pull request review and merge workflows, see per-file diffs and diffstats, check mergeability and CI build status, fetch a PR into a local git checkout, post inline review comments, triage and respond to received review comments (with resolution / task status and --unresolved filters), and preview every write with --dry-run or lock the session with read-only mode. Supports Bitbucket Cloud and Data Center / Server. Use when the user mentions Bitbucket, a PR or pull-request URL or ID, repository browsing, file content at a ref, code review, responding to or addressing PR review comments, approve/decline/merge a PR, asks to read a diff, or wants a dry-run / read-only / safe-mode session."
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
bitbucket-cli config init                # interactive setup (humans: add --pretty for the TUI)
bitbucket-cli doctor                     # verify connectivity + auth
bitbucket-cli whoami
```

See `references/getting-started.md` for auth schemes, env vars, and config
contexts.

`--pretty` is **human-only** (interactive TUI + colorized JSON) and errors without a
TTY — agents should never pass it.

## Core workflows

- **Review a PR (with local codebase)** — start with `pr status` (mergeable + CI),
  then `pr files` (diffstat) to budget context, then `pr diff --path <p>` per
  file (or `pr fetch --exec` to bring the PR into your local clone and read
  files directly). Finish with `pr threads` to see inline discussions,
  `comment add --inline` to reply, and `pr approve` / `pr merge`. See
  `references/reviewing-locally.md` for the full decision tree.
- **Respond to received review comments** — when the user is the PR *author*
  addressing feedback. Usually they hand you a specific PR (ref or URL) — list its
  open threads with `pr threads <ref> --unresolved`, or target a single thread the
  user named with `pr threads <ref> --comment <id>`. (No PR in hand? Discover with
  `pr inbox --role author`.) For each thread, locate the code (local checkout
  preferred for real verification), judge whether the comment is valid, propose a
  fix + verification, and draft a reply. Read-only analysis by default; post
  replies only after confirmation. See `references/responding-to-review-comments.md`.
- **Browse source at any ref** — `bitbucket-cli file list/get/tree` reads
  directories and files at a branch, tag or commit. See `references/files.md`.
- **Comment** — `bitbucket-cli comment add --pr <ws>/<repo>/<id> --content "<text>"`,
  add `--inline <path>:<line>` for inline review comments.
- **Repository / branches / commits** — see `references/reading-repos.md`.

See the topic references in `references/` for details and decision trees.

## AI attribution (agent writes)

When you, as an AI agent, write to Bitbucket on the user's behalf, mark the content as
AI-authored with a link back to the tool. This applies **only** to agent-driven
writes — PR comments (`comment add`, incl. `--inline` / `--reply-to`) and PR
descriptions (`pr create` / `pr update`) — never to anything a human authored.
Bitbucket comments and PR descriptions are Markdown, so prefix with a Markdown link:

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --content "[AI](https://angelmsger.github.io/bitbucket-cli/) XXX 有 YYY 问题。"
```

Write the rest of the text in the **user's language**; keep the `AI` label and the URL
`https://angelmsger.github.io/bitbucket-cli/` constant. See
`references/commenting.md` and `references/pr-workflows.md`.

## Configuration & credentials (agents)

The user has normally already configured `bitbucket-cli`. **Reuse their existing
config and credentials** from `~/.angelmsger/bitbucket/config.yaml` + the OS keychain
— do not run `config init` to create a fresh setup, and never pass `--pretty`.

If you run inside a **sandbox** that cannot read the user's home or keychain you'll
see a `config` (3) or `auth` (4) error from `doctor` / `auth status`. **Request
elevated permissions / re-run with access to the user's real environment, then retry
— do not give up, and do not re-initialize config inside the sandbox.** Never launch
interactive `config init` / `auth login` yourself (no TTY → they fail fast, and
historically could hang); if credentials are truly missing, ask the user to run
`config init` in their own terminal or to export `BITBUCKET_*` env vars. See
`references/getting-started.md` › "For agents and sandboxes".
