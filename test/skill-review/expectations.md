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

Review requests only produce a user report; none authorize posting findings,
replies, approval, or merge. Creation cases describe authorized creation without
performing it in this exercise. An evaluator may report that tools were not run;
it must not invent successful remote operations.

Record the Skill revisions, model/settings, expected issues found, missed
issues, false positives, duplicate findings, and unnecessary reads/actions in
the change's validation report. Confirm each expected behavior above; do not
require the baseline to fail. A fixture pass does not establish production
precision or general reliability. Subsequent real-use feedback should guide
targeted corrections, especially false positives and non-actionable comments.
