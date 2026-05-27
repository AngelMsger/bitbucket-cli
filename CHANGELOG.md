# Changelog

## Unreleased

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
