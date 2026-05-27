# Reading repositories, branches, and commits

## Repositories

- `bitbucket-cli repo list --workspace <ws>` lists repos. `--role` and
  `--query` are Cloud-only filters.
- `bitbucket-cli repo get <ws>/<repo>` returns the normalized metadata
  including `default_branch`, clone URLs, visibility.
- `bitbucket-cli repo clone-url <ws>/<repo> --protocol ssh|https` prints just
  the URL, suitable for `git clone $(bitbucket-cli repo clone-url …)`.

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
  yields the set of commits reachable from `to` but not from `from`.
