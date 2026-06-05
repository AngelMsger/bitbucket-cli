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

Pass `--inline <path>:<line>`. The line refers to the destination side of the
diff (the "to" file).

```sh
bitbucket-cli comment add --pr myws/myrepo/42 \
  --inline src/server.go:142 \
  --content "Can we hoist this allocation out of the loop?"
```

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
DC) — pass `--inline <path>:<line>` and let the client translate.
