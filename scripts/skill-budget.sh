#!/usr/bin/env bash
# Enforce the companion Skill's context budget. Every review loads SKILL.md plus
# several references, so unbounded prose growth is a regression even when the
# offline cases still pass. Budgets are in words (wc -w); raise one only with a
# recorded reason in test/skill-review/validation.md.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKILL="$ROOT/skills/bitbucket"

ENTRY_MAX=2200          # SKILL.md
REFERENCE_MAX=2400      # any single references/*.md
REVIEW_PATH_MAX=5600    # what a single-PR review loads before it publishes

# Files a single-PR review loads before publishing anything. Commenting and
# vote-command references load only when a write is actually authorized.
REVIEW_PATH=(
  SKILL.md
  references/reviewing-locally.md
  references/replying-to-people.md
  references/pr-permissions.md
)

status=0
words() { wc -w < "$SKILL/$1" | tr -d ' '; }
check() {
  local label=$1 actual=$2 max=$3
  if (( actual > max )); then
    echo "OVER  $label: $actual words > $max"
    status=1
  else
    echo "ok    $label: $actual words <= $max"
  fi
}

check SKILL.md "$(words SKILL.md)" "$ENTRY_MAX"
for f in "$SKILL"/references/*.md; do
  rel="references/$(basename "$f")"
  check "$rel" "$(words "$rel")" "$REFERENCE_MAX"
done

total=0
for f in "${REVIEW_PATH[@]}"; do
  total=$(( total + $(words "$f") ))
done
check "review path (${#REVIEW_PATH[@]} files)" "$total" "$REVIEW_PATH_MAX"

exit $status
