---
name: bitbucket
version: 0.14.2
description: "Use Bitbucket as a code-hosting backend for coding agents. Browse repositories and source files at any ref, create PRs and write concise descriptions, drive review and merge workflows, see per-file diffs and diffstats, check mergeability and CI build status, fetch a PR into a local git checkout, post inline review comments, resolve or reopen comment threads, triage and respond to received review comments, and preview every write with --dry-run or lock the session with read-only mode. Supports Bitbucket Cloud and Data Center / Server. Use when the user mentions Bitbucket, a PR or pull-request URL or ID, creating a PR or editing its description, repository browsing, file content at a ref, code review, responding to or addressing PR review comments, resolving a comment thread or task, approve/decline/merge a PR, asks to read a diff, or wants a dry-run / read-only / safe-mode session."
metadata:
  requires:
    bins: ["bitbucket-cli"]
  cliHelp: "bitbucket-cli --help; bitbucket-cli pr --help; bitbucket-cli comment --help; bitbucket-cli file --help"
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
- Asks to create a pull request, write or update its description, look it up,
  review, comment on, approve, decline, or merge it.
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

- **Create a PR or update its description** — before drafting, read
  [Writing PR descriptions](references/writing-pr-descriptions.md). Lead with
  the problem and resulting behavior; add review guidance only where it helps.
  Keep routine check results out of the description. For preparation and command
  details, see [Creating](references/pr-workflows.md#creating) and
  [Updating descriptions](references/pr-workflows.md#updating-descriptions).
- **Review a PR (with local codebase)** — before using local files, verify that
  the checkout belongs to the PR repo and record its branch, HEAD, and dirty
  state; never assume the current worktree is the PR source. Start with `pr status`
  (mergeable + CI), then `pr files` (diffstat) to budget context, then use
  `pr diff --path <p>` per file (or `pr fetch --exec` to fetch the PR source and
  base locally). Treat the fetched `source_ref` and `review_diff` as
  authoritative; read worktree files only after verifying that HEAD matches the
  source ref and the tree is clean. Finish with `pr threads` to see discussions,
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
  fix + verification, and draft a reply. Read-only analysis by default, and a comment
  a person wrote is answered by that person: classify each thread's author, and post
  a reply to a human-authored thread only once the author has seen the reviewer's
  point and approved that specific reply. See
  `references/responding-to-review-comments.md` for the triage flow and
  `references/replying-to-people.md` for the confirmation gate.
- **Collect review activity for a worklog** — narrow Data Center candidates with
  `pr inbox --role any --state MERGED --closed-since 48h` (repeat for declined;
  query open PRs separately), pipe `.items[].ref` into `pr activity -`, then use
  `--actor me`, `--kind approval,comment,decline`, and a bounded `--since` or
  `--from` / `--to` window. Filtered queries exclude recognized system-generated
  comments by default; use `--include-system` only when auditing the complete
  timeline. Candidate discovery remains scoped to the authenticated user's inbox;
  `--actor <user>` filters known PRs but does not discover every PR involving that
  user. For a known repository, use `pr list --author/--reviewer <user>` (add
  `--all` on Data Center). See `references/pr-workflows.md`.
- **Browse source at any ref** — `bitbucket-cli file list/get/tree` reads
  directories and files at a branch, tag or commit. See `references/files.md`.
- **Comment** — `bitbucket-cli comment add --pr <ws>/<repo>/<id> --content "<text>"`,
  add `--inline <path>:<line>` for inline review comments. Resolve or reopen a
  thread with `comment resolve <id> --pr <ref>` (`--unresolve` to reopen); on
  Data Center this also completes/reopens a task.
- **Repository / branches / commits** — browse, create, fork, and delete
  repositories with `repo ...`; see `references/reading-repos.md` for the
  Cloud/DC `repo fork --into/--name` rules.
- **Batch writes** — `pr approve`, `pr decline` and `comment delete` take several
  references/IDs in one call, or a single `-` to read them from stdin (e.g.
  `pr inbox --format json | jq -r '.items[].ref' | bitbucket-cli pr approve -`).
  With more than one, output is an `{items, has_more}` aggregate with a per-item
  `ok`/`error`; the run continues past failures and exits non-zero if any failed.

## Commands

A PR reference is `<workspace>/<repo>/<id>` or a PR URL; a repo reference is
`<workspace>/<repo>` or a repo URL. On Data Center, `<workspace>` is the project key.

```
bitbucket-cli pr list <workspace>/<repo>     # PRs in a repository (--author/--reviewer/--state)
bitbucket-cli pr inbox                       # PRs involving me across repos (--role reviewer|author|any)
bitbucket-cli pr get <ref>                   # one PR (--scope summary trims the body)
bitbucket-cli pr status <ref>                # merge readiness: mergeable, conflicts, reviewers, CI
bitbucket-cli pr files <ref>                 # changed files (diffstat) — budget context before diffing
bitbucket-cli pr diff <ref>                  # unified diff (--path scopes to one file)
bitbucket-cli pr commits <ref>               # commits included in the PR
bitbucket-cli pr threads <ref>               # review threads by file (--unresolved, --comment <id>)
bitbucket-cli pr activity <ref>...           # activity timeline; '-' reads refs from stdin
bitbucket-cli pr fetch <ref>                 # print (or --exec) git fetch for the PR source + base
bitbucket-cli pr checkout <ref>              # print (or --exec) fetch + checkout of the PR source
bitbucket-cli pr create                      # open a PR (--repo --source --target --title)
bitbucket-cli pr update <ref>                # edit --title / --description / --reviewer
bitbucket-cli pr approve <ref>...            # approve one or more PRs ('-' reads from stdin)
bitbucket-cli pr unapprove <ref>             # withdraw an approval
bitbucket-cli pr request-changes <ref>       # request-changes / needs-work vote (Cloud only)
bitbucket-cli pr decline <ref>...            # close without merging (needs --yes)
bitbucket-cli pr merge <ref>                 # merge (--strategy, needs --yes)
bitbucket-cli comment list --pr <ref>        # PR comments (--unresolved, --tasks)
bitbucket-cli comment add --pr <ref>         # comment (--inline <path>:<line>, --reply-to <id>)
bitbucket-cli comment update <comment-id>    # edit a comment (--pr <ref>)
bitbucket-cli comment resolve <comment-id>   # resolve a thread (--unresolve reopens; --pr <ref>)
bitbucket-cli comment delete <comment-id>... # delete one or more comments (needs --yes)
bitbucket-cli file list <repo-ref>           # directory entries at a ref (--ref, --path)
bitbucket-cli file get <repo-ref>            # raw file contents at a ref (-o writes to a file)
bitbucket-cli file tree <repo-ref>           # recursive file listing under a path
bitbucket-cli repo list <workspace>          # repositories in a workspace / project
bitbucket-cli repo get <repo-ref>            # one repository
bitbucket-cli repo create <slug>             # create a repository
bitbucket-cli repo fork <repo-ref>           # fork (--into / --name)
bitbucket-cli repo delete <repo-ref>         # delete a repository (irreversible; needs --yes)
bitbucket-cli repo clone-url <repo-ref>      # HTTPS or SSH clone URL
bitbucket-cli branch list <repo-ref>         # branches (also: get, create, delete — delete needs --yes)
bitbucket-cli tag list <repo-ref>            # tags (also: get)
bitbucket-cli commit list <repo-ref>         # commits (also: get <hash>, compare)
bitbucket-cli workspace list|get <name>      # workspaces (Cloud) / projects (Data Center)
bitbucket-cli user list|get <selector>       # discover users; resolves --author/--reviewer selectors
bitbucket-cli whoami                         # the user the credentials act as (alias: user me)
bitbucket-cli config init|show|path          # configuration
bitbucket-cli config get-contexts|use-context|delete-context   # named contexts
bitbucket-cli auth login|logout|status       # stored credentials
bitbucket-cli doctor                         # diagnose setup + connectivity
bitbucket-cli skill install|status|path|show|uninstall   # manage the companion Skill
```

Every write above accepts `--dry-run`; see `references/safety-modes.md`.

## Agent-facing conventions

- **Treat projected fields as record-relative.** JSON list commands keep the
  `{items, next, has_more}` envelope, but `--fields` applies to each item: use
  `--fields id,title,repository`, never `--fields items.id,items.title`.
  Selecting a nested path emits a literal dotted key (`--fields
  repository.workspace` produces `{"repository.workspace":"myws"}`), so
  either select the containing object for normal jq access or read the flat key
  as `.["repository.workspace"]`. Inspect `.items[0]` before composing a
  longer pipeline.
- **Skill handshake — set `BITBUCKET_CLI_SKILL=0.14.2`.** Once you have loaded
  this Skill, export that exact value in the environment used to run the CLI.
  The CLI compares it with the embedded Skill version and emits a structured
  stderr notice when the Skill is missing, old, or uses the legacy unversioned
  handshake. `bitbucket-cli skill status` reports loaded, installed, and
  embedded versions. To suppress the notice without loading the Skill, set
  `BITBUCKET_CLI_NO_SKILL_HINT=1`.
- **Update notices on stderr.** When a newer release exists, commands print a
  one-line `{"_notice":{"update":{…}}}` to **stderr** (never stdout, so parsing
  the data is unaffected). Follow every `next_steps` entry: upgrade the CLI,
  run `bitbucket-cli skill install`, then reload the agent context. `doctor`
  reports CLI and Skill status too. Silence update notices with
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

## Replying to people, not to bots

Review is where a team exchanges reasoning. When you answer a human reviewer's
comment on the author's behalf, both sides lose that exchange — so **help the
author answer, do not answer for them.**

- **Classify the thread's root author first** — the `[[AI]](…)` marker in the body,
  or an app/bot `author.type`, means a machine wrote it. Anything else is a person.
- **For a human-authored thread:** state the reason once per session, then go one
  thread at a time — quote the reviewer's point, show the code, give your reasoning,
  and hand over a labeled *draft* for the author to approve or rewrite. One approval
  covers one thread. If the author knowingly asks for bulk replies anyway, comply and
  keep the `[AI]` marker on every one.
- **Never on your own:** `comment resolve` a person's thread, fan `comment add
  --reply-to` across threads in one pass, or push code fixes.
- **Bot or agent counterparts** (linters, CI reporters, another agent's review) do
  not need the per-thread gate — confirm the first write and keep attribution.

Full protocol: `references/replying-to-people.md`.

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

When the human writes or rewrites the text themselves, post it verbatim **without**
the `[AI]` marker — they authored it. For PR descriptions, follow the language and
template rules in [Writing PR descriptions](references/writing-pr-descriptions.md).
For comments, write the rest of the text in the **user's language**; keep the
`[AI]` label and the URL
`https://angelmsger.github.io/bitbucket-cli/` constant. For PR descriptions use the
attribution line in `references/pr-workflows.md`. See also `references/commenting.md`.

## Configuration & credentials (agents)

The user has normally already configured `bitbucket-cli`. **Reuse their existing
config and credentials** from `~/.angelmsger/bitbucket/config.yaml` + the OS keychain
— do not run `config init` to create a fresh setup, and never pass `--pretty`.

If a failure has code `CREDENTIAL_STORE_INACCESSIBLE` or
`CREDENTIAL_NOT_VISIBLE_OR_MISSING`, or its `recovery.scope` is `host`, **request
elevated permissions / re-run the same command with access to the user's real
environment, then retry once — do not re-initialize config inside the sandbox.** Never launch
interactive `config init` / `auth login` yourself (no TTY → they fail fast, and
historically could hang); if credentials are truly missing, ask the user to run
`config init` in their own terminal or to export `BITBUCKET_*` env vars. See
`references/getting-started.md` › "For agents and sandboxes".

## Team service presets and authentication

- Inspect existing configuration and reuse it. `config set-context <name>` is the
  offline installer entrypoint; it accepts `--base-url`, `--auth-scheme`,
  `--credential-url`, `--activate`, `--overwrite`, and `--dry-run`, plus `--flavor`.
- `BITBUCKET_AUTH_SCHEME` and `BITBUCKET_CREDENTIAL_URL` complement the existing
  service variables. Presets never copy a personal username or secret from the
  environment. Conflicts preserve existing values unless explicitly overwritten.
- Run `auth guide` to obtain the current instance's credential page, its source,
  navigation steps, and limitations. Links are hints, not evidence of server
  capabilities. Follow the returned product-specific instructions; do not invent
  a token URL or assume ingestion credentials authorize queries.
- Once a service is preset, direct the member to `auth login` in their terminal
  to save their verified personal identity and secret. Do not ask for secrets in
  chat. In non-interactive environments use transient credential variables.
- Preserve host-keychain recovery for inaccessible credentials. A server/context
  mismatch requires selecting or creating a matching context; a partial login
  write error identifies what was stored and provides recovery steps.

See [team setup](references/team-setup.md) for the output fields, conflict
semantics, credential URL overrides, and failure recovery.
