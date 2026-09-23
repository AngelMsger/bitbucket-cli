# Contributing

Thanks for working on `bitbucket-cli`. This guide covers the repository layout,
the build and test workflow, and the conventions a change is expected to follow.
For deeper background see [`docs/`](docs/) — start with
[`technical-design.md`](docs/technical-design.md) for the architecture and
[`releasing.md`](docs/releasing.md) for the release process.

## Project Structure & Module Organization

This repository is a Go CLI for Bitbucket. The executable entrypoint is in `cmd/bitbucket-cli/`. Core implementation is split between `internal/` (CLI-only: `app` for Cobra commands, `auth` and `config` for setup, `output` for presentation) and `pkg/`, which holds the importable client library (`apiclient` for Bitbucket APIs, `transport` for HTTP behavior, `errors` for the structured error model) alongside other public helpers. Tests sit beside the code as `*_test.go`; the mock Bitbucket server is in `test/mockserver/`. Documentation is in `docs/`, companion agent skill files are in `skills/bitbucket/`, and npm wrapper/release assets are in `build/npm/`.

## Build, Test, and Development Commands

- `make build` builds `bin/bitbucket-cli` with version metadata.
- `make test` runs `go test ./...` across all packages.
- `make e2e` builds the CLI and runs end-to-end checks against the in-repo mock server; Python 3 validates NDJSON pagination across real CLI invocations.
- `make e2e-live` also runs read-only checks against a real server configured through `.env`.
- `make lint` runs formatting and vetting via `make fmt` and `make vet`.
- `make cross` creates release binaries in `dist/`.
- `make tidy` updates `go.mod` and `go.sum`.

## Coding Style & Naming Conventions

Use standard Go formatting; CI requires `gofmt` cleanliness and `go vet ./...`. Keep package names short, lowercase, and aligned with their directory purpose. Prefer focused internal packages over broad shared utilities. Command behavior should stay in `internal/app`, API mapping in `pkg/apiclient`, and user-facing constants in `pkg/constants`. Export identifiers only when they are consumed across package boundaries or by public `pkg` APIs.

## Testing Guidelines

Use Go’s standard `testing` package. Name test files `*_test.go` and test functions `TestXxx`. Place unit tests beside the package under test, and use `test/mockserver` for CLI-level integration coverage. Before opening a PR, run `make test` and `make e2e`; run `make e2e-live` only when real Bitbucket credentials are available and read-only live validation is needed.

For changes to the companion Skill's review decisions, also compare baseline and
candidate behavior with the [offline PR workflow cases](test/skill-review/expectations.md).
These exercise judgment and scope; CLI tests verify packaging and command behavior.
`make e2e` also runs `scripts/skill-budget.sh`, which caps the Skill's word count
per file and across the review load path.

## Commit & Pull Request Guidelines

Use concise, imperative commit messages such as `Add a --version flag on the
root command`. Keep commits scoped to one logical change.

Follow [Writing PR descriptions](skills/bitbucket/references/writing-pr-descriptions.md):
explain the problem and resulting behavior, and add review guidance only where
it helps. Link related issues when applicable; include CLI output or screenshots
only when they clarify a user-facing output change.

Review functional correctness and significant maintainability concerns using the
Skill's [evidence and information-placement rules](skills/bitbucket/references/reviewing-locally.md#verify-findings).
PR creation follows its [incidental quality reminder](skills/bitbucket/references/pr-workflows.md#incidental-quality-reminder)
without adding a full review or a new submission gate.

Before opening a PR, complete the checks required by the build, style, and
testing sections above for the changed scope. Agents should report results,
skipped checks and their reasons, and blockers to the user during preparation.
Keep that report separate from the PR description; do not append a routine
validation section or passing-test statement.

## Changelog & Versioning

Actively maintain [`CHANGELOG.md`](CHANGELOG.md). Any user-facing change — a
flag, command, output format, behavior, or bug fix — must be recorded under the
`[Unreleased]` section in the same commit that makes the change; do not defer
it.

The CLI's own version is derived from the git tag via `-ldflags`, so bumping the
version is only complete once the commit is tagged. To bump the version: rename
`[Unreleased]` in `CHANGELOG.md` to the new version with today's date, add a
fresh empty `[Unreleased]` heading, update the comparison links, bump
`build/npm/package.json` to match, commit, then tag that commit:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

A version bump that is not tagged is incomplete. See
[`docs/releasing.md`](docs/releasing.md) for the full release procedure.

## Documentation

Treat the documentation as part of the change, not an afterthought. When a
change affects architecture, installation, commands, flags, or the release
process, update the relevant file under [`docs/`](docs/) in the same commit:

- [`docs/technical-design.md`](docs/technical-design.md) — architecture and the
  `internal/` package layout.
- [`docs/installation.md`](docs/installation.md) — install methods, shell
  completion, the companion Skill.
- [`docs/releasing.md`](docs/releasing.md) — versioning and the release/CI
  workflows.

This includes the **GitHub Pages site**: [`docs/index.html`](docs/index.html) is
the published landing page (served at
<https://angelmsger.github.io/bitbucket-cli/>), redeployed by
`.github/workflows/pages.yml` on every push to `main` that touches `docs/`. When
the command list, feature highlights, or install instructions change, update
`docs/index.html` so the landing page, the README, and the CLI stay in sync.

## Security & Configuration Tips

Do not commit `.env`, credentials, personal access tokens, or generated release artifacts in `bin/` or `dist/`. Use `.env.example` for documented configuration keys. Secrets should remain in environment variables, OS keychain storage, or local config files outside version control.
