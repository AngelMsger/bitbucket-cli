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

## Scope and authorization

For a request to assess feedback, produce a triage report and useful draft
replies. Apply fixes or publish replies when the user's request authorizes them;
reuse that authorization rather than asking again after a dry run. Permission
to reply does not itself authorize a push or a PR state change.

Before replying, apply [Replying to people](replying-to-people.md): classify the
root and the message being answered, show human feedback with your reasoning
and concrete draft, and obtain per-item approval. Explain the reason once per
session. Bot or agent replies use existing authorization. Preview writes and
respect [read-only mode](safety-modes.md).

## The decision tree

```
PR ref / PR URL  (have one already)        pr inbox --role author  (don't)
    └──────────────────┬───────────────────────────┘
                       ▼
pr get --scope full      ─ title + description (the PR's intent)
pr commits / pr status   ─ what shipped, is it mergeable / CI green?
    │
    ▼
pr threads --unresolved              ─ all open threads on the PR
pr threads --comment <id>            ─ OR just the one thread the user named
    │                                  (a targeted read can include a resolved thread)
    │
    ▼
Local codebase present?  ─ preflight: right repo, branch/HEAD known, clean?
    │
    ├─ yes ─→ pr fetch --exec
    │         <Use source_ref/review_diff; read/test only from an aligned tree>
    │
    └─ no  ─→ degrade to remote read-only: pr diff --path <file>
              (offer to clone if the user wants local verification)
    │
    ▼
For each open thread: assess → act or draft a reply when useful
    │
    ▼
Emit a triage report (table)
    │
    ▼  (within authorization and the human-reply gate)
comment add --reply-to <id> --dry-run   ─ preview, then post
<apply code fixes on the local checkout for the human to review/push>
```

## Prefer a local codebase

A local checkout is what turns "reasoning from the diff" into **genuine
verification**. Before per-thread work, follow the repository identity, branch,
HEAD, and dirty-state preflight in `reviewing-locally.md`, then check whether the
agent's `cwd` can safely represent the PR:

- **Local checkout present** → `pr fetch --exec`, then use its `source_ref` and
  `review_diff`. To use filesystem tools (Read/Grep) or run tests / build /
  linter, first verify that a clean checkout or isolated worktree matches
  `source_ref`. Use `pr checkout --exec` only when changing the current worktree
  is safe.
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

0. **Identify who wrote it** — check the root and the message being answered
   for the `[[AI]](…)` marker and author metadata. Marker or bot account →
   AI-authored; anything you cannot classify → treat it as a person. This decides
   which confirmation gate applies (`replying-to-people.md`).
1. **Understand / verify** the concern before judging. Reproduce behavioral
   defects when a checkout is available; assess maintainability against the
   [review evidence rules](reviewing-locally.md#verify-findings).
2. **Classify** the comment: bug · maintainability · style/nit · question · out-of-scope ·
   already-handled.
3. **Judge validity** against the *actual code* **and** the PR's intent (from
   `pr get` description), not just the comment text. Reviewers can be wrong or
   working from stale context.
4. **Decide the action**: fix · explain · push back (with rationale) · defer
   (file a follow-up).
5. **Define a verification step**: prefer an executable check (a test, a build, a
   repro) on the local checkout; fall back to a described manual check when
   remote-only. For comment or structural cleanup, inspect the final change
   against the constraint or behavior it must preserve.
6. **Draft a concise reply when there is something new to communicate**, such as
   a fix, new evidence, or an answer the user requested. Leave already-covered
   issues without new evidence alone; follow the
   [publication rules](reviewing-locally.md#decide-what-to-publish). Cite the diff
   line and state the outcome
   (e.g. "Fixed in <commit> — added a test covering the empty-input case.").

## Emit a triage report

Summarize before writing anything back. A compact table the human can scan:

| Thread | Author | Location | Comment (summary) | Verdict | Action | Verification | Draft reply |
|--------|--------|----------|-------------------|---------|--------|--------------|-------------|
| 9003 | human | src/app.go:20 | "Please rename this" | valid (nit) | fix | `go build` | "Renamed to `…`." |
| 9014 | bot (linter) | src/app.go:88 | "unchecked error" | valid | fix | `go test ./...` | "Handled; test added." |

The **Author** column tells the human which threads they need to answer themselves
and which ones you can close out with a lighter touch.

## Writing back

For a human-authored thread, "confirmation" means the author saw the reviewer's
point in the reviewer's own words, saw your reasoning, and approved *that* reply —
one thread at a time. If they rewrite the draft, post their text verbatim and drop
the `[AI]` marker. See `replying-to-people.md`.

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
  **Do not resolve a person's thread on your own** — it asserts "I agree, and this
  is handled" under the author's name. Propose it and let them confirm; if they
  disagree with the reviewer, leave it open for the reviewer to answer.

## Inputs the agent needs

- A PR reference, or `BITBUCKET_DEFAULT_WORKSPACE` set so `pr inbox --role author`
  can find the user's PRs.
- For local verification: `cwd` must be a Git worktree whose chosen remote
  matches the PR repo. The agent verifies that mapping; the CLI checks only that
  `cwd` is inside a worktree.
