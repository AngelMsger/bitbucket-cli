# Pull request workflows

The `pr` and `comment` subtrees cover the entire PR lifecycle.

## Reading a PR

When the user pastes a PR URL or names `<workspace>/<repo>/<id>`:

1. **Merge readiness first** — `bitbucket-cli pr status <ref>` returns the
   aggregated view: mergeable + conflicts + reviewer states + CI build
   status. Often this alone answers "why is this PR not going in?".
2. **Diffstat (per-file metadata) next** — `bitbucket-cli pr files <ref>`
   returns one row per changed file with `path / status / added / removed`,
   sorted by churn. **Diffstat-first** is the right default for large PRs —
   don't pull the whole patch until you know which files matter.
3. **Per-file diff** — `bitbucket-cli pr diff <ref> --path <path>` returns
   just one file's unified patch. The full `pr diff <ref>` (no `--path`) is
   still available when you really want the whole patch.
4. **Local fetch** — `bitbucket-cli pr fetch <ref>` (or `pr checkout`) prints
   the equivalent `git` commands; `--exec` runs them in your current checkout.
   It fetches the PR source ref (→ `refs/remotes/<remote>/pr/<id>`) **and the PR's
   base branch**, picks the remote (`upstream` over `origin` when both exist), and
   prints a ready-to-run `review_diff` that diffs against the merge-base. Before
   `--exec`, verify the checkout's repo, branch, HEAD, and dirty state; the CLI
   does not verify that the selected remote belongs to the PR repo. Fetching the
   authoritative refs is the cheapest way to review many large files locally
   against the correct base. See `reviewing-locally.md` › "Local checkout
   preflight".
5. **Commits / activity** — `pr commits` / `pr activity` enumerate the
   contained commits and the timeline (approvals, comments, state changes).

The PR record itself comes from `pr get <ref> --scope summary|full` (`full`
adds the description and reviewer detail); `--scope diff|commits|activity`
mirror the standalone `pr diff`/`pr commits`/`pr activity` subcommands.

For code review, follow [Reviewing a pull request](reviewing-locally.md),
including its publication rules and checkout preflight.

## Collecting review activity for a worklog

`pr activity` accepts one or more PR refs, or a single `-` to read
newline-delimited refs from stdin. Filter the resulting timelines by the
authenticated user, activity kind and a half-open time window:

```sh
bitbucket-cli pr activity PROJ/repo/42 PROJ/other/7 \
  --actor me \
  --from 2026-09-03T00:00:00+08:00 \
  --to 2026-09-04T00:00:00+08:00 \
  --kind approval,comment,decline
```

Batch queries require `--since` or `--from`, preventing an automation from
accidentally walking every candidate PR's full history. Each activity carries
`pull_request.id`, `pull_request.ref`, and `pull_request.repository`, so the
result can be associated with an issue or deduplicated by PR without another
lookup. `--from` is inclusive and `--to` is exclusive. Date-only values are UTC;
use RFC 3339 with an explicit offset for a local calendar day. Filtered queries
exclude recognized system-generated comments by default. Those records remain
available in an unfiltered single-PR timeline, where `system: true` distinguishes
them, or with `--include-system` on a filtered query. Do not count them as human
review evidence.

On Data Center, avoid listing years of merged or declined PRs by using its
native close-time filter before reading the activity streams:

```sh
bitbucket-cli pr inbox --role any --state MERGED --closed-since 48h --all \
  --fields ref \
  | jq -r '.items[].ref' \
  | bitbucket-cli pr activity - --actor me --since 24h \
      --kind approval,comment,decline
```

Run the inbox query separately for `OPEN`, `MERGED`, and `DECLINED` when all
three states matter; `--closed-since` describes the PR's close time and is not
an update-time filter. Bitbucket Cloud has no equivalent cross-repository
close-time filter, so it rejects `--closed-since` rather than silently treating
`updated_on` as the same concept. On Cloud, feed known PR refs (for example,
refs retained by the scheduled agent while they were in its inbox) to
`pr activity`.

`pr inbox` is always scoped to the authenticated user. `pr activity --actor
<user>` can filter timelines for another user after the PR refs are known, but
it does not widen the candidate set. For each known repository, `pr list --repo
<project>/<repo> --author <user> --all` and `--reviewer <user> --all` provide
repository-scoped Data Center discovery; `--all` is mandatory because the
server has no native user predicate and filtering a single server page would
break pagination correctness. Cloud resolves the selector and filters natively,
so it does not require `--all`. The Data Center scan can be expensive for a
large repository; narrow `--state` whenever possible. Data Center's dashboard
endpoint still has no arbitrary-user selector, and a complete substitute would
require an unbounded scan across accessible projects and repositories. The CLI
deliberately does not present such a scan as complete. Treat cross-repository
results for another user as partial coverage.

## Reviewing

Apply the [review publication rules](reviewing-locally.md#decide-what-to-publish)
before changing PR state. A clean, authorized approval needs no comment. For
permission failures, use [PR permissions and recovery](pr-permissions.md).

- Approve: `bitbucket-cli pr approve <ref>`
- Withdraw: `bitbucket-cli pr unapprove <ref>`
- Request changes (Cloud only): `bitbucket-cli pr request-changes <ref>`
  (aliases: `need-work`, `needs-work`; add `--withdraw` to remove a previous
  request). On Data Center the CLI has not implemented the participant-status
  flow. Use the same user's browser session for an authorized needs-work vote
  when available, or report the limitation. Publish only verified findings under
  the review rules; do not substitute decline, which closes the PR.
- Decline: `bitbucket-cli pr decline <ref> --yes` (destructive — requires `--yes`).
  An explicitly requested reason can be supplied with `--message`; Data Center
  maps it to the optional decline comment. Its `--dry-run` reads the current PR
  version and previews the same versioned request shape used by execution.
- Merge: `bitbucket-cli pr merge <ref> --strategy <merge_commit|squash|fast_forward> --yes`.
  Run with `--dry-run` first to preview the request body. Add
  `--close-source-branch` to delete the source branch on merge — native on
  Cloud, emulated on Data Center via a follow-up branch delete after the merge.

## Creating

Before drafting, read [Writing PR descriptions](writing-pr-descriptions.md).
Use the final changes and the target repository's conventions to write the body.

### Resolve the target first

Settle *which* PR before writing anything. Each step is one cheap command;
stop at the first one that answers the question, and ask the user only when a
step is genuinely ambiguous.

1. **Repository.** Derive `<workspace>/<repo>` from the git remote — strip a
   trailing `.git`, and on Data Center take the project key and slug out of the
   `.../scm/<KEY>/<repo>.git` or `.../projects/<KEY>/repos/<repo>` path. A clone
   URL is not a browse URL: pass the derived shorthand to `--repo`, not the
   remote URL itself.
2. **Source and target branches.** The source is the current branch. The target
   is the repository's default branch unless the user, the repository's agent or
   contribution guide, or an existing convention names another.
3. **Existing PR.** Check before creating a second one:

   ```sh
   bitbucket-cli pr list --repo myws/myrepo --source feature/x \
     --state OPEN --fields id,title,state
   ```

   A hit means this task is [Updating descriptions](#updating-descriptions),
   not `pr create`.
4. **Pushed branch.** `git status --short --branch` must show an upstream and no
   unpushed commits; `pr create` opens against the remote ref, so unpushed work
   is silently absent from the PR. Push (or ask the user to) before creating.

### Budget the diff before drafting

Read the shape of the change before its contents, the same way
[Reading a PR](#reading-a-pr) does:

```sh
git diff --stat origin/main...HEAD      # or the resolved target branch
```

Read the full diff only for the files that carry the behavior change. A
generated file, a lockfile, or a mass rename is described from its diffstat
row; it does not need to be read line by line. For a change small enough that
the diffstat already names every file, just read the whole diff — the budget
step costs more than it saves.

### Run the repository's checks

Check the target repository's agent guide, contribution guide, build scripts,
and CI configuration for the checks appropriate to the changed scope. Where
applicable, run its build, formatting, lint, and test checks and address failures
within the task's scope. Follow the project's requirements; do not assume a
particular toolchain or impose a universal command list.

Report the results, skipped checks and their reasons, and any blockers to the
user as part of the creation workflow. Do not claim success for checks that did
not run, or silently proceed past a required check that failed. Keep this report
out of the PR description by default; it is not a testing section or a routine
"all checks passed" sentence to append to every PR.

### Create the request

```sh
bitbucket-cli pr create \
  --repo myws/myrepo \
  --source feature/x \
  --target main \
  --title "Add X" \
  --description-file PR.md \
  --reviewer alice --reviewer bob
```

On Cloud, `--reviewer` takes a UUID; on Data Center, a username. Pass
`--dry-run` to see the request envelope before committing.

**Cross-fork PRs (from a fork into upstream).** When the source branch lives in a
fork rather than the target repo, name the fork with `--source-repo <ws>/<repo>`;
`--repo` stays the upstream repo the PR opens against (the PR's `fromRef` points
at the fork, `toRef` at upstream):

```sh
bitbucket-cli pr create \
  --repo UPSTREAM/repo \        # upstream — where the PR opens
  --source-repo MYFORK/repo \   # the fork that holds the source branch
  --source feature/x \
  --target dev \                # the upstream destination branch
  --title "Add X"
```

Works on both flavors. On Bitbucket Cloud `--target` may be omitted (defaults to
the upstream default branch); on **Data Center a cross-fork PR requires an
explicit `--target`** — omitting it is a usage error, not a guess.

`pr create` also accepts `--close-source-branch` (delete the source branch when
the PR later merges). This is a Cloud-only property at creation time — on Data
Center it is rejected with a usage error; pass `--close-source-branch` to
`pr merge` instead.

### AI attribution

When you create or update a PR description on the user's behalf as an AI agent,
prepend a single attribution line to the top of the description (it's Markdown):

```markdown
> [AI] This PR was created with the help of AI via [bitbucket-cli](https://angelmsger.github.io/bitbucket-cli/).
```

Write the sentence in the description's language, following
[Writing PR descriptions](writing-pr-descriptions.md); keep the `[AI]` marker,
the URL, and the `bitbucket-cli` label constant. The marker is plain-ASCII `[AI]`
(it renders as literal text), **never an emoji** — some Data Center databases
(e.g. MySQL `utf8mb3`) can't store 4-byte characters and would reject or truncate
the description. On `pr update` keep a single line — replace an existing one
rather than stacking another.

## Updating descriptions

Before using `pr update` to edit a description, read
[Writing PR descriptions](writing-pr-descriptions.md), including its guidance on
existing text. Read the current body with `pr get <ref> --scope full` and compare
it with the final PR changes. Preserve useful context and links, remove stale
claims within the requested edit, and keep the attribution line above only once.
Do not rewrite an existing description when the task only changes reviewers or
other metadata.

## Editing reviewers

`bitbucket-cli pr update <ref> --reviewer alice --reviewer carol` replaces the
reviewer list. Omit the flag to keep existing reviewers.

## Exit codes

The same category-coded exit codes the rest of the tool uses apply here —
see `errors-and-exit-codes.md`.
