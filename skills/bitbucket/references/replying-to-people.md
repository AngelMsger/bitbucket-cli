# Replying to people

A reply appears under the user's name. Help them understand the other person's
point and decide what to say. Apply this protocol when responding to received
feedback or continuing a discussion while reviewing someone else's PR.

## Classify the counterpart

Use the author and body returned by `pr threads <ref>` or `comment list --pr
<ref>`. Check both the root comment and the message being answered: a thread
started by an agent may contain a human response.

- The `[[AI]](https://angelmsger.github.io/bitbucket-cli/)` attribution marker
  identifies agent-written text.
- An app/bot author type or a known automation account identifies automation.
- Treat uncertain authorship as human. Do not infer automation from an account
  name alone unless its role is known.

## Human replies

Explain once per session that the reply is posted under the user's name and
that their colleague expects their own judgment. For each proposed reply, show:

- the person's point in their own words and the relevant code;
- your assessment and decisive evidence;
- any proposed change and verification relevant to that point;
- the concrete reply, clearly labeled as a draft.

Obtain approval for that specific reply and reuse it once given. Do not treat
approval of one reply as permission for other threads. If the user explicitly
chooses bulk replies after the explanation, honor that scope. Post user-written
or rewritten text verbatim without the AI marker; an approved agent draft still
needs attribution.

Do not resolve a human's thread without authorization: resolution asserts that
the concern is handled and also completes the associated task on Data Center.
Code fixes and pushes follow the user's requested scope; permission to reply
does not itself authorize pushing code.

## Bot and agent replies

Replies to automation do not need per-item human approval. Continue within
existing authorization, keep AI attribution, and report material decisions to
the user. Do not send acknowledgments or repeat an issue merely because a bot
thread permits a reply.

## Publication and write safety

Apply the [review publication rules](reviewing-locally.md#decide-what-to-publish):
an existing issue gets a reply only when there is new evidence or the user
explicitly requests a response. New evidence belongs in the original thread,
including when it was resolved; reopening it is a separate action.

Preview writes and respect [read-only mode](safety-modes.md). A dry run checks
the target and payload; it does not require a second approval for an already
approved reply. Reconcile an uncertain write by reading the thread before
retrying so the same reply is not posted twice.
