# Safety modes — `--dry-run` and read-only

`bitbucket-cli` ships two orthogonal safety mechanisms for agents driving
Bitbucket on a user's behalf. Both protect against the most common failure
mode — an unintended remote mutation — but they answer different questions.

| | Question it answers | Scope |
|---|---|---|
| `--dry-run` | "What HTTP request *would* this command send?" | Per command |
| Read-only mode | "Block all writes for this session." | Per invocation / session |

## `--dry-run` — preview, never send

Every mutating command accepts `--dry-run`. It resolves the operation
through `Client.DescribeWrite(...)` and prints the equivalent HTTP request
as JSON, without sending it:

```bash
bitbucket-cli pr merge myws/myrepo/42 --dry-run
# {
#   "method": "POST",
#   "url": "https://api.bitbucket.org/2.0/repositories/myws/myrepo/pullrequests/42/merge",
#   "payload": { "merge_strategy": "merge_commit" }
# }
```

Use `--dry-run` before any destructive command (`pr merge`, `pr decline`,
`comment delete`, `branch delete`, `repo delete`) when you want to verify
exactly which URL and payload would be sent.

Available on: `pr create`, `pr update`, `pr approve`, `pr unapprove`,
`pr request-changes`, `pr decline`, `pr merge`, `comment add`,
`comment update`, `comment delete`, `branch create`, `branch delete`,
`repo create`, `repo delete`.

## Read-only mode — lock the session

A session-level switch that blocks every mutating client method (and
`pr fetch/checkout --exec`) before any HTTP request is sent. Enable it by
either:

- `defaults.read_only: true` in `~/.angelmsger/bitbucket/config.yaml` (the
  legacy `~/.bitbucket/config.yaml` location is still honored when only it
  exists), or
- `BITBUCKET_CLI_READ_ONLY=1` in the environment.

Blocked operations return a structured error:

```json
{
  "error": {
    "category": "permission",
    "code": "READONLY_BLOCKED",
    "message": "operation \"MergePR\" blocked: read-only mode is enabled",
    "next_steps": [
      "Add --allow-writes to the command line",
      "unset BITBUCKET_CLI_READ_ONLY",
      "Set defaults.read_only=false in ~/.angelmsger/bitbucket/config.yaml"
    ]
  }
}
```

Exit code: 5 (`permission`).

### Per-call override: `--allow-writes`

When you genuinely need to write under a read-only posture, add the
root-level `--allow-writes` flag:

```bash
BITBUCKET_CLI_READ_ONLY=1 bitbucket-cli --allow-writes pr approve myws/myrepo/42
```

This is the only way to flip the posture for one invocation without
changing config or env.

### What read-only does NOT block

CLI self-configuration is intentionally out of scope, otherwise an agent
that flipped on read-only would lose the ability to recover:

- `config init`, `auth login`, `auth logout`, `config use-context`
- `skill install`, `skill uninstall`
- `file get --output <path>` (read-only of remote content into a local file)

Read-only protects **the remote Bitbucket service and the user's git
worktree**, not `bitbucket-cli`'s own state.

## Recommended pattern for agents

When you receive a task that involves any mutation:

1. **Always run the operation with `--dry-run` first**, especially if the
   target resource (PR id, branch name, repo slug) was inferred and not
   pasted in literally. Confirm the URL ends with the expected resource.
2. If the user mentioned "read-only", "don't change anything", or "just
   review" — set `BITBUCKET_CLI_READ_ONLY=1` for the rest of the session.
   Then every read-and-summarize command works as normal, and any
   write you try by mistake hits `READONLY_BLOCKED` before reaching
   the server.
3. The two compose: `BITBUCKET_CLI_READ_ONLY=1 bitbucket-cli pr merge
   <ref> --dry-run` is fine and useful — it shows what the merge call
   would send without ever sending one.
