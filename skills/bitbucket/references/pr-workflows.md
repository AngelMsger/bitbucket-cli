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
   The PR's source ref ends up at `refs/remotes/origin/pr/<id>`. Bridging to a
   local checkout is the cheapest way to read many large files.
5. **Commits / activity** — `pr commits` / `pr activity` enumerate the
   contained commits and the timeline (approvals, comments, state changes).

See `reviewing-locally.md` for the end-to-end review decision tree (combines
diffstat-first navigation with a local clone).

## Reviewing

- Approve: `bitbucket-cli pr approve <ref>`
- Withdraw: `bitbucket-cli pr unapprove <ref>`
- Request changes (Cloud only): `bitbucket-cli pr request-changes <ref>`
  (add `--withdraw` to remove a previous request).
- Decline: `bitbucket-cli pr decline <ref> --yes` (destructive — requires `--yes`).
- Merge: `bitbucket-cli pr merge <ref> --strategy <merge_commit|squash|fast_forward> --yes`.
  Run with `--dry-run` first to preview the request body.

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

## Editing reviewers

`bitbucket-cli pr update <ref> --reviewer alice --reviewer carol` replaces the
reviewer list. Omit the flag to keep existing reviewers.

## Exit codes

The same category-coded exit codes the rest of the tool uses apply here —
see `errors-and-exit-codes.md`.
