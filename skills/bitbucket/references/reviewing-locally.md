# Reviewing a pull request

Review the PR's actual changes, verify candidate findings, and report the result
to the user. Publishing comments and changing PR state depend on the requested
scope. **Completing a review does not require leaving a comment.**

For feedback received on the user's own PR, use
[Responding to review comments](responding-to-review-comments.md).

## Choose the next read

Reuse the supplied PR URL or `<workspace>/<repo>/<id>` and context already
verified for it. If no PR is known, use `pr inbox` (open PRs where the user is a
reviewer) or `pr inbox --role author`. Cloud reviewer discovery requires
`--workspace`; Data Center uses the cross-repository inbox.

- **Intent:** `pr get <ref> --scope full` includes the description. Read linked
  requirements or commit history only when they help judge the change.
- **Readiness:** `pr status <ref>` reports conflicts, reviewer states, and CI.
  Failed CI or conflicts are context, not an automatic reason to stop reviewing.
- **Scope:** `pr files <ref>` lists paths and churn. Use `pr diff --path <path>`
  for focused reads; a full diff can be cheaper for a small PR. For broad code
  searches or local verification, use the checkout preflight below.
- **Existing discussion:** `pr threads <ref>` includes resolved and unresolved
  threads. Read it before publishing, and earlier when it can answer a question
  or prevent duplicate investigation. Use `--comment <id>` for a known thread;
  an unresolved-only view is insufficient for deduplicating a new review.

Budget reads around the requested coverage and plausible failure paths. Inspect
callers and tests when needed to establish impact; do not equate green CI with
correctness or claim coverage for files you skipped.

## Verify findings

A publishable finding identifies a concrete defect introduced or exposed by the
PR, a reachable trigger, and a meaningful consequence. Support it with the
current code, a focused reproduction, or a relevant failing test. Explain why it
matters and what needs to change in a short, self-contained comment.

Keep speculative concerns, style preferences without a project requirement,
and unrelated pre-existing defects out of default PR feedback. When evidence is
incomplete, investigate further or report the uncertainty to the user; do not
turn an unverified suspicion into a question on the PR.

## Decide what to publish

By default, publish only **new, actionable, high-confidence findings**, within
existing authorization. Compare each candidate with the full discussion,
including resolved threads, by underlying issue rather than wording.

| Situation | Action |
| --- | --- |
| New verified issue with a reasonable code anchor | Post one inline comment at the relevant line. |
| Existing issue, no new evidence | Do not comment or repeat the finding in a summary. |
| Existing issue, new evidence | Reply in the original thread with the evidence and its implication; do not open a duplicate thread. |
| New verified issue that cannot reasonably be anchored to code | Post a concise PR-level comment explaining the issue and affected locations. |
| User explicitly requests an overall summary on the PR | Publish the requested summary; distinguish confirmed findings from incomplete coverage. |
| No publishable findings | Report the result to the user; leave no comment. |

Completion records, passing-review summaries, test procedures, and environment
limitations belong in the **user report by default**, not on the PR. A focused
reproduction that establishes a defect is evidence for that finding, not a
routine test log to append. Do not post a PR-level comment merely to record that
the review happened or that comments were left elsewhere. An inline failure
alone does not establish that an issue lacks a reasonable code anchor; use the
[anchor recovery rules](commenting.md#inline-line-anchored-comments).

Before publishing, refresh PR metadata and relevant threads. If source or target
commits moved, refresh the diff and revalidate affected findings and anchors.
For local reviews, fetch again and compare the source and base commits. Reuse
verified context while those inputs remain unchanged.

For replies, classify authorship and apply the
[human-reply gate](replying-to-people.md), including when a human has replied
inside an agent-started thread. New evidence does not waive that gate. Preview
each write with `--dry-run`, retain AI attribution on agent-written text, and
respect [read-only mode](safety-modes.md). See [Commenting](commenting.md) for
command syntax. Approval, request-changes, decline, resolution, and merge are
separate actions; perform them only when covered by the user's request.

## Missing intent or context

First check the full description, existing discussion, relevant code and
history, and linked requirements. If a gap still blocks judgment, report the
specific uncertainty and a concise question to the user, and continue reviewing
unaffected changes. A context gap alone is not a publishable defect.

If the user asks you to seek clarification on the PR, anchor the question when
possible; use a PR-level question only when no reasonable code location exists.
Reuse an existing thread rather than repeating an unanswered question, and
apply the human-reply gate. Report any deferred items to the user. Do not poll
for a human response unless monitoring was requested. When the review resumes,
read the relevant thread, incorporate the answer, and revalidate the concern;
do not automatically send an acknowledgment or restatement.

## Local checkout preflight

Before treating worktree files as PR code, record the repository root, remotes,
current branch, HEAD, and dirty state.

- **Match the repository and remote.** Confirm that the chosen remote belongs
  to the PR repository or its canonical upstream. The CLI checks only that
  `cwd` is a Git worktree. Use remote diffs if the mapping is uncertain.
- **Fetch authoritative refs.** `pr fetch --exec` fetches the PR source and base
  without switching the worktree. Use its `source_ref`, `base_ref`, and
  `review_diff`; an existing local `pr/<id>` branch may still be stale.
- **Preserve user changes.** Never overwrite, stash, or mix uncommitted changes
  into the review. Use remote diffs, inspect the fetched ref directly with
  `git show <source_ref>:<path>`, or create an isolated worktree.
- **Verify filesystem alignment.** Read or test worktree files only when the
  tree is clean and HEAD matches the fetched source commit. `pr checkout --exec`
  switches branches; use it only when that change is safe and authorized.
- **Honor read-only posture.** It also blocks `pr fetch/checkout --exec`; use
  remote reads when the session must remain read-only.

## Reviewing against the right base

Use the PR's change relative to its merge-base, not just the final source files
or a diff against a stale local `main`.

`pr fetch` and `pr checkout` select an explicit `--remote` first, otherwise
prefer `upstream` over `origin`. Remote selection does not verify its URL. They
fetch both the PR source ref and its destination branch; `--base <branch>`
overrides base discovery and should be used only when that override is intended.

The output includes `remote`, `source_ref`, `base_branch`, `base_ref`,
`base_commit`, and `review_diff`. The latter is a triple-dot diff:
`git diff <remote>/<base>...<remote>/pr/<id>`. Run that emitted diff and inspect
the fetched source. Avoid fetching into an unrelated checkout or fetching at all
when remote reads already provide enough evidence.

## Report to the user

Lead with findings or state that no actionable findings were found within the
reviewed scope. Include decisive code references, material coverage gaps,
verification results and environment limits. If comments were published, link
them and distinguish posted findings from drafts or skipped duplicates. Keep
this report in the conversation unless the user asks to publish it on the PR.
