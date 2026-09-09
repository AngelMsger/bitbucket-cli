# Replying to people, not to bots

A review comment written by a person is one half of a conversation. When an agent
answers it on the author's behalf, both halves stop being a conversation: the
author never engages with feedback they did not read, and the reviewer gets an
answer nobody stands behind. Review is where a team exchanges reasoning — that is
most of its value, and it is the part an agent can destroy without anyone
noticing.

So the rule is not "never reply". It is: **help the author answer, do not answer
for them.** Do the analysis, do the verification, draft the words — then hand the
draft back and let the person decide what goes out under their name.

This gate applies to human-authored threads only. Replying to a bot linter or to
another agent's review is a different situation; see "AI-authored counterparts"
below.

## Classify the counterpart before you draft

`pr threads <ref>` and `comment list --pr <ref>` already return everything you
need — you do not need an extra call. For each thread, look at its **root**
comment:

- Body starts with the `[[AI]](https://angelmsger.github.io/bitbucket-cli/)`
  attribution marker → **AI-authored**. Every agent driving this CLI is required
  to add it (see `commenting.md` › "AI attribution").
- `author.type` names an app / bot account, or `author.display_name` is a
  recognizable automation (a linter, a CI reporter, a security scanner) →
  **automation**.
- Anything else → **treat it as human.**

Never resolve the ambiguity in the permissive direction. An unmarked comment from
an account you cannot classify is a person until proven otherwise. Carry the
classification into the triage report so the author can see it too.

## The gate for human-authored threads

**Say why, once.** Before the first reply of a session, tell the author plainly
what is about to happen and why they should read the feedback themselves: the
reply will be posted under their name, the reviewer is a colleague expecting a
colleague's answer, and the understanding they get from working through the
comment is the point of the review. Say it **once per session** — a reminder that
repeats on every thread stops being read and becomes a thing to click past.

**Then work one thread at a time.** For each human-authored thread, present:

- the reviewer's point **in their own words** (quote it, don't summarize it away);
- the anchored code, so the author can see what is actually being discussed;
- your assessment — agree or disagree, **and the reasoning**, not just a verdict;
- the change you propose and how you verified it;
- the draft reply, clearly labeled a draft.

Then ask for a go-ahead on **that thread**, and invite the author to correct or
rewrite the draft. If they rewrite it, post their text verbatim and **drop the
`[AI]` marker** — they authored it, and the marker is only for agent-driven
writes.

**One approval covers one thread.** Do not carry a blanket "yes, go ahead" across
several human-authored threads. If the author explicitly asks for exactly that
after the reminder, do it — they have made an informed call, and it is theirs to
make — but keep the `[AI]` marker on every reply you post, so the reviewer can
see what they are talking to.

## What you do not do on your own

- **Do not resolve a human's thread.** `comment resolve <id> --pr <ref>` asserts
  "I agree, and this is handled" under the author's name, and on Data Center it
  also completes the associated task. That claim is the author's to make. Propose
  it, let them confirm — and if they disagree with the reviewer, the thread stays
  open for the reviewer to answer.
- **Do not batch replies.** Never fan `comment add --reply-to` across several
  threads in one pass, and never build a reply list from a single instruction like
  "answer all the comments". Batch mode exists for approvals and deletes, not for
  conversations.
- **Do not push code fixes.** Apply them on the local checkout and let the author
  review and push, as in `responding-to-review-comments.md`.

## AI-authored counterparts

When the root comment is AI-authored or automation — a bot linter, a CI reporter,
another agent's review — the per-thread ritual is unnecessary; nobody is being
answered in a person's place. Still:

- confirm before the first write of the session, as with any mutation;
- keep the `[AI]` attribution on what you post;
- surface anything the author should know rather than quietly closing it out — a
  finding you judged wrong, a rule you are suppressing, a fix you applied. A bot
  can be wrong too, and the author still owns the outcome.

## This composes with the existing gates

The confirmation gate is about *who is being answered*. It sits on top of, not
instead of, `--dry-run` and read-only mode — preview the write, respect
`BITBUCKET_CLI_READ_ONLY`, and see `safety-modes.md`. In a read-only session you
cannot post at all: give the author the draft and let them post it themselves.
