# Reviewing a PR alongside a local codebase

This is the recommended flow for a coding agent (Claude Code etc.) reviewing
a Bitbucket PR while it has read access to a local checkout of the repo. It
budgets remote calls and context tokens by going coarse → fine.

## Finding what to review

When the agent doesn't already have a specific PR in hand, start with the
cross-repo inbox:

```sh
bitbucket-cli pr inbox                        # default: --role reviewer --state OPEN
bitbucket-cli pr inbox --role author          # PRs I authored
bitbucket-cli pr inbox --workspace myws       # required on Cloud for --role reviewer
```

On Data Center this hits `/rest/api/1.0/dashboard/pull-requests` and is a
single API call. On Bitbucket Cloud `--role reviewer` requires `--workspace`
(Cloud has no global reviewer index — the CLI fans out across repos in the
named workspace); `--role author` works globally without `--workspace`.

## The decision tree

```
PR URL or <ws>/<repo>/<id>
    │
    ▼
pr status  ─ mergeable? conflicts? required reviewers? CI green?
    │
    ▼ if reviewable
pr files   ─ diffstat: which files changed and how much?
    │
    ├─ small / focused PR  ─→  pr diff --path <file>   (one call per interesting file)
    │
    └─ large PR / wants to grep
                            ─→  pr fetch --exec        (brings PR to local refs/remotes/origin/pr/<id>)
                                pr checkout --exec     (switches to pr/<id> branch)
                                <Read local files directly via the agent's filesystem tools>
    │
    ▼
pr threads ─ existing inline discussion, grouped by file/line
    │
    ▼
comment add --inline <path>:<line>   ─ write inline feedback
comment add --reply-to <id>           ─ continue an existing thread
    │
    ▼
pr approve   or   pr request-changes (Cloud)   or   pr decline --yes
    │
    ▼
pr merge --strategy <merge_commit|squash|fast_forward> --yes
```

## Why this order

1. **`pr status` first** — if `can_merge=false` because of conflicts or a
   failing CI build, the agent should surface that before sinking tokens into
   diff reading. The same call returns reviewers' approve/state, so the agent
   can also tell whether its own approve actually unblocks merge.
2. **`pr files` (diffstat) before any diff** — returns just metadata
   (`path`, `status`, `added`, `removed`, `binary`), sorted by total churn.
   Lets the agent decide which files actually deserve attention.
3. **Per-file diff vs local read** — for files under ~200 lines of churn,
   `pr diff --path <p>` is cheapest. For larger changes, fetching the PR
   locally (`pr fetch --exec` + `pr checkout --exec`) lets the agent use its
   filesystem tooling (Read, Grep) at full power.
4. **`pr threads` before commenting** — see whether the question has already
   been asked / answered; reply with `--reply-to` instead of starting a new
   thread.

## Inputs the agent needs

- `BITBUCKET_DEFAULT_WORKSPACE` set (or `--workspace` on each call) so the
  agent doesn't have to repeat the workspace in every reference.
- For `pr fetch --exec` / `pr checkout --exec`: the agent's `cwd` must be a
  git working tree with the right `origin` remote. The CLI checks this and
  returns a `usage` error otherwise.

## When NOT to fetch locally

- The PR is in a different repository than the agent's `cwd` clone (Bitbucket
  Cloud forks). Use `pr diff --path` instead.
- The agent only needs to read 1–2 files. The remote round-trip is cheaper
  than a local fetch.
