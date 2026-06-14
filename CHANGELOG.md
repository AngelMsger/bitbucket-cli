# Changelog

## [Unreleased]

## [0.9.0] - 2026-06-14

### Added

- **Runtime update notice.** Every command — except setup/meta commands like
  `doctor` (which already reports it), `config` and `auth` — now emits a one-line
  `{"_notice":{"update":{…}}}` to **stderr** when a newer release is available,
  backed by a 24h on-disk cache (so at most one command per day touches the
  network, with an ~800ms bound). stdout data is byte-identical; silence it with
  `BITBUCKET_CLI_NO_UPDATE_NOTIFIER=1`.
- **Batch writes.** `pr approve`, `pr decline` and `comment delete` accept
  several references/IDs at once, or a single `-` to read newline-separated items
  from stdin. A single argument behaves exactly as before; with more than one the
  output is an `{items, has_more}` aggregate carrying a per-item `ok`/`error`,
  every item runs even if some fail, and the exit code is non-zero on any failure
  (`--yes` / `--dry-run` apply to the whole batch).
- **`comment resolve` (resolve / reopen a thread).** Mark a PR comment thread
  resolved, or reopen it with `--unresolve` — the `resolved` status surfaced by
  `comment list` / `pr threads` was previously read-only. Works on Bitbucket
  Cloud (dedicated resolve endpoint) and Data Center (comment state), where it is
  also how a task (a BLOCKER-severity comment) is completed or reopened. Bitbucket
  Cloud's separate task objects remain out of scope.
- **Forgiving flag input.** Common argv slips are now corrected before cobra
  parses — camelCase / snake_case flag names (`--userId` → `--user-id`) and a
  flag stuck to its value (`--limit100` → `--limit 100`) — but only when the
  result is a flag the command actually defines, so unknown flags still error as
  usual. Each fix is echoed as a `{"_notice":{"corrections":[…]}}` line on stderr.
- **The skill (0.11.0) now tells reviewing agents to ask the PR author when
  context is missing.** When a gap in intent or background genuinely blocks the
  review, the agent posts a clarifying comment (inline or general, batched,
  AI-attributed), keeps reviewing the unaffected files, defers only the blocked
  items, and on resume checks the thread for the author's reply before
  finishing — instead of guessing or giving up. See
  `references/reviewing-locally.md` › "When you don't understand the PR".

## [0.8.1] - 2026-06-08

### Changed

- **The getting-started banner now prints only at `npm install` (postinstall), not
  on first CLI run.** The first-run banner shipped in 0.8.0 could surface during an
  agent/script invocation (e.g. inside a PTY) and intrude on a command's output, so
  it was removed — the CLI now emits nothing beyond a command's own output. The
  welcome moved to the postinstall script. (Heads-up: npm v7+ hides postinstall
  output by default; run `npm install --foreground-scripts` to see it.)

## [0.8.0] - 2026-06-08

### Fixed

- **Inline comments and `pr diff` now work against Data Center instances that
  return a JSON diff.** Some Bitbucket Data Center deployments serve the PR diff
  endpoint as a JSON hunk model even when the CLI asks for `text/plain`. The diff
  pipeline only understood unified-diff text, so against those servers every
  `comment add --inline` failed with a misleading `INLINE_LINE_NOT_IN_DIFF` /
  "no new-side lines are part of the diff", and `pr diff --line-numbers` printed
  raw JSON. The CLI now parses whichever format the server returns into one
  structured diff model (the segment type gives Data Center the authoritative
  ADDED/REMOVED/CONTEXT classification, retiring the old CONTEXT guess), so inline
  anchors resolve and `pr diff` renders readable text regardless of the wire
  format.
- **A diff the CLI cannot parse now fails as `DIFF_PARSE_FAILED`**, a
  parse/compatibility error carrying a snippet of the response, instead of
  masquerading as a bad inline line number — so an agent stops probing anchors and
  falls back to a general comment.

### Added

- **`pr diff --commentable`** lists, per file, the new-side and old-side line
  numbers that accept an inline comment, so valid `--inline <path>:<line>` anchors
  can be picked up front instead of probed one at a time.
- **First-run getting-started banner (npm).** The first time `bitbucket-cli` runs
  in an interactive terminal, it prints a one-time banner pointing at
  `config init --pretty` and `skill install` plus a couple of everyday commands. It
  writes only to stderr, is shown once (recorded by a marker file), and is skipped
  for non-TTY / CI / agent use, so it never pollutes JSON output or scripted runs.
  (A `postinstall` banner was avoided: npm hides postinstall stdout by default.)

### Skill

- **AI attribution now renders the `[AI]` tag with its brackets visible.** Comments
  are CommonMark, where the previous single-bracket `[AI](url)` has its brackets eaten
  by the link syntax and renders as a plain `AI`. The guidance now uses the
  double-bracket form `[[AI]](url)`, whose link text is literally `[AI]`, so the tag
  renders as a clickable **[AI]** with the brackets intact.
- **The PR-description attribution banner no longer uses the 🤖 emoji.** Its prefix
  is now the plain-ASCII `[AI]` marker. A leading 4-byte emoji could be rejected or
  silently truncated by Data Center databases that aren't `utf8mb4` (e.g. MySQL
  `utf8mb3`), potentially dropping the description body that followed it.
- `commenting.md` corrects the line-number gutter example, documents
  `--commentable`, and adds a fallback rule: when anchoring fails with
  `DIFF_PARSE_FAILED` (a server/format incompatibility, not a bad number), stop
  probing and post a general comment naming `path:line`. `reviewing-locally.md`
  points at the same guidance.
- Skill bumped to `0.9.0`.

## [0.7.0] - 2026-06-05

### Added

- **`pr fetch` / `pr checkout` now fetch the PR's base branch too.** Alongside the
  source ref they bring the PR's destination branch up to date (looked up via the
  API, or `--base <branch>` to override / run offline), so a local review diffs
  against the correct merge-base instead of a stale local branch. The output gains
  `remote`, `source_ref`, `base_branch`, `base_ref`, `base_commit`, and a
  ready-to-run `review_diff` (`git diff <remote>/<base>...<remote>/pr/<id>`).
- **Remote auto-selection.** When `--remote` is not given, the commands prefer an
  `upstream` remote over `origin` (in a fork workflow `upstream` is the canonical
  repo carrying the authoritative base branch and PR refs); pass `--remote` to force
  one.

### Skill

- `reviewing-locally.md` gains a "Reviewing against the right base" section, and the
  decision tree / `pr-workflows.md` now point the agent at the fetched base and the
  `review_diff` instead of diffing against a possibly-stale local branch. Skill
  bumped to `0.8.0`.

## [0.6.0] - 2026-06-05

### Added

- **`pr diff --line-numbers`.** Annotates each diff line with its old/new file
  line numbers in a gutter, so the exact NEW-file line for an inline comment can be
  read off instead of counted from hunk offsets.
- **`comment add --side new|old`.** Selects which diff side the `--inline` line
  refers to (default `new` = post-change); `old` anchors a comment on a removed /
  pre-change line.

### Fixed

- **Inline comments could land on the wrong line.** `comment add --inline
  <path>:<line>` now resolves the line against the file's own diff: it classifies it
  as added / removed / context and emits the correct anchor for both flavors, and
  **errors with the commentable line ranges** when the number isn't on that side
  instead of silently mis-placing the comment. This removes the old/new line-number
  mix-up where a comment meant for new-file line N landed on old-file line N.
- **Data Center inline `lineType` was always `CONTEXT`.** Comments on added or
  removed lines now send `ADDED` / `REMOVED` with the matching `fileType`, so Data
  Center anchors them correctly (added-line comments were previously mis-anchored).

### Changed

- **`auth login` fails fast without a TTY.** Rather than blocking on the secret
  prompt when stdin is not an interactive terminal (a sandboxed agent, CI without a
  PTY), it now returns a structured `AUTH_LOGIN_NEEDS_TTY` error that points at the
  non-interactive paths — run it in a real terminal, or supply `BITBUCKET_*`
  credentials via the environment.
- **`--pretty` clarified as human-only.** The flag help now states it is for
  interactive terminal use and that agents/scripts should omit it, and error
  `next_steps` / hints no longer suggest `config init --pretty` — plain
  `config init` is the non-TTY-safe form.

### Skill

- AI attribution guidance for agent writes: mark AI-authored PR comments (general,
  inline, reply) and PR descriptions with a `[AI](…)` link back to `bitbucket-cli`,
  written in the user's language.
- New "For agents and sandboxes" guidance: reuse the user's existing config and
  credentials, request elevation rather than giving up or re-initializing inside a
  sandbox, and never run interactive `config init` / `auth login` or pass
  `--pretty`.
- Inline-comment guidance rewritten around new-file line numbers: `--inline` line is
  the NEW (post-change) file line; read it from `pr diff --line-numbers`; use
  `--side old` for removed lines; wrong numbers now fail with the commentable ranges.
  Skill bumped to `0.7.0`.

## [0.5.0] - 2026-06-04

### Added

- **Comment resolution & task status.** PR comments now carry `resolved` and
  `task` fields. Cloud derives `resolved` from the comment's `resolution`
  object; Data Center from `state == "RESOLVED"`, and `task` from
  `severity == "BLOCKER"` (Cloud tasks live on a separate endpoint and are not
  surfaced yet). `comment list` gains `--unresolved` and `--tasks` filters.
- **Single-thread targeting on `pr threads`.** `pr threads <ref>` gains
  `--unresolved` (drop resolved threads) and `--comment <id>` (return only the
  thread containing that comment, whether it is the root or a reply; unknown ids
  return a `not_found` error with a discovery hint). Threads now expose a
  `resolved` field mirroring their root comment.

### Skill

- New `responding-to-review-comments.md` reference: a triage workflow for PR
  authors addressing received feedback — start from a specified PR (ref/URL) or a
  single comment id (inbox as discovery fallback), locate the code (local
  checkout preferred for real verification), judge whether the comment is valid,
  propose a fix + verification, and draft a reply. Read-only analysis by default;
  post replies only after confirmation. Wired into `SKILL.md`; skill bumped to
  `0.5.0`.

## [0.4.0] - 2026-05-28

### Changed

- **Default config location moved to `~/.angelmsger/bitbucket/`.** New
  installs and `config init` now write `config.yaml` (and the
  credentials fallback file) under `~/.angelmsger/bitbucket/`, grouping
  every angelmsger CLI under one shared dotfile root. The legacy
  `~/.bitbucket/` directory is still honored — if it has a `config.yaml`
  and the new location does not, the CLI reads and writes there as
  before, so existing installations keep working without a migration
  step. To migrate manually:
  `mkdir -p ~/.angelmsger && mv ~/.bitbucket ~/.angelmsger/bitbucket`.
  Keychain entries are unaffected (the service key has not changed).

## [0.3.0] - 2026-05-27

### Added

- **`--dry-run` on every mutating command.** `bitbucket-cli pr update`,
  `pr approve`, `pr unapprove`, `pr request-changes`, `pr decline`,
  `comment add`, `comment update`, `comment delete`, `branch delete`, and
  `repo delete` now accept `--dry-run` (the existing `pr create`,
  `pr merge`, `branch create`, `repo create` flags are unchanged). Every
  preview goes through a single `Client.DescribeWrite(ctx, op)` path that
  shares the same build helper as the live write, so the previewed HTTP
  request can never diverge from the one that would be sent. Four new
  `DescribeWrite` cases — `DeleteRepoReq`, `RequestChangesReq`,
  `UpdatePRCommentReq`, `DeletePRCommentReq` — round out coverage of the
  full mutating surface.
- **Read-only mode.** A session-level safety switch that blocks every
  mutating client method before any HTTP request is sent, and also gates
  `pr fetch/checkout --exec` (which mutates the local git worktree).
  Enable it via `defaults.read_only: true` in `~/.bitbucket/config.yaml`
  or `BITBUCKET_CLI_READ_ONLY=1` in the environment. Blocked operations
  return a structured `READONLY_BLOCKED` error (`category=permission`,
  exit code 5) whose `next_steps[0]` is `--allow-writes`. The new
  root-level `--allow-writes` persistent flag overrides the posture for
  a single invocation, so
  `BITBUCKET_CLI_READ_ONLY=1 bitbucket-cli --allow-writes pr approve <ref>`
  is the documented escape hatch. `--dry-run` remains usable under
  read-only — `DescribeWrite` sends no HTTP, so the wrapper passes it
  through unchanged. CLI self-configuration (`config init`, `auth login`,
  `skill install`, `file get --output`) is intentionally out of scope.

## [0.2.0] - 2026-05-27

### Features

- `bitbucket-cli user list` / `user get` / `user me` close the
  discoverability gap for `--reviewer` and `--author` identifiers. Cloud
  uses workspace membership (`GET /2.0/workspaces/{ws}/members`, requires
  `--workspace`); Data Center uses the global `/rest/api/1.0/users`
  endpoint and ignores `--workspace`. `--query` filters by display-name
  substring. `user me` is an in-subtree alias for the top-level `whoami`.
- `bitbucket-cli tag list` / `tag get` enumerate repository tags. Tags
  were already silently accepted by any `--ref`-bearing command (`file
  get --ref v1.2.3`, `commit get <hash>`), but the CLI previously had
  no listing path for them. Cloud uses `/2.0/repositories/{ws}/{slug}/refs/tags`;
  DC uses `/rest/api/1.0/projects/{key}/repos/{slug}/tags`.
- `bitbucket-cli workspace list` / `workspace get` enumerate the Bitbucket
  workspaces (Cloud) / projects (Data Center) the current credentials can see
  — the universe of values every other command's `--workspace` flag accepts.
  Error messages that previously told users to "Pass --workspace <name>" now
  also point at `bitbucket-cli workspace list`.
- `bitbucket-cli pr inbox` lists PRs involving the authenticated user across
  repositories. Data Center uses the `/dashboard/pull-requests` endpoint in
  a single call; Bitbucket Cloud uses `/2.0/pullrequests/<uuid>` for
  `--role author` and fans out across the repos under `--workspace` for
  `--role reviewer` / `--role participant` (Cloud has no global reviewer
  index). `--state OPEN | MERGED | DECLINED | ALL` filters the results.

## v0.2.0 — PR review closure with local codebase

Adds the four capabilities a coding agent (Claude Code etc.) needs to drive
end-to-end PR review while also reading a local checkout of the repo.

### New: `file` subtree — browse and read source at any ref

- `bitbucket-cli file list <ws>/<repo> --ref <ref> --path <dir>` lists entries
- `bitbucket-cli file get <ws>/<repo> --ref <ref> --path <file> [--range L1:L2] [--output -|<file>]`
  reads raw file bytes; `--range` slices line range client-side
- `bitbucket-cli file tree <ws>/<repo> --ref <ref> --path <dir> [--depth N]`
  recursively walks the tree

### New: PR file-level operations

- `bitbucket-cli pr files <ref>` — diffstat (per-file path / status /
  added / removed), sorted by churn. Use this before pulling diff payloads
  on large PRs.
- `bitbucket-cli pr diff <ref> --path <file>` — single-file unified diff
- `bitbucket-cli pr threads <ref>` — PR comments regrouped into inline
  threads (by file + anchor), with general discussion in a separate bucket

### New: PR merge readiness aggregation

- `bitbucket-cli pr status <ref>` — single command, single JSON object that
  combines the PR detail, mergeable / conflicts verdict, reviewer states, and
  CI build statuses. Parallel calls under the hood.

### New: PR ↔ local git bridge

- `bitbucket-cli pr fetch <ref>` and `bitbucket-cli pr checkout <ref>` —
  default behaviour is **print-only**: emit the equivalent
  `git fetch refs/pull-requests/<id>/from:refs/remotes/origin/pr/<id>`
  command for the agent / user to run. Pass `--exec` to shell out to `git`
  in the current checkout (CLI verifies it is inside a git worktree first).

### Companion Skill (0.2.0)

- `skills/bitbucket/SKILL.md` description expanded to surface the new
  capabilities for agent triggering.
- New references: `files.md`, `reviewing-locally.md` (the diffstat-first
  review decision tree).
- `pr-workflows.md` updated to put `pr status` and `pr files` ahead of `pr diff`.
- `errors-and-exit-codes.md` notes the `NOT_A_GIT_WORKTREE` failure mode for
  `--exec`.

### Other

- `internal/apiclient/dialect.go` adds path helpers for src / files / raw /
  diffstat / changes / merge / commit statuses.
- `test/mockserver/main.go` extended with DC routes for `/files`, `/raw`,
  `/changes`, `/diff/{path}`, `/merge`, and `/rest/build-status/1.0/commits`.
- `scripts/e2e.sh` covers the 11 new v0.2 commands; **47/47** assertions
  green.

## v0.1.0 — Initial release

PR-centric MVP supporting Bitbucket Cloud (REST 2.0) and Data Center (REST
1.0) behind a flavor-agnostic client. Includes the `repo`, `pr`, `comment`,
`branch`, `commit`, `config`, `auth`, `doctor`, `whoami`, `skill`, `version`
subtrees; layered configuration with keychain-backed auth; structured error
model; and an embedded companion Skill.
