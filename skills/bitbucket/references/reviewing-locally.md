# Reviewing a pull request

Review the PR's final changes for functional correctness and significant
maintainability problems within the requested coverage. Verify candidate
findings and report the result to the user. Publishing comments and changing PR
state depend on the requested scope. **Completing a review does not require
leaving a comment.**

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

Start from the final diff relative to the merge-base. For a candidate concern,
read the surrounding function or file, relevant callers, contracts, tests, and
applicable existing implementations as needed to establish its impact. Check
project guidance before treating a local pattern as unnecessary. Use history
only to resolve a specific uncertainty. Bound these reads to the requested
coverage; do not equate green CI with correctness or claim coverage for files
you skipped.

## Verify findings

A publishable finding identifies a problem introduced or materially worsened by
the PR, supports it with current code or a verified contract or project rule,
and proposes a concrete, minimal correction that preserves required behavior.

- **Functional defects:** identify a reachable trigger and a meaningful behavior
  consequence. Use code evidence, a focused reproduction, or a relevant failing
  test to establish the defect.
- **Maintainability problems:** identify a significant, avoidable burden, such as
  misleading future edits, maintaining the same policy in several places, or
  obscuring the main control flow. Explain the burden and what the simplification
  must preserve. A runtime failure or failing test is not required; do not invent
  one to justify a code-health finding. Prioritize by actual impact rather than
  automatically treating all maintainability feedback as optional.

Assess the final artifact regardless of who or what wrote it. Describe the
specific problem; do not guess AI authorship or label a contribution "AI slop."

Keep speculative concerns, style preferences without a project requirement,
and unrelated pre-existing defects out of default PR feedback. When evidence is
incomplete, investigate further or report the uncertainty to the user; do not
turn an unverified suspicion into a question on the PR.

## Review maintainability

Use these as investigation cues, not automatic findings:

- **Exploration residue:** abandoned approaches, temporary debugging paths,
  obsolete TODOs, and comments recording intermediate attempts. Check whether
  the information still explains a current constraint or a shipped version.
- **Comment noise or drift:** narration of obvious operations, repeated
  explanations that hide important constraints, or claims about code that no
  longer exists. Apply [information placement](#place-information-for-its-reader)
  before suggesting removal or relocation.
- **Repeated policy or unnecessary divergence:** compare new implementations
  with suitable existing helpers and project conventions. Establish that the
  contracts and dependency boundaries permit reuse; identify the maintenance
  points that would otherwise have to change together.
- **Unneeded abstraction:** trace the actual consumers and requirements of
  extra layers, registries, configuration, or generic interfaces. A single
  production implementation may still support an important test or extension
  boundary. Propose a simpler path only after checking those uses.
- **Unsupported defensive paths:** verify input guarantees and error contracts
  before challenging repeated checks or fallbacks. Preserve validation at trust
  boundaries and distinguish a fallback that hides failure from useful recovery.
- **Ineffective tests or incidental expansion:** determine which production
  behavior the test exercises and which regression its assertions would catch.
  Check whether added documentation, wrappers, and configuration serve the
  requested behavior. Test doubles and repeated test cases can be justified.

Length, repetition, or an absent caller in the diff alone is insufficient
evidence. Explain the concrete burden and minimal remedy; omit isolated wording
preferences and broad cleanup requests unrelated to this change.

## Place information for its reader

Ask what still-valid information a maintainer would lose by removing a comment,
and whether the code already communicates it clearly. Keep information near the
reader who needs it:

| Information | Destination |
| --- | --- |
| Local invariants, protocol limits, compatibility reasons, or a subtle boundary needed for safe edits | A concise code comment; link detailed background when useful. |
| API purpose, inputs, outputs, errors, and usage contracts | API documentation or documentation comments. |
| The problem this PR solves, final behavior, important tradeoffs, and migration effects | The PR description, following [Writing PR descriptions](writing-pr-descriptions.md). |
| Lasting architectural decisions and significant alternatives across modules | A design document or ADR, linked from relevant code and the PR. |
| Intermediate attempts with no remaining maintenance value | Remove them; do not relocate the development diary into another document. |

For example, if A and B were abandoned inside this PR before selecting C, remove
that chronology. Retain the current server constraint that requires C, such as
following a next-page cursor even for an empty page. Compatibility history for
supported releases, incident lessons, and necessary algorithm explanations can
remain valuable. Preserve license notices, tool directives, and required API
documentation. Simplification must retain this information and required behavior.

## Decide what to publish

By default, publish only **new, actionable, high-confidence findings**, within
existing authorization. Compare each candidate with the full discussion,
including resolved threads, by underlying issue rather than wording.
Prioritize findings by impact and confidence. Combine manifestations of one
root cause into one comment at the most useful anchor, mentioning other affected
locations there; do not produce one notification per occurrence or pad the review
with minor preferences. Distinct verified defects can still warrant separate comments.

| Situation | Action |
| --- | --- |
| New verified issue with a reasonable code anchor | Post one inline comment at the relevant line. |
| Existing issue, no new evidence | Do not comment or repeat the finding in a summary. |
| Existing issue, new evidence | Reply in the original thread with the evidence and its implication; do not open a duplicate thread. |
| New verified issue that cannot reasonably be anchored to code | Post a concise PR-level comment explaining the issue and affected locations. |
| User explicitly requests an overall summary on the PR | Publish the requested summary; distinguish confirmed findings from incomplete coverage. |
| No publishable findings | Report the result to the user; leave no comment. |
| No findings and approval is authorized | Approve without a companion comment; verify the approval and report it to the user. |

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
Existing authorization for the specific action remains valid; do not ask again
just because the review or dry run is complete. Approval needs no "LGTM", passing
test summary, or completion comment, including in a browser's optional comment
box. Bitbucket's automatic activity entry is sufficient. If a PR action fails,
follow [PR permissions and recovery](pr-permissions.md); keep tooling blockers
in the user report rather than posting them on the PR.

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
