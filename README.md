# bitbucket-cli

[![Go version](https://img.shields.io/github/go-mod/go-version/angelmsger/bitbucket-cli.svg)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Bitbucket](https://img.shields.io/badge/Bitbucket-Cloud%20%26%20Data%20Center-0052CC.svg)](https://www.atlassian.com/software/bitbucket)

> Drive Bitbucket from your terminal — built for coding agents.

`bitbucket-cli` lets coding agents (Claude Code and others) — and humans —
browse repositories, walk the full pull-request lifecycle, post inline review
comments, and query branches and commits. It supports **Bitbucket Cloud**
(REST 2.0) and **Data Center / Server** (REST 1.0) behind one flavor-agnostic
command tree, returns agent-friendly JSON with structured errors, and ships a
companion Skill that teaches an agent how to use it. Write commands support
`--dry-run`, and destructive ones require `--yes`.

## Install

```sh
make install                       # local source build → $GOBIN
# or
go install github.com/angelmsger/bitbucket-cli/cmd/bitbucket-cli@latest
```

## Quickstart

```sh
bitbucket-cli config init --pretty
bitbucket-cli doctor
bitbucket-cli pr list --repo myws/myrepo --state OPEN
bitbucket-cli pr get  myws/myrepo/42 --scope diff
bitbucket-cli comment add --pr myws/myrepo/42 --inline src/main.go:88 \
    --content "Why not log this at debug?"
bitbucket-cli pr approve myws/myrepo/42
bitbucket-cli pr merge   myws/myrepo/42 --strategy squash --yes
```

## Layout

- `cmd/bitbucket-cli/` — entry point.
- `internal/app/` — Cobra command tree (`repo`, `pr`, `comment`, `branch`,
  `commit`, `config`, `auth`, `doctor`, `whoami`, `skill`, `version`).
- `internal/apiclient/` — flavor-agnostic Bitbucket REST client. Per-flavor
  endpoint differences live in `dialect.go` and `mapping.go`.
- `internal/auth/`, `internal/config/`, `internal/transport/`,
  `internal/output/`, `internal/errors/` — layered configuration, keychain-
  backed auth, retrying HTTP, formatters, structured errors.
- `skills/bitbucket/` — companion Skill embedded into the binary.

## Companion Skill

`bitbucket-cli skill install --agent claude-code` drops the embedded
`bitbucket` Skill into your agent's skills directory, version-matched with the
binary. The Skill teaches the agent the PR review/merge decision tree, how to
write inline comments, and how to recover from each exit-code category.

## Status

Early in v0.x; PR-centric MVP. Pipelines, issues, webhooks and OAuth 2.0 are
deliberately out of scope for v0.1.
