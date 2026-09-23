# Reviewing a pull request

Review the final change for functional correctness and significant
maintainability problems, then publish only what the request authorizes.
**Completing a review does not require a comment or a vote.** Feedback on the
user's own PR follows [Responding to review comments](responding-to-review-comments.md).

## Checklist

1. **Scope and authorization.** Reuse the supplied PR URL or
   `<workspace>/<repo>/<id>`; without one, run a bounded `pr inbox` (Cloud
   reviewer discovery needs `--workspace`). Record separately whether the request
   authorizes analysis, comments, approval, or Needs Work: "review" alone
   authorizes a report, and "approve if ready" does not authorize Needs Work.
   Reuse existing authorization instead of asking again.
2. **Read context with the cheapest reads.** `pr get <ref> --scope full`
   (description, `source` / `destination` branches and commits, reviewers);
   `pr status <ref>` (reviewer states, conflicts, CI); `pr files <ref>` to budget
   paths and churn; `pr diff <ref>`, or `pr diff <ref> --path <path>` for a
   large change; `pr threads <ref>` for the full discussion, resolved threads
   included (`--comment <id>` for one thread). A green check does not prove
   correctness; a failed check or conflict does not prevent independent review.
3. **Pin revisions.** Record `source.commit` and `destination.commit`, and
   review the change relative to their merge-base, never a stale local default
   branch. Use a [local checkout](#local-checkout)
   only when broad searches or tests need one.
4. **Verify findings** with the [evidence rules](#verify-findings), including
   claims in existing comments. Read callers, contracts, tests, history, or
   linked requirements only to settle a specific uncertainty; check generated or
   mechanical changes with a tool. Cover all meaningful changed code before a
   whole-PR approval; a deliberately partial review remains a partial report.
   Several PRs, or one too large for a pass: [Reviewing batches](reviewing-batches.md).
5. **Choose a verdict** from the [verdict table](#choose-a-review-verdict).
6. **Publish** within the [publication rules](#decide-what-to-publish), after
   [refreshing revisions and votes](#refresh-execute-and-verify).
7. **Report** to the user: [Report to the user](#report-to-the-user).

## Local checkout

Find an existing matching clone or worktree before creating one, and record
its root, remotes, branch, HEAD, and dirty state (staged and untracked files
included).

- **Match the repository and remote.** Verify the complete host/repository
  identity, including the canonical destination and a fork source; a matching
  directory or branch name is not evidence. `pr fetch` checks only that `cwd` is
  a Git worktree and defaults to `upstream` before `origin` without checking its
  URL, so pass the verified remote explicitly and never fetch into an unrelated
  checkout.
- **Fetch without switching.** Preview, then fetch when local writes are allowed:

  ```sh
  bitbucket-cli pr fetch <ref> --remote <remote> --fields source_ref,base_ref
  bitbucket-cli pr fetch <ref> --remote <remote> --exec
  ```

  Resolve `source_ref` and `base_ref` to commit IDs and compare both with
  refreshed PR metadata; an old `pr/<id>` branch or the `base_commit` field
  does not prove what the fetch obtained.
- **Preserve the user's workspace.** Reuse an existing tree only when it is
  clean, at the verified source commit, and the intended checks will not disturb
  user work; otherwise create a detached worktree at that commit. Do not stash,
  reset, clean, force checkout, or move the user's branch. `pr checkout --exec`
  switches branches; reserve it for an explicitly authorized checkout.
- **Honor read-only posture.** It also blocks `pr fetch/checkout --exec`. An
  explicit no-local-write scope rules out worktrees and Git bypasses; use the
  remote diff and `file get --ref <source.commit>` instead.

### Diff against the right base

The destination can be a release branch or another PR; use `--base` only for an
intended override and report that scope. If history is shallow or the merge-base
is not unique, obtain the missing history when allowed or use the server diff
and state the limitation; never silently substitute another base. With
`source_ref` and `base_ref` from the verified fetch output and `review_dir` a
new path for this review:

```sh
source_commit=$(git rev-parse --verify "${source_ref}^{commit}")
destination_commit=$(git rev-parse --verify "${base_ref}^{commit}")
merge_base=$(git merge-base "$destination_commit" "$source_commit")
git diff "$merge_base" "$source_commit" --
git worktree add --detach "$review_dir" "$source_commit"
```

Stop on any failure after confirming the resolved commits match the PR. The
diff equals `git diff <destination-commit>...<source-commit>`; the CLI's
`review_diff` uses mutable refs, so pin IDs before handing work to other agents.
Distinguish the PR delta from compatibility with the current destination and
check changed contracts when destination-only changes could matter; approval is
not a claim that merge checks pass. Read and test from the verified source
tree, remove only review-created worktrees after preserving useful artifacts,
never force-remove a dirty or user-owned tree, and report any retained worktree.

## Verify findings

A publishable finding identifies a problem introduced or materially worsened by
the PR, supports it with current code or a verified contract or project rule,
and proposes a concrete, minimal correction that preserves required behavior.

- **Functional defects** need a reachable trigger and a meaningful behavior
  consequence, shown by code evidence, a focused reproduction, or a relevant
  failing test.
- **Maintainability problems** need a significant, avoidable burden: misleading
  future edits, one policy maintained in several places, or obscured control
  flow. Explain the burden and what a simplification must preserve. Do not
  invent a runtime failure to justify a code-health finding, and do not treat
  every maintainability point as optional; rank by actual impact.

Assess the final artifact regardless of who or what wrote it; describe the
problem rather than guessing AI authorship. Keep speculative concerns, style
preferences without a project rule, and unrelated pre-existing defects out of
default feedback. When evidence is incomplete, investigate further or report
the uncertainty to the user; never turn a suspicion into a question on the PR.

### Maintainability cues

Investigation cues, not automatic findings:

- **Exploration residue:** abandoned approaches, debugging paths, obsolete
  TODOs, comments recording intermediate attempts. Keep text that still
  explains a current constraint or a shipped version.
- **Comment noise or drift:** narration of obvious operations, repetition that
  hides an important constraint, claims about code that no longer exists. Apply
  [information placement](#place-information-for-its-reader) first.
- **Repeated policy or needless divergence:** compare new code with existing
  helpers and conventions; confirm the contracts permit reuse and name the
  places that would otherwise have to change together.
- **Unneeded abstraction:** trace the real consumers of extra layers,
  registries, or generic interfaces. One production implementation can still
  serve a test or extension boundary; check those uses before proposing less.
- **Unsupported defensive paths:** verify input guarantees and error contracts
  before challenging checks or fallbacks. Keep validation at trust boundaries and
  distinguish a fallback that hides failure from useful recovery.
- **Ineffective tests or incidental expansion:** identify which production
  behavior a test exercises and which regression its assertions would catch;
  check that added documentation, wrappers, and configuration serve the change.

Length, repetition, or an absent caller in the diff is not evidence by itself.
Explain the concrete burden and minimal remedy; omit wording preferences and
cleanup requests unrelated to this change.

## Place information for its reader

Ask what still-valid information a maintainer would lose by removing a comment
and whether the code already communicates it:

| Information | Destination |
| --- | --- |
| Local invariants, protocol limits, compatibility reasons, or a subtle boundary needed for safe edits | A concise code comment; link detailed background when useful. |
| API purpose, inputs, outputs, errors, and usage contracts | API documentation or documentation comments. |
| The problem this PR solves, final behavior, tradeoffs, and migration effects | The PR description, per [Writing PR descriptions](writing-pr-descriptions.md). |
| Lasting architectural decisions and significant alternatives across modules | A design document or ADR, linked from code and the PR. |
| Intermediate attempts with no remaining maintenance value | Remove; do not relocate the development diary into another document. |

If A and B were abandoned inside this PR before choosing C, remove that
chronology but keep the current constraint that requires C (for example,
following a next-page cursor even for an empty page). Compatibility history for
supported releases, incident lessons, necessary algorithm explanations, license
notices, tool directives, and required API documentation stay.

## Missing intent or context

If a gap still blocks judgment after the description, discussion, code,
history, and linked requirements, report the specific uncertainty and a concise
question to the user and keep reviewing the unaffected changes; a context gap
is not a publishable defect. Raise it on the PR only when asked: anchor it to
code, reuse an existing thread, apply the human-reply gate, and do not poll for
the answer unless monitoring was requested. On resuming, incorporate the answer
and revalidate without posting an acknowledgment.

## Choose a review verdict

Separate **confidence** (how well the evidence establishes a conclusion) from
**impact** (whether the issue must block this PR). High confidence requires
verified source/base identity, sufficient coverage of the changed behavior and
its contracts, risk-appropriate verification, and no material open question.
No comments, green CI, a worker's verdict, or another reviewer's approval alone
cannot supply it; do not invent numerical scores.

**Discussion and votes.** Verify comments against current code and contracts,
regardless of authorship. Classify claims as confirmed, non-blocking, disproved,
fixed, or uncertain; model branding and repeated agreement are not evidence.
Read current votes from `participants[].state` on Cloud and from both
`reviewers[].state` and `participants[].state` on Data Center. Match users by
stable identity; Cloud's `reviewers` lists assignments without vote states.
Votes are `approved`, `changes_requested` (Cloud), or `needs_work` (Data Center).
Older approvals do not cover new changes; withdrawn requests are not blockers.
If authorship or state is unclear, make a bounded read and withhold dependent
votes. Uncertain authorship counts as human for the reply gate, not as proof
of a human vote.

Apply in order, within the action scope:

| Evidence | Verdict |
| --- | --- |
| A current, independently verified defect or significant maintainability problem warrants correction before acceptance | **Needs Work.** Cite the concrete trigger/impact or maintenance burden; existing comments may already hold the evidence. |
| Another verified human reviewer currently has Needs Work | **Needs Work as a collaboration hold.** Inspect their reasoning and confirm it where possible. Do not counter-approve an active human block. If the reason is missing, disputed, or disproved, report that distinction to the user instead of inventing a defect or claiming agreement. |
| No verified blocker or active human hold, but coverage, revision identity, or a material question remains open | **No vote yet.** Continue bounded investigation or report the precise gap. Uncertainty is neither approval nor a technical Needs Work finding. |
| Sufficient current evidence supports acceptance; no blocker or active human hold remains | **Approve.** Other approvals corroborate but are not required. Minor, genuinely non-blocking suggestions may remain. |

The collaboration hold is a policy of this workflow, not proof of a defect: it
follows a currently active human vote, never an old event, an unresolved
comment, or an unverified automated verdict, and it is re-evaluated when the
human updates the vote. Keep it distinct from technical findings in the user
report. Approve for demonstrated acceptability, not perfection; another reviewer
cannot waive a real defect or fill an unreviewed area by voting, and unresolved
CI failures relevant to acceptance can prevent confidence.

## Decide what to publish

Publish only new, actionable, high-confidence findings, and only when
commenting is authorized. Compare against the full discussion, including
resolved threads, by underlying issue. Combine manifestations of one root cause
at the most useful anchor and mention the other locations there.

| Situation | Action |
| --- | --- |
| New verified issue with a reasonable code anchor | One inline comment at the relevant line. |
| Existing issue, no new evidence | No comment and no repeat in a summary. A justified Needs Work vote may still be appropriate. |
| Existing issue, new evidence | Reply in the original thread, subject to the human-reply gate; do not duplicate it. |
| New verified issue with no reasonable anchor | One concise PR-level finding naming the affected locations. |
| User explicitly requests a summary on the PR | Publish it, distinguishing findings, coverage limits, and any collaboration hold. |
| No publishable findings | No comment. An independently justified, authorized vote may still proceed. |

Completion records, passing summaries, test procedures, environment limits,
and disputed collaboration holds belong in the user report; never post a
comment merely to record that review happened. A failed inline write does not
make an issue unanchorable: use [anchor recovery](commenting.md#inline-line-anchored-comments).

## Refresh, execute, and verify

Immediately before any comment or vote, refresh what the decision depends on:

```sh
bitbucket-cli pr get <ref> --scope full --fields source.commit,destination.commit,reviewers,participants
bitbucket-cli pr threads <ref>
```

If revisions moved, inspect the new diff as allowed and revalidate affected
findings, anchors, coverage, and the verdict. If only discussion or votes
changed, reconsider the decision without repeating unchanged code review. A dry
run does not lock PR revisions.

For replies, classify authorship and apply the
[human-reply gate](replying-to-people.md), including a human reply inside an
agent-started thread; new evidence does not waive it. Keep AI attribution on
agent-written text and use [Commenting](commenting.md) for syntax.

Preview each authorized write with `--dry-run`, respect
[read-only mode](safety-modes.md), execute once, and verify by reading state;
if the intended vote is already current, report it without writing again.
Approval, request-changes, unapproval, decline, thread resolution, and merge
are separate actions, approval needs no companion comment, and decline is never
a substitute for Needs Work ([vote commands](pr-workflows.md#reviewing)). For
failures or uncertain writes, use [PR permissions and recovery](pr-permissions.md)
and reconcile state before any retry or tool switch. Batch approvals only after
each target independently passes review and authorization; inspect each result
after a partial failure.

## Report to the user

Lead with findings, or state that no actionable findings were found in the
reviewed scope. Include decisive references, source/base identity, coverage
gaps, checks run, and environment limits; identify a collaboration hold
separately from verified defects. Link published comments and votes, and
distinguish verified writes, existing state, proposals, skipped duplicates, and
blockers. Keep this report in the conversation unless asked to publish it.
