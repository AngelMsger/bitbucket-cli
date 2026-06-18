---
name: bitbucket
version: 0.11.1
description: "Use Bitbucket as a code-hosting backend for coding agents. Browse repositories and source files at any ref, drive pull request review and merge workflows, see per-file diffs and diffstats, check mergeability and CI build status, fetch a PR into a local git checkout, post inline review comments, resolve or reopen comment threads, triage and respond to received review comments (with resolution / task status and --unresolved filters), and preview every write with --dry-run or lock the session with read-only mode. Supports Bitbucket Cloud and Data Center / Server. Use when the user mentions Bitbucket, a PR or pull-request URL or ID, repository browsing, file content at a ref, code review, responding to or addressing PR review comments, resolving a comment thread or task, approve/decline/merge a PR, asks to read a diff, or wants a dry-run / read-only / safe-mode session."
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
  `comment add --inline` to reply, and `pr approve` / `pr merge`. If missing
  intent or background genuinely blocks the review, post a clarifying comment
  to the author and defer just the blocked items until they reply. See
  `references/reviewing-locally.md` for the full decision tree and the
  "ask the author" protocol.
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
  add `--inline <path>:<line>` for inline review comments. Resolve or reopen a
  thread with `comment resolve <id> --pr <ref>` (`--unresolve` to reopen); on
  Data Center this also completes/reopens a task.
- **Repository / branches / commits** — see `references/reading-repos.md`.
- **Batch writes** — `pr approve`, `pr decline` and `comment delete` take several
  references/IDs in one call, or a single `-` to read them from stdin (e.g.
  `pr inbox --format json | jq -r '.items[].ref' | bitbucket-cli pr approve -`).
  With more than one, output is an `{items, has_more}` aggregate with a per-item
  `ok`/`error`; the run continues past failures and exits non-zero if any failed.

## Agent-facing conventions

- **Update notices on stderr.** When a newer release exists, commands print a
  one-line `{"_notice":{"update":{…}}}` to **stderr** (never stdout, so parsing
  the data is unaffected). `doctor` reports it too. Silence with
  `BITBUCKET_CLI_NO_UPDATE_NOTIFIER=1`.
- **Forgiving flags.** camelCase/snake_case flag names (`--userId`) and a flag
  stuck to its value (`--limit100`) are auto-corrected to the canonical form when
  it is a real flag; each fix is echoed as a `{"_notice":{"corrections":[…]}}`
  line on stderr. Prefer the canonical `--kebab-case value` form regardless.
- **Literal `\n` in body flags.** The shell does not expand `\n` inside double
  quotes, so the free-text body flags (`--content`, `--description`, `--message`)
  decode the `\n` `\r` `\t` `\\` whitelist into real characters before sending —
  `--content "a\n\nb"` posts two paragraphs, not a literal `a\n\nb` — and echo an
  `{"_notice":{"corrections":[{"kind":"escape",…}]}}` line on stderr. Use the
  `--content-file` / `--description-file` flags for exact bytes (read verbatim,
  no decoding). See `references/commenting.md` › "Multi-line bodies".

See the topic references in `references/` for details and decision trees.

## AI attribution (agent writes)

When you, as an AI agent, write to Bitbucket on the user's behalf, mark the content as
AI-authored with a link back to the tool. This applies **only** to agent-driven
writes — PR comments (`comment add`, incl. `--inline` / `--reply-to`) and PR
descriptions (`pr create` / `pr update`) — never to anything a human authored.

Comments are CommonMark, where `[`/`]` are link syntax. Prefix with a clickable
**`[AI]`** tag whose brackets stay visible by **doubling the outer brackets** —
`[[AI]](url)`, **not** `[AI](url)` (the single-bracket form drops the brackets and
renders a plain `AI`):

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --content "[[AI]](https://angelmsger.github.io/bitbucket-cli/) XXX 有 YYY 问题。"
```

Write the rest of the text in the **user's language**; keep the `[AI]` label and the
URL `https://angelmsger.github.io/bitbucket-cli/` constant. For PR descriptions use the
attribution line in `references/pr-workflows.md`. See also `references/commenting.md`.

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
