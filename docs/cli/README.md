# bitbucket-cli command reference

This index is generated from the CLI command tree — do not edit it by
hand; run `make docs`. The full reference, with every flag and example,
is published at <https://angelmsger.github.io/bitbucket-cli/cli/>.

## auth

| Command | Description |
| --- | --- |
| [`bitbucket-cli auth`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-auth) | Inspect and manage stored credentials |
| [`bitbucket-cli auth login`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-auth-login) | Store a credential for the configured server |
| [`bitbucket-cli auth logout`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-auth-logout) | Remove the stored credential for the configured server |
| [`bitbucket-cli auth status`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-auth-status) | Show whether a usable credential is configured |

## branch

| Command | Description |
| --- | --- |
| [`bitbucket-cli branch`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-branch) | List and manage repository branches |
| [`bitbucket-cli branch create`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-branch-create) | Create a branch from a starting ref |
| [`bitbucket-cli branch delete`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-branch-delete) | Delete a branch |
| [`bitbucket-cli branch get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-branch-get) | Show a single branch |
| [`bitbucket-cli branch list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-branch-list) | List branches in a repository |

## comment

| Command | Description |
| --- | --- |
| [`bitbucket-cli comment`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-comment) | Read and write pull-request comments |
| [`bitbucket-cli comment add`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-comment-add) | Add a comment on a PR (general or inline) |
| [`bitbucket-cli comment delete`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-comment-delete) | Delete a comment |
| [`bitbucket-cli comment list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-comment-list) | List comments on a PR |
| [`bitbucket-cli comment update`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-comment-update) | Edit a comment |

## commit

| Command | Description |
| --- | --- |
| [`bitbucket-cli commit`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-commit) | Query commits in a repository |
| [`bitbucket-cli commit compare`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-commit-compare) | List the commits between two refs |
| [`bitbucket-cli commit get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-commit-get) | Show a single commit |
| [`bitbucket-cli commit list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-commit-list) | List commits in a repository |

## config

| Command | Description |
| --- | --- |
| [`bitbucket-cli config`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-config) | Manage bitbucket-cli configuration |
| [`bitbucket-cli config delete-context`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-config-delete-context) | Delete a context and its stored credential |
| [`bitbucket-cli config get-contexts`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-config-get-contexts) | List the configured contexts |
| [`bitbucket-cli config init`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-config-init) | Interactively set up server URL and credentials |
| [`bitbucket-cli config path`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-config-path) | Print the config file path |
| [`bitbucket-cli config show`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-config-show) | Show the resolved configuration |
| [`bitbucket-cli config use-context`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-config-use-context) | Switch the current context |

## doctor

| Command | Description |
| --- | --- |
| [`bitbucket-cli doctor`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-doctor) | Diagnose configuration, credentials and connectivity |

## file

| Command | Description |
| --- | --- |
| [`bitbucket-cli file`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-file) | Browse and read repository source files at any ref |
| [`bitbucket-cli file get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-file-get) | Read a file's raw contents at a ref |
| [`bitbucket-cli file list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-file-list) | List entries under a repository path at a ref |
| [`bitbucket-cli file tree`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-file-tree) | Recursively list files under a path at a ref |

## pr

| Command | Description |
| --- | --- |
| [`bitbucket-cli pr`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr) | Drive Bitbucket pull requests (list, review, merge) |
| [`bitbucket-cli pr activity`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-activity) | List the activity timeline of a PR |
| [`bitbucket-cli pr approve`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-approve) | Approve a PR |
| [`bitbucket-cli pr checkout`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-checkout) | Print (or run, with --exec) git fetch (source + base) + checkout for a PR |
| [`bitbucket-cli pr commits`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-commits) | List commits included in a PR |
| [`bitbucket-cli pr create`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-create) | Open a new pull request |
| [`bitbucket-cli pr decline`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-decline) | Decline (close without merging) a PR |
| [`bitbucket-cli pr diff`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-diff) | Print the unified diff of a PR (use --path to scope to one file) |
| [`bitbucket-cli pr fetch`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-fetch) | Print (or run, with --exec) git fetch for a PR's source ref and base branch |
| [`bitbucket-cli pr files`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-files) | List changed files in a PR (diffstat: path / status / added / removed) |
| [`bitbucket-cli pr get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-get) | Show a pull request |
| [`bitbucket-cli pr inbox`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-inbox) | List PRs involving me across repositories (--role reviewer by default) |
| [`bitbucket-cli pr list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-list) | List pull requests in a repository |
| [`bitbucket-cli pr merge`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-merge) | Merge a PR |
| [`bitbucket-cli pr request-changes`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-request-changes) | Cast (or withdraw) a request-changes vote (Cloud only) |
| [`bitbucket-cli pr status`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-status) | Show merge readiness: mergeable, conflicts, reviewers, CI builds |
| [`bitbucket-cli pr threads`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-threads) | List PR review threads grouped by file and anchor |
| [`bitbucket-cli pr unapprove`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-unapprove) | Withdraw an approval |
| [`bitbucket-cli pr update`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-pr-update) | Edit a PR's title, description, or reviewers |

## repo

| Command | Description |
| --- | --- |
| [`bitbucket-cli repo`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-repo) | Browse and manage Bitbucket repositories |
| [`bitbucket-cli repo clone-url`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-repo-clone-url) | Print the HTTPS or SSH clone URL of a repository |
| [`bitbucket-cli repo create`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-repo-create) | Create a repository |
| [`bitbucket-cli repo delete`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-repo-delete) | Delete a repository (irreversible) |
| [`bitbucket-cli repo get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-repo-get) | Show a repository's details |
| [`bitbucket-cli repo list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-repo-list) | List repositories in a workspace (Cloud) or project (Data Center) |

## skill

| Command | Description |
| --- | --- |
| [`bitbucket-cli skill`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-skill) | Install the companion Skill for coding agents (Claude Code, Codex) |
| [`bitbucket-cli skill install`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-skill-install) | Deploy the embedded Skill into a coding agent's skills directory |
| [`bitbucket-cli skill path`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-skill-path) | Print where the Skill would be installed, and whether it is |
| [`bitbucket-cli skill show`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-skill-show) | Print the embedded SKILL.md to stdout |
| [`bitbucket-cli skill uninstall`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-skill-uninstall) | Remove the companion Skill from a coding agent's skills directory |

## tag

| Command | Description |
| --- | --- |
| [`bitbucket-cli tag`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-tag) | List and inspect repository tags |
| [`bitbucket-cli tag get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-tag-get) | Show a single tag (commit hash + date / message on Cloud) |
| [`bitbucket-cli tag list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-tag-list) | List tags in a repository |

## user

| Command | Description |
| --- | --- |
| [`bitbucket-cli user`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-user) | Discover Bitbucket users (workspace members on Cloud / global users on DC) |
| [`bitbucket-cli user get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-user-get) | Show details of a single user (UUID/account_id on Cloud; slug/username on DC) |
| [`bitbucket-cli user list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-user-list) | List users (workspace members on Cloud / global users on DC) |
| [`bitbucket-cli user me`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-user-me) | Print the user the configured credentials authenticate as (alias for whoami) |

## version

| Command | Description |
| --- | --- |
| [`bitbucket-cli version`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-version) | Print version information |

## whoami

| Command | Description |
| --- | --- |
| [`bitbucket-cli whoami`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-whoami) | Print the user the configured credentials authenticate as |

## workspace

| Command | Description |
| --- | --- |
| [`bitbucket-cli workspace`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-workspace) | List and inspect Bitbucket workspaces (Cloud) / projects (DC) |
| [`bitbucket-cli workspace get`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-workspace-get) | Show details of a single workspace / project |
| [`bitbucket-cli workspace list`](https://angelmsger.github.io/bitbucket-cli/cli/#bitbucket-cli-workspace-list) | List every workspace / project the current credentials can see |

