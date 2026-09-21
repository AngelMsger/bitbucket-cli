# bitbucket-cli technical design

## 1. Goals and scope

`bitbucket-cli` is a Go command-line tool that lets a coding agent
(Claude Code, Codex, etc.) drive Bitbucket's pull request review and
merge workflow from the terminal, closing the loop between **remote PR
state** and a **local code checkout** for end-to-end code review.

- **Cross-flavor**: works with Bitbucket Cloud (REST 2.0) and Bitbucket
  Data Center / Server (REST 1.0, self-hosted).
- **Agent-first**: JSON output by default, structured errors, per-file
  diff reading (`pr files` → `pr diff --path`), error messages that
  carry actionable next steps.
- **Layered configuration**: CLI flags / environment variables / `.env`
  / config file, with an interactive `init` wizard.
- **Operation surface**:
  - **Full PR lifecycle**: list / inbox / get / create / update / diff /
    commits / activity / threads / status / files / approve /
    unapprove / request-changes / decline / merge / fetch / checkout.
  - **Source browsing** at any ref: list / tree / get (with optional
    line-range slicing).
  - **Repos / branches / tags / commits**: list and detail for
    `repo` / `branch` / `tag` / `commit`, plus repository create, fork,
    and delete operations.
  - **Comments**: list / add (with inline anchor support) / update /
    delete.
  - **Discovery commands**: `workspace list`, `user list`,
    `user search`, `tag list`, and so on, so every identifier the CLI
    accepts (`--workspace`, `--reviewer`, `--ref`, …) has a CLI-internal
    discovery path.
  - **`whoami` / `user me`** report the user attached to the current
    credentials. Data Center obtains the username from the authenticated
    response's `X-AUSERNAME` header, then resolves the full user record.
  - Every write command accepts `--dry-run` to preview the request;
    delete / merge / decline operations additionally require `--yes`.

Non-goals (out of scope for this cycle): Pipelines (CI/CD trigger and
log reading), webhooks, deploy keys, SSH key management, third-party
OAuth 2.0 authorization, draft PRs, cross-repo code search, and PR
tasks (Cloud todo list).

## 2. API flavor difference matrix

The CLI uses a `Flavor` value to distinguish two backends:

| Flavor | Description | REST base |
|--------|-------------|-----------|
| `cloud` | Bitbucket Cloud (`*.bitbucket.org`) | `/2.0` |
| `datacenter` | Data Center / Server (self-hosted) | `/rest/api/1.0` |

Per-operation differences in endpoint / pagination / body (`{base}` is
the site root):

| Operation | cloud | datacenter |
|-----------|-------|------------|
| List repositories | `GET /2.0/repositories/{ws}` | `GET /rest/api/1.0/projects/{key}/repos` |
| Fork repository | `POST /2.0/repositories/{ws}/{repo}/forks`; body must include target `workspace.slug`, and same-workspace forks also need a new `name` | `POST /rest/api/1.0/projects/{key}/repos/{repo}`; optional `project.key` / `name`, with an omitted project defaulting to the caller's personal project |
| List workspaces | `GET /2.0/workspaces` | `GET /rest/api/1.0/projects` |
| List PRs | `GET /2.0/repositories/{ws}/{repo}/pullrequests?state=&q=`; author/reviewer selectors resolve to UUID predicates | `GET /rest/api/1.0/projects/{key}/repos/{repo}/pull-requests?state=`; author/reviewer filtering requires explicit `--all` and is applied after the repository scan |
| Get PR | `GET .../pullrequests/{id}` | `GET .../pull-requests/{id}` |
| Update PR metadata | `PUT .../pullrequests/{id}` accepts partial metadata; omit `reviewers` to preserve them | `PUT .../pull-requests/{id}` requires the current `version` and treats `reviewers` as a complete replacement set; the client fetches and round-trips both |
| Decline PR | `POST .../pullrequests/{id}/decline` with `message` | `POST .../pull-requests/{id}/decline?version=N` with `version` and optional `comment`; preview and execution share the version-reading builder |
| PR diff (whole) | `GET .../pullrequests/{id}/diff` (text) | `GET .../pull-requests/{id}/diff` (JSON hunks; `Accept: text/plain` for raw text) |
| PR diff (per file) | `GET .../pullrequests/{id}/diff?path=` | `GET .../pull-requests/{id}/diff/{path}` |
| PR diffstat | `GET .../pullrequests/{id}/diffstat` | `GET .../pull-requests/{id}/changes` |
| PR activity feed | `GET .../pullrequests/{id}/activity`; activity filters interpret merge/decline state updates without reshaping the response | `GET .../pull-requests/{id}/activities`; predefined-reviewer automation is classified as system activity |
| PR merge precheck | Derived from `pullrequests/{id}` + `/statuses` | `GET .../pull-requests/{id}/merge` returns `{canMerge,conflicted,outcome,vetoes}` directly |
| CI build status | `GET /2.0/repositories/{ws}/{repo}/commit/{hash}/statuses` | `GET /rest/build-status/1.0/commits/{hash}` (not under `/rest/api/1.0`) |
| Inbox (my PRs) | `/2.0/pullrequests/{uuid}` (author) or workspace-scoped fan-out (reviewer/participant/any); no cross-repository close-time filter | `GET /rest/api/1.0/dashboard/pull-requests?role=&closedSince=`; `role` is omitted for any role |
| List branches | `GET .../refs/branches` | `GET .../branches` |
| List tags | `GET .../refs/tags` | `GET .../tags` |
| Source file metadata | `GET .../src/{ref}/{path}?format=meta` | `GET .../files/{path}?at={ref}` |
| Source file raw | `GET .../src/{ref}/{path}` | `GET .../raw/{path}?at={ref}` |
| List users | `GET /2.0/workspaces/{ws}/members` (workspace-scoped) | `GET /rest/api/1.0/users?filter=` |
| Inline comment anchor | `inline:{path,from,to}` | `anchor:{path,line,lineType,fileType}` |
| Ping | `GET /2.0/user` | `GET /rest/api/1.0/application-properties` |

**Pagination**: Cloud is cursor-based (the `next` field is an absolute
URL; follow it as-is). Data Center accepts `start` / `limit`; when
`isLastPage` is false, clients must reuse the response's `nextPageStart`
verbatim because the next offset is not guaranteed to equal `start + limit`
or `start + size`. `pagination.go`'s `FetchPage[T]` plus `CollectAll` abstract
both behind one callback.

**Flavor detection**: explicit `--flavor` / config wins; otherwise URL
heuristics (host `*.bitbucket.org` → cloud); otherwise `auto` probes
`/2.0/user` vs `/rest/api/1.0/application-properties`.

**Endpoint differences are isolated to two files**:
`pkg/apiclient/dialect.go` (path helpers — `repoPath`, `prPath`,
`branchesPath`, `commitStatusesPath`, `srcPath`, `filesPath`, …) and
`pkg/apiclient/mapping.go` (raw two-flavor responses → unified
models).

## 3. Normalized data model

Every API method returns flavor-agnostic models
(`pkg/apiclient/models.go`):

```
ServerInfo  { Flavor, BaseURL, Reachable }
Workspace   { Slug, Name, UUID, Type, Description, URL, CreatedAt }
Repository  { UUID, Slug, Name, Workspace, FullName, Description, Private,
              DefaultBranch, Language, Size, URL, CloneHTTPS, CloneSSH, ... }
Branch      { Name, Target, Default, LastCommit, LastUpdated }
Tag         { Name, Target, Date, Message }
Commit      { Hash, Message, Author, Date, Parents, URL }
User        { AccountID, UUID, Name, Slug, DisplayName, Email, Type }
PRRef       { Branch, Commit, Repository }
Participant { User, Role, Approved, State }
PullRequest { ID, Ref, Title, Description, State, Author, Source, Destination,
              Reviewers, Participants, Repository, URL, CommentCount,
              MergeCommit, CreatedAt, UpdatedAt, ClosedAt }
InlineAnchor{ Path, Line, From, To }
Comment     { ID, Content, Author, Inline, ParentID, PRID, CommitID,
              URL, CreatedAt, UpdatedAt }
Activity    { Kind, Actor, When, PullRequest, Comment, Approved, State, System }
Diffstat    { Path, OldPath, Status, LinesAdded, LinesRemoved, Binary }
Thread      { File, Anchor, Comments[] }              // inline threads grouped by file
MergeCheck  { CanMerge, Conflicted, Outcome, Vetoes }
BuildStatus { Key, Name, State, URL, Description, CommitHash, ... }
PRStatus    { PR *PullRequest, MergeCheck, Reviewers, Builds }  // `pr status` aggregate
FileEntry   { Path, Name, Type, Size, Hash, Commit }
FileContent { Path, Ref, Bytes []byte, Size, Encoding, Truncated }
```

JSON output fields use snake_case. Timestamps are RFC 3339 on Cloud and
epoch milliseconds on DC (normalized via `epochToISO`).
`FileContent.Bytes` is not base64-wrapped, so `file get --output -`
writes raw bytes straight to stdout.

## 4. Configuration and authentication

### 4.1 Config structure

```
Config   { BaseURL, Flavor, Auth, Defaults, DetectedFlavor }
AuthConfig { Scheme: pat | basic, Username }
Defaults { Format, PageSize, Timeout, MaxRetries, Workspace, ReadOnly }
                                              # ↑ default workspace; overridable via --workspace
                                              # ↑ ReadOnly is the session-level write block
```

### 4.2 Sources and precedence

Highest → lowest: CLI flags > environment variables (`BITBUCKET_*`) >
`.env` file > `~/.angelmsger/bitbucket/config.yaml` (or the legacy
`~/.bitbucket/config.yaml` when only that exists) > built-in defaults. Each
layer is a sparse `Config`; non-zero fields override lower layers.
Provenance is recorded per-field so `config show --explain` can report
it.

Environment variable mapping:

| Variable | Field |
|----------|-------|
| `BITBUCKET_SERVER` | `BaseURL` |
| `BITBUCKET_FLAVOR` | `Flavor` |
| `BITBUCKET_TOKEN` / `BITBUCKET_PERSONAL_ACCESS_TOKEN` | PAT secret (scheme=pat) |
| `BITBUCKET_USERNAME` | `Auth.Username` |
| `BITBUCKET_PASSWORD` | basic secret (DC) |
| `BITBUCKET_API_TOKEN` | basic secret (Cloud, paired with email) |
| `BITBUCKET_DEFAULT_WORKSPACE` | `Defaults.Workspace` |
| `BITBUCKET_FORMAT` | `Defaults.Format` |
| `BITBUCKET_CONTEXT` | currently selected context name |
| `BITBUCKET_CLI_READ_ONLY` | `Defaults.ReadOnly` |

### 4.3 Authentication

- **pat**: `Authorization: Bearer <token>`.
  - Cloud: Workspace / Repository / Project Access Token.
  - DC: HTTP Access Token.
- **basic**: `Authorization: Basic base64(user:secret)`.
  - Cloud: email + API token (issued from id.atlassian.com) or App
    Password (kept for backward compatibility).
  - DC: username + password.

Secrets are never persisted to `config.yaml`. `config init` stores them
in the OS keychain (`go-keyring`, service `bitbucket-cli`, account
`<host>:<scheme>`); on failure it falls back to a `credentials` file
inside the resolved config directory (per-user DPAPI on Windows; file 0600 and
dir 0700 on macOS/Linux) —
`~/.angelmsger/bitbucket/credentials` by default, or
`~/.bitbucket/credentials` when the CLI is running against the legacy
location.

Credential reads distinguish "not found" from "store inaccessible". When a
sandbox cannot inspect the host keychain/file, resolution returns
`CREDENTIAL_STORE_INACCESSIBLE`; an ambiguous absence returns
`CREDENTIAL_NOT_VISIBLE_OR_MISSING`. Both carry an optional structured
`recovery` action requesting one retry in host scope, without marking the error
normally retryable or allowing the CLI to elevate itself.

### 4.4 The `init` wizard

Enter base URL → detect and confirm the flavor → pick the auth scheme
(Cloud defaults to basic, DC defaults to pat) → enter credentials →
live `Ping` validation → choose where to store the secret → write
non-secret fields to `config.yaml`, secret to keychain / file → print
suggested next commands.

## 5. Command surface

Global persistent flags: `--base-url`, `--flavor`,
`--format` (json|table|ndjson), `--fields`, `--timeout`, `--config`,
`--use-context`, `--verbose`, `--pretty`, `--allow-writes`.

Commands group by resource: `repo`, `workspace`, `pr`, `file`,
`comment`, `branch`, `tag`, `commit`, `user`, `config`, `auth`,
`doctor`, `whoami`, `skill`, `version`. Cross-command conventions:

- **Identifier parsing**: `pkg/urlref` accepts PR / repo URLs and
  unpacks workspace / slug / PR id / commit; commands also accept the
  `<ws>/<repo>` and `<ws>/<repo>/<id>` short forms.
- **Writes**: every create / fork / update / delete / merge / decline /
  comment / branch-mutation is a write. Each accepts `--dry-run` to
  preview the request; delete / merge / decline additionally require
  `--yes`.
- **Pagination**: list commands accept `--limit/--all/--cursor` and
  emit `{items, next, has_more}`. Cloud's `next` is an absolute URL —
  follow it directly.
- **Discoverability** (see [AGENTS.md](../AGENTS.md) "Discoverability —
  no dead-end inputs"): every `--workspace` / `--reviewer` /
  `--author` / `--ref` identifier has a CLI-internal discovery path,
  and "required `--workspace`" style errors put the discovery command
  at `next_steps[0]`.

The full command / flag / example reference is auto-generated from the
command tree — see [docs/cli/](cli/) (`make docs` produces it, CI
checks for drift). This section deliberately does not maintain a
parallel command list.

## 6. PR review loop (the v0.2 core addition)

Design goal: let a coding agent flip between the remote PR view and a
local checkout with the smallest number of remote round-trips and the
least context-token waste.

### 6.1 Review workflow

The companion Skill's [review guide](../skills/bitbucket/references/reviewing-locally.md)
owns the decision rules: reuse the known PR, read intent, inspect changes and
existing discussion, and verify local source/base alignment when using a
checkout. CI and conflicts inform the review without preventing independent
analysis. Findings go through its publication rules; a clean, authorized
approval has no companion comment. State changes remain within the requested
scope. Use [PR permission recovery](../skills/bitbucket/references/pr-permissions.md)
when the CLI cannot complete an authorized action.

The agent applies the guide's evidence standards to functional correctness and
significant maintainability concerns, using the final diff and bounded context
reads. The CLI supplies existing PR, source, and discussion data; it does not
run a model, assign a quality score, or add a review command. PR creation uses
only the [incidental quality reminder](../skills/bitbucket/references/pr-workflows.md#incidental-quality-reminder)
from context already read. The [offline cases](../test/skill-review/expectations.md)
exercise these decisions separately from CLI packaging and transport tests.

### 6.2 `pr status` — parallel aggregation

`pkg/apiclient/merge_check.go::GetPRStatus` uses `sync.WaitGroup`
to fire in parallel:

1. `GetPR` — PR details (including reviewers / state).
2. `CheckPRMerge` — DC calls `/merge` directly; Cloud derives the
   verdict from `PR.state`.
3. `ListPRStatuses` — Cloud calls `/pullrequests/{id}/statuses`; DC
   pulls `pr.toRef.commit` then queries
   `/rest/build-status/1.0/commits/{hash}`.

If any sub-request fails the response **degrades**: the corresponding
field is left empty and the call as a whole still returns. The returned
`PRStatus { PR, MergeCheck, Reviewers, Builds }` is a one-shot
"PR-review readiness dashboard" — it replaces three serial calls
(`pr get` + `/statuses` + `/merge`) for the agent.

### 6.3 `pr fetch` / `pr checkout` — dual-mode

Default is **print-only**: returns JSON

```json
{ "commands": ["git fetch origin refs/pull-requests/42/from:refs/remotes/origin/pr/42"],
  "executed": false,
  "hint": "re-run with --exec to actually run these (must be inside a git checkout)." }
```

With `--exec`: first `git rev-parse --is-inside-work-tree` checks the
cwd is inside a git work tree (otherwise a `usage` error with a clear
hint); on success `exec.Command("git", ...)` runs the commands in
sequence with stderr passed through. The PR refspec
(`refs/pull-requests/<id>/from`) is the same on Cloud and DC, so
**no flavor branching is needed**.

`--exec` is also gated by read-only mode: if `defaults.read_only` or
`BITBUCKET_CLI_READ_ONLY=1` is in effect, the exec branch returns
`READONLY_BLOCKED`, since this is the only command that mutates local
state outside the CLI's own config. Print-only mode is always safe.

### 6.4 `pr threads` — client-side regrouping

No additional request. After `ListPRComments` walks pagination to
completion, comments are regrouped in Go: keyed by `Inline.Path` plus
top-level `ParentID`, into `Thread{File, Anchor, Comments[]}`. Inline
threads come first (sorted by file path); general discussion lands in
a final `Thread{File: ""}` bucket.

## 7. Output and error model

### 7.1 Output

Three `Formatter` implementations: `json` (default, agent-oriented,
stdout), `table` (human-readable), and `ndjson` (streaming for large
result sets). `--fields a,b.c` projects by dot-path relative to each record.
For list commands, omit the envelope prefix (`--fields id`, not `--fields
items.id`); nested projections retain the full dotted path as a literal output
key (`{"b.c": ...}`). Selecting the containing object preserves nested access
for downstream `jq`. List commands emit `{items, next, has_more}`; `--cursor`
continues from a prior page's `next`.

Successful output is unified as JSON on stdout, with these exceptions:

- `version` prints a plain text version line.
- `pr diff` / `pr diff --path` print unified diff text.
- `file get --output -` writes raw bytes to stdout.
- `file get --output <path>` writes raw bytes to disk.
- `pr fetch` / `pr checkout` with `--exec` pass the git subprocess's
  stdout / stderr through to the parent process's stderr.
- `skill show` prints the embedded `SKILL.md` verbatim.

Prompts from interactive wizards (`config init`, `auth login`) and all
errors go to stderr.

### 7.2 Errors

Errors are JSON on **stderr**:

```json
{"error":{"category":"config","code":"CREDENTIAL_STORE_INACCESSIBLE",
  "message":"stored Bitbucket credentials cannot be read in this execution environment",
  "hint":"The configured credential store is inaccessible from the current process.",
  "next_steps":["Retry the same command with access to the host user environment."],
  "retryable":false,
  "recovery":{"action":"retry_current_command","scope":"host",
    "requires":["user_home","os_keychain"]}}}
```

`recovery` is optional and describes an environment change; `retryable` still
means the same invocation may succeed in the current environment. `doctor`
mirrors this distinction with per-check `status` and optional
`recovery_scope` fields.

Categories: `usage config auth permission not_found conflict rate_limit
network server parse internal`. `extractAPIMessage` extracts the human
message from both Cloud's `{"error":{"message":"..."}}` and DC's
`{"errors":[{"message":"..."}]}` shapes.

### 7.3 Exit codes

| Code | Category | Code | Category |
|------|----------|------|----------|
| 0 | success | 6 | not_found |
| 1 | internal | 7 | rate_limit |
| 2 | usage | 8 | network |
| 3 | config | 9 | server |
| 4 | auth | 10 | parse |
| 5 | permission | 11 | conflict |

`hints.go` maps each category to `next_steps`, guiding agents to
self-correct. **Discoverability rule**: any "missing input" error must
put the discovery command at `next_steps[0]` (see the AGENTS.md
Discoverability section).

## 8. Safety modes

Two orthogonal write-protections, layered on top of `--yes`:

1. **`--dry-run`** is wired on every mutating command. It resolves the
   operation via `Client.DescribeWrite(ctx, op)` and emits the resulting
   `WriteRequestPlan{Method, URL, Payload}` instead of sending the
   HTTP request. The build helper is shared with the live write so the
   preview cannot drift from the actual call. Even Data Center routes
   that depend on a version number (PR decline / merge, comment
   update / delete) reflect the right `?version=N` in the preview,
   because the helper still performs the read-only GET that fetches it.
2. **Read-only mode** is session-level. `defaults.read_only: true` in
   `config.yaml` or `BITBUCKET_CLI_READ_ONLY=1` in the environment
   makes `appState.newClient()` wrap the client in
   `apiclient.NewReadOnly(...)`, which returns a structured
   `READONLY_BLOCKED` (`category=permission`, exit code 5) from every
   mutating client method before any HTTP request is sent. The same
   posture also gates `pr fetch/checkout --exec`. The root persistent
   flag `--allow-writes` overrides the posture for a single
   invocation. `DescribeWrite` (used by `--dry-run`) is intentionally
   not overridden by the wrapper, so previews still work under a
   locked session.

Out of scope: `config init`, `auth login|logout`, `skill install`, and
`file get --output` are CLI self-configuration / local IO, not remote
mutations and not local-worktree mutations — they remain available
under read-only.

## 9. Skill outline

`skills/bitbucket/SKILL.md` (YAML frontmatter: `name: bitbucket`,
trigger description, `metadata.requires.bins: ["bitbucket-cli"]`) +
`references/`:

- `getting-started.md` — configuration / auth / `doctor` /
  `workspace list` discovery and Data Center `whoami` identity validation.
- `pr-workflows.md` — `pr status` → `pr files` → `pr diff --path` flow.
- `reviewing-locally.md` — end-to-end "remote + local" review, including
  `pr fetch --exec`.
- `commenting.md` — inline vs general / `--reply-to`.
- `reading-repos.md` — repo create / fork / delete plus branch / commit browsing.
- `files.md` — the `file` subtree's ref semantics and `--range` usage.
- `safety-modes.md` — `--dry-run` and read-only mode for agents.
- `errors-and-exit-codes.md` — exit codes plus per-category recovery
  steps.

The Skill ships in lock-step with the CLI: bump `version:` in
`SKILL.md` whenever the Skill or its `references/` change. The `skill
show` assertion in `make e2e` smoke-tests that the embedded file is
readable.

`skill install` uses an agent path table (`agentSpecs` in
`internal/app/skill.go`) mapping each agent to its global / project
skills directory: Claude Code uses `~/.claude/skills` and `./.claude/skills`; Codex uses `~/.codex/skills` and `./.agents/skills`; Cursor uses `~/.cursor/skills` and `./.cursor/skills`; the shared Agents tree uses `~/.agents/skills` and `./.agents/skills`; Gemini CLI uses `~/.gemini/skills` and `./.gemini/skills`; GitHub Copilot uses `~/.copilot/skills` and `./.agents/skills`; OpenCode uses `~/.config/opencode/skills` and `./.opencode/skills`; Continue uses `~/.continue/skills` and `./.continue/skills`; Windsurf uses `~/.codeium/windsurf/skills` and `./.windsurf/skills`; Grok Build uses `~/.grok/skills` and `./.grok/skills`; Pi uses `~/.pi/agent/skills` and `./.pi/skills`; Kilo Code uses `~/.kilocode/skills` and `./.kilocode/skills`; Roo Code uses `~/.roo/skills` and `./.roo/skills`.
With no flag it probes which directories exist and installs / removes
for each hit; `--agent` selects explicitly; `--dir` is the
agent-agnostic explicit path.

The Skill handshake carries its version, not a boolean. Runtime hints compare
the loaded version with the embedded copy; `skill status` also reads deployed
`SKILL.md` frontmatter and classifies each installation as current, outdated,
or unknown. Update notices return ordered steps to upgrade the CLI, refresh the
Skill, and reload the agent context. `doctor` reports Skill state as an
informational check that does not change connectivity health.

## 10. Testing strategy

- **Unit tests**: stdlib `testing`, table-driven, `t.Parallel()`.
  Coverage includes config precedence, auth resolution and file
  permissions, offset / cursor pagination, two-flavor mapping
  normalization, every output format and `--fields`, errors mapping,
  urlref, and the read-only wrapper (every mutating method is asserted
  to return a uniform `READONLY_BLOCKED`).
- **HTTP-layer tests**: `httptest.Server` drives every client method;
  assertions cover path / parameters / auth header.
- **End-to-end**: `scripts/e2e.sh` builds the binary against an
  in-process mock Bitbucket (DC REST 1.0 coverage plus Cloud-shaped
  routes) and exercises every command, asserting stdout contract and
  exit codes — **including the discoverability rule** (e.g.
  `repo list` without `--workspace` must include `workspace list` in
  stderr). The dry-run and read-only safety modes are covered here as
  well — every blocked-write path is paired with its `--allow-writes`
  override and `--dry-run` counter-test. Current coverage exceeds 100
  assertions, including live mock fork execution, Cloud fork validation,
  Data Center `whoami`, dry-run, and read-only rejection.
- **Read-only live verification**: `BITBUCKET_E2E_LIVE=1
  ./scripts/e2e.sh` runs only `doctor` / `whoami`-style read-only
  commands against a real server.

## 11. Key design points worth remembering

1. **`FileContent.Bytes` is not base64-wrapped** — Cloud and DC raw
   endpoints both return original bytes, `file get --output -` writes
   straight to stdout, and `--range L1:L2` performs the client-side
   slice via `bytes.Split(b, '\n')[L1-1:L2]`.
2. **`pr files` returns diffstat, not patch** — sorted by
   `LinesAdded+LinesRemoved` descending so the agent can decide which
   files merit a follow-up `pr diff --path`.
3. **`pr status` runs in parallel** — uses `sync.WaitGroup` plus a
   mutex rather than `golang.org/x/sync/errgroup`, keeping the
   dependency footprint zero.
4. **`pr fetch` print-only by default, `--exec` opt-in** — agents
   normally take the print-only output and run it inside their own
   Bash; humans use `--exec`. `--exec` is also gated by read-only
   mode, since it is the only command that mutates the local worktree.
5. **`pr threads` makes no extra request** — reuses the full result of
   `ListPRComments` and regroups in memory, avoiding an extra
   round-trip.
6. **Cloud `pr inbox --role reviewer` fan-out** — Cloud has no global
   reviewer index, so the command requires `--workspace`, iterates
   repos under that workspace, applies server-side
   `q=reviewers.uuid="..."`, and swallows per-repo errors so a single
   bad repo cannot block the aggregate response.
7. **Safety modes are uniform across the surface** — every new
   mutating method must be added to `readOnlyClient`, get a
   `DescribeWrite` case, and have e2e assertions for both the
   `READONLY_BLOCKED` rejection and the `--dry-run` preview. AGENTS.md
   codifies this contract.
8. **Filtered activity is evidence-oriented** — the unfiltered single-PR
   timeline remains complete, while filtered queries omit entries marked
   `system` unless `--include-system` is explicit. Data Center's predefined
   reviewer event arrives as an ordinary comment attributed to the PR author,
   so the mapping layer recognizes the server's native template before the
   command layer applies evidence filters.
9. **Another user's cross-repository inbox is not available on Data Center** —
   the dashboard endpoint is bound to the authenticated user. `pr list
   --repo ... --author/--reviewer ... --all` can filter one known repository,
   following the server's page cursors before applying the user predicate, but
   enumerating every accessible project and repository would be unbounded and
   still could not prove completeness. `--actor <user>` therefore filters known
   PR refs; it does not claim to discover them.

## Team service setup and login persistence

`internal/config/setup.go` owns offline service-field validation, acquisition
links and the pure `PlanServiceContext` merge used by execution and dry-run.
`LoadOptions.Setup` selects the requested destination even when it is new and
filters personal environment fields before credential-based scheme inference.
Ordinary runtime precedence and transient environment secrets remain supported.
`AuthConfig.CredentialURL` is additive, non-secret, and serialized through every
config shape.

`internal/app/setup.go` wires `config set-context`, `auth guide`, and wizard
prefill. `internal/app/auth_login.go` checks the complete normalized service URL,
verifies authentication, stores the secret, and persists the associated identity.
Expected storage failures preserve their causes and distinguish a stored secret
from a completed login. Error payloads may include optional non-secret `details`;
context conflicts use field-level `before`/`after` values. Setup does not access a
credential store and is available under the remote read-only posture.

The end-to-end setup harness (`scripts/e2e-setup.sh`, part of `make e2e`) runs
without a service or user credentials. Unit tests cover target-specific
precedence, persistence failures, guide URLs, and reloading a stored login.
See [installation](installation.md#team-distribution-and-personal-login) for the
canonical user-facing contract and product-specific authentication guidance.
