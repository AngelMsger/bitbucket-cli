# Pull request workflows

The `pr` and `comment` subtrees cover the entire PR lifecycle.

## Reading a PR

When the user pastes a PR URL or names `<workspace>/<repo>/<id>`:

1. **Summary first** — `bitbucket-cli pr get <ref> --scope summary`. Returns
   the metadata: title, description, author, source, destination, reviewers,
   state, counts.
2. **Diff when needed** — `bitbucket-cli pr get <ref> --scope diff` (or
   `bitbucket-cli pr diff <ref>`) returns the raw unified patch. Pipe to
   `delta`, `bat` or a model that consumes diffs.
3. **Commits** — `bitbucket-cli pr get <ref> --scope commits` enumerates the
   commit set included in the PR.
4. **Activity** — `bitbucket-cli pr get <ref> --scope activity` returns the
   timeline (approvals, comments, state changes).

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
