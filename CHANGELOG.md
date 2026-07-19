# Changelog

## [Unreleased]

## [0.13.1] - 2026-07-19

### Fixed

- `pr list --state ALL` now returns every state on Cloud and Data Center, and
  `pr inbox --state ALL` now does the same on Cloud. Cloud enumerates
  OPEN/MERGED/DECLINED/SUPERSEDED as repeated `state` params; the Data Center
  repository endpoint receives `state=ALL` natively. Data Center inbox already
  returns every state by omitting `state`, as required by its dashboard API.

## [0.13.0] - 2026-07-16

### Added

- Windows is now exercised on a native CI runner, including the Go test suite,
  PowerShell completion generation, npm platform mapping and the npm launcher.
- Windows credential-file fallback is encrypted with per-user DPAPI. Existing
  plaintext fallback files remain readable and migrate on the next write.

### Changed

- Installation documentation now includes native PowerShell setup, checksum,
  `PATH`, environment-variable and persistent completion examples.
- README and the docs-site footer now list all five sibling CLIs, including
  the new [jira-cli](https://github.com/AngelMsger/jira-cli).
- README now follows the family-canonical section order and gained an
  "Errors and exit codes" section; the npm package README links the
  installation guide; the docs-site "Go deeper" cards now follow the
  family-canonical set (installation, technical design, releasing, Skill).

## [0.12.0] - 2026-07-14

### Added

- Credential-resolution failures now include an optional machine-readable
  `recovery` action for Agent hosts. When the user's home or OS keychain is not
  visible, the CLI requests one retry in host scope; `doctor` also reports a
  per-check `status` and `recovery_scope`.

### Fixed

- Keychain access failures are no longer collapsed into `AUTH_NO_TOKEN` and no
  longer steer sandboxed agents toward re-running `config init`. The CLI now
  distinguishes an inaccessible store from a credential that is missing or not
  visible in the current environment.

## [0.11.0] - 2026-06-29

### Added

- **The CLI now flags which context your commands will hit when several are
  configured.** A config can hold multiple named contexts, but an agent shelling
  out usually has no idea more than one exists — when none is selected
  explicitly it silently uses the saved `current_context` and can query the
  wrong Bitbucket instance. Now, gated on `>1` context (single-context setups see
  nothing): `--help` ends with the active context, the full list, and how it was
  selected; and a real command run emits a structured `_notice` on stderr when
  the active context was chosen implicitly, so the ambiguity is visible before
  results are trusted. The notice self-silences once a context is selected
  explicitly (`--use-context` or `BITBUCKET_CONTEXT`); opt out entirely with
  `BITBUCKET_CLI_NO_CONTEXT_HINT=1`.

## [0.10.1] - 2026-06-28

### Fixed

- **An unknown subcommand of a command group no longer looks like success.** A
  typo such as `config use-contexts` (for `config use-context`) printed the group
  help to stdout and exited `0`, so an agent or script read it as a successful
  no-op. Cobra flags unknown commands only at the root; a nested non-runnable
  group instead falls through to help-and-exit-0. Every command group (`config`,
  `auth`, `repo`, `pr`, `workspace`, `skill`, …) now returns a structured
  `UNKNOWN_COMMAND` usage error on stderr with exit code 2 and a "Did you mean"
  suggestion; a bare group invocation still prints help.

## [0.10.0] - 2026-06-25

### Added

- **The API client is now an importable Go library.** The HTTP client that
  powers the CLI moved out of `internal/` into `pkg/` (`pkg/apiclient`, `pkg/transport` and `pkg/errors`), so external
  Go projects — e.g. a GUI — can import and reuse it: the `Client` interface, the
  `Build` factory, the normalized models and the structured `*errors.CLIError`
  values. See the "Use as a Go library" section in the README. No CLI behavior
  change — a package-path move plus documentation.

## [0.9.6] - 2026-06-25

### Fixed

- **The companion Skill drifted out of sync with the CLI.** The agent-facing
  Skill (`skills/bitbucket/`) — which coding agents read instead of `--help` —
  omitted or misdescribed several shipping capabilities, so agents missed them.
  Corrected: `comment resolve` / `--unresolve` (the Skill wrongly claimed
  resolving threads "stays manual in the UI"); documented the
  `need-work` / `needs-work` aliases for `pr request-changes`, cross-fork
  `pr create --source-repo`, `pr merge --close-source-branch` (emulated on Data
  Center) and its rejection on `pr create` for DC, `repo create` / `repo delete`,
  and the `pr get --scope full|diff|commits|activity` variants. Added an AGENTS.md
  rule requiring the Skill to be updated in lockstep with the CLI. (Skill content
  only — no behavior change.)

## [0.9.5] - 2026-06-24

### Fixed

- **The "update available" notice was suppressed on failed commands.** It was
  emitted from a `PersistentPostRunE`, which cobra runs only after a command
  succeeds — so a command that errored (auth/API failure, a missing `--yes`,
  etc.) never surfaced the notice, even when a newer release existed. It now
  fires from `Execute` after the command runs, on success and failure alike, so
  a failure-heavy (agent) workflow still learns an upgrade is available. The
  stderr-only delivery, the skip list, and the `BITBUCKET_CLI_NO_UPDATE_NOTIFIER`
  opt-out are unchanged.

## [0.9.4] - 2026-06-24

### Added

- **Companion-Skill discovery for agents.** Agents sometimes shell out to this
  CLI without loading the `bitbucket` Skill, bypassing the usage recipes and
  safety guidance it maintains. Three nudges now close that gap: the root
  `--help` carries an `AGENT NOTE` pointing at the Skill; a new
  `bitbucket-cli skill status` reports whether the Skill is loaded (via the
  `BITBUCKET_CLI_SKILL` handshake) and installed, with the single next action;
  and any real command run non-interactively without that handshake prints a
  one-line `{"_notice":{"skill":…}}` hint to **stderr** (stdout stays clean).
  The hint is silent for humans (TTY), self-silences once the Skill sets
  `BITBUCKET_CLI_SKILL=1`, and can be turned off with `BITBUCKET_CLI_NO_SKILL_HINT=1`.
- **Cloud/Data-Center capability registry** (`internal/apiclient/capability.go`).
  Features that differ between flavors (request-changes, close-source-branch,
  cross-fork create) are declared once with a support level + reason; the
  runtime guard, help, and a new parity test all read that single table instead
  of hard-coded inline checks, so an "X-only" claim can no longer drift from
  reality. Backed by parity tests that keep the table complete and snapshot the
  per-flavor create-PR wire payload.

### Fixed

- **`pr update` failed on Bitbucket Data Center (missing optimistic-lock
  version).** DC guards a PR update with a version: the `PUT
  .../pull-requests/{id}` must echo the PR's current `version` or the server
  rejects it (400/409). The builder sent title/description/reviewers without it,
  so every DC `pr update` (e.g. editing a description) failed. It now fetches the
  current version first and includes it, matching how `pr merge` / `pr decline`
  and the comment writers already handle DC's lock. `--dry-run` resolves the
  version too, so the previewed PUT is the real one.
- **Cross-fork `pr create` was impossible on Data Center.** `--source-repo`
  pointed the PR's `fromRef` at the fork only on Cloud; the DC branch hard-coded
  `fromRef.repository` to the target repo and ignored the flag, so a
  fork → upstream PR could not be opened from the CLI. The DC builder now points
  `fromRef` at the fork named by `--source-repo`. Because DC distinguishes a
  cross-fork PR by comparing the from/to repositories, a fork PR also requires an
  explicit `--target` (the upstream destination branch) — omitting it is now a
  clear usage error instead of a malformed request.

### Changed

- **`pr request-changes` gained `need-work` / `needs-work` aliases.** Bitbucket
  Data Center labels this verdict "Needs work" in the UI, so the alias matches
  what reviewers actually call it. `bitbucket-cli pr need-work <ref>` is now
  equivalent to `pr request-changes <ref>` (the same Cloud-only caveat applies).
- **`pr merge --close-source-branch` now works on Data Center.** Cloud deletes
  the source branch atomically via a body flag; DC has no such flag, so the CLI
  used to silently drop the opt-in. It now deletes the source branch after a
  successful DC merge, resolving the (possibly forked) source repository from the
  PR's own `fromRef`. If the merge succeeds but the branch delete fails, the
  merge is reported as done with a distinct `PR_MERGED_BRANCH_KEPT` error so the
  residual cleanup is visible. The flag remains strictly opt-in — nothing is
  deleted unless you pass it. DC has no equivalent at PR *creation*, so
  `pr create --close-source-branch` is now rejected on DC (use it on `pr merge`)
  rather than silently dropped.

### Known limitations

- **`pr request-changes` is still Data Center-only-unimplemented.** DC models a
  "needs work" vote through the participant-status API
  (`PUT .../participants/{userSlug}`), which needs the caller's user slug; the DC
  client has no working whoami yet, so the command returns a clear "not yet
  implemented" error pointing at decline / comment instead. (Was previously
  mislabelled "only available on Bitbucket Cloud".)

## [0.9.3] - 2026-06-18

### Fixed

- **Literal `\n` in comment/PR bodies rendered as `\n` instead of a line
  break.** A shell does not expand `\n` inside double quotes, so
  `comment add --content "para 1\n\npara 2"` sent a literal backslash-n that
  Bitbucket rendered verbatim on one line. The free-text body flags — `comment
  add/update --content`, `pr create/update --description`, `pr decline/merge
  --message`, and `repo --description` — now decode a small escape whitelist
  (`\n`, `\r`, `\t`, `\\`) into the real characters before sending, echoing a
  `{"_notice":{"corrections":[{"kind":"escape",…}]}}` line to **stderr** so the
  rewrite is visible and stdout stays clean. `\\` stays expressible (`\\n` →
  literal `\n`) and any other escape (a regex `\d`, a Windows path) passes
  through untouched; the `--content-file` / `--description-file` flags are read
  verbatim, never decoded, and remain the exact-bytes path.

## [0.9.2] - 2026-06-16

### Fixed

- **Data Center inline comments lost their anchor when read back, and review
  threads were grouped too coarsely.** Two activities-stream bugs on Bitbucket
  Data Center, both follow-ups to the reply fix in 0.9.1:
  - **Anchors.** DC hoists an inline comment's anchor onto the activity
    (`activity.commentAnchor`), a sibling of `comment` — not into
    `comment.anchor`, which is only populated on the single-comment and
    create-comment responses. The activities parser read only the latter, so
    every inline comment came back unanchored and collapsed into the general
    bucket. It now reads `commentAnchor` and applies it to the thread root.
  - **Thread scope.** `pr threads` keyed threads by `{file, line}`, which merged
    every non-inline comment into one "general discussion" thread, so
    `pr threads --comment <id>` returned that whole bucket instead of the one
    discussion. Threads are now one-per-root-comment (matching how Bitbucket
    models a discussion), so `--comment` scopes to exactly that root and its
    replies, and same-line comments stay distinct.

## [0.9.1] - 2026-06-16

### Fixed

- **Data Center replies were invisible to every read command.** A reply posted
  with `comment add --reply-to` returned a real comment id but never showed up
  again — `comment list` and `pr threads` omitted it, and `pr threads --comment
  <id>` failed with `COMMENT_NOT_FOUND` — so an agent could not see its own
  reply and was misled into posting duplicates. Bitbucket Data Center nests a
  comment's replies inside the parent's `comments` array rather than emitting
  them as separate activity entries, and the activity-stream parser only read
  top-level comments. It now walks the full reply tree and stamps each reply's
  `parentId`, so replies surface in listings and thread lookups. Bitbucket Cloud
  (a flat comment list carrying `parent`) was unaffected.

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

[Unreleased]: https://github.com/AngelMsger/bitbucket-cli/compare/v0.13.1...HEAD
[0.13.1]: https://github.com/AngelMsger/bitbucket-cli/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/AngelMsger/bitbucket-cli/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/AngelMsger/bitbucket-cli/compare/v0.11.0...v0.12.0
