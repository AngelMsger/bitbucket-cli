# Reviewing a PR alongside a local codebase

This is the recommended flow for a coding agent (Claude Code etc.) reviewing
a Bitbucket PR while it has read access to a local checkout of the repo. It
budgets remote calls and context tokens by going coarse → fine.

> Addressing feedback on your *own* PR instead of producing a review? See
> `responding-to-review-comments.md` for the triage flow (locate code → judge
> the comment → propose a fix + verification → draft a reply).

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
                            ─→  local checkout preflight
                                                        (right repo? branch/HEAD? clean?)
                                pr fetch --exec         (fetches PR source + base branch;
                                                         prints source_ref + review_diff)
                                <Run review_diff; inspect source_ref directly, or
                                 use a verified clean checkout / isolated worktree>
    │
    ▼
pr threads ─ existing inline discussion, grouped by file/line
    │
    ├─ intent/context unclear and it blocks judgement?
    │                       ─→  ask the author (see "When you don't understand
    │                            the PR" below) and defer just the blocked items
    ▼
pr diff --path <f> --line-numbers    ─ read the exact NEW-file line number
comment add --inline <path>:<line>   ─ write inline feedback (line = new-file line;
                                        add --side old for a removed line)
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
   locally with `pr fetch --exec` provides authoritative refs and the exact
   `review_diff`. A verified clean checkout or isolated worktree at `source_ref`
   then lets the agent use filesystem tooling (Read, Grep) at full power.
   **Fetch brings the base too** — see "Reviewing against the right base" below;
   never `git diff` against a stale local branch.
4. **`pr threads` before commenting** — see whether the question has already
   been asked / answered; reply with `--reply-to` instead of starting a new
   thread.
5. **`pr diff --line-numbers` (or `--commentable`) before an inline comment** —
   `--inline <path>:<line>` wants the NEW-file (post-change) line number; read it from
   the right-hand gutter instead of counting hunk offsets (the classic old-vs-new
   mix-up), or list the valid anchors directly with `pr diff --path <f> --commentable`.
   The CLI validates the number against the diff and errors with the commentable
   ranges if it's wrong, so a bad number fails loudly rather than landing on the wrong
   line. If anchoring fails with `DIFF_PARSE_FAILED` (a server/format incompatibility,
   not a bad number), stop probing anchors and fall back to a general comment that
   names `path:line` in its body. See `commenting.md`.

## Local checkout preflight

Before treating files in the current worktree as PR code, record the repository
root, remotes, current branch, HEAD, and dirty state. Then apply all of these
checks:

- **Match the repository.** Confirm that the selected remote belongs to the PR's
  repository (or its canonical upstream in a fork workflow). The CLI verifies
  only that `cwd` is inside a Git worktree; it does not verify repository
  identity. If the checkout is unrelated or the mapping is uncertain, use
  `pr diff --path` instead of fetching into it.
- **Do not trust the current branch.** Its name and HEAD are context, not proof
  that it contains the PR. Run `pr fetch --exec` and treat the returned
  `source_ref`, `base_ref`, and `review_diff` as authoritative. A previously
  created local `pr/<id>` branch can be stale even after the remote PR ref is
  refreshed.
- **Keep user changes out of the review.** `pr fetch --exec` updates refs but
  does not switch the worktree. `pr checkout --exec` does switch it, so use that
  only when changing the current worktree is safe. Never overwrite, stash, or
  mix in uncommitted user changes; use remote diffs or an isolated worktree
  instead.
- **Prove filesystem alignment.** Read or test files from the current worktree
  only when it is clean and its HEAD resolves to the same commit as
  `source_ref`. Otherwise inspect the fetched ref directly (for example with
  `git show <source_ref>:<path>`), run the emitted `review_diff`, or create an
  isolated worktree at the source ref.
- **Refresh before writing a verdict.** Before posting comments or changing PR
  state after a local review, re-run `pr status` and `pr fetch --exec`. If the
  fetched source commit changed, review the new diff before continuing.

## When you don't understand the PR — ask the author

Sometimes the diff alone doesn't tell you *why*, and without the why you cannot
judge whether a change is correct or complete. When such a gap in intent or
background **genuinely blocks the review**, ask the PR author with a comment
instead of guessing or giving up. Style preferences and questions you could
answer yourself don't clear that bar.

**Exhaust self-service first.** Most context gaps close without the author:

- `pr get <ref>` — the PR title and description state the intent.
- `pr threads <ref>` — the question may already be asked and answered.
- Commit messages — after `pr fetch --exec`, run
  `git log <remote>/<base>..<remote>/pr/<id>`.
- The local codebase and its history (Read / Grep, `git log -p` on the touched
  files).
- Issue / ticket links referenced by the PR description or branch name.

**Ask via a comment.** If the gap survives self-service:

- Anchor the question to code with `comment add --inline <path>:<line>` when it
  is about a specific change; use a general `comment add` for PR-wide intent
  questions.
- Batch related questions into as few comments as possible. State what you
  understood, what is missing, and why it blocks the review, so the author can
  answer in one pass.
- Apply the AI-attribution prefix and write in the user's language (see
  SKILL.md › "AI attribution").
- In a read-only session you cannot post — report the gap and a suggested
  question text to the user instead.

**Don't block the whole review.** Keep reviewing the files the gap doesn't
touch and deliver those findings; list explicitly which files / questions are
deferred pending the author's reply.

**Pause, don't poll.** You cannot hold a session open until a human replies.
After posting, report to the user — partial findings, the question comment
id(s), and the deferred items — then end the turn. When the review resumes,
first check for replies with `pr threads <ref> --comment <id>` (or
`comment list --pr <ref> --unresolved`); if the author has answered, fold the
answer in and finish the deferred items, otherwise tell the user the question
is still open. While a blocking question is unanswered, don't `pr approve`,
`pr request-changes`, or `pr decline` on guesswork.

## Reviewing against the right base

A PR is "what changed **relative to its base**". Reading the PR's files at their
new state is not enough — to judge the change you must diff against the base the PR
targets, and a stale local clone gets this wrong (extra or missing changes).

`pr fetch` / `pr checkout` provide the refs needed to handle this correctly:

- They fetch **both** the PR source ref (→ `<remote>/pr/<id>`) **and the PR's base
  branch** (its destination branch, looked up via the API), so the local merge-base
  is correct. Override the base with `--base <branch>` (also lets the command run
  offline).
- They pick a remote name: an explicit `--remote` wins, otherwise **`upstream` is
  preferred over `origin`** when both exist. In a fork workflow `upstream` is the
  canonical repo the PR was opened against — it carries the authoritative base
  branch and the `refs/pull-requests/*` refs, so it is the accurate source; `origin`
  (your fork) is often behind. This selection does not verify the remote URL;
  the agent must confirm that the checkout and chosen remote match the PR repo.
- The JSON output includes `remote`, `source_ref`, `base_branch`, `base_ref`,
  `base_commit`, and a ready-to-run **`review_diff`** — a triple-dot diff
  (`git diff <remote>/<base>...<remote>/pr/<id>`) that shows exactly the PR's changes
  against the merge-base, staying correct even as the base branch advances.

So the local review flow is: complete the preflight → `pr fetch --exec` → run
the printed `review_diff` and inspect `source_ref` → use a checkout only after
proving its clean HEAD matches that ref. Never hand-pick a base or trust a stale
`main` or existing `pr/<id>` branch.

- `BITBUCKET_DEFAULT_WORKSPACE` set (or `--workspace` on each call) so the
  agent doesn't have to repeat the workspace in every reference.
- For `pr fetch --exec` / `pr checkout --exec`: the agent's `cwd` must be a Git
  worktree. The CLI checks this and returns a `usage` error otherwise; repository
  and remote identity remain part of the agent's preflight.

## When NOT to fetch locally

- The PR is in a different repository than the agent's `cwd` clone (Bitbucket
  Cloud forks). Use `pr diff --path` instead.
- The agent only needs to read 1–2 files. The remote round-trip is cheaper
  than a local fetch.
