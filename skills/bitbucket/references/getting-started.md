# Getting started

`bitbucket-cli` speaks to Bitbucket Cloud (REST 2.0) and Data Center / Server
(REST 1.0). Pick the flow that matches your environment.

## One-shot interactive setup

```sh
bitbucket-cli config init --pretty
```

`--pretty` is **human-only** (the interactive TUI + colorized JSON output) and errors
without a TTY — agents should run plain `config init`, or better, reuse the user's
existing config (see "For agents and sandboxes" below). The TUI walks through: base
URL → flavor detection → auth scheme → credential entry → keychain storage choice. It
writes `~/.angelmsger/bitbucket/config.yaml`
(or, on legacy installs that still have a `~/.bitbucket/config.yaml`, that
location); secrets go to the OS keychain (`service="bitbucket-cli"`), or to a
`credentials` file inside the same directory (mode 0600) when no keychain is
available.

## Environment-driven setup

| Variable | Effect |
|---|---|
| `BITBUCKET_SERVER` | base URL (Cloud: `https://api.bitbucket.org`; DC: `https://bitbucket.your.host`) |
| `BITBUCKET_FLAVOR` | `cloud` / `datacenter` / `auto` |
| `BITBUCKET_TOKEN` or `BITBUCKET_PERSONAL_ACCESS_TOKEN` | bearer token (PAT, HTTP Access Token, Workspace Access Token) |
| `BITBUCKET_USERNAME` + `BITBUCKET_API_TOKEN` | Cloud basic auth (email + API token) |
| `BITBUCKET_USERNAME` + `BITBUCKET_PASSWORD` | DC basic auth |
| `BITBUCKET_DEFAULT_WORKSPACE` | default workspace slug (Cloud) or project key (DC) |
| `BITBUCKET_FORMAT` | default output format (`json` / `table` / `ndjson`) |

## Verify

```sh
bitbucket-cli doctor          # DNS, TLS, API ping, auth probe
bitbucket-cli whoami          # confirms the active identity
bitbucket-cli workspace list  # discover the workspaces (Cloud) / projects (DC) you can see
```

The `slug` field of each workspace entry is what every other command's
`--workspace` flag (and `BITBUCKET_DEFAULT_WORKSPACE`) accepts.

`bitbucket-cli auth login` re-stores credentials for an existing context.
`bitbucket-cli auth logout` deletes them.

## For agents and sandboxes

If you are an AI agent driving `bitbucket-cli`, the user has normally already
configured it. **Reuse their existing config and credentials** from
`~/.angelmsger/bitbucket/config.yaml` + the OS keychain — do not run `config init`
to create a fresh setup, and do not pass `--pretty`.

When you run inside a **sandbox** that cannot read the user's home directory or
keychain, `doctor` / `auth status` fail with a `config` (3) or `auth` (4) error.
Do **not** give up, and do **not** re-initialize config inside the sandbox. Instead:

- **Request elevated permissions** (or otherwise re-run with access to the user's
  real environment / home / keychain) and retry the same command — that is almost
  always why a configured machine looks "unconfigured" from inside a sandbox.
- Never launch interactive `config init` or `auth login` yourself: without a TTY they
  fail fast (and historically could hang). If credentials are genuinely missing, ask
  the user to run `config init` in their own terminal, or to export `BITBUCKET_*`
  env vars (`BITBUCKET_SERVER` + `BITBUCKET_TOKEN`, or `BITBUCKET_USERNAME` +
  `BITBUCKET_API_TOKEN` / `BITBUCKET_PASSWORD`) which you can then use directly.

## Contexts

The resolved config file (`~/.angelmsger/bitbucket/config.yaml`, falling back
to `~/.bitbucket/config.yaml`) supports named contexts, one per Bitbucket
instance.
Switch with `--use-context <name>` for one invocation, or rerun
`config init --use-context <name>` to set up another.
