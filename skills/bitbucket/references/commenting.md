# Posting PR comments

The `comment` subtree writes pull-request comments. All commands target a PR
identified by `--pr <workspace>/<repo>/<id>` or a PR URL.

## Choose the comment type

For PR review, apply the [publication rules](reviewing-locally.md#decide-what-to-publish)
first. Use an inline comment for a new finding, a reply for new evidence on an
existing issue, and a PR-level comment only for a finding without a reasonable
code anchor or an overall summary the user explicitly asked to publish.

For an authorized PR-level comment, read the prepared body from a file:

```sh
bitbucket-cli comment add --pr myws/myrepo/42 --content-file review.md --dry-run
bitbucket-cli comment add --pr myws/myrepo/42 --content-file review.md
```

The examples below illustrate syntax; they are not instructions to publish at
the end of every review. Include AI attribution on agent-written bodies.

## Multi-line bodies (newlines)

A shell does **not** expand `\n` inside double quotes, so a naive
`--content "para 1\n\npara 2"` would send a literal backslash-n and Bitbucket
would render `para 1\n\npara 2` on one line. To remove that footgun, `--content`
(and the other free-text body flags — `pr create/update --description`,
`pr decline/merge --message`, `repo --description`) **decode a small escape
whitelist** — `\n`, `\r`, `\t`, and `\\` — into the real characters before
sending, echoing a one-line `{"_notice":{"corrections":[{"kind":"escape",…}]}}`
to **stderr** so the rewrite is visible and stdout stays clean:

```sh
bitbucket-cli comment add --pr myws/myrepo/42 --content "para 1\n\npara 2"
# stderr: {"_notice":{"corrections":[{"kind":"escape","flag":"--content","detail":"\\n→newline"}]}}
# posted: two paragraphs, rendered with a real blank line between them
```

`\\` is honored so a **literal** backslash sequence is still expressible
(`\\n` → `\n`), and any other escape — a regex `\d`, a Windows path — passes
through untouched. For exact bytes with **no** decoding at all (e.g. a body that
must keep a literal `\n`), use `--content-file` / `--description-file`; those
flags are read verbatim and never rewritten. You can also keep using a real
shell newline (`$'a\nb'`, a heredoc) — that already contains a newline, so
there's nothing to decode.

## AI attribution (agent writes)

When you post a comment **on the user's behalf as an AI agent**, prefix the content
with a clickable **`[AI]`** tag linking back to the tool — apply it to general,
inline, and reply comments alike.

Comment bodies are CommonMark, where `[` and `]` are link syntax. To keep the tag's
**square brackets visible** in the rendered link text, **double the outer brackets**:
write `[[AI]](url)`, which renders the link text literally as `[AI]`. A single-bracket
`[AI](url)` renders as a plain `AI` with the brackets eaten by the link syntax — that
was the bug.

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --inline src/server.go:142 \
  --content "[[AI]](https://angelmsger.github.io/bitbucket-cli/) An empty request reaches items[0] and panics; handle empty input before indexing."
```

That renders as a clickable **[AI]** (brackets visible) followed by the note. (If any
Bitbucket instance mangles the doubled form, the escaped equivalent `[\[AI\]](url)`
renders the same `[AI]`.)

Write the note in the **user's language**; keep the `[AI]` label and URL
constant. Human-written text is posted verbatim without the marker. Attribution
is Skill guidance for agents, not a fixed CLI behavior.

## Inline (line-anchored) comments

Pass `--inline <path>:<line>`. **`<line>` is the line number in the NEW (post-change,
right-hand) file** — the number you'd see in the file *after* the PR is merged. This
is the single most common mistake: a unified diff carries two independent line
counters (old/left from the `@@ -old` side, new/right from the `+new` side), and a
bare number anchored to the wrong counter lands the comment on the wrong line.

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --inline src/server.go:142 \
  --content "[[AI]](https://angelmsger.github.io/bitbucket-cli/) An empty request reaches items[0] and panics; handle empty input before indexing."
```

**Get the number right — read it, don't count it.** Use the line-numbered diff and
copy the value from the **new** (right) gutter. Each line is prefixed with two
right-aligned columns, `old` then `new` (blank where the line is absent on that
side), followed by the original diff line:

```sh
bitbucket-cli pr diff myws/myrepo/42 --path src/server.go --line-numbers
#    old    new   (the diff line follows, with its +/-/space prefix intact)
   141    141  	items := req.Items
          142 +	first := items[0]     ← new-file line 142 (an added line)
   142    143  	return handler(first)
```

Or skip the gutter entirely and ask for the commentable lines directly — this lists,
per file, exactly which new-side and old-side numbers accept an inline comment:

```sh
bitbucket-cli pr diff myws/myrepo/42 --path src/server.go --commentable
# {"path":"src/server.go","new_side":"258-263","old_side":"258-260, 262"}
```

The CLI then **resolves the anchor against that file's diff** and validates it:

- It classifies the line as added / removed / context and sends the correct anchor
  shape for both Cloud (`inline.to` / `inline.from`) and Data Center
  (`fileType` + `lineType`) — you don't pick those. This works whether the server
  returns a unified-diff text or a Data Center JSON hunk model; you never see or
  handle that difference.
- If the number isn't a commentable line on that side, it **fails with the
  commentable ranges** instead of silently mis-placing the comment, e.g.
  `line 261 is not part of the diff for src/server.go on the new side; commentable
  new-side lines: 258-264`. Re-read with `--line-numbers` (or `--commentable`) and
  correct the number.

**When inline anchoring fails — don't keep probing.** Two failure codes need
different responses:

- `INLINE_LINE_NOT_IN_DIFF` with a non-empty `commentable … lines:` range — you used
  the wrong number. Re-read the diff and choose a relevant line in the range;
  do not select an unrelated line merely because it is accepted.
- `DIFF_PARSE_FAILED`, or `INLINE_LINE_NOT_IN_DIFF` whose hint says *no … lines are
  part of the diff* for a file you can plainly see changed — this is a CLI/server
  format incompatibility, **not** a bad number, and retrying other anchors will not
  help. Report the posting limitation to the user and retain the finding with
  its intended `path:line`. A tooling failure does not make a code-specific
  finding PR-wide; do not automatically turn it into a general comment. Apply
  the publication rules above before choosing another destination.

Before posting several inline comments, you can validate every anchor first by adding
`--dry-run` to each `comment add` — it resolves the anchor against the diff without
sending, so a bad or unsupported anchor surfaces before you post anything.

**Commenting on a removed/deleted line** — that line has no new-file number, so anchor
it on the old (left) side with `--side old` and the **old**-gutter number:

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --inline src/legacy.go:88 --side old \
  --content "[[AI]](https://angelmsger.github.io/bitbucket-cli/) Removing this retry makes transient connection resets abort the upload; retain retry handling."
```

`--side` defaults to `new`; you only set `old` for a line that exists solely on the
pre-change side. (`--dry-run` works here too, but note it now fetches the file diff to
resolve the anchor, so it needs network access — it previews the real, resolved
payload.)

## Replies

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --reply-to 9876 \
  --content "[[AI]](https://angelmsger.github.io/bitbucket-cli/) New evidence: the empty-input test reproduces the same panic through the batch endpoint."
```

**Check who you are replying to first.** If comment `9876` or the message you
are answering in that thread was written by a person,
the reply belongs to the human whose name it will carry: draft it, show them the
reviewer's point and your reasoning, and post only that reply once they approve it.
Threads opened by a bot or another agent (recognizable by the `[[AI]](…)` marker or
an app `author.type`) take a lighter path. See `replying-to-people.md`.

## Listing, editing, deleting

```sh
bitbucket-cli comment list   --pr myws/myrepo/42
bitbucket-cli comment update --pr myws/myrepo/42 9876 --content "edited"
bitbucket-cli comment delete --pr myws/myrepo/42 9876 --yes
bitbucket-cli comment delete --pr myws/myrepo/42 9876 9877 9878 --yes   # batch
```

`comment delete` accepts several IDs (or a single `-` to read newline-separated
IDs from stdin). With more than one it prints an `{items, has_more}` aggregate
with a per-comment `ok`/`error`, deletes every one even if some fail, and exits
non-zero on any failure. `--yes` / `--dry-run` cover the whole batch.

## Resolution & task status

Each listed comment carries two triage signals:

- `resolved` — `true` when the comment's thread has been marked resolved.
  Cloud derives it from the comment's `resolution` object; Data Center from
  `state == "RESOLVED"`.
- `task` — `true` for an actionable review task (Data Center `severity ==
  "BLOCKER"`). Cloud tasks live on a separate endpoint and are not surfaced yet,
  so `task` is currently DC-only.

Filter to just the comments that still need attention:

```sh
bitbucket-cli comment list --pr myws/myrepo/42 --unresolved   # drop resolved threads
bitbucket-cli comment list --pr myws/myrepo/42 --tasks        # only actionable tasks
bitbucket-cli comment list --pr myws/myrepo/42 --fields id,resolved,task,inline.path
```

The same `--unresolved` flag exists on `pr threads` and is the recommended entry
point for triaging *received* review feedback — see
`responding-to-review-comments.md`.

Set the resolution too, not just read it:

```sh
bitbucket-cli comment resolve   --pr myws/myrepo/42 9876            # mark resolved
bitbucket-cli comment resolve   --pr myws/myrepo/42 9876 --unresolve # reopen
```

On Cloud this hits the dedicated resolve endpoint; on Data Center it sets the
comment's `state`, which is also how a task (a BLOCKER-severity comment) is
completed or reopened. Both honor `--dry-run` and read-only mode.

> Note: this resolves comment *threads* (the `resolved` field). Bitbucket
> Cloud's separate task objects are still not covered.

## Cloud vs Data Center

Both flavors are supported. The CLI hides the inline anchor shape difference
(`inline.{path,from,to}` on Cloud, `anchor.{path,line,lineType,fileType}` on
DC): you pass `--inline <path>:<line>` (plus `--side` if needed) and the client
resolves the line against the file's diff, then emits the correct shape —
including Data Center's `lineType` (`ADDED` / `REMOVED` / `CONTEXT`), which it now
derives from the diff rather than always sending `CONTEXT`.
