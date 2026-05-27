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

## Cloud vs Data Center

Both flavors are supported. The CLI hides the inline anchor shape difference
(`inline.{path,from,to}` on Cloud, `anchor.{path,line,lineType,fileType}` on
DC) — pass `--inline <path>:<line>` and let the client translate.
