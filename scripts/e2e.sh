#!/usr/bin/env bash
# End-to-end test for bitbucket-cli.
#
# Default mode: start an in-repo mock Bitbucket server and run the CLI against
# it, asserting output and exit codes.
#
# Live mode (BITBUCKET_E2E_LIVE=1): additionally run READ-ONLY commands against
# the real server configured in .env. No write commands are issued.
set -uo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
BIN="$ROOT/bin/bitbucket-cli"

PASS=0
FAIL=0
pass() { echo "  PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL: $1"; FAIL=$((FAIL + 1)); }

# assert_ok <description> <command...>
assert_ok() {
  local desc="$1"; shift
  if out="$("$@" 2>/dev/null)"; then
    pass "$desc"
  else
    fail "$desc (exit $?)"
  fi
}

# assert_contains <description> <needle> <command...>
assert_contains() {
  local desc="$1" needle="$2"; shift 2
  out="$("$@" 2>/dev/null)"
  if [[ "$out" == *"$needle"* ]]; then
    pass "$desc"
  else
    fail "$desc (output did not contain '$needle')"
  fi
}

# assert_err_contains <description> <needle> <command...>  (captures stderr too)
assert_err_contains() {
  local desc="$1" needle="$2"; shift 2
  out="$("$@" 2>&1)"
  if [[ "$out" == *"$needle"* ]]; then
    pass "$desc"
  else
    fail "$desc (combined output did not contain '$needle')"
  fi
}

# assert_exit <description> <expected-code> <command...>
assert_exit() {
  local desc="$1" want="$2"; shift 2
  "$@" >/dev/null 2>&1
  local got=$?
  if [[ "$got" -eq "$want" ]]; then
    pass "$desc"
  else
    fail "$desc (exit $got, want $want)"
  fi
}

echo "==> building bitbucket-cli"
# Pin a release-like version so the update check exercises real comparison.
LDFLAGS="-X github.com/angelmsger/bitbucket-cli/pkg/constants.Version=0.0.1"
go build -ldflags "$LDFLAGS" -o "$BIN" ./cmd/bitbucket-cli || { echo "build failed"; exit 1; }

echo "==> starting mock Bitbucket server"
MOCK_LOG="$(mktemp)"
go run ./test/mockserver >"$MOCK_LOG" 2>/dev/null &
MOCK_PID=$!
trap 'kill "$MOCK_PID" 2>/dev/null' EXIT

MOCK_URL=""
for _ in $(seq 1 50); do
  MOCK_URL="$(head -n1 "$MOCK_LOG" 2>/dev/null)"
  [[ -n "$MOCK_URL" ]] && break
  sleep 0.1
done
if [[ -z "$MOCK_URL" ]]; then
  echo "mock server did not start"; exit 1
fi
echo "    mock server at $MOCK_URL"

export BITBUCKET_SERVER="$MOCK_URL"
export BITBUCKET_FLAVOR="datacenter"
export BITBUCKET_PERSONAL_ACCESS_TOKEN="e2e-token"
export BITBUCKET_DEFAULT_WORKSPACE="PROJ"
# Point the release-update check at the mock server, not the real GitHub API.
export BITBUCKET_RELEASE_API="$MOCK_URL/releases/latest"
TMPCFG="$(mktemp -d)"
CLI=("$BIN" --config "$TMPCFG")

echo "==> mock e2e checks"
assert_contains  "version"                   "bitbucket-cli"  "${CLI[@]}" version
assert_contains  "doctor healthy"            '"healthy": true' "${CLI[@]}" doctor
assert_contains  "doctor reports update"     '"available": true' "${CLI[@]}" doctor
assert_contains  "doctor --no-update-check"  '"healthy": true' \
                                             "${CLI[@]}" doctor --no-update-check
assert_contains  "repo list"                 "demo"           "${CLI[@]}" repo list --workspace PROJ
assert_contains  "repo get"                  "demo"           "${CLI[@]}" repo get PROJ/demo
assert_contains  "repo clone-url https"      "bitbucket.example.com/scm" \
                                             "${CLI[@]}" repo clone-url PROJ/demo --protocol https
assert_contains  "pr list"                   "Add login flow" "${CLI[@]}" pr list --repo PROJ/demo
assert_contains  "pr get summary"            "Add login flow" "${CLI[@]}" pr get PROJ/demo/1
assert_contains  "pr get diff"               "@@ -1 +1 @@"    "${CLI[@]}" pr get PROJ/demo/1 --scope diff
assert_contains  "pr diff command"           "@@ -1 +1 @@"    "${CLI[@]}" pr diff PROJ/demo/1
assert_contains  "pr commits"                "aaaa111"        "${CLI[@]}" pr commits PROJ/demo/1
assert_contains  "pr activity"               "Looks good"     "${CLI[@]}" pr activity PROJ/demo/1
assert_contains  "comment list"              "Looks good"     "${CLI[@]}" comment list --pr PROJ/demo/1
assert_contains  "comment add"               "added"          "${CLI[@]}" comment add --pr PROJ/demo/1 --content "added"
assert_contains  "pr approve"                '"approved": true' "${CLI[@]}" pr approve PROJ/demo/1
assert_contains  "pr unapprove"              '"approved": false' "${CLI[@]}" pr unapprove PROJ/demo/1
assert_contains  "branch list"               "main"           "${CLI[@]}" branch list --repo PROJ/demo
assert_contains  "commit list"               "aaaa111"        "${CLI[@]}" commit list --repo PROJ/demo
assert_contains  "commit get"                "aaaa111"        "${CLI[@]}" commit get --repo PROJ/demo aaaa111
assert_contains  "pr create dry-run"         '"method": "POST"' \
                                             "${CLI[@]}" pr create --repo PROJ/demo --source feature/x --target main --title "X" --dry-run
# v0.2 — file browsing + PR review aggregation
assert_contains  "file list"                 "README.md"      "${CLI[@]}" file list PROJ/demo --ref main
assert_contains  "file get full"             "line 1"         "${CLI[@]}" file get PROJ/demo --ref main --path README.md
assert_contains  "file get --range"          "line 2"         "${CLI[@]}" file get PROJ/demo --ref main --path README.md --range 2:3
assert_contains  "file tree"                 "src/server.go"  "${CLI[@]}" file tree PROJ/demo --ref main
assert_contains  "pr files (diffstat)"       "src/server.go"  "${CLI[@]}" pr files PROJ/demo/1
assert_contains  "pr diff --path"            "+new"           "${CLI[@]}" pr diff PROJ/demo/1 --path src/server.go
assert_contains  "pr status mergeable=false" '"can_merge": false' "${CLI[@]}" pr status PROJ/demo/1
assert_contains  "pr status has builds"      "SUCCESSFUL"     "${CLI[@]}" pr status PROJ/demo/1
assert_contains  "pr threads"                "Looks good"     "${CLI[@]}" pr threads PROJ/demo/1
assert_contains  "pr fetch print-only"       "git fetch"      "${CLI[@]}" pr fetch PROJ/demo/1
assert_contains  "pr checkout print-only"    "git checkout"   "${CLI[@]}" pr checkout PROJ/demo/1
assert_contains  "pr inbox (DC dashboard)"   "Wire payment retry" "${CLI[@]}" pr inbox --role reviewer
assert_contains  "workspace list"            "PROJ"           "${CLI[@]}" workspace list
assert_contains  "workspace get"             "Demo project"   "${CLI[@]}" workspace get PROJ
assert_contains  "user list (DC global)"     "alice"          "${CLI[@]}" user list
assert_contains  "user get"                  "Alice"          "${CLI[@]}" user get alice
assert_contains  "tag list"                  "v1.2.3"         "${CLI[@]}" tag list --repo PROJ/demo
assert_contains  "tag get"                   "aaaa111"        "${CLI[@]}" tag get --repo PROJ/demo v1.2.3
# Hint surfaces workspace discovery when --workspace is missing.
assert_err_contains "repo list hint"         "workspace list" \
                                             env -u BITBUCKET_DEFAULT_WORKSPACE "${CLI[@]}" repo list
assert_contains  "fields projection"         '"id"'           "${CLI[@]}" pr get PROJ/demo/1 --fields id,title
SKILL_DIR="$(mktemp -d)"
assert_contains  "skill install"             '"installed"' \
                                             "${CLI[@]}" skill install --dir "$SKILL_DIR"
assert_contains  "skill uninstall"           '"removed"' \
                                             "${CLI[@]}" skill uninstall --dir "$SKILL_DIR"
assert_contains  "skill show"                "name: bitbucket" "${CLI[@]}" skill show
assert_exit      "missing PR -> 6"           6                "${CLI[@]}" pr get PROJ/demo/404
assert_exit      "bad flag -> 2"             2                "${CLI[@]}" pr get PROJ/demo/1 --bogus
assert_exit      "pr merge needs --yes -> 2" 2                "${CLI[@]}" pr merge PROJ/demo/1 </dev/null

# --dry-run additions for v0.3 (every mutating command must accept --dry-run).
assert_contains  "pr update --dry-run"       '"method": "PUT"' \
                                             "${CLI[@]}" pr update PROJ/demo/1 --title "new title" --dry-run
assert_contains  "pr approve --dry-run"      '"method": "POST"' \
                                             "${CLI[@]}" pr approve PROJ/demo/1 --dry-run
assert_contains  "pr unapprove --dry-run"    '"method": "DELETE"' \
                                             "${CLI[@]}" pr unapprove PROJ/demo/1 --dry-run
assert_contains  "pr decline --dry-run"      '"method": "POST"' \
                                             "${CLI[@]}" pr decline PROJ/demo/1 --dry-run
assert_contains  "pr merge --dry-run"        '"method": "POST"' \
                                             "${CLI[@]}" pr merge PROJ/demo/1 --dry-run
assert_contains  "comment add --dry-run"     '"method": "POST"' \
                                             "${CLI[@]}" comment add --pr PROJ/demo/1 --content "x" --dry-run
assert_contains  "branch create --dry-run"   '"method"'        \
                                             "${CLI[@]}" branch create feat-x --repo PROJ/demo --from-ref main --dry-run
assert_contains  "branch delete --dry-run"   '"method": "DELETE"' \
                                             "${CLI[@]}" branch delete feat-x --repo PROJ/demo --dry-run
assert_contains  "repo delete --dry-run"     '"method": "DELETE"' \
                                             "${CLI[@]}" repo delete PROJ/demo --dry-run

# Read-only mode: env BITBUCKET_CLI_READ_ONLY blocks writes; --allow-writes
# overrides it; --dry-run remains usable.
RO_ENV=(env BITBUCKET_CLI_READ_ONLY=1)
assert_err_contains "read-only blocks pr approve"    "READONLY_BLOCKED" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" pr approve PROJ/demo/1
assert_err_contains "read-only error names --allow-writes" "--allow-writes" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" pr approve PROJ/demo/1
assert_exit         "read-only exit category=permission -> 5" 5 \
                                                     "${RO_ENV[@]}" "${CLI[@]}" pr approve PROJ/demo/1
assert_contains     "read-only + --dry-run still previews" '"method": "POST"' \
                                                     "${RO_ENV[@]}" "${CLI[@]}" pr approve PROJ/demo/1 --dry-run
assert_contains     "--allow-writes overrides read-only"   '"approved": true' \
                                                     "${RO_ENV[@]}" "${CLI[@]}" --allow-writes pr approve PROJ/demo/1
assert_err_contains "read-only blocks pr fetch --exec"     "READONLY_BLOCKED" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" pr fetch PROJ/demo/1 --exec
assert_contains     "read-only allows pr fetch (print-only)" "git fetch" \
                                                     "${RO_ENV[@]}" "${CLI[@]}" pr fetch PROJ/demo/1

echo "==> multi-context checks"
TMPCFG2="$(mktemp -d)"
cat >"$TMPCFG2/config.yaml" <<EOF
current_context: default
contexts:
  - name: default
    server: $MOCK_URL
    flavor: datacenter
    auth: {scheme: pat}
  - name: alt
    server: $MOCK_URL
    flavor: datacenter
    auth: {scheme: pat}
defaults:
  format: json
EOF
CLI2=("$BIN" --config "$TMPCFG2")
assert_contains  "get-contexts lists default" "default"      "${CLI2[@]}" config get-contexts
assert_contains  "get-contexts lists alt"     "alt"          "${CLI2[@]}" config get-contexts
assert_ok        "use-context alt"                           "${CLI2[@]}" config use-context alt
assert_exit      "unknown context -> 3"       3              "${CLI2[@]}" --use-context ghost doctor
assert_contains  "--use-context selects ctx"  '"healthy": true' \
                                              "${CLI2[@]}" --use-context default doctor
assert_contains  "config show exposes context" '"context"'  "${CLI2[@]}" config show
assert_ok        "delete-context alt"                        "${CLI2[@]}" config delete-context alt
assert_exit      "delete last context -> 2"   2              "${CLI2[@]}" config delete-context default

if [[ "${BITBUCKET_E2E_LIVE:-0}" == "1" ]]; then
  echo "==> live read-only checks (real server from .env)"
  unset BITBUCKET_SERVER BITBUCKET_FLAVOR BITBUCKET_PERSONAL_ACCESS_TOKEN BITBUCKET_RELEASE_API
  LIVECLI=("$BIN" --config "$(mktemp -d)")
  assert_ok "live doctor"        "${LIVECLI[@]}" doctor
  assert_ok "live whoami"        "${LIVECLI[@]}" whoami
fi

echo
echo "==> e2e summary: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
