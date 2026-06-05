# Changelog

## [Unreleased]

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
  `--pretty`. Skill bumped to `0.6.0`.

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
