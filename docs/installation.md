# Installation & setup guide

This guide covers three things:

1. [Installing the `bitbucket-cli` binary](#1-install-the-cli)
2. [Enabling shell completion](#2-enable-shell-completion)
3. [Installing & updating the companion `bitbucket` Skill](#3-install-the-companion-skill)

---

## 1. Install the CLI

**npm is the recommended way to install** — it downloads the prebuilt binary
for your platform, verifies its checksum, and keeps upgrades a single
`npm update -g` away. The *Other methods* below are alternatives.

### npm (recommended)

```bash
npm install -g @angelmsger/bitbucket-cli
```

Installing downloads the prebuilt binary for your platform from the matching
GitHub Release and verifies its SHA-256 checksum. If your npm setup disables
install scripts (`--ignore-scripts`, some pnpm setups), the binary is fetched
on first run instead.

### Other methods

Prefer not to use npm? Any of these also work.

#### go install

```bash
go install github.com/angelmsger/bitbucket-cli/cmd/bitbucket-cli@latest
```

Installs into `go env GOBIN` (or `$GOPATH/bin`). Requires Go 1.24+.

#### Prebuilt binary

Download the binary for your platform from the
[Releases page](https://github.com/AngelMsger/bitbucket-cli/releases), verify
it against `checksums.txt`, then put it on your `PATH`.

On macOS/Linux:

```bash
chmod +x bitbucket-cli-* && mv bitbucket-cli-* /usr/local/bin/bitbucket-cli
```

On Windows PowerShell, download `bitbucket-cli-windows-amd64.exe` (or
`windows-arm64.exe`) together with `checksums.txt`, then:

```powershell
$asset = "bitbucket-cli-windows-amd64.exe"
$checksumLine = Get-Content .\checksums.txt | Where-Object { $_ -match "\s+$([regex]::Escape($asset))$" } | Select-Object -First 1
if (-not $checksumLine) { throw "No checksum found for $asset" }
$expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
$actual = (Get-FileHash ".\$asset" -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "SHA-256 mismatch for $asset" }
$binDir = Join-Path $HOME "bin"
New-Item -ItemType Directory -Force $binDir | Out-Null
Move-Item ".\$asset" (Join-Path $binDir "bitbucket-cli.exe")
[Environment]::SetEnvironmentVariable("Path", ([Environment]::GetEnvironmentVariable("Path", "User") + ";$binDir"), "User")
```

Open a new PowerShell window after changing `PATH`.

#### From source

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

For headless setup in PowerShell, environment variables use `$env:` syntax:

```powershell
$env:BITBUCKET_SERVER = "https://api.bitbucket.org"
$env:BITBUCKET_USERNAME = "alice@example.com"
$env:BITBUCKET_API_TOKEN = "<api-token>"
bitbucket-cli doctor
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

The `bitbucket` Skill teaches a coding agent — **Claude Code**, **Codex**, **Cursor**, **Agents** (shared), **Gemini CLI**, **GitHub Copilot**, **OpenCode**, **Continue**, **Windsurf**, **Grok Build**, **Pi**, **Kilo Code**, and **Roo Code** —
how to drive this CLI. It is **embedded in the `bitbucket-cli` binary**, so
whichever way you installed the CLI — npm, `go install`, a prebuilt binary —
you already have a version-matched copy of the Skill.

### Recommended: `bitbucket-cli skill install`

With no flags, `skill install` **probes for installed agents** and installs
the Skill into every one it finds:

```bash
bitbucket-cli skill install              # auto-detect; install for each agent found
bitbucket-cli skill install --agent codex          # only Codex
bitbucket-cli skill install --agent cursor,agents,gemini
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
| Cursor | `~/.cursor/skills/bitbucket` | `./.cursor/skills/bitbucket` |
| Agents (shared) | `~/.agents/skills/bitbucket` | `./.agents/skills/bitbucket` |
| Gemini CLI | `~/.gemini/skills/bitbucket` | `./.gemini/skills/bitbucket` |
| GitHub Copilot | `~/.copilot/skills/bitbucket` | `./.agents/skills/bitbucket` |
| OpenCode | `~/.config/opencode/skills/bitbucket` | `./.opencode/skills/bitbucket` |
| Continue | `~/.continue/skills/bitbucket` | `./.continue/skills/bitbucket` |
| Windsurf | `~/.codeium/windsurf/skills/bitbucket` | `./.windsurf/skills/bitbucket` |
| Grok Build | `~/.grok/skills/bitbucket` | `./.grok/skills/bitbucket` |
| Pi | `~/.pi/agent/skills/bitbucket` | `./.pi/skills/bitbucket` |
| Kilo Code | `~/.kilocode/skills/bitbucket` | `./.kilocode/skills/bitbucket` |
| Roo Code | `~/.roo/skills/bitbucket` | `./.roo/skills/bitbucket` |

Auto-detection looks for each agent's home or project marker (`~/.claude`, `~/.codex`, `~/.cursor`, `~/.agents`, `~/.gemini`, `~/.copilot`, `~/.config/opencode`, `~/.continue`, `~/.codeium/windsurf`, `~/.grok`, `~/.pi`, `~/.kilocode`, `~/.roo`, and the matching project dirs). If nothing is detected, pass `--agent` or `--dir` explicitly.

The matching Skill ships inside the binary, but a deployed copy is not replaced
by the package manager. After every CLI upgrade, run `bitbucket-cli skill
install`, then reload the agent context. `bitbucket-cli skill status` compares
the loaded, installed, and embedded versions and reports the next steps when
they differ.

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

## Team distribution and personal login

Distribute service settings separately from each member's credentials. An installer
can write a named context without a network connection or access to the keychain:

```bash
bitbucket-cli config set-context team \
  --base-url https://service.example.com/deploy --flavor datacenter \
  --auth-scheme pat --activate

# The member completes personal authentication in a terminal.
bitbucket-cli --use-context team auth guide
bitbucket-cli --use-context team auth login
```

`config set-context <name>` resolves **flags > environment > `.env` > the named
target context > defaults**. It ignores personal environment fields and secrets,
including secret-based scheme inference. It never verifies connectivity, reads or
writes the keychain, or changes another context's values or shared defaults.
Existing usernames remain unchanged. The first context becomes current;
subsequent calls change the current context only with `--activate`.

Identical presets do not rewrite the file. Conflicting non-empty service fields
return `CONFIG_CONTEXT_CONFLICT` with a `details` object containing each field's
`before` and `after` values. Inspect those differences, then use `--overwrite`
to update the supplied service fields, or use another context name. Unspecified
fields are retained. `--dry-run` uses the same merge and conflict checks and
returns the proposed changes without writing anything; use `--overwrite
--dry-run` to preview a deliberate conflicting update.

A launcher or CI environment can instead inject service settings on every run:

```bash
export BITBUCKET_SERVER=https://service.example.com/deploy
export BITBUCKET_FLAVOR=datacenter
export BITBUCKET_AUTH_SCHEME=pat
# Optional: a verified page for the team's version or an internal setup guide.
export BITBUCKET_CREDENTIAL_URL=https://help.example.com/bitbucket/credentials
bitbucket-cli auth guide
bitbucket-cli auth login
```

Exports must be sourced into the member's shell or injected by a launcher/CI;
an executed child script cannot export values back into its parent shell.
`--auth-scheme` and `--credential-url` override these variables. The optional
page is persisted as `auth.credential_url` by `config set-context` and is
**display-only**: the CLI never sends an API request or credential to it.
Service paths such as `/deploy` are retained when deriving page links.

`auth guide` works offline and emits `server`, `flavor` where applicable,
`scheme`, `credential_url`, `source`, `instructions`, `documentation_url`, and
`next_steps`. Sources are `flag`, `env`, `dotenv`, `file`, `builtin`, or `fallback`.
There is no server-version probe; navigation instructions accompany version-
dependent links.

Data Center guidance uses the common `/plugins/servlet/access-tokens/manage` page, with
avatar → Manage account → HTTP access tokens as the version-independent navigation
fallback. Cloud personal login uses basic auth with an Atlassian account email and an
API token with Bitbucket scopes; resource access tokens use `pat`. The guide does not
verify token scopes or server capabilities.

`auth login` reuses the resolved service, shows the same guide on stderr, asks
only for the missing username and the secret, and verifies authentication before
saving. It saves the username/scheme in the config and the secret in the existing
secure store; an environment-only service becomes a default context if none
exists. A later process can resolve that identity without another username
prompt. A different full service URL (including deployment path) in the selected
context fails with `CONTEXT_BASE_URL_MISMATCH` before storing a credential.

`CREDENTIAL_SAVE_FAILED` means validation succeeded but storage failed.
`LOGIN_CONFIG_WRITE_FAILED` means the credential was stored but its config
identity could not be recorded; its details preserve the server/context/scheme
and `credential_stored: true`. Fix file access and run `auth login` again.
Configuration files are replaced atomically. Normal credential environment
variables remain transient and are never copied by `config set-context`.
Non-interactive users supply credentials through the documented environment
variables rather than piping secrets into `auth login`. `config init` retains its
edit/add/replace flow and now uses target-specific presets and the same guide.
