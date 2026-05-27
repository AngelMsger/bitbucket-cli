# Getting started

`bitbucket-cli` speaks to Bitbucket Cloud (REST 2.0) and Data Center / Server
(REST 1.0). Pick the flow that matches your environment.

## One-shot interactive setup

```sh
bitbucket-cli config init --pretty
```

The TUI walks through: base URL → flavor detection → auth scheme → credential
entry → keychain storage choice. It writes `~/.angelmsger/bitbucket/config.yaml`
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

## Contexts

The resolved config file (`~/.angelmsger/bitbucket/config.yaml`, falling back
to `~/.bitbucket/config.yaml`) supports named contexts, one per Bitbucket
instance.
Switch with `--use-context <name>` for one invocation, or rerun
`config init --use-context <name>` to set up another.
