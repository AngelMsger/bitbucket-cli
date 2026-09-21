# PR workflow cases

Each case is an independent request to an agent using the supplied Bitbucket
Skill. Use only that Skill and the case inputs; do not read the expectations or
other repository files. No network access, remote writes, or code edits are
permitted. Describe any next command instead of executing it.

For review cases, the user asks: "Review this PR and report findings here."
Metadata, changed lines, contracts, and discussion below are verified inputs.
The shown changes are relative to the merge-base; omitted boilerplate and
unchanged tests are valid. There are no other changes or discussions unless
stated. Source and target commits have not moved. Report each case separately,
including findings, locations, and any further reads or actions needed.

## 1. PROJ/client/101

The PR replaces local page-offset calculation with the server's next cursor.
The following is the complete changed function, `paging.go:10` onward. The old
function used `start + len(page.Items)` and had no comments. The server contract
states that `NextPageStart` is authoritative, including for an empty page.

```go
func nextStart(page Page) int {
    // First we tried computing the next start from the current page length.
    // Then we introduced a special case for empty pages, but that still failed.
    // We experimented with incrementing start by the configured limit next.
    // That was also abandoned during this PR, so the helper below no longer
    // uses any of those strategies. This is the third revision of this logic.
    // Read the next page start from the response.
    // Return that next page start to the caller.
    return page.NextPageStart
}
```

## 2. PROJ/client/102

The PR adds these comments to existing correct code. The described contracts
are verified. No project convention prohibits explanatory comments.

```go
// ParseCursor accepts a decimal offset and returns an error for negative values.
func ParseCursor(raw string) (int, error) { /* unchanged, tested */ }

// Server 7.x can return an empty page with a next cursor; using the item count
// here would skip records. Keep the server cursor until 7.x support ends.
// See the supported-server matrix in docs/compatibility.md.
func nextStart(page Page) int { return page.NextPageStart }

// Find the first value >= target. The half-open interval [lo, hi) keeps the
// insertion point valid when every value is smaller than target.
func lowerBound(values []int, target int) int { /* unchanged, tested */ }
```

## 3. PROJ/labels/103

The PR adds `imports.py` and `bulk.py`. Both use the same label contract as
`labels.py`, which is already used by create, update, and search. All callers
use in-process Python strings. The shared function is importable without cycles
or extra dependencies. Limits and normalization rules must change together.

Existing `labels.py`:

```python
def normalize_labels(values):
    labels = []
    for value in values:
        label = value.strip().lower()
        if not label or len(label) > 30:
            raise ValueError("labels must contain 1 to 30 characters")
        if label not in labels:
            labels.append(label)
    return sorted(labels)
```

Both new files contain the following complete helper at line 1 and call it:

```python
def prepare_labels(values):
    labels = []
    for value in values:
        label = value.strip().lower()
        if not label or len(label) > 30:
            raise ValueError("labels must contain 1 to 30 characters")
        if label not in labels:
            labels.append(label)
    return sorted(labels)
```

## 4. PROJ/labels/104

The PR adds the second function below. Imported archive labels must preserve
case and order, including duplicates, to verify the original archive signature.
Archive labels and live labels have separate versioned limits. Tests cover both
contracts. These are their full implementations:

```python
# Existing labels.py
def normalize_labels(values):
    labels = []
    for value in values:
        label = value.strip().lower()
        if not label or len(label) > 30:
            raise ValueError("invalid label")
        if label not in labels:
            labels.append(label)
    return sorted(labels)

# New archive.py
def read_archive_labels(values):
    labels = []
    for label in values:
        if not label or len(label) > 80:
            raise ValueError("invalid archive label")
        labels.append(label)
    return labels
```

## 5. PROJ/reports/105

The PR adds CSV export. The accepted scope specifies one fixed CSV exporter;
format selection and plugins are out of scope. The only caller is
`render_report(rows)`. No external extension, test replacement, or lifecycle
boundary uses the new types. `CsvExporter` is correct and tested. The new
`reports.py:1` onward contains:

```python
class ExporterRegistry:
    def __init__(self):
        self.factories = {}

    def register(self, name, factory):
        self.factories[name] = factory

    def resolve(self, name):
        return self.factories[name]()

class ExportPlan:
    def __init__(self, name, rows):
        self.name, self.rows = name, rows

class ExportExecutor:
    def __init__(self, registry):
        self.registry = registry

    def execute(self, plan):
        return self.registry.resolve(plan.name).render(plan.rows)

def render_report(rows):
    registry = ExporterRegistry()
    registry.register("csv", CsvExporter)
    return ExportExecutor(registry).execute(ExportPlan("csv", rows))
```

## 6. PROJ/reports/106

The PR makes report creation testable without contacting the production server.
`ServerClient` is the only production implementation of the new `ReportSource`.
`test_reports.py` supplies a fake that implements the same method and verifies
normalization and error propagation through `render_report`. There is no
registry, reflection, or runtime selection.

```python
from typing import Protocol

class ReportSource(Protocol):
    def fetch(self) -> list[dict]: ...

def render_report(source: ReportSource):
    return encode_csv(normalize_rows(source.fetch()))
```

## 7. PROJ/labels/107

The PR changes `normalize_labels` to trim and lowercase labels, remove
duplicates, and sort them. Its implementation is correct. The only new test,
`test_labels.py:1` onward, is:

```python
from unittest.mock import Mock

def test_normalize_labels():
    normalize = Mock(return_value=["blue", "red"])
    assert normalize([" Red ", "BLUE", "red"]) == ["blue", "red"]
```

## 8. PROJ/reports/108

This PR changes error handling in `reports.py`. The contract requires reporting
fetch failures to the caller. `fetch_rows()` raises `PermissionError` when access
is revoked. The only changed lines are the `try`/`except` below; previously the
function returned `encode_csv(fetch_rows())` and propagated failures.

```python
def render_report():
    try:
        rows = fetch_rows()
    except Exception:
        rows = []
    return encode_csv(rows)
```

The unchanged `legacy.py` contains a verbose 20-line development diary. An
existing resolved thread already discussed duplication in an unchanged CSV
helper and links a follow-up issue. No new evidence affects that discussion.

## 9. PROJ/plugins/109

The PR adds `AuditFactory`, implementing an existing plugin interface. The
provided diff contains no caller. Plugin registration, loading configuration,
and third-party consumers are not available in the inputs. The PR description
only says "Add audit export integration." The added factory is:

```python
class AuditFactory(PluginFactory):
    def create(self, config):
        return AuditExporter(config)
```

## 10. PROJ/client creation

User request: "Create the PR for the pushed cursor fix."

The repository, source branch, and destination are verified; no existing PR
matches. The source is pushed and all required repository checks have passed.
The agent has already read the full diff while preparing the description. That
diff and the server contract are exactly the inputs in case 1. The title and
description can be drafted from those inputs. For this offline exercise,
describe the next actions and give the user-facing message instead of writing.

## 11. PROJ/client creation

User request: "Create the PR for the pushed cursor fix."

The repository, source branch, destination, and absence of an existing PR are
verified. The source is pushed and all required repository checks have passed.
The already-read diff changes `return start + len(page.Items)` to
`return page.NextPageStart`, with this comment:

```go
// Empty pages can still have a next cursor; follow the server's cursor.
```

The server contract verifies that behavior. For this offline exercise, describe
the next actions and give the user-facing message instead of writing.
