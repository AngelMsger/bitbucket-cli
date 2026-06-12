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
                            ─→  pr fetch --exec        (fetches PR source + base branch;
                                                        prints the merge-base `review_diff`)
                                pr checkout --exec     (switches to pr/<id> branch)
                                <Read local files / run the printed review_diff>
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
   locally (`pr fetch --exec` + `pr checkout --exec`) lets the agent use its
   filesystem tooling (Read, Grep) at full power. **Fetch brings the base
   too** — see "Reviewing against the right base" below; never `git diff`
   against a stale local branch.
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

`pr fetch` / `pr checkout` handle this for you:

- They fetch **both** the PR source ref (→ `<remote>/pr/<id>`) **and the PR's base
  branch** (its destination branch, looked up via the API), so the local merge-base
  is correct. Override the base with `--base <branch>` (also lets the command run
  offline).
- They pick the remote: an explicit `--remote` wins, otherwise **`upstream` is
  preferred over `origin`** when both exist. In a fork workflow `upstream` is the
  canonical repo the PR was opened against — it carries the authoritative base
  branch and the `refs/pull-requests/*` refs, so it is the accurate source; `origin`
  (your fork) is often behind.
- The JSON output includes `remote`, `source_ref`, `base_branch`, `base_ref`,
  `base_commit`, and a ready-to-run **`review_diff`** — a triple-dot diff
  (`git diff <remote>/<base>...<remote>/pr/<id>`) that shows exactly the PR's changes
  against the merge-base, staying correct even as the base branch advances.

So the local review flow is: `pr checkout --exec` → run the printed `review_diff`
(or read files at `pr/<id>`) → never hand-pick a base or trust a stale `main`.

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
