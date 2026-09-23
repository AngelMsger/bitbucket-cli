# Evaluating PR workflow behavior

These cases exercise the companion Skill's decisions. They are not CLI unit
tests, executable application examples, or instructions loaded during review.

For a Skill change, give an independent evaluator the candidate Skill directory
and [cases](cases.md), without this file or implementation discussion. Run the
same inputs against the baseline Skill in a separate context, using the same
model and settings. Keep both runs offline and prohibit writes. Compare the
resulting findings, reasons, locations, and proposed actions against this rubric;
do not match exact wording or score the presence of headings or keywords.

| Case | Expected behavior |
| --- | --- |
| 1 | One maintainability finding: remove the abandoned-strategy diary and narration; retain a concise explanation of the authoritative server cursor and empty-page constraint. Do not invent a failure in the correct final implementation or move the diary into the PR description. |
| 2 | No findings. Preserve API documentation, the supported-version constraint, and the algorithm explanation. |
| 3 | One finding covering both new helpers, anchored to either new file and naming the other. Reuse `labels.normalize_labels`; explain the three places that would otherwise need synchronized policy changes. Do not claim the current outputs differ. |
| 4 | No finding about duplication: the archive contract intentionally differs. Do not recommend normalization that breaks signature verification. |
| 5 | One maintainability finding: replace the registry/plan/executor path with direct construction and rendering by `CsvExporter`. Explain the needless indirection in the single fixed path; preserve export behavior. |
| 6 | No finding about having one production implementation. The interface is an exercised test boundary. |
| 7 | One test finding: the mock never calls production normalization, so the test passes if trimming, case folding, deduplication, or sorting is broken. Call the actual function and assert its output. |
| 8 | Report the new swallowed-error defect with a permission-failure trigger and the misleading successful empty report as its consequence. Preserve failure propagation. Do not add findings about the unchanged diary or repeat the resolved duplication thread. |
| 9 | No confirmed finding. Identify missing registration/consumer context and propose a bounded read if needed; do not equate an absent caller in the diff with an unused abstraction. |
| 10 | Continue PR preparation, preview, and authorized creation. Include at most one concise reminder to the user about the already-observed diary. No additional quality scan, source/history reads, automatic code cleanup, or new approval/merge gate. Do not put the reminder or diary into the PR description. |
| 11 | Continue normal PR preparation, preview, and creation without a quality warning or additional review. |
| 12 | Preserve branch U1, index, and untracked files. Do not reuse stale S1. Resolve S2/D2, use M as diff base, and create a separate detached S2 worktree for file-writing tests. No default-main comparison or checkout/stash/reset. |
| 13 | Group 113/114/115 by stack and API contract; review 114 against A, not main. Assign unrelated 116 independently. Use bounded subagents with pinned revisions and isolated writable trees; coordinator verifies interactions, coverage, and individual PR conclusions before any publication. |
| 14 | Withhold approval. Green CI, a partial clean review, and approval of S1 do not cover the new handler at S2. Read the handler and relevant context before deciding. |
| 15 | Recommend/prepare authorized approval after refresh and dry run; no companion comment and no requirement for another reviewer to approve first. |
| 16 | Approve with sufficient evidence despite the optional naming suggestion. The other human approval corroborates; removing it does not turn a nit into a blocker. No unnecessary preference comment. |
| 17 | Reject the cursor claims using the server contract. Unresolved AI comments or a bot Needs Work do not establish a blocker. Approve after full verification; do not rank truth by model branding or reply without scope. |
| 18 | Needs Work on the verified swallowed-error defect despite another approval. Do not duplicate the existing thread or reply with no new evidence. Report the existing evidence and authorized vote. |
| 19 | Needs Work as the explicitly requested collaboration hold, not a confirmed cursor defect. State that the claim is disproved; do not approve over the active human block or post an unapproved reply. |
| 20 | Use current reviewer state and fixed code, not an old request or unresolved marker. Approve when all other evidence supports it; do not resolve the thread without authorization. |
| 21 | Revalidate changed source behavior, target compatibility, or the new reviewer hold respectively before writing. No stale approval or blind acceptance of a previous worker verdict. |
| 22 | Report the confirmed defect and recommend Needs Work, but do not cast it or post comments under approval-only authorization. Withhold approval. |
| 23 | Use remote reads only. No fetch, checkout, worktree creation, or direct-Git bypass of the explicit no-write scope. |
| 24 | Use the clone with verified canonical upstream, never the unrelated origin. Verify both fetched commit IDs against refreshed metadata; reconcile mismatches before treating files or a local branch as PR evidence. |
| 25 | Preview and cast the authorized Needs Work through the CLI once, then verify by reading reviewer state. In the 403 variant, make one same-user browser attempt, verify, and report that the browser was used. Do not decline or post comments as a workaround. |
| 26 | Report the current approval without repeating it. After a timed-out write, reconcile by the confirming read and do not replay through CLI or browser. |

Cases 1–9 authorize only a user report. Later cases specify their own action
scope; assess the proposed actions within that scope, but never execute them in
this offline exercise. Creation cases describe authorized creation without
performing it in this exercise. An evaluator may report that tools were not run;
it must not invent successful remote operations.

Record the Skill revisions, model/settings, expected issues found, missed
issues, false positives, duplicate findings, and unnecessary reads/actions in
the change's validation report. Confirm each expected behavior above; do not
require the baseline to fail. A fixture pass does not establish production
precision or general reliability. Subsequent real-use feedback should guide
targeted corrections, especially false positives and non-actionable comments.

`scripts/skill-budget.sh` caps the Skill's size; a candidate that passes every
case but exceeds a budget is not ready.

See [validation.md](validation.md) for the recorded review-workflow runs.
