# Agent Guide

This file orients coding agents (Claude Code and others) working in this
repository. It is intentionally short — the real guidance lives elsewhere.

## Start here

1. Read [`CONTRIBUTING.md`](CONTRIBUTING.md) first. It covers the project
   structure, the build/test/lint commands, coding and testing conventions, and
   the commit/PR expectations every change must follow.

2. Then read, **only as the task needs them**, the documents under
   [`docs/`](docs/):

   - [`docs/technical-design.md`](docs/technical-design.md) — architecture, the
     `internal/` package layout, the API-client/flavor abstraction, the config
     and error models, and the rendering pipeline. Read before changing core
     behavior.
   - [`docs/installation.md`](docs/installation.md) — install methods, shell
     completion, and the companion Skill. Read for distribution/UX changes.
   - [`docs/releasing.md`](docs/releasing.md) — versioning, the changelog step,
     tagging, and the release/CI workflows. Read before cutting a release or
     touching `.github/workflows/`.

Pull in a document when the task touches its area; do not read all of `docs/`
up front.

## Ground rules

- Run `make test` and `make e2e` before claiming a change is complete.
- Keep commits scoped to one logical change; follow the commit and PR
  conventions in `CONTRIBUTING.md`.
- Never commit `.env`, credentials, tokens, or build artifacts.

## Cloud / Data Center parity — touch both branches, always

This CLI targets **two backends** (Bitbucket Cloud and Data Center / Server),
and the same concept is reached through different APIs. The recurring bug class
here is a Cloud branch honouring an input that the DC branch silently drops (or
vice versa): a missing optimistic-lock `version`, a `--close-source-branch` that
no-ops, a feature wrongly hard-coded as "Cloud only".

When you touch any read/write path that branches on `c.flavor`:

1. **Handle both branches or consciously diverge.** Every request field set by
   the command must be consumed by *both* flavor branches, or the divergence
   must be declared — see next point. Compiling proves nothing here: a DC branch
   that never reads `req.Foo` is silently wrong.
2. **Declare divergence in one place.** Asymmetric behaviour lives in the
   capability registry (`internal/apiclient/capability.go`), never as an inline
   `cerrors.New("...only on Cloud")`. The runtime guard, help/docs, and the
   parity test all read that table, so they cannot drift. Add a `Capability`
   row (with a reason) before you write an "X-only" guard.
3. **Add/extend the parity tests** in `internal/apiclient/capability_test.go`:
   keep the registry complete (both flavors covered) and snapshot the per-flavor
   wire payload (golden) so a one-sided change shows up as a diff in review.
4. **Never write `flavor-only` without a registry entry.** A mislabelled
   "Cloud-only" that the other flavor actually supports is itself the bug.

## Discoverability — no dead-end inputs

**Every non-trivial identifier a command accepts as input must be discoverable
through another command in this CLI.** Examples of inputs covered by this
rule: workspace slugs, repository slugs, branch / tag names, PR IDs, commit
hashes, user identifiers (UUID on Cloud, username on Data Center), comment
IDs, file paths within a ref.

The rule is symmetric: an input is *only* discoverable if (a) the CLI has a
command that lists / searches values of that kind, **and** (b) that listing
command itself is reachable without already knowing some other value the CLI
can't tell you about. A `repo list` that demands a workspace slug is only
useful if `workspace list` also exists.

When you add a command or flag that takes a new kind of input:

1. Walk every parameter the new surface accepts. For each, answer:
   *"Where does the caller (especially an AI agent) get this value?"*
2. If the answer is **another command in this CLI**, you are done.
3. If the answer is **an existing identifier the user already had in hand**
   (e.g. they pasted a Bitbucket URL), that counts too — `pkg/urlref` already
   parses workspace / repo / PR / commit out of URLs.
4. If the answer is **out-of-band** (a web UI, another tool, the API
   directly), that is a gap. Add the missing discovery command in the same
   PR, or surface it as a follow-up issue and document the dead end in
   `CHANGELOG.md` under *Known gaps*.

The same rule applies to error messages. When a command rejects an
invocation for missing a required identifier, the resulting `CLIError`'s
`next_steps` must include the discovery command — see
`internal/apiclient/repos.go` (`REPO_NO_WORKSPACE`) and
`internal/apiclient/pulls_inbox.go` (`INBOX_NO_WORKSPACE`) for the canonical
shape: list the discovery command first, then the flag or environment
variable fallback. Errors that say "Pass `--workspace <name>`" without
showing the user how to find a valid `<name>` are defects.

The e2e harness exercises this contract via `assert_err_contains` — error
paths that should include a discovery hint are asserted on (see
`scripts/e2e.sh`'s `repo list hint` check). New "missing input" errors
should grow a matching assertion.

## Safety modes — `--dry-run` and read-only posture

Two orthogonal protections guard every operation that mutates remote state
*or* local user state (`pr fetch/checkout --exec`):

1. **`--dry-run`** is a per-command flag on every mutating command. It must
   resolve the request via `Client.DescribeWrite(ctx, op)` and emit the
   resulting `WriteRequestPlan{Method, URL, Payload}` instead of sending the
   HTTP request. Use the shared `emitDryRun(s, client, ctx, op)` helper —
   never re-implement the dispatch inline. Every write request type that
   reaches a command must also have a `DescribeWrite` case; the read path
   (build helper) must be the same code path the live write uses so the
   preview cannot drift from the actual request.
2. **Read-only posture** is a session-level switch: `defaults.read_only`
   in the config file, or `BITBUCKET_CLI_READ_ONLY` in the environment.
   When active, `appState.newClient()` wraps the client in
   `apiclient.NewReadOnly(...)`, which returns a structured
   `READONLY_BLOCKED` (`category=permission`) error from every mutating
   method *before* any HTTP request is sent. `--allow-writes` (root
   persistent flag) overrides the posture for one invocation. Local-only
   side effects that mutate user state outside the CLI's own configuration
   — `pr fetch/checkout --exec` shelling out to `git` — MUST also check
   `appState.readOnly()` before executing.

When you add a new mutating method on `Client`:

- Add the method override on `readOnlyClient` in `internal/apiclient/readonly.go`
  so the wrapper actually blocks it.
- Add a `DescribeWrite` case (`internal/apiclient/pulls_write.go`) + a
  `--dry-run` branch on the calling command.
- Add an e2e assertion for both the `--dry-run` happy path and the
  `READONLY_BLOCKED` rejection (see `scripts/e2e.sh`).
- Add a row to the wrapper's table test in
  `internal/apiclient/readonly_test.go`.

`--dry-run` must *not* be blocked by read-only mode — `DescribeWrite` sends
no HTTP and is the right tool to inspect what a write would look like under
a read-only session. The wrapper intentionally does not override it.

CLI self-configuration (`config init`, `auth login|logout`, `skill install`,
`file get --output`) is **out of scope** for read-only mode. Read-only
protects the remote service and the user's working tree; it must not block
the CLI from managing its own state.

## Documentation — keep it current

- **Actively maintain the docs.** When a change affects architecture,
  installation, commands, flags, or the release process, update the relevant
  file under [`docs/`](docs/) in the same commit. Stale docs are a defect.
- **This includes the GitHub Pages site.** [`docs/index.html`](docs/index.html)
  is the published landing page (served at
  <https://angelmsger.github.io/bitbucket-cli/>) and
  `.github/workflows/pages.yml` redeploys it on every push to `main` that
  touches `docs/`. When commands, the feature
  list, or install instructions change, update `docs/index.html` to match — do
  not let the landing page drift from the README and the CLI.
- **Keep the companion Skill in sync — it is the agent-facing source of truth.**
  The embedded Skill under [`skills/bitbucket/`](skills/bitbucket/) (`SKILL.md` +
  `references/`) is what coding agents read *instead of* `--help`, so a
  capability the Skill omits effectively does not exist for them. Any new
  command, subcommand, flag, or alias — and any change to flavor-specific
  behavior (Cloud vs Data Center, e.g. a feature that becomes supported,
  emulated, or rejected on one flavor) — must be reflected in the Skill's
  `## Commands` list and the relevant `references/` file in the *same* commit. In
  particular: a flag whose help text points at another command (e.g. "find IDs
  with `user search`") must have that command documented in the Skill; and never
  leave a Skill claim that contradicts the code (e.g. "the CLI does not set
  resolution" when `comment resolve` exists).

## Changelog & versioning — required

- **Actively maintain [`CHANGELOG.md`](CHANGELOG.md).** Whenever a change is
  user-facing (a flag, command, output, behavior, or bug fix), add an entry to
  the `[Unreleased]` section in the same commit — do not leave it for later.
- **If you bump the version, you must tag the commit.** "Bumping the version"
  means renaming `[Unreleased]` in `CHANGELOG.md` to the new version with
  today's date and updating `build/npm/package.json`. The CLI's own version is
  derived from the git tag via `-ldflags`, so a version bump is not real until
  the commit carrying it is tagged:

  ```bash
  git tag vX.Y.Z <commit>
  git push origin vX.Y.Z
  ```

  See [`docs/releasing.md`](docs/releasing.md) for the full release procedure.
