# Errors and exit codes

Every error is emitted on stderr as a single-line JSON envelope:

```json
{
  "error": {
    "category": "auth",
    "code": "HTTP_Unauthorized",
    "message": "Bitbucket returned HTTP 401: ...",
    "hint": "Check that your token or password is current.",
    "next_steps": ["bitbucket-cli auth login", "bitbucket-cli doctor"],
    "retryable": false,
    "http_status": 401
  }
}
```

The process exit code matches the category:

| Code | Category       | Meaning                                                                          |
|------|----------------|----------------------------------------------------------------------------------|
| 2    | `usage`        | A flag/argument was malformed (e.g. bad PR ref, missing `--yes`).                |
| 3    | `config`       | Configuration is missing or invalid; run `config init` or set env vars.          |
| 4    | `auth`         | 401/403 from Bitbucket. Refresh the token or revisit `auth login`.               |
| 5    | `permission`   | 403 from Bitbucket, **or** `READONLY_BLOCKED` from local read-only mode.         |
| 6    | `not_found`    | 404 — the workspace, repo, PR, branch or commit does not exist for this user.    |
| 7    | `rate_limit`   | 429 — back off; `retryable=true`.                                                |
| 8    | `network`      | DNS/TLS/socket failure; `retryable=true`.                                        |
| 9    | `server`       | 5xx from Bitbucket; `retryable=true`.                                            |
| 10   | `parse`        | The response body did not match the expected shape — likely a client bug.       |
| 12   | `internal`     | An unexpected client-side error.                                                 |

## Common recovery flows

- **`auth`** → `bitbucket-cli auth logout && bitbucket-cli auth login` (or set
  `BITBUCKET_TOKEN`).
- **`not_found`** → confirm with `bitbucket-cli repo get <ref>` / `pr get <ref>`.
- **`usage` on `pr decline`/`pr merge`/`pr delete`/`comment delete`** → add `--yes`.
- **`usage` `NOT_A_GIT_WORKTREE` on `pr fetch --exec` / `pr checkout --exec`** →
  the current directory is not inside a git checkout. `cd` into a local clone
  of the repo first, or drop `--exec` to keep the print-only behaviour.
- **`rate_limit`** → wait the retry-after window then re-run.
- **`permission` `READONLY_BLOCKED`** → the current session is in read-only
  mode (`defaults.read_only` or `BITBUCKET_CLI_READ_ONLY=1`). To send the
  write anyway, add `--allow-writes` to the command line; to preview the
  request without sending, add `--dry-run`. See `safety-modes.md`.

## Diagnostic mode

`bitbucket-cli doctor` walks DNS, TLS, API reachability, and auth probes in
order, returning a structured report.
