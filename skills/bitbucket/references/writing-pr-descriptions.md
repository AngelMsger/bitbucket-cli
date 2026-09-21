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
3. **A change outline, only when the shape is hard to see.** One compact
   structural sketch when a reviewer would otherwise have to reconstruct the
   design by opening several files — see [Change outline](#change-outline).

A small change can be just the opening paragraph. Expand complex changes only
as needed to explain their effects. There is no minimum word count and no set
of sections to fill. Do not add a validation section, test results, command
logs, or a routine "all checks passed" sentence by default. Checks belong to the
[creation workflow](pr-workflows.md#run-the-repositorys-checks).

## Change outline

Prose explains *why*; a sketch explains *shape*. When a change moves
responsibilities between files, alters a schema or an API contract, or reroutes
control flow, a few lines of structure carry more than a paragraph describing
them — and far less than the file-by-file inventory this guide otherwise
forbids.

**Use one when, and only when, the shape is the hard part.** A single-file fix,
a copy change, or a dependency bump never needs one; the opening paragraph
already says everything. Add an outline for a change whose parts a reviewer must
hold together at once. Pick the one view that answers the reviewer's real
question and stop — two views are the practical ceiling, and an outline longer
than the paragraph above it has stopped helping.

Useful views, in whatever order tells the story:

- A **shallow file tree** with responsibilities, for a move or a split.
- A **call, control-flow or data-flow tree**, for a reroute.
- A **schema or endpoint contract**, for a storage or API change.
- **Pseudocode** of the changed rule, for logic a diff obscures.
- A **key type or data structure**, when it anchors the rest.

Use a `diff` block when the surrounding shape already exists and the point is
what changed; show the complete target shape when most of it is new or when
diff markers would hide ownership and order. Sketches are illustrative — keep
identifiers exact, but do not paste real diff hunks. The reviewer has the diff.

### Example

````markdown
Pagination was decoded separately in each endpoint, so a cursor fix had to be
repeated four times and one endpoint kept skipping records. Move cursor decoding
into a single helper that every endpoint calls.

**Change outline**

```diff
 internal/api/
-├── repos.go        # decodes its own cursor
-├── prs.go          # decodes its own cursor
-└── comments.go     # decodes its own cursor
+├── paging/
+│   └── cursor.go   # owns cursor decoding for every endpoint
+├── repos.go
+├── prs.go
+└── comments.go
```

**Review focus**

- The helper treats an empty intermediate page as "continue" rather than "end".
  Check that against the endpoint that previously stopped early.
````

## Write for the reviewer

- **Use concrete subjects and verbs.** Name the condition and what happens.
  Replace claims such as "improves robustness" with the behavior that improves.
- **Explain the change, not the work session.** Omit the conversation history,
  implementation diary, and abandoned approaches unless they explain an
  important final tradeoff. Use the review guide's
  [information-placement rules](reviewing-locally.md#place-information-for-its-reader)
  when deciding what belongs in comments, the PR, or a design document. Removing
  a diary from code does not make it useful PR context.
- **Do not narrate the diff.** Avoid file-by-file inventories, lists of function
  names, and call-chain tours. Include implementation details only when they
  explain the behavior or guide a review decision. A
  [change outline](#change-outline) is not an exception to this: it shows the
  resulting structure, not a walk through the changes.
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
the text describes the final change rather than an earlier iteration. If a
change outline restates what the paragraph already said, delete the outline.
