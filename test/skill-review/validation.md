# Review-state fixes — 2026-09-23

Skill `0.18.1` corrects the Cloud refresh to include `participants`, where the
existing mapper exposes votes. The verdict policy and public response shape
are unchanged. The exact refresh command now runs against a Cloud fixture in
`make e2e`; this validates field acquisition rather than repeating a model
judgment benchmark.

Data Center withdrawal now requires the caller's confirmed Needs Work vote in
both preview and execution. Other, missing, or conflicting states return
`PR_NO_CHANGE_REQUEST` without a write. Unit regressions cover reviewer and
participant records, approvals, neutral/unknown states, another reviewer's
vote, inconsistent records, read failure, and an approval made after preview.
Cloud retains its native withdrawal endpoint; the Data Center guard handles
its broader status-update endpoint. Code comments were shortened to contracts
and non-obvious constraints.

Validation: `make test`, `make e2e` (140 checks plus offline team setup),
`go vet ./...`, read-only `gofmt -l .`, Skill budgets, link checks, and
`git diff --check` passed. Generated help reflects withdrawal recovery.
Only Bitbucket is applicable to these PR-specific fixes; sibling contracts and
root submodule pointers are unchanged. Pre-existing edits were preserved.
No live service write, release, or global Skill install was performed.

# Review workflow validation — 2026-09-23

## Scope and design decisions

This change pairs a CLI fix with a Skill restructure. The CLI now casts a Data
Center Needs Work vote through the participant-status update addressed to the
caller's own user slug, so the Skill no longer has to route that vote through a
browser; browser recovery stays only for a server permission rejection of an
authorized action. The [review guide](../../skills/bitbucket/references/reviewing-locally.md)
opens with a command-bound checklist and names the `pr get` fields and
`pr fetch` outputs each step reads; multi-PR grouping and delegation moved to
[Reviewing batches](../../skills/bitbucket/references/reviewing-batches.md);
the guide's design rationale moved to `docs/technical-design.md`. The entry
file's review, reply, and attribution sections were shortened to the operative
rule plus a link. No verdict, publication, evidence, or reply-gate rule changed.

`scripts/skill-budget.sh` now caps the Skill by word count and runs from
`make e2e`. Budgets: entry file 2200, any reference 2400, and 5600 across the
four files a single-PR review loads before publishing (entry, review guide,
reply gate, permission recovery). They sit just above the current sizes on
purpose: the budget is a regression guard against growth, not a target, and
the review path ended at 5598 words after the guide shrank from 3056 to 2344
and the entry file from 2364 to 2157.

## Family applicability

The Data Center vote path is Bitbucket-specific; no sibling casts review votes.
The 2026-09-22 applicability matrix below still holds, and no shared contract
changed. The archived WeCom Calendar project is excluded.

## Independent behavioral comparison

Two separate subagents received only their Skill snapshot and the same 26
[cases](cases.md), without the rubric, implementation discussion, or each
other's output; both inherited the parent model and settings with no override.
Execution, network access, and writes were prohibited; each saved only its
report. Case 25 and its expectation were updated first so both runs saw the
same CLI capability as input.

- Baseline: working-tree Skill `0.17.0` before this change, SHA-256 over sorted
  paths and contents `6e9960b37c3ba98827b5c378002bd627a0852aa1c350f9760c536916ba3b2514`.
- Candidate: Skill `0.18.0` as evaluated,
  `694fb8d7e53ab3058081bb147ba8b9984d9736af94bc902f6eaa1c983c33585a`.
  After the evaluation one cross-reference in `references/pr-workflows.md`
  was reworded to the renamed "Local checkout" heading (no rule change); the
  recorded working tree is `b7814ebb869b588eb757fbc6cff42ff9a5ed267bdc611d55e7986c11e705ab48`.

The coordinator compared decisions with [expectations](expectations.md), not
wording. The candidate matched all 26 cases and every variant. The baseline
also matched 1–24 and 26; in case 25 it followed the case's verified CLI
capability but flagged that its own text still claimed the CLI could not vote
on Data Center, which is the drift this change removes.

| Cases | Baseline | Candidate |
| --- | --- | --- |
| 1–9 | Five seeded findings found; three justified comments/abstractions/contracts preserved; context gap in 9 reported, not invented. | Same; no missed findings, false positives, or duplicate findings. |
| 10–11 | Creation continued; one incidental reminder only in 10. | Same; no extra scan, cleanup, or submission gate. |
| 12–13 | Dirty checkout preserved, release base used; sensible grouping and pinned hand-offs. | Same, citing the checklist's pinned fields and the batches reference. |
| 14–17 | Withheld approval for uncovered S2; approved verified changes; ignored naming preference and disproved automated claims. | Same; the refresh step names the exact `--fields` read before each vote. |
| 18–22 | Needs Work on the verified defect; collaboration hold kept distinct from a defect; withdrawn vote and revision drift handled; approval-only scope respected. | Same. |
| 23–24 | Remote-only reads; canonical remote chosen; fetched commits verified against refreshed metadata. | Same, and 23 additionally avoids `file get -o` as a local write. |
| 25–26 | CLI vote per case input with browser fallback; existing and timed-out approvals reconciled by read. | CLI participant-status vote with dry run, then verify; one browser attempt only in the 403 variant; no replay after the timeout. |

These synthetic cases remain a regression check, not an estimate of production
accuracy. No live Bitbucket write or browser action was executed.

## Executable and documentation checks

All checks below apply to `src/bitbucket-cli`:

- `go vet ./...`, read-only `gofmt -l .`, and `make test`: passed. New tests pin
  both flavors' request-changes previews to their live requests, including the
  Data Center participant target and body, and prove that no write is sent when
  the identity lookup fails.
- `make e2e`: passed, 137 checks. New checks cover the Data Center vote and its
  withdrawal, the dry-run target and status body, read-only blocking with a
  still-working preview, and the Skill budget. The sandbox blocks loopback
  connections to the mock server, so the suite ran outside it, as on 2026-09-22.
- `make docs`, twice: the regenerated CLI reference reflects the new command
  help and shows no further drift.
- `scripts/skill-budget.sh`: all files within budget.
- Relative links and anchors across the Skill: all resolve after the heading
  and file moves; references to the renamed section were updated.
- `git diff --check` in the repository and at the workspace root: passed.

Pre-existing user edits, including the NDJSON continuation work and the
2026-09-22 Skill changes, were preserved; the Skill version advanced from that
work's `0.17.0` to `0.18.0`. No commit, release, submodule pointer update, or
global Skill installation was made.

# Review workflow validation — 2026-09-22

## Scope and design decisions

This change updates Bitbucket's companion Skill, not CLI commands or transport.
The canonical [review guide](../../skills/bitbucket/references/reviewing-locally.md)
now connects scope, revisions, work allocation, evidence, verdict, publication,
and verification. Entry points and permission recovery link to the same verdict
table. Existing maintainability guidance, comment deduplication, human-reply
confirmation, quiet approvals, and lightweight PR creation remain in place.

The guide cites the primary sources used for the design. Google's review
standard supports accepting a sound improvement with non-blocking suggestions;
requiring an earlier approval for those suggestions would add an unnecessary
condition. Other reviewers' approval can corroborate a completed review but
cannot establish coverage by itself. AI/tool feedback is independently checked
without assuming a model's name establishes its reliability.

Following an active human Needs Work even when its technical reason is disputed
is the requested conservative collaboration policy. It is explicitly separate
from a confirmed defect, does not extend to unverified automated votes, and
never expands authorization. This preserves the requested hold without claiming
that peer disagreement proves a bug. Historical or withdrawn votes do not hold
the current review. Insufficient evidence normally produces no vote.

## Family applicability

The shared companion-Skill collaboration rules, sibling Skills, and command
roots were inspected. Existing authorization, reply gates, bounded reads, and
uncertain-write recovery remain aligned; this change adds no shared CLI contract.

| Repository | Applicability | Reason |
| --- | --- | --- |
| Bitbucket | Applicable | Owns PR source/base data, review discussion, reviewer votes, and local review setup. |
| Confluence | Not applicable to PR verdicts | Page/comment workflows; existing human-reply and write-recovery rules remain unchanged. |
| Jira | Not applicable to PR verdicts | Issue/comment workflows; existing human-reply and write-recovery rules remain unchanged. |
| Jenkins | Not applicable to PR verdicts | Build evidence can inform review; the CLI does not cast PR review votes. |
| OpenObserve | Not applicable to PR verdicts | Read-only observability queries, not a source review workflow. |
| Prometheus | Not applicable to PR verdicts | Metrics/operations workflows, not a source review workflow. |

No family drift was introduced. Data Center's existing CLI Needs Work limitation
remains documented; an authorized same-user browser action is the fallback.
The archived WeCom Calendar project is excluded.

## Independent behavioral comparison

Two separate subagents received only their Skill snapshot and the same 26 raw
[cases](cases.md), with no rubric, implementation discussion, or other agent's
output. Both inherited the same parent model and reasoning settings with no
model/effort override and no conversation history (`fork_turns=none`). The tool
did not expose an exact backend model identifier. Scenario execution, network
access, and writes were prohibited; only evaluation reports could be saved in
the temporary evaluation directory.

Baseline: working-tree Skill `0.16.1` at repository HEAD `06bb4b0`, including the
pre-existing output-documentation changes. Candidate: Skill `0.17.0`.
SHA-256 fingerprints over sorted relative file paths and contents:

- Baseline: `ee20e49845c4b22d2ccf0fd62c916629932ccd1bbbeb9b51c31b1cb4dbd482a4`
- Candidate: `8a107781991fa346c3ff1459ca7c9670becc309c5863aeb17b7c3e21de0111c0`

The coordinator compared actual decisions with [expectations](expectations.md),
not wording. Candidate decisions matched all 26 cases, including variants.
Baseline decisions differed in case 19 and the human-vote variant of case 21.
The baseline otherwise reached the expected outcomes, sometimes supplying
reasonable judgment where its text lacked an explicit procedure.

| Cases | Baseline | Candidate |
| --- | --- | --- |
| 1–8 | Found all five seeded findings; preserved the three justified comments/abstractions/contracts. | Same; no missed findings, false positives, or duplicate findings. |
| 9 | Reported missing plugin context rather than inventing a defect. | Same; further reads remained conditional on the context gap. |
| 10–11 | Continued creation; one incidental reminder only in case 10. | Same; no extra scan, cleanup, or submission gate. |
| 12 | Isolated the dirty checkout and used the actual release base. | Explicit detached S2 worktree, pinned revisions, and artifact-preserving cleanup. |
| 13 | Proposed sensible parallel assignments, noting the missing Skill procedure. | Grouped stack/API consumers, assigned independent work, and returned revision/coverage evidence to the coordinator. |
| 14 | Withheld approval for incomplete S2 coverage. | Same; green CI and S1 approval did not substitute for review. |
| 15–17 | Approved verified changes; ignored naming preferences and disproved automated cursor allegations. | Same, including absent peer approval and a disproved automated Needs Work vote. |
| 18 | Needs Work for the verified existing error-handling defect; no duplicate comment. | Same; another approval did not cancel the defect. |
| 19 | Withheld approval but did not propose the requested hold vote. | Proposed Needs Work specifically as a collaboration hold; no invented defect or human reply. |
| 20 | Used current approval and fixed code, not old Needs Work history. | Same; left thread state untouched. |
| 21 | Revalidated source/target moves; allowed approval if a new human vote's reason was disproved. | Revalidated moves and preserved the human hold; did not cast Needs Work under approval-only authorization. |
| 22–24 | Preserved approval-only scope, remote-only scope, and canonical-remote verification. | Same, with explicit vote and source/base identity gates. |
| 25–26 | Used the authorized Data Center browser fallback; reconciled existing/timed-out approvals. | Same; no decline, extra comment, or replay. |

No candidate case introduced an unnecessary remote write, duplicate publication,
creation gate, or mandatory broad scan. These synthetic cases are a regression
check, not an estimate of production review accuracy. Browser actions and live
Bitbucket votes were not executed.

## Executable and documentation checks

All checks below apply to `src/bitbucket-cli`:

- `make test`: passed. The first sandbox attempt could not access Go build
  cache entries; the host-environment retry passed.
- `make e2e`: passed, 129 checks plus offline team-setup checks; includes build,
  embedded Skill/version handling, dry runs, and read-only safeguards.
- `go vet ./...` and read-only `gofmt -l .`: passed with no findings. Used the
  read-only formatting check to preserve existing user edits.
- `make docs`, twice: no task-induced generated-documentation drift.
- The exact Git snippet ran in a temporary repository with divergent source and
  release-target commits, a stale local PR branch, staged user edits, and an
  untracked file. It produced the merge-base PR delta, excluded destination-only
  changes, preserved user state, and created a detached source worktree. Moving
  a shared ref did not change the pinned diff or detached HEAD. Clean worktree
  removal preserved the original checkout.
- Relative Markdown links and anchors in changed guides and fixtures passed.
- The generic skill-creator validator rejects the repository's pre-existing
  top-level `version` extension. Its remaining checks passed on a temporary copy
  omitting only that field. Actual version/handshake and embedding behavior
  passed repository tests; the maintained Skill retains its required version.
- Repository and workspace `git diff --check`: passed.

Root worktree and staged diffs were compared byte-for-byte with the initial
snapshot and were unchanged. The submodule index and HEAD were unchanged.
Pre-existing user edits, including the NDJSON continuation work, were preserved;
the Skill version advanced from that work's `0.16.1` to `0.17.0`. No submodule
pointer was updated, no release was made, and no global Skill installation was
replaced. Other repositories were inspected but not modified or retested.
