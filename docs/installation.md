# Installation & setup guide

This guide covers three things:

1. [Installing the `bitbucket-cli` binary](#1-install-the-cli)
2. [Enabling shell completion](#2-enable-shell-completion)
3. [Installing & updating the companion `bitbucket` Skill](#3-install-the-companion-skill)

---

## 1. Install the CLI

Pick whichever method suits you.

### npm

```bash
npm install -g @angelmsger/bitbucket-cli
```

Installing downloads the prebuilt binary for your platform from the matching
GitHub Release and verifies its SHA-256 checksum. If your npm setup disables
install scripts (`--ignore-scripts`, some pnpm setups), the binary is fetched
on first run instead.

### go install

```bash
go install github.com/angelmsger/bitbucket-cli/cmd/bitbucket-cli@latest
```

Installs into `go env GOBIN` (or `$GOPATH/bin`). Requires Go 1.24+.

### Prebuilt binary

Download the binary for your platform from the
[Releases page](https://github.com/AngelMsger/bitbucket-cli/releases), verify
it against `checksums.txt`, then put it on your `PATH`:

```bash
chmod +x bitbucket-cli-* && mv bitbucket-cli-* /usr/local/bin/bitbucket-cli
```

### From source

```bash
git clone https://github.com/AngelMsger/bitbucket-cli.git && cd bitbucket-cli
make install        # builds and installs into `go env GOBIN` or $GOPATH/bin
```

`make install` prints the install path. Make sure that directory is on your
`PATH`:

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
```

Other build targets: `make build` (to `./bin/`), `make cross` (every platform
into `./dist/`).

### First-time configuration

```bash
bitbucket-cli config init --pretty   # interactive TUI: server URL, flavor, credentials
bitbucket-cli doctor                 # verify configuration and connectivity
bitbucket-cli workspace list         # discover the workspaces / projects you can see
```

The `--pretty` flag opts into a `huh`-based TUI with arrow-key selection,
masked password input, and Shift-Tab back-navigation. Without it,
`config init` runs as a plain line-by-line wizard — keep that form for
scripted setup, dotfiles bootstrap, and non-TTY environments where a TUI
cannot render.

When the server URL points at Bitbucket Cloud (`api.bitbucket.org`,
`bitbucket.org`, or any `*.bitbucket.org` subdomain), the wizard defaults the
auth scheme to **basic** and asks for your Atlassian email plus an API token
from
[id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens).
Atlassian API tokens (and App Passwords) authenticate over HTTP Basic; if you
have a Workspace / Repository / Project Access Token instead, pick **pat**
(Bearer) at the scheme prompt. Data Center / Server defaults to **pat** for
HTTP Access Tokens.

---

## 2. Enable shell completion

`bitbucket-cli` completes subcommands and enum flag values (`--format`,
`--flavor`, `--state`, `--role`, `--scope`, `--strategy`, …).

The CLI ships the completion *logic*, but every shell needs the completion
*script* loaded once. Pick your shell below.

### bash

```bash
# try it in the current shell
source <(bitbucket-cli completion bash)

# make it permanent (Linux)
bitbucket-cli completion bash | sudo tee /etc/bash_completion.d/bitbucket-cli >/dev/null

# make it permanent (macOS, Homebrew bash-completion)
bitbucket-cli completion bash > "$(brew --prefix)/etc/bash_completion.d/bitbucket-cli"
```

bash needs the `bash-completion` package installed and sourced from your
`~/.bashrc`.

### zsh

```bash
# ensure compinit runs — add this to ~/.zshrc if it is not there already:
#   autoload -Uz compinit && compinit

# install the completion into a directory on $fpath
bitbucket-cli completion zsh > "${fpath[1]}/_bitbucket-cli"
```

Open a new shell afterwards. If completions still do not appear, run
`rm -f ~/.zcompdump*` and start a new shell.

### fish

```bash
bitbucket-cli completion fish > ~/.config/fish/completions/bitbucket-cli.fish
```

### PowerShell

```powershell
# current session
bitbucket-cli completion powershell | Out-String | Invoke-Expression

# persistent — append to your profile
bitbucket-cli completion powershell >> $PROFILE
```

Run `bitbucket-cli completion --help` for the authoritative per-shell notes.

### Verifying

After loading the script, type `bitbucket-cli pr get x --scope ` and press
`<TAB>` — you should see `summary full diff commits activity`.

---

## 3. Install the companion Skill

The `bitbucket` Skill teaches a coding agent — **Claude Code** and **Codex** —
how to drive this CLI. It is **embedded in the `bitbucket-cli` binary**, so
whichever way you installed the CLI — npm, `go install`, a prebuilt binary —
you already have a version-matched copy of the Skill.

### Recommended: `bitbucket-cli skill install`

With no flags, `skill install` **probes for installed agents** and installs
the Skill into every one it finds:

```bash
bitbucket-cli skill install              # auto-detect; install for each agent found
bitbucket-cli skill install --agent codex          # only Codex
bitbucket-cli skill install --agent claude-code,codex
bitbucket-cli skill install --project    # project dirs instead of $HOME
bitbucket-cli skill install --dir <path> # explicit base -> <path>/bitbucket

bitbucket-cli skill path                 # show every agent's location + status
bitbucket-cli skill show                 # print SKILL.md to stdout
```

Install locations per agent:

| Agent | Global (default) | Project (`--project`) |
|-------|------------------|-----------------------|
| Claude Code | `~/.claude/skills/bitbucket` | `./.claude/skills/bitbucket` |
| Codex | `~/.codex/skills/bitbucket` | `./.agents/skills/bitbucket` |

Auto-detection looks for `~/.claude` / `~/.codex` (global) or `./.claude` /
`./.agents` / `./AGENTS.md` (project). If nothing is detected, pass `--agent`
or `--dir` explicitly.

Because the Skill ships inside the binary, **updating is automatic**: upgrade
the CLI (`npm update -g @angelmsger/bitbucket-cli`, `go install ...@latest`,
etc.) and re-run `bitbucket-cli skill install` — the deployed Skill always
matches the CLI version.

### Alternative: the `skills` CLI

If you manage agent skills with the [`skills` tool](https://github.com/vercel-labs/skills)
(`npx skills`), you can install the Skill straight from the repository:

```bash
npx skills add AngelMsger/bitbucket-cli --skill bitbucket          # this project
npx skills add AngelMsger/bitbucket-cli --skill bitbucket -g       # all projects
npx skills add ./skills/bitbucket                                  # local checkout
npx skills update bitbucket                                        # refresh later
```

Useful flags: `-a claude-code` targets a specific agent, `-y` runs
non-interactively, `--list` previews a repo's skills.

> **Maintainers:** bump `version:` in `skills/bitbucket/SKILL.md` on every
> change to the Skill or its `references/`, so both `bitbucket-cli skill show`
> and `npx skills update` report the new version.

### Removing the Skill

```bash
bitbucket-cli skill uninstall          # auto-detect; remove from each agent found
bitbucket-cli skill uninstall --agent codex
bitbucket-cli skill uninstall --dir <path>
npx skills remove bitbucket            # if installed via the skills CLI
```

`skill uninstall` takes the same `--agent` / `--project` / `--dir` flags as
`skill install`; removing a Skill that is not installed is a no-op.
