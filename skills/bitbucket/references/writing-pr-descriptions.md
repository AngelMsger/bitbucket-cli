# Writing PR descriptions

Read this before drafting or rewriting a PR description. Help a colleague who
has not seen the conversation understand why the change is needed, what behaves
differently, and where their judgment matters. Base claims on the final diff and
known context; do not invent motivation or benefits.

## Default shape

After the existing [AI attribution line](pr-workflows.md#ai-attribution), use:

1. **A short opening paragraph.** In one to three sentences, state the concrete
   problem or need and the resulting behavior. A separate "Summary" heading is
   unnecessary. For a refactor, explain the maintenance problem and what becomes
   easier to change; do not invent a user-visible behavior change.
2. **Review focus, only when useful.** Usually one to three bullets identifying
   a non-obvious tradeoff, boundary condition, compatibility change, or migration
   requirement. Say what deserves attention and why; include a code location
   when it helps the reviewer find the decision.

A small change can be just the opening paragraph. Expand complex changes only
as needed to explain their effects. There is no minimum word count and no set
of sections to fill. Do not add a validation section, test results, command
logs, or a routine "all checks passed" sentence by default. Checks belong to the
[creation workflow](pr-workflows.md#before-opening-the-pr).

## Write for the reviewer

- **Use concrete subjects and verbs.** Name the condition and what happens.
  Replace claims such as "improves robustness" with the behavior that improves.
- **Explain the change, not the work session.** Omit the conversation history,
  implementation diary, and abandoned approaches unless they explain an
  important tradeoff.
- **Do not narrate the diff.** Avoid file-by-file inventories, lists of function
  names, and call-chain tours. Include implementation details only when they
  explain the behavior or guide a review decision.
- **Make review guidance specific.** "Check correctness" applies to every PR.
  Name the actual boundary or choice instead. If there is no special review
  focus, omit the section rather than writing "None".
- **Cut filler.** Remove empty headings, repeated summaries, self-praise,
  unsupported "no risk" claims, and unrelated caveats. Use short paragraphs or
  flat bullets; do not turn a small change into a report or checklist.
- **Keep material effects visible.** Compatibility changes, migration steps,
  and known behavior limitations belong in the description. Do not bury them
  to save space. Link to existing design documents or related issues for
  background instead of copying them into the body.

## User and repository conventions

Follow explicit user instructions first, then the target repository's PR
template and language rules. Otherwise use the user's language, including the
review-focus label and attribution sentence. Keep technical identifiers exact.

If a template is required, put the useful information into its existing fields;
do not append a second template. Keep required fields, including verification
fields when explicitly required, but do not add optional boilerplate. The
default structure above applies when the repository has no required structure.

When updating an existing description, reflect the final change rather than
appending a chronological update log. Preserve useful context, issue links,
and human-authored text outside the requested edit; remove obsolete claims in
the part being updated. Do not rewrite unrelated text just to impose a style.
Follow the Skill's authorship rules: human-written text posted verbatim receives
no AI marker; an agent-authored description uses the existing attribution line
once, without stacking duplicates.

## Examples

These are illustrative English descriptions, not claims about a particular PR.
The first example includes attribution; the remaining excerpts omit that same
line for brevity. Translate naturally when the target description uses another
language.

### Small fix

```markdown
> [AI] This PR was created with the help of AI via [bitbucket-cli](https://angelmsger.github.io/bitbucket-cli/).

Repository names containing spaces produced broken browser links. Encode the
repository path so those links open the intended repository.
```

### A change with a tradeoff

```markdown
Search currently waits for every repository, so one slow server delays all
results. Return the available results after five seconds and identify any
repositories that did not respond.

**Review focus**

- Partial results are useful here, but callers must see which repositories are
  missing. Check that the timeout path preserves that information.
```

### A compatibility change

```markdown
Ambiguous timestamps currently use the machine's timezone, so identical queries
can cover different periods. Require an explicit UTC offset for timestamps.

**Review focus**

- Existing scripts passing timestamps without an offset must add `Z` or their
  intended offset. Keep the error message clear about that migration.
```

### Replace vague claims

| Instead of | Write the concrete effect |
| --- | --- |
| Enhance request-path robustness. | Retry a timed-out read once. |
| Optimize pagination handling. | Use the server's next-page cursor so records are not skipped. |
| Refactor A, B, and C for better maintainability. | Move cursor decoding into one helper so each endpoint follows the same pagination rule. |
| Please review correctness and edge cases. | Check that an empty intermediate page does not stop the query when a next-page cursor exists. |

## Before posting

Read the body as a colleague unfamiliar with the work. Can they tell why the
change matters and what changes without reconstructing the diff? Does each
review bullet identify a real decision or boundary? Remove repetitions and
routine check reports, retain material compatibility effects, and verify that
the text describes the final change rather than an earlier iteration.
