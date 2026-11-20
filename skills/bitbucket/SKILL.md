---
name: bitbucket
version: 0.21.0
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

## Workflow precedence

Separate non-negotiable tool invariants from default policies that a user or
upper-layer team/task Skill may explicitly override:

- **Invariants (never override):** honor explicit read-only intent
  (`BITBUCKET_CLI_READ_ONLY=1` or a read-only/report-only request — never add
  `--allow-writes`); refresh `source.commit`, votes, and threads before any
  write-back (never vote on a stale commit); diff against the real `merge-base`
  (`destination.commit...source.commit`), not a stale local default branch;
  validate inline anchors and keep AI attribution truthful (agent text carries
  attribution, verbatim human text does not); reconcile state on HTTP
  403/409/timeout or uncertain writes instead of fabricating outcomes.
- **Default policies (explicitly overridable):** report-only when no upper
  policy authorizes writes (honor explicit auto-write scope); per-item
  human-reply draft confirmation for interactive defaults (allow bounded
  automated replies when explicitly authorized); collaboration hold on another
  human's `Needs Work` (allow withholding a vote / leaving human disagreement);
  no duplicate confirmations on existing comments (allow explicit confirmation
  on adopted blockers).

## Getting started

```sh
bitbucket-cli config init                # interactive setup (humans: add --pretty for the TUI)
bitbucket-cli doctor                     # verify connectivity + auth
bitbucket-cli whoami
```

Auth, environment and contexts: `references/getting-started.md`. `--pretty` is
human-only (TUI, colorized JSON) and errors without a TTY; agents never pass it.

## Core workflows

- **Create a PR or update its description** — settle target first: repo from git
  remote, source from current branch, `pr list --source <branch>` to catch an
  existing PR (a hit means update, not create), branch pushed. Read diffstat
  before diff. Follow user requirements, then repo template, or
  [Writing PR descriptions](references/writing-pr-descriptions.md). Apply
  [description attribution](#ai-attribution-agent-writes). Details:
  [Creating](references/pr-workflows.md#creating),
  [Updating descriptions](references/pr-workflows.md#updating-descriptions).
- **Review a PR** — follow [Reviewing a pull request](references/reviewing-locally.md):
  record authorized scope; read `pr get <ref> --scope full`, `pr status`,
  `pr files`, `pr diff`, `pr threads`; pin `source.commit` / `destination.commit`
  and review against their merge-base; verify each finding and existing claims;
  apply verdict rules; refresh before any authorized write (or submit atomically
  via `pr review <ref> --expected-source-commit <sha>`). Several PRs:
  [Reviewing batches](references/reviewing-batches.md).
- **Recover a blocked PR action** — see
  [PR permissions and recovery](references/pr-permissions.md). Separate server
  403s, local read-only mode, and inaccessible credentials. For an authorized
  action blocked by a DC read-only PAT, make one attempt in the same user's
  browser session when available, then verify.
- **Respond to received review comments** — list open threads with
  `pr threads <ref> --unresolved` (or `--comment <id>`); discover authored PRs
  with `pr inbox --role author`. Locate code, judge validity, propose a fix +
  verification, and draft a reply. Read-only by default; see
  `references/responding-to-review-comments.md`.
- **Collect review activity for a worklog** — narrow with
  `pr inbox --role any --state MERGED --closed-since 48h` (DC; repeat for
  declined, query open separately), pipe `.items[].ref` into
  `pr activity - --actor me --kind approval,comment,decline` with `--since` or
  `--from`/`--to`. System comments are excluded unless `--include-system`. Other
  users in a known repo: `pr list --author/--reviewer <user>` (`--all` on DC).
  See `references/pr-workflows.md`.
- **Browse source / repos / comments** — `file list|get|tree`
  (`references/files.md`), `repo ...` (`references/reading-repos.md`),
  `comment add|resolve|react` (`references/commenting.md`).
- **Batch operations** — `pr get`, `pr status`, `pr threads`, `pr activity`,
  `pr approve`, `pr decline`, and `comment delete` accept several refs/IDs or
  `-` from stdin. Multi-item runs return `{items, has_more}` with per-item
  `ok`/`result`/`error`, continue past failures, and exit non-zero if any fail.
  Batch writes only for individually reviewed, authorized targets.

## Commands

A PR ref is `<workspace>/<repo>/<id>` or a PR URL; a repo ref is
`<workspace>/<repo>` or a repo URL. On Data Center, `<workspace>` is the project key.

```
bitbucket-cli pr list <workspace>/<repo>     # PRs in a repo (--author/--reviewer/--state)
bitbucket-cli pr inbox                       # my PRs across repos (--role, --my-vote)
bitbucket-cli pr get <ref>...                # one or more PRs ('-' stdin; --scope)
bitbucket-cli pr status <ref>...             # merge readiness, CI, and my_review ('-' stdin)
bitbucket-cli pr files <ref>                 # changed files (diffstat) — budget before diffing
bitbucket-cli pr diff <ref>                  # unified diff (--path, --line-numbers, --commentable)
bitbucket-cli pr commits <ref>               # commits included in the PR
bitbucket-cli pr threads <ref>...            # review threads ('-' stdin; --unresolved, --comment)
bitbucket-cli pr activity <ref>...           # activity timeline ('-' reads refs from stdin)
bitbucket-cli pr fetch <ref>                 # verify remote + print (or --exec) git fetch (--force-remote)
bitbucket-cli pr checkout <ref>              # verify remote + print (or --exec) fetch + checkout
bitbucket-cli pr create                      # open a PR (--repo --source --target --title)
bitbucket-cli pr update <ref>                # edit --title / --description / --reviewer
bitbucket-cli pr review <ref>                # pre-validate + submit comments & verdict (--verdict, --expected-source-commit, --input)
bitbucket-cli pr approve <ref>...            # approve one or more PRs ('-' stdin)
bitbucket-cli pr unapprove <ref>             # withdraw an approval
bitbucket-cli pr request-changes <ref>       # request-changes / needs-work vote (--withdraw)
bitbucket-cli pr decline <ref>...            # close without merging (needs --yes)
bitbucket-cli pr merge <ref>                 # merge (--strategy, needs --yes)
bitbucket-cli comment list --pr <ref>        # PR comments (--unresolved, --tasks; includes is_self, is_ai_attributed, author_kind)
bitbucket-cli comment add --pr <ref>         # comment (--inline <path>:<line>, --reply-to <id>)
bitbucket-cli comment update <comment-id>    # edit a comment (--pr <ref>)
bitbucket-cli comment resolve <comment-id>   # resolve a thread (--unresolve reopens; --pr <ref>)
bitbucket-cli comment react <comment-id>     # --emoji <name> (--remove; --pr <ref>)
bitbucket-cli comment delete <comment-id>... # delete one or more comments (needs --yes)
bitbucket-cli file list|get|tree <repo-ref>  # browse files at --ref (--path, -o)
bitbucket-cli repo list|get|create|fork|delete|clone-url   # manage repositories
bitbucket-cli git -- <git-args>
bitbucket-cli branch list|get|create|delete <repo-ref>     # manage branches (delete needs --yes)
bitbucket-cli tag list|get <repo-ref>        # tags
bitbucket-cli commit list|get|compare <repo-ref>           # commits
bitbucket-cli workspace list|get <name>      # workspaces (Cloud) / projects (DC)
bitbucket-cli user list|get <selector>       # discover users
bitbucket-cli whoami                         # authenticated user (alias: user me)
bitbucket-cli config init|show|path|set-context|get-contexts|use-context|delete-context
bitbucket-cli auth guide|reuse|login|logout|status         # credentials
bitbucket-cli doctor                         # diagnose setup + connectivity
bitbucket-cli skill install|status|path|show|uninstall     # manage companion Skill
```

Writes accept `--dry-run`. `--allow-writes` is a **root-level flag**
(`bitbucket-cli --allow-writes <subcommand> ...`): pass it only when local
read-only config is active **and** the current action is authorized — never
append it unconditionally. See `references/safety-modes.md` and
[Git authentication](references/git-transport.md).

## Agent-facing conventions

- **NDJSON pagination on stderr.** `--format ndjson` prints rows on stdout;
  when more pages exist, stderr emits
  `{"_notice":{"pagination":{"next":"<opaque>","has_more":true},...}}`. Pass
  `next` verbatim as `--cursor`, even after a filtered empty page. `--all`
  collects every page with no notice.
- **Projected fields are record-relative.** `--fields` applies to each item
  inside `{items, next, has_more}` (`--fields id,title`, never `items.id`).
  Nested paths emit literal dotted keys (`repository.workspace`).
- **Skill handshake — set `BITBUCKET_CLI_SKILL=0.21.0`.** Export this exact
  value after loading the Skill. `skill status` compares loaded, installed, and
  embedded versions. Silence without loading via `BITBUCKET_CLI_NO_SKILL_HINT=1`.
- **Update notices on stderr.** `{"_notice":{"update":{…}}}` never hits stdout;
  follow `next_steps` (upgrade, `skill install`, reload). Silence with
  `BITBUCKET_CLI_NO_UPDATE_NOTIFIER=1`.
- **Forgiving flags & literal `\n`.** Non-canonical flags and `\n`/`\r`/`\t`/`\\`
  in `--content`/`--description`/`--message` are auto-corrected and logged on
  stderr; use `--content-file`/`--description-file` for exact bytes.

See `references/` for detailed decision trees.

## Replying to people

Replies post under the user's name. Apply
[Replying to people](references/replying-to-people.md): classify both root and
target comment (`is_self`, `is_ai_attributed`, `author_kind`; uncertain means
human). Under the interactive default, show the point, reasoning, and draft and
get per-item approval (explaining why once per session) unless the user or an
upper workflow explicitly authorized bounded automated replies. Do not resolve
human threads or push fixes without authorization.

## AI attribution (agent writes)

**PR descriptions.** Start every agent-written description with this blockquote
and a blank line, in the description's language, keeping literal `[AI]` and the
linked tool name:

```markdown
> [AI] 本 PR 由 AI 通过 [bitbucket-cli](https://angelmsger.github.io/bitbucket-cli/) 协助创建。
```

**Comments** (general, inline, replies). Start with
`[[AI]](https://angelmsger.github.io/bitbucket-cli/) `, then the comment.

On edits, replace existing attribution once. Post human-written or
human-rewritten text verbatim without attribution; approved agent drafts keep
attribution.

## Configuration & credentials (agents)

**Reuse existing config and credentials** from `~/.angelmsger/bitbucket/config.yaml`
and the OS keychain — never run `config init` or `--pretty`.

If an error returns `CREDENTIAL_STORE_INACCESSIBLE`,
`CREDENTIAL_NOT_VISIBLE_OR_MISSING`, or `recovery.scope: "host"`, request host
access and retry the same command once — do not re-initialize inside a sandbox.
Never launch interactive `config init` / `auth login`; if truly missing, ask the
user to run them in their own terminal or export `BITBUCKET_*`. See
`references/getting-started.md`.

## Team service presets and authentication

- `config set-context <name>` is the offline preset entrypoint (`--base-url`,
  `--auth-scheme`, `--credential-url`, `--flavor`, `--activate`, `--overwrite`,
  `--dry-run`); it never copies personal credentials and preserves existing
  values unless `--overwrite`.
- `auth guide` reports the instance credential URL, source, steps, and limits
  without requesting it; never invent URLs.
- After presetting, try `auth reuse` before sign-in (confirm its `account`
  before writes), or direct the user to run `auth login` in their own terminal.
  See [team setup](references/team-setup.md) and
  [reuse](references/getting-started.md#reuse-existing-authentication).
