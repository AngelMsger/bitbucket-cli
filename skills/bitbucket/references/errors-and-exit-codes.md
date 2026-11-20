# Errors and exit codes

Every error is emitted on stderr as a JSON envelope:

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
| 1    | `internal`     | An unexpected client-side error.                                                 |
| 2    | `usage`        | A flag/argument was malformed (e.g. bad PR ref, missing `--yes`).                |
| 3    | `config`       | Configuration/credential resolution failed; inspect `code` and `recovery`.       |
| 4    | `auth`         | 401 from Bitbucket. The server rejected the resolved credential.                 |
| 5    | `permission`   | 403 from Bitbucket, **or** `READONLY_BLOCKED` from local read-only mode.         |
| 6    | `not_found`    | 404 — the workspace, repo, PR, branch or commit does not exist for this user.    |
| 7    | `rate_limit`   | 429 — back off; `retryable=true` for a read (a write: see below).                |
| 8    | `network`      | DNS/TLS/socket failure; `retryable=true` for a read (a write: see below).        |
| 9    | `server`       | 5xx from Bitbucket; `retryable=true` for a read (a write: see below). A 3xx `http_status` is a redirect the CLI does not follow: `retryable=false`; the hint names where it points; check `--base-url` with `doctor`. |
| 10   | `parse`        | The response body did not match the expected shape — likely a client bug.       |
| 11   | `conflict`     | HTTP 409 or incompatible current state; read it before deciding whether to retry. |

## Common recovery flows

- **"Write outcome is unknown for …"** → a write failed after it may have
  reached Bitbucket (network error, 429, 5xx, or a redirect: code
  `HTTP_REDIRECTED` with a 3xx `http_status`, which usually means a login
  page or single sign-on proxy; check the base URL with `doctor`). It is
  `retryable=false`: do not repeat it. Run the read in `next_steps` (for
  example `pr get`, `comment list --pr`, `branch get`), follow pagination if
  needed, and repeat the write only if the read shows it did not happen.
  `WRITE_SUCCEEDED_RESPONSE_INVALID` and `WRITE_SUCCEEDED_READ_FAILED` mean
  the write happened but its result could not be shown; read it, never redo it.
- **`BATCH_PARTIAL_FAILURE`** (`pr approve|decline`, `comment delete` or batch
  reads `pr get|status|threads`) → for write batches, items marked `ok` on
  stdout were applied, so never rerun the whole batch; retry only failed items
  whose own error is retryable. For read batches or `--dry-run`, nothing was
  modified; retry failed refs or fix invalid inputs.
- **`PR_HEAD_MOVED`** (exit 11 `conflict` on `pr review`) → the remote PR's
  `source.commit` changed since `--expected-source-commit` was captured; zero
  writes were sent. Refresh the PR and diff, revalidate anchors, and retry with
  the new head SHA.
- **`REVIEW_PARTIAL_FAILURE`** (`pr review`) → Phase 1 validation passed, but a
  Phase 2 write failed mid-flight. Inspect `posted_comments`, `failed_item`, and
  `vote_applied` emitted on stdout before retrying only the remaining unposted
  items (never blindly resubmit already-posted comments).

- **`CREDENTIAL_STORE_INACCESSIBLE` / `CREDENTIAL_NOT_VISIBLE_OR_MISSING`** →
  inspect the optional `recovery` object. When it says
  `{"action":"retry_current_command","scope":"host"}`, request host access
  and retry the same invocation once. This is not a normal `retryable=true`
  retry: repeating it in the same sandbox will not help. Only configure
  credentials when the host retry also reports them missing.
- **`auth` / HTTP 401** → check the selected instance and identity; the server
  rejected the credential. Preserve existing configuration and credentials.
  When replacement is needed, direct the user to `auth login` in their terminal;
  do not launch interactive login or log them out automatically. Host credential
  access failures use the separate recovery above.
- **`not_found`** → confirm with `bitbucket-cli repo get <ref>` / `pr get <ref>`.
- **`HTTP_Conflict` / HTTP 409** → read `message` for the server's actual
  reason. A closed PR, merge conflict, or existing resource is not necessarily
  a version conflict. For PR actions, read `pr get <ref> --scope full`;
  approval requires an open PR. Resolve the reported cause before retrying;
  refreshing alone cannot fix a closed PR. If no reason is returned, inspect
  the current state instead of guessing.
- **`PR_NO_CHANGE_REQUEST`** → Data Center found no confirmed Needs Work vote
  to withdraw. Read `pr get <ref> --scope full --fields reviewers,participants`;
  preserve approvals and do not retry through the browser.
- **`usage` on `pr decline`/`pr merge`/`comment delete`** → add `--yes` when the
  destructive action is authorized.
- **`usage` `NOT_A_GIT_WORKTREE` on `pr fetch --exec` / `pr checkout --exec`** →
  the current directory is not inside a git checkout. `cd` into a local clone
  of the repo first, or drop `--exec` to keep the print-only behaviour.
- **`usage` `GIT_REMOTE_MISMATCH` on `pr fetch` / `pr checkout`** → no remote
  (or the explicit `--remote`) matches the PR's destination or source
  repository. `cd` into a checkout of the target repo, pick a matching remote,
  or pass `--force-remote` only for an intentional mirror.
- **`rate_limit`** → wait the retry-after window then re-run.
- **`permission` `READONLY_BLOCKED`** → the current session is in read-only
  mode (`defaults.read_only` or `BITBUCKET_CLI_READ_ONLY=1`). Pass root-level
  `--allow-writes` (`bitbucket-cli --allow-writes <subcommand>`) only when
  local read-only config is active and the write is authorized; `--dry-run`
  remains available. See `safety-modes.md`.
- **`permission` `HTTP_Forbidden` / HTTP 403 on a PR action** → distinguish
  token scope, account rights and deployment policy. For an authorized Data
  Center action, try the same user's existing browser session when available.
  Follow [PR permissions and recovery](pr-permissions.md), including state
  reconciliation before retrying uncertain writes.

## Diagnostic mode

`bitbucket-cli doctor` walks DNS, TLS, API reachability, and auth probes in
order, returning a structured report. Each check includes `status`; credential
checks can also include `recovery_scope: "host"`.
