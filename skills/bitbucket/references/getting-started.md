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
`credentials` file inside the same directory when no keychain is available.
Windows encrypts that fallback with per-user DPAPI; macOS/Linux use mode 0600.

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

On Data Center, `whoami` reads the authenticated response's `X-AUSERNAME`
header and then resolves the matching user record. If a reverse proxy strips
that header, the command returns `AUTH_IDENTITY_UNAVAILABLE` instead of
claiming success without a username; check `auth status`, `doctor`, and the
proxy's response-header configuration.

`bitbucket-cli auth login` re-stores credentials for an existing context.
`bitbucket-cli auth logout` deletes them.

## For agents and sandboxes

If you are an AI agent driving `bitbucket-cli`, the user has normally already
configured it. **Reuse their existing config and credentials** from
`~/.angelmsger/bitbucket/config.yaml` + the OS keychain — do not run `config init`
to create a fresh setup, and do not pass `--pretty`.

When you run inside a **sandbox** that cannot read the user's home directory or
keychain, credential resolution returns `CREDENTIAL_STORE_INACCESSIBLE` or
`CREDENTIAL_NOT_VISIBLE_OR_MISSING` with `recovery.scope=host`.
Do **not** give up, and do **not** re-initialize config inside the sandbox. Instead:

- **Request elevated permissions** (or otherwise re-run with access to the user's
  real environment / home / keychain) and retry the same command — that is almost
  always why a configured machine looks "unconfigured" from inside a sandbox.
- Never launch interactive `config init` or `auth login` yourself: without a TTY they
  fail fast (and historically could hang). If credentials are genuinely missing, ask
  the user to run `config init` in their own terminal, or to export `BITBUCKET_*`
  env vars (`BITBUCKET_SERVER` + `BITBUCKET_TOKEN`, or `BITBUCKET_USERNAME` +
  `BITBUCKET_API_TOKEN` / `BITBUCKET_PASSWORD`) which you can then use directly.

The `recovery` object is a machine-readable instruction to the Agent host, not a
request for the CLI to escape its sandbox. Retry the same invocation in the host
scope once; if that host retry also reports missing credentials, configuration is
genuinely required.

## Contexts

The resolved config file (`~/.angelmsger/bitbucket/config.yaml`, falling back
to `~/.bitbucket/config.yaml`) supports named contexts, one per Bitbucket
instance.
Switch with `--use-context <name>` for one invocation, or rerun
`config init --use-context <name>` to set up another.

Contexts on one host and auth scheme share a stored secret — a team preset and
the personal context whose login it reuses, for example. `config delete-context
<name>` removes that secret only when no remaining context uses it; `auth
logout` removes it for all of them.

For preset team services, use `config set-context` and `auth guide` before personal login; see [team setup](team-setup.md).

## Reuse existing authentication

`bitbucket-cli --use-context team auth reuse` points a context without a working
login at one this machine already verified for the same complete service URL and
deployment flavor; `--dry-run` previews the same decision. Workspace setup tools
run it for you, and the CLI lists it ahead of any sign-in step, including after an
HTTP 401. It looks at contexts under the context's own scheme first, then at a
login under either scheme (`pat` or `basic`, including a secret stored for this
service that no context points at any more, paired with the usernames recorded on
the service). A username under `pat` is only a label: it does not stop reuse, and
a context with a label only adopts a password for that same username. A working
login is never replaced automatically. The result reports `scheme`, `username`,
`account` (Cloud account UUID, Data Center username) and, when the source is a
context, `source_context`; its `next_steps` name the check to run next
(`whoami`). No secret is copied and the current context does not change.

Comments, approvals and merges then belong to `account`. Before the first write
through a context that reuse changed, tell the user which account it acts as and
confirm it is the intended one; this is part of the write's authorization, not a
separate approval.

`AUTH_REUSE_AMBIGUOUS` lists `details.candidates`; choose one with
`--from-context <name>`, `--from-scheme <scheme>`, or both (the stored secret of
that scheme paired with that context's username). Store, network and permission
errors require their named recovery, not a new login. When the result says the
keychain could not be read here, retry on the host before signing in.
