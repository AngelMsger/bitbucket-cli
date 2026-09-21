---
name: bitbucket
version: 0.16.0
description: "Work with Bitbucket Cloud and Data Center / Server: browse repositories and source, create or update pull requests, review diffs, address review feedback, and manage comments or PR state. Use for Bitbucket repository or PR URLs, code review, inline findings, review threads, approvals, merges, and CLI dry-run or read-only workflows."
metadata:
  requires:
    bins: ["bitbucket-cli"]
  cliHelp: "bitbucket-cli --help; bitbucket-cli pr --help; bitbucket-cli comment --help; bitbucket-cli file --help"
---

# Bitbucket

`bitbucket-cli` reads repositories and manages pull requests on Bitbucket Cloud
and Data Center / Server through one command tree. Select the workflow that
matches the user's request; reviewing a PR does not imply commenting or merging.

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

- **Create a PR or update its description** — settle the target first: repo from
  the git remote, source from the current branch, `pr list --source <branch>` to
  catch an existing PR (a hit makes this an update, not a create), branch pushed.
  Read the diffstat before the diff. Then draft per
  [Writing PR descriptions](references/writing-pr-descriptions.md) — lead with the
  problem and resulting behavior, add review guidance only where it helps, and add
  a compact change outline only when the shape is hard to see from the diff. Keep
  routine check results out of the description. Full sequence:
  [Creating](references/pr-workflows.md#creating),
  [Updating descriptions](references/pr-workflows.md#updating-descriptions).
- **Review a PR** — read [Reviewing a pull request](references/reviewing-locally.md).
  Reuse the known PR, inspect intent, changed files and existing threads, and
  verify local repository and source/base alignment before using worktree files.
  Include functional correctness and significant maintainability problems using
  the guide's evidence and information-placement rules. Default to new,
  actionable, high-confidence findings; reply to an existing
  issue only with new evidence. Keep completion records, passing summaries,
  test procedures and environment limits in the user report. Use PR-level
  comments only when a finding cannot reasonably be anchored to code or the
  user explicitly requests an overall summary. A completed review may leave
  no comments. When approval is authorized and there are no findings, approve
  without a companion comment; PR state changes must stay within that authorization.
- **Recover a blocked PR action** — read
  [PR permissions and recovery](references/pr-permissions.md). Distinguish a
  server permission rejection from local read-only mode and inaccessible
  credentials. A Data Center read-only PAT can read a PR while being unable to
  approve or decline it. For an authorized action, try the same user's existing
  browser session when available, then verify the result.
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
- **Comment** — for review feedback, apply the
  [publication rules](references/reviewing-locally.md#decide-what-to-publish) first.
  `bitbucket-cli comment add --pr <ws>/<repo>/<id> --content "<text>"`,
  add `--inline <path>:<line>` for inline review comments. Resolve or reopen a
  thread with `comment resolve <id> --pr <ref>` (`--unresolve` to reopen); on
  Data Center this also completes/reopens a task.
- **Repository / branches / commits** — browse, create, fork, and delete
  repositories with `repo ...`; see `references/reading-repos.md` for the
  Cloud/DC `repo fork --into/--name` rules.
- **Batch writes** — `pr approve`, `pr decline` and `comment delete` take several
  references/IDs in one call, or a single `-` to read them from stdin. Batch only
  individually reviewed, authorized targets; inbox membership is not approval
  evidence. For example, `bitbucket-cli pr approve myws/myrepo/7 myws/myrepo/8`
  is appropriate after both reviews support approval.
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
- **Skill handshake — set `BITBUCKET_CLI_SKILL=0.16.0`.** Once you have loaded
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

## Replying to people

Before replying, read [Replying to people](references/replying-to-people.md).
Classify both the root author and the message being answered; treat uncertain
authorship as human. For a human reply, show the point, reasoning and concrete
draft for per-item approval, explain why once per session, and reuse approval
already given for that reply. Bot or agent replies use existing authorization.
Keep AI attribution on agent-written replies and do not resolve human threads
or push fixes without authorization.

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
  --content "[[AI]](https://angelmsger.github.io/bitbucket-cli/) An empty request reaches items[0] and panics; handle empty input before indexing."
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
