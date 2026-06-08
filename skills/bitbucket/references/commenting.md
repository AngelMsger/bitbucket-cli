# Posting PR comments

The `comment` subtree writes pull-request comments. All commands target a PR
identified by `--pr <workspace>/<repo>/<id>` or a PR URL.

## General comments

```sh
bitbucket-cli comment add --pr myws/myrepo/42 --content "LGTM 🎉"
```

Or read the body from a file:

```sh
bitbucket-cli comment add --pr myws/myrepo/42 --content-file review.md
```

## AI attribution (agent writes)

When you post a comment **on the user's behalf as an AI agent**, prefix the content
with a link back to the tool. Comment bodies are Markdown, so a `[AI](url)` link works
directly — apply it to general, inline, and reply comments alike:

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --inline src/server.go:142 \
  --content "[AI](https://angelmsger.github.io/bitbucket-cli/) 这里的分配可以提到循环外。"
```

So a review note that would have read `XXX 有 YYY 问题` becomes
`[AI](https://angelmsger.github.io/bitbucket-cli/) XXX 有 YYY 问题`. Write the note
itself in the **user's language**; keep the `AI` label and the URL constant. This is
skill-level guidance for agents, not a fixed CLI behaviour.

## Inline (line-anchored) comments

Pass `--inline <path>:<line>`. **`<line>` is the line number in the NEW (post-change,
right-hand) file** — the number you'd see in the file *after* the PR is merged. This
is the single most common mistake: a unified diff carries two independent line
counters (old/left from the `@@ -old` side, new/right from the `+new` side), and a
bare number anchored to the wrong counter lands the comment on the wrong line.

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --inline src/server.go:142 \
  --content "Can we hoist this allocation out of the loop?"
```

**Get the number right — read it, don't count it.** Use the line-numbered diff and
copy the value from the **new** (right) gutter. Each line is prefixed with two
right-aligned columns, `old` then `new` (blank where the line is absent on that
side), followed by the original diff line:

```sh
bitbucket-cli pr diff myws/myrepo/42 --path src/server.go --line-numbers
#    old    new   (the diff line follows, with its +/-/space prefix intact)
   141    141  	ctx := r.Context()
          142 +	buf := make([]byte, 0, 1024)     ← new-file line 142 (an added line)
   141    142  	return handler(ctx)
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
  the wrong number. Pick one from the listed range and retry.
- `DIFF_PARSE_FAILED`, or `INLINE_LINE_NOT_IN_DIFF` whose hint says *no … lines are
  part of the diff* for a file you can plainly see changed — this is a CLI/server
  format incompatibility, **not** a bad number, and retrying other anchors will not
  help. Fall back to a general comment that names the location in its body, e.g.
  `comment add --pr … --content "src/server.go:900 — this allocation can be hoisted"`.
  Don't switch styles mid-review: if one file forces the fallback, it's fine to keep
  inline anchors on the files that accept them.

Before posting several inline comments, you can validate every anchor first by adding
`--dry-run` to each `comment add` — it resolves the anchor against the diff without
sending, so a bad or unsupported anchor surfaces before you post anything.

**Commenting on a removed/deleted line** — that line has no new-file number, so anchor
it on the old (left) side with `--side old` and the **old**-gutter number:

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --inline src/legacy.go:88 --side old \
  --content "This deletion drops the retry — intended?"
```

`--side` defaults to `new`; you only set `old` for a line that exists solely on the
pre-change side. (`--dry-run` works here too, but note it now fetches the file diff to
resolve the anchor, so it needs network access — it previews the real, resolved
payload.)

## Replies

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --reply-to 9876 \
  --content "Yep, opening a follow-up PR."
```

## Listing, editing, deleting

```sh
bitbucket-cli comment list   --pr myws/myrepo/42
bitbucket-cli comment update --pr myws/myrepo/42 9876 --content "edited"
bitbucket-cli comment delete --pr myws/myrepo/42 9876 --yes
```

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

> Note: the CLI reads resolution status but does not yet *set* it. Marking a
> thread resolved/unresolved remains a manual action in the Bitbucket UI.

## Cloud vs Data Center

Both flavors are supported. The CLI hides the inline anchor shape difference
(`inline.{path,from,to}` on Cloud, `anchor.{path,line,lineType,fileType}` on
DC): you pass `--inline <path>:<line>` (plus `--side` if needed) and the client
resolves the line against the file's diff, then emits the correct shape —
including Data Center's `lineType` (`ADDED` / `REMOVED` / `CONTEXT`), which it now
derives from the diff rather than always sending `CONTEXT`.
