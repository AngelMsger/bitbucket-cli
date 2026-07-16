# @angelmsger/bitbucket-cli

npm distribution of [`bitbucket-cli`](https://github.com/AngelMsger/bitbucket-cli)
— a command-line tool that drives Bitbucket pull requests and repositories
from the terminal, built for coding agents (Claude Code and others) and humans
alike. Supports Bitbucket Cloud and Data Center / Server.

```bash
npm install -g @angelmsger/bitbucket-cli
bitbucket-cli config init --pretty   # interactive TUI: server URL + credentials
bitbucket-cli skill install          # deploy the companion agent Skill
bitbucket-cli pr inbox               # what's waiting for your review?
```

Installing this package downloads the prebuilt binary for your platform from
the matching GitHub Release and verifies its SHA-256 checksum. If your npm
setup disables install scripts, the binary is fetched on first run instead.

The companion `bitbucket` Skill for coding agents is embedded in the binary;
`bitbucket-cli skill install` deploys a copy that always matches the installed
CLI version.

See the [project README](https://github.com/AngelMsger/bitbucket-cli) and the
[installation guide](https://github.com/AngelMsger/bitbucket-cli/blob/main/docs/installation.md)
for full documentation.
