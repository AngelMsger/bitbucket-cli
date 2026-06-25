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
   prints a ready-to-run `review_diff` that diffs against the merge-base. Bridging to
   a local checkout is the cheapest way to read many large files — and the only way
   to review against a correct base. See `reviewing-locally.md` ›
   "Reviewing against the right base".
5. **Commits / activity** — `pr commits` / `pr activity` enumerate the
   contained commits and the timeline (approvals, comments, state changes).

The PR record itself comes from `pr get <ref> --scope summary|full` (`full`
adds the description and reviewer detail); `--scope diff|commits|activity`
mirror the standalone `pr diff`/`pr commits`/`pr activity` subcommands.

See `reviewing-locally.md` for the end-to-end review decision tree (combines
diffstat-first navigation with a local clone).

## Reviewing

- Approve: `bitbucket-cli pr approve <ref>`
- Withdraw: `bitbucket-cli pr unapprove <ref>`
- Request changes (Cloud only): `bitbucket-cli pr request-changes <ref>`
  (aliases: `need-work`, `needs-work`; add `--withdraw` to remove a previous
  request). On Data Center this is not implemented — decline or post a comment
  instead.
- Decline: `bitbucket-cli pr decline <ref> --yes` (destructive — requires `--yes`).
- Merge: `bitbucket-cli pr merge <ref> --strategy <merge_commit|squash|fast_forward> --yes`.
  Run with `--dry-run` first to preview the request body. Add
  `--close-source-branch` to delete the source branch on merge — native on
  Cloud, emulated on Data Center via a follow-up branch delete after the merge.

## Creating

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

**AI attribution (agent writes).** When you create or update a PR description on the
user's behalf as an AI agent, prepend a single attribution line to the top of the
description (it's Markdown):

```markdown
> [AI] 本 PR 由 AI 通过 [bitbucket-cli](https://angelmsger.github.io/bitbucket-cli/) 协助创建。
```

Write the sentence in the **user's language** (en: `[AI] This PR was created with the
help of AI via [bitbucket-cli](…).`); keep the `[AI]` marker, the URL, and the
`bitbucket-cli` label constant. The marker is plain-ASCII `[AI]` (it renders as literal
text), **never an emoji** — some Data Center databases (e.g. MySQL `utf8mb3`) can't store
4-byte characters and would reject or truncate the description. On `pr update` keep a
single line — replace an existing one rather than stacking another.

## Editing reviewers

`bitbucket-cli pr update <ref> --reviewer alice --reviewer carol` replaces the
reviewer list. Omit the flag to keep existing reviewers.

## Exit codes

The same category-coded exit codes the rest of the tool uses apply here —
see `errors-and-exit-codes.md`.
