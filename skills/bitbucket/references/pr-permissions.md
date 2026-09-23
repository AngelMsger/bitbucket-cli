# PR permissions and recovery

Use this reference when a PR action fails. Continue independent review reads
while resolving the action; keep the failure report with the user.

## Account rights and token permissions are separate

Atlassian's [HTTP access token permission table](https://confluence.atlassian.com/bitbucketserver/http-access-tokens-939515499.html)
places PR actions under **Repository write**, not Repository read. A read-only
Data Center Personal Access Token (PAT) can therefore read a PR yet be rejected
when approving, declining, or voting Needs Work on it. The token also remains
limited by its owner's access; adding token permissions does not grant the
account new repository rights. A browser login uses its own session and may
allow an action blocked for the PAT. Deployment policy and PR state can still
prevent the action.

The CLI sends `POST .../approve`, `POST .../decline` with the current PR
version, and, on Data Center, `PUT .../participants/<your-slug>` for Needs
Work. A server rejection is not by itself evidence of a malformed request. Keep
the error code, HTTP status, sanitized server message, action, and PR URL; do
not label every permission failure a token-scope problem without evidence.

## Classify before retrying

| Evidence | Next action |
| --- | --- |
| `READONLY_BLOCKED` | The CLI blocked the write locally. Preserve an explicit read-only task across all tools. Use `--allow-writes` only when existing authorization covers that override. |
| `CREDENTIAL_STORE_INACCESSIBLE`, `CREDENTIAL_NOT_VISIBLE_OR_MISSING`, or `recovery.scope=host` | Follow the single host retry in [Getting started](getting-started.md#for-agents-and-sandboxes). This does not diagnose server permissions. |
| HTTP 401 | Check the selected instance and identity; the server rejected the credential. Do not erase or replace credentials automatically. |
| HTTP 403 (`HTTP_Forbidden`) | Inspect the server message and known token/account permissions. Stop repeating the same write with unchanged credentials; `--allow-writes` and sandbox elevation cannot grant server permissions. Use the browser recovery below when authorized. |
| HTTP 409 or changed PR state | Refresh the PR and re-evaluate the action against its current state and commits. A browser is not a way to ignore a conflict. |
| Timeout, disconnected response, 5xx, or response decoding failure | The write may have succeeded. Read PR/reviewer state and relevant activity before another attempt through any tool. If still uncertain, report it and stop replaying the write. |

A successful `--dry-run` checks the request plan, **not** write permission. A
successful `whoami`, PR read, or authentication probe does not prove write
access. For batch output, inspect each item's `ok`/`error` and reconcile only
failed or uncertain targets; never replay the whole batch after partial success.

## Browser recovery for an authorized action

When the server rejected an authorized CLI write for permissions and a browser
tool is available, make one bounded attempt in the normal Bitbucket UI using
the user's existing signed-in session:

1. Confirm the instance URL, PR, and signed-in account match the intended
   user's verified identity; never substitute another account. No matching
   session: report the blocker.
2. Re-read PR and reviewer state. If the action is already recorded, report it
   without repeating it. Apply the [verdict table](reviewing-locally.md#choose-a-review-verdict)
   to the refreshed revisions and votes before a review vote.
3. Carry out only the action already authorized. Decline closes the PR and
   needs authorization to close; it is not a substitute for Needs Work. Leave
   an approval's optional comment blank. The
   [publication rules](reviewing-locally.md#decide-what-to-publish), AI
   attribution, and human-reply gate apply to browser comments.
4. Verify the resulting state through the UI or a CLI read, then report the
   action, the PR link, the verified result, and that the browser was used
   after the CLI rejection. A click is not proof.

If the action is unavailable, denied, or uncertain after a state check, stop
and report the concrete blocker. Do not switch accounts, extract cookies into
the CLI, or change token permissions automatically. This recovery never
overrides an explicit read-only instruction or an automation restriction from
the user or an administrator.

Without a usable browser session, finish the review and report the pending
action; the user can perform it in the UI or arrange token/account permissions
(`auth guide` supplies the credential-management link). Never claim the action
happened, or post an explanatory comment, because the review is complete.
