# PR permissions and recovery

Use this reference when a PR action fails, especially Data Center approval or
decline with a Personal Access Token (PAT). Continue independent review reads
while resolving the action; keep the failure report with the user.

## Account rights and token permissions are separate

Atlassian's [HTTP access token permission table](https://confluence.atlassian.com/bitbucketserver/http-access-tokens-939515499.html)
places PR actions under **Repository write**, not Repository read. A read-only
PAT can therefore read a PR yet be rejected when approving or declining it.
The token also remains limited by its owner's access; adding token permissions
does not grant the account new repository rights.

The Data Center REST references for
[legacy approval](https://developer.atlassian.com/server/bitbucket/rest/v818/api-group-deprecated/)
and [decline](https://developer.atlassian.com/server/bitbucket/rest/v902/api-group-pull-requests/)
document `REPO_READ` as the account permission for those endpoints. That
does not mean a PAT restricted to Repository read permits those writes. A
browser login uses its own session and may allow an action blocked for the PAT.
Deployment policy and PR state can still prevent the action.

The CLI sends `POST .../pull-requests/<id>/approve` for approval and
`POST .../pull-requests/<id>/decline` with the current PR version for Data Center
decline. A server rejection is not by itself evidence of a malformed request.
Keep the error code, HTTP status, sanitized server message, action and PR URL;
do not label every permission failure a token-scope problem without evidence.

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
successful `whoami`, PR read, or authentication probe does not prove write access.
For batch output, inspect each item's `ok`/`error` and reconcile only failed or
uncertain targets; never replay the whole batch after partial success.

## Browser recovery for an authorized action

When the CLI is blocked by server permissions and a browser tool is available,
try the normal Bitbucket UI using the user's existing signed-in session:

1. Confirm the complete instance URL, PR repository/ID, and the signed-in
   account. Match the intended user's verified identity; do not substitute
   another account. If no matching session is available, report the blocker.
2. Read the current PR and reviewer state before acting. If the requested
   approval or decline is already recorded, report it without repeating it.
   If reviewed commits changed, revalidate the review before approval.
3. Inspect the enabled UI action and its target. Carry out only the action
   already authorized, using the browser tool's normal interaction and preview
   facilities. Existing approval needs no new confirmation merely because the
   tool changed. Decline closes the PR and requires authorization to close it;
   it is not a substitute for requesting changes or withholding approval.
4. For a clean approval, leave any optional comment blank. The same
   [review publication rules](reviewing-locally.md#decide-what-to-publish), AI
   attribution, and human-reply gate apply to browser comments.
5. Verify the resulting reviewer/PR state through the UI or a CLI read. Report
   the action, PR link, and verified result to the user, including that the
   browser was used after the CLI rejection. Do not treat a click as proof.

Use one bounded browser attempt. If the action is unavailable, denied, or has
an uncertain result after a state check, stop and report the concrete remaining
blocker. Do not switch accounts, extract cookies into the CLI, or change token
permissions automatically. This recovery never overrides an explicit read-only
instruction or an automation restriction imposed by the user or administrator.

If no usable browser session exists, finish the review and report the pending
action. The user can perform it in the UI or arrange suitable token/account
permissions; `auth guide` supplies the configured credential-management link.
Do not claim the PR was approved or declined, or post an explanatory PR comment,
merely because the review is complete.
