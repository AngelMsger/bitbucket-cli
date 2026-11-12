# Git transport

Use `bitbucket-cli git -- ls-remote <clone-url>` to verify actual Git reading,
then `bitbucket-cli git -- clone --origin upstream <clone-url> <directory>`.
Fetch with `bitbucket-cli git -- -C <directory> fetch upstream`. The same entry
supports push when authorized and the token has write permission.

An SSH URL can remain in the manifest and checkout without a local SSH key:
the companion looks up the HTTPS endpoint and authenticates only that invocation.
Data Center PATs use Bearer; Cloud API/access tokens use their static Git username.
Never extract tokens or construct token-bearing URLs, shell arguments or Git config.
Use `--use-context` when multiple identities are configured. A token can read the
API while HTTPS Git is disabled or denied; ls-remote verifies the actual path.

Git arguments follow `--`. `--dry-run` prints a public plan. Read-only posture
blocks clone, fetch and push for matched Bitbucket services; ls-remote is a read.
Unmatched `--fallback` operations retain ordinary Git's behavior.
One explicit remote is required;
recursive submodules and multi-remote operations are unsupported. Git `-c` accepts
only `core.quotepath` and `protocol.version`; transport overrides are rejected. `--fallback`
permits normal Git only for an unmatched service or absent credential, never a
rejected login. Operational Git streams and exit statuses pass through unchanged.
Old companions need an upgrade before this command is available. An ordinary
Git invocation does not automatically acquire companion authentication.
