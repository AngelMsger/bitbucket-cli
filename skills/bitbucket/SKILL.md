---
name: bitbucket
version: 0.18.2
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
contexts. `--pretty` is human-only (TUI, colorized JSON) and errors without a
TTY; agents never pass it.

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
- **Review a PR** — follow the checklist in
  [Reviewing a pull request](references/reviewing-locally.md): record what the
  request authorizes; read `pr get <ref> --scope full`, `pr status`, `pr files`,
  then `pr diff` and `pr threads`; pin `source.commit` / `destination.commit`
  and review against their merge-base (a detached worktree when local checks
  need isolation); verify each finding, including claims in existing comments;
  apply the verdict table; refresh revisions and reviewer states before any
  authorized comment or vote. Routine summaries and limits go in the user
  report. A review may finish without comments or a vote; an authorized
  approval needs no companion comment. Several related PRs:
  [Reviewing batches](references/reviewing-batches.md).
- **Recover a blocked PR action** — read
  [PR permissions and recovery](references/pr-permissions.md). Distinguish a
  server permission rejection from local read-only mode and inaccessible
  credentials. A Data Center read-only PAT can read a PR while being unable to
  approve, decline, or vote on it. For an authorized action the server rejected,
  make one attempt in the same user's existing browser session when available,
  then verify the result.
- **Respond to received review comments** — when the user is the PR *author*
  addressing feedback. Usually they hand you a specific PR (ref or URL) — list its
  open threads with `pr threads <ref> --unresolved`, or target a single thread the
  user named with `pr threads <ref> --comment <id>`. (No PR in hand? Discover with
  `pr inbox --role author`.) For each thread, locate the code (local checkout
  preferred for real verification), judge whether the comment is valid, propose a
  fix + verification, and draft a reply. Read-only analysis by default; replies
  follow the human-reply gate below. See
  `references/responding-to-review-comments.md` for the triage flow.
- **Collect review activity for a worklog** — narrow candidates with
  `pr inbox --role any --state MERGED --closed-since 48h` (Data Center; repeat
  for declined, query open PRs separately), pipe `.items[].ref` into
  `pr activity -` with `--actor me`, `--kind approval,comment,decline`, and a
  bounded `--since` or `--from` / `--to` window. Filtered queries drop recognized
  system comments unless `--include-system` is set. Inbox discovery covers only
  the authenticated user; for another user in a known repository use
  `pr list --author/--reviewer <user>` (`--all` on Data Center). See
  `references/pr-workflows.md`.
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
  evidence. With more than one, output is an `{items, has_more}` aggregate with a
  per-item `ok`/`error`; the run continues past failures and exits non-zero if
  any failed.

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
bitbucket-cli pr request-changes <ref>       # request-changes / needs-work vote (--withdraw removes it)
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

- **Follow NDJSON pagination on stderr.** `--format ndjson` emits only item rows
  on stdout. When more pages exist, stderr includes a compact
  `{"_notice":{"pagination":{"next":"<opaque>","has_more":true},"next_steps":["Pass next as --cursor to retrieve the next page."]}}`.
  Pass `next` verbatim as `--cursor`, even when a filtered page contains no rows.
  Projection preserves this notice; completed pages emit none. `--all` collects
  every page before rendering and emits no continuation notice. Start with a
  bounded page and follow only as far as the task requires.

- **Treat projected fields as record-relative.** JSON list commands keep the
  `{items, next, has_more}` envelope, but `--fields` applies to each item: use
  `--fields id,title,repository`, never `--fields items.id,items.title`.
  Selecting a nested path emits a literal dotted key (`--fields
  repository.workspace` produces `{"repository.workspace":"myws"}`), so
  either select the containing object for normal jq access or read the flat key
  as `.["repository.workspace"]`. Inspect `.items[0]` before composing a
  longer pipeline.
- **Skill handshake — set `BITBUCKET_CLI_SKILL=0.18.2`.** Once you have loaded
  this Skill, export that exact value in the environment used to run the CLI.
  The CLI compares it with the embedded Skill version and emits a structured
  stderr notice when the Skill is missing, old, or uses the legacy unversioned
  handshake. `bitbucket-cli skill status` reports loaded, installed, and
  embedded versions. To suppress the notice without loading the Skill, set
  `BITBUCKET_CLI_NO_SKILL_HINT=1`.
- **Update notices on stderr.** When a newer release exists, commands print a
  one-line `{"_notice":{"update":{…}}}` to stderr, never stdout. Follow every
  `next_steps` entry: upgrade the CLI, run `bitbucket-cli skill install`, then
  reload the agent context. Silence them with `BITBUCKET_CLI_NO_UPDATE_NOTIFIER=1`.
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

A reply is posted under the user's name, so a comment a person wrote is
answered by that person. Before replying, apply
[Replying to people](references/replying-to-people.md): classify the root
author and the message being answered (uncertain means human); for a human
reply, show the point, your reasoning, and the concrete draft, and get per-item
approval, explaining why once per session. Bot or agent replies use existing
authorization. Do not resolve human threads or push fixes without authorization.

## AI attribution (agent writes)

Mark agent-written Bitbucket content — PR comments (`comment add`, including
`--inline` / `--reply-to`) and PR descriptions (`pr create` / `pr update`) —
with a clickable `[AI]` tag whose brackets stay visible:
`[[AI]](https://angelmsger.github.io/bitbucket-cli/)` (double the outer brackets;
`[AI](url)` renders as plain `AI`). Write the rest in the user's language; keep
the label and URL constant. Text the human wrote or rewrote is posted verbatim
**without** the marker. Details and the comment example are in
[Commenting](references/commenting.md#ai-attribution-agent-writes); the PR
description form is in `references/pr-workflows.md`.

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
  offline installer entrypoint (`--base-url`, `--auth-scheme`, `--credential-url`,
  `--flavor`, `--activate`, `--overwrite`, `--dry-run`). Presets never copy a
  personal username or secret from the environment; conflicts preserve existing
  values unless explicitly overwritten. `BITBUCKET_AUTH_SCHEME` and
  `BITBUCKET_CREDENTIAL_URL` complement the existing service variables.
- `auth guide` returns the instance's credential page, its source, navigation
  steps, and limitations; links are hints, not evidence of server capabilities.
  Do not invent a token URL or assume ingestion credentials authorize queries.
- Once a service is preset, direct the member to run `auth login` in their own
  terminal; never ask for secrets in chat. Non-interactive environments use
  transient credential variables. A server/context mismatch requires selecting
  or creating a matching context; a partial login write error names what was
  stored and how to recover.

See [team setup](references/team-setup.md) for output fields, conflict
semantics, credential URL overrides, and failure recovery.


## Reuse existing authentication

Before repeating login, preview `bitbucket-cli --use-context <target> auth reuse
--dry-run`, then apply. Keep the separate `auth status` check. See
[reuse and ambiguity recovery](references/getting-started.md#reuse-existing-authentication).
