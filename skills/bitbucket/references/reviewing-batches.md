# Reviewing several or large pull requests

Use this reference when the request covers more than one PR, or one PR is too
large for a single pass. Every rule in [Reviewing a pull request](reviewing-locally.md)
still applies to each PR: separate findings, coverage record, verdict, and
publication decision per PR.

## Group by verified dependencies

Group PRs only by evidence: stacked source/target branches, shared commits, a
changed producer/consumer contract, or an explicit linked requirement. Same
author or a similar title is not a dependency. Review a stack in dependency
order against each PR's actual target (`pr get <ref> --scope full` shows
`destination.branch`), assess combined compatibility, and keep one verdict per
PR. A defect in one PR does not automatically block a related PR.

## Delegate when it pays

When delegation is available and allowed, assign independent PRs or dependency
groups to subagents. Keep a tightly coupled group with one owner; split a large
PR by coherent component or risk, with the coordinator owning cross-component
interactions. Avoid one agent per file and avoid several agents repeating the
full review. Without delegation, use the same grouping sequentially.

Give each worker:

- the PR refs, pinned `source.commit` / `destination.commit`, and merge-base;
- verified repository or worktree paths (`pr fetch` shares refs across
  worktrees, so the coordinator serializes fetches and workers use pinned
  commits and separate worktrees when tests or tools write files);
- the intended behavior, assigned coverage, and relevant discussion.

Workers return evidence, locations, checks run, uncertainty, and the coverage
they actually completed. They do not post comments, change votes, or switch
shared branches.

## Coordinate before publishing

The coordinator reconciles conflicting findings, checks dependency boundaries
and uncovered code, deduplicates issues across PRs, refreshes revisions and
reviewer states, and owns every publication. Agreement between agents is not
additional evidence. Report each PR's coverage, dependency implications,
verdict with its reason, and the action actually taken.
