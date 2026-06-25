# Responding to review comments on your own PR

This is the recommended flow when the human is the PR **author** and has
*received* review feedback. The agent's job is to triage each open thread:
understand the comment, locate the related code, judge whether the comment is
correct, decide how to handle and verify it, and draft a reply.

This is the inverse of `reviewing-locally.md` (where the agent *produces* a
review). The two share the same coarse→fine navigation and local-checkout
posture.

## Entry points

Most of the time the user **already has a specific PR** — they paste a PR URL or
a `<ws>/<repo>/<id>` ref ("address the review comments on this PR"), and sometimes
point at one comment ("reply to this comment"). That is the primary path:

- **Given a PR** → use the ref directly: `pr threads <ref> --unresolved`.
- **Given one comment** (id, or a Bitbucket comment permalink whose `#comment-<id>`
  anchor you can read) → target just that thread: `pr threads <ref> --comment <id>`.
- **No PR in hand** → discover via the inbox: `pr inbox --role author` lists the
  user's open PRs; pick one and continue with its ref.

## Default posture: recommend & draft, then confirm

By default this workflow is **read-only analysis**. The agent reads the PR and
produces a triage report plus draft replies, but does **not** post anything,
push code, or change PR state until the human confirms. Preview every write with
`--dry-run` first, and respect `BITBUCKET_CLI_READ_ONLY` when the user asks for a
locked session. See `safety-modes.md`.

## The decision tree

```
PR ref / PR URL  (have one already)        pr inbox --role author  (don't)
    └──────────────────┬───────────────────────────┘
                       ▼
pr get --scope summary   ─ title + description (the PR's intent)
pr commits / pr status   ─ what shipped, is it mergeable / CI green?
    │
    ▼
pr threads --unresolved              ─ all open threads on the PR
pr threads --comment <id>            ─ OR just the one thread the user named
    │                                  (skips resolved threads; see commenting.md)
    │
    ▼
Local codebase present?  ─ is cwd a git checkout of THIS PR's repo?
    │
    ├─ yes ─→ pr fetch --exec ; pr checkout --exec
    │         <Read/Grep the real tree; run tests/build to VERIFY>
    │
    └─ no  ─→ degrade to remote read-only: pr diff --path <file>
              (offer to clone if the user wants local verification)
    │
    ▼
For each open thread: apply the checklist (below) → draft a reply
    │
    ▼
Emit a triage report (table)
    │
    ▼  (only after the human confirms)
comment add --reply-to <id> --dry-run   ─ preview, then post
<apply code fixes on the local checkout for the human to review/push>
```

## Prefer a local codebase

A local checkout is what turns "reasoning from the diff" into **genuine
verification**. Before per-thread work, check whether the agent's `cwd` is a git
checkout of the PR's repo:

- **Local checkout present** → `pr fetch --exec` then `pr checkout --exec`, then
  use the agent's filesystem tools (Read/Grep) on the real tree. Crucially, you
  can now *run* the repo's tests / build / linter and reproduce the concern
  instead of guessing from the patch.
- **No local checkout** → say so, offer to clone (or note the user can), and
  otherwise degrade gracefully to remote read-only analysis via
  `pr diff --path <file>`.
- **Different repo than `cwd`** (e.g. a Cloud fork) → use `pr diff --path`; don't
  fetch into an unrelated tree.

This mirrors the headline "Review a PR (with local codebase)" workflow — a local
clone is opportunistic but strongly encouraged.

## Per-thread checklist

For each open thread, read the anchored code (local Read/Grep when available,
else `pr diff --path <file>` mapping the inline `line` to its hunk), then:

1. **Understand / reproduce** the concern before judging — *run it locally if a
   checkout exists* (failing test, build error, the actual behaviour).
2. **Classify** the comment: bug · style/nit · question · out-of-scope ·
   already-handled.
3. **Judge validity** against the *actual code* **and** the PR's intent (from
   `pr get` description), not just the comment text. Reviewers can be wrong or
   working from stale context.
4. **Decide the action**: fix · explain · push back (with rationale) · defer
   (file a follow-up).
5. **Define a verification step**: prefer an executable check (a test, a build, a
   repro) on the local checkout; fall back to a described manual check when
   remote-only.
6. **Draft a concise reply** that cites the diff line and states the outcome
   (e.g. "Fixed in <commit> — added a test covering the empty-input case.").

## Emit a triage report

Summarize before writing anything back. A compact table the human can scan:

| Thread | Location | Comment (summary) | Verdict | Action | Verification | Draft reply |
|--------|----------|-------------------|---------|--------|--------------|-------------|
| 9003 | src/app.go:20 | "Please rename this" | valid (nit) | fix | `go build` | "Renamed to `…`." |

## Writing back (only after confirmation)

- **Reply to a thread** — preview, then post:
  ```sh
  bitbucket-cli comment add --pr myws/myrepo/42 --reply-to 9003 \
    --content "Renamed to fooBar; thanks." --dry-run
  bitbucket-cli comment add --pr myws/myrepo/42 --reply-to 9003 \
    --content "Renamed to fooBar; thanks."
  ```
- **Apply code fixes** on the local checkout and let the human review/push — this
  workflow does not push on the user's behalf by default.
- **Resolve or reopen a thread:** `comment resolve <comment-id> --pr <ref>`
  (add `--unresolve` to reopen). Filter to open threads when listing with
  `comment list --pr <ref> --unresolved`. On Data Center this also completes /
  reopens the associated task. Works on both flavors; `--dry-run` previews it.

## Inputs the agent needs

- A PR reference, or `BITBUCKET_DEFAULT_WORKSPACE` set so `pr inbox --role author`
  can find the user's PRs.
- For local verification: `cwd` must be a git working tree for the PR's repo with
  the right `origin` remote (the CLI checks this and errors otherwise).
