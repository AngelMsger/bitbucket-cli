# Browsing source files at any ref

The `file` subtree reads repository contents at a branch, tag, or commit hash
— without requiring a local clone.

## Quick map

| Command | Purpose |
|---|---|
| `bitbucket-cli file list <ws>/<repo> --ref <ref> --path <dir>` | one-level directory listing |
| `bitbucket-cli file tree <ws>/<repo> --ref <ref> --path <dir> [--depth N]` | recursive walk |
| `bitbucket-cli file get  <ws>/<repo> --ref <ref> --path <file> [--range L1:L2]` | read raw file (optionally a line range) |

`--ref` accepts any of:
- a branch name (`main`, `feature/login`)
- a tag (`v1.2.3`)
- a full or short commit hash

If `--ref` is omitted, the repository's default branch is used.

## Reading a single file

```sh
# read full file
bitbucket-cli file get myws/myrepo --ref main --path src/server.go

# only lines 88–120 (1-based, inclusive)
bitbucket-cli file get myws/myrepo --ref main --path src/server.go --range 88:120

# stream raw bytes to disk (useful for binaries / images)
bitbucket-cli file get myws/myrepo --ref main --path assets/logo.png --output logo.png

# raw bytes to stdout (no JSON envelope)
bitbucket-cli file get myws/myrepo --ref main --path src/server.go --output -
```

Without `--output`, `file get` returns a JSON envelope with `path`, `ref`,
`size`, `encoding` (`utf-8` or `binary`), and `content` (the body). Binary
files in JSON mode include the raw bytes verbatim in `content`; pass
`--output -` if you need clean bytes.

## Reading at a specific commit (e.g., to compare versions)

```sh
# the same file at two commits
bitbucket-cli file get myws/myrepo --ref <hash-before> --path src/server.go --output - > before.go
bitbucket-cli file get myws/myrepo --ref <hash-after>  --path src/server.go --output - > after.go
diff -u before.go after.go
```

## Walking a directory

```sh
# list immediate children
bitbucket-cli file list myws/myrepo --ref main --path src/

# recursive tree, depth-limited
bitbucket-cli file tree myws/myrepo --ref main --path src/ --depth 2
```

## Cloud vs Data Center

On Cloud, listings include rich metadata (file vs directory, size, last-touched
commit). On Data Center, the underlying `/files` endpoint returns only paths
— directories are inferred from depth and entries surface as `type=file`.
