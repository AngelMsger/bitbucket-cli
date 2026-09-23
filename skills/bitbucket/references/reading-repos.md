# Reading repositories, branches, and commits

## Repositories

- `bitbucket-cli repo list --workspace <ws>` lists repos. `--role` and
  `--query` are Cloud-only filters.
- `bitbucket-cli repo get <ws>/<repo>` returns the normalized metadata
  including `default_branch`, clone URLs, visibility.
- `bitbucket-cli repo clone-url <ws>/<repo> --protocol ssh|https` prints just
  the URL, suitable for `git clone $(bitbucket-cli repo clone-url …)`.
- `bitbucket-cli repo create <slug> --workspace <ws>` creates a repository
  (`--name`, `--description`; `--private` defaults to **true**). Writes —
  preview with `--dry-run`.
- `bitbucket-cli repo fork <ws>/<repo> [--into <target>] [--name <name>]`
  creates a fork. On **Cloud**, `--into` is required because the API has no
  personal-workspace default; if source and target are the same workspace,
  `--name` is also required. On **Data Center**, omitting `--into` targets the
  authenticated user's personal project; use `whoami` to discover the username
  for an explicit personal-project key such as `~alice`. Preview with
  `--dry-run`; read-only mode blocks the live fork.
- `bitbucket-cli repo delete <ws>/<repo> --yes` deletes one (destructive —
  requires `--yes`; `--dry-run` previews).

If `BITBUCKET_DEFAULT_WORKSPACE` is set, the workspace can be omitted from
shorthand arguments.

## Branches

- `bitbucket-cli branch list --repo <ws>/<repo>` — filter with `--query`.
- `bitbucket-cli branch get --repo <ws>/<repo> <name>` returns target commit
  and whether it is the default branch.
- `bitbucket-cli branch create --repo <ws>/<repo> --from-ref <hash|name> <new-name>`
- `bitbucket-cli branch delete --repo <ws>/<repo> --yes <name>`

## Commits

- `bitbucket-cli commit get --repo <ws>/<repo> <hash>` — show one commit.
- `bitbucket-cli commit list --repo <ws>/<repo> [--branch main] [--path src/]`
- `bitbucket-cli commit compare --repo <ws>/<repo> --from <ref> --to <ref>`
  yields the set of commits reachable from `to` but not from `from`. It accepts
  `--limit N`, `--cursor <next>`, and `--all`; the default returns one page with
  `next` and `has_more`, while `--all` fetches the complete comparison.
  With `--format ndjson`, rows stay on stdout and an incomplete page exposes
  `next` and `has_more` in stderr's `_notice.pagination`; pass that cursor
  verbatim to the next call. Empty filtered pages can still have a next page.
  `--all` collects all pages before rendering and emits no continuation notice.
