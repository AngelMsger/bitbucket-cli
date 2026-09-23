"""Verify that a real CLI process can resume NDJSON without polluting rows."""

import json
import os
import subprocess
import sys


cli = sys.argv[1:]
env = dict(os.environ, BITBUCKET_CLI_NO_UPDATE_NOTIFIER="1", BITBUCKET_CLI_NO_SKILL_HINT="1")
command = ["commit", "compare", "--repo", "PROJ/demo", "--from", "main", "--to", "dev"]


def run(*args):
    result = subprocess.run(cli + command + list(args), env=env, capture_output=True, text=True, check=True, timeout=30)
    notices = [json.loads(line) for line in result.stderr.splitlines()]
    pagination = [notice["_notice"] for notice in notices if "pagination" in notice.get("_notice", {})]
    return result.stdout, pagination


raw, notices = run("--format", "json")
first = json.loads(raw)
assert not notices and first["has_more"] and first["next"] == "37"

raw, notices = run("--format", "ndjson", "--fields", "hash")
rows = [json.loads(line) for line in raw.splitlines()]
assert rows == [{"hash": item["hash"]} for item in first["items"]]
assert len(notices) == 1 and notices[0]["pagination"] == {"next": first["next"], "has_more": True}
assert notices[0]["next_steps"] == ["Pass next as --cursor to retrieve the next page."]

raw, notices = run("--format", "ndjson", "--fields", "hash", "--cursor", notices[0]["pagination"]["next"])
remaining = [json.loads(line) for line in raw.splitlines()]
assert remaining == [{"hash": "cccc333"}] and not notices

raw, notices = run("--format", "ndjson", "--fields", "hash", "--all")
assert [json.loads(line) for line in raw.splitlines()] == rows + remaining
assert not notices
