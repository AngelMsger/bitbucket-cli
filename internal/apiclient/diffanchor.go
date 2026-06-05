package apiclient

import (
	"fmt"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// Diff sides for inline comment anchors. "new" is the post-change (right / TO)
// file; "old" is the pre-change (left / FROM) file.
const (
	DiffSideNew = "new"
	DiffSideOld = "old"
)

// diffLine is one body line of a unified diff with its resolved old/new line
// numbers. A number is 0 when the line does not exist on that side (an added
// line has no old number; a removed line has no new number).
type diffLine struct {
	old  int
	new  int
	kind string // ADDED | REMOVED | CONTEXT
}

// parseUnifiedDiff walks a single-file unified diff and returns its body lines
// (added / removed / context) carrying their old and new line numbers. File and
// hunk headers seed the counters but are not emitted.
func parseUnifiedDiff(diff string) []diffLine {
	var out []diffLine
	oldNo, newNo := 0, 0
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			oldNo, newNo, inHunk = parseHunkHeader(line)
			continue
		case !inHunk:
			// File headers (diff --git, index, ---, +++, etc.) before the first hunk.
			continue
		case line == "":
			// Trailing artifact of splitting on the final newline (a real blank
			// context line is " ", not ""). Never a numbered body line.
			continue
		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" — metadata, no line number.
			continue
		case strings.HasPrefix(line, "+"):
			out = append(out, diffLine{new: newNo, kind: "ADDED"})
			newNo++
		case strings.HasPrefix(line, "-"):
			out = append(out, diffLine{old: oldNo, kind: "REMOVED"})
			oldNo++
		default:
			// Context line (starts with a space, or a bare empty line inside a hunk).
			out = append(out, diffLine{old: oldNo, new: newNo, kind: "CONTEXT"})
			oldNo++
			newNo++
		}
	}
	return out
}

// parseHunkHeader reads "@@ -oldStart[,n] +newStart[,m] @@ ...". On a malformed
// header it reports inHunk=false so subsequent lines are treated as headers.
func parseHunkHeader(line string) (oldStart, newStart int, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "@@" {
		return 0, 0, false
	}
	oldStart, ok1 := parseHunkRangeStart(fields[1], '-')
	newStart, ok2 := parseHunkRangeStart(fields[2], '+')
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return oldStart, newStart, true
}

// parseHunkRangeStart parses "-258,5" / "+258,6" / "-0,0" into the start line.
func parseHunkRangeStart(field string, sign byte) (int, bool) {
	if len(field) == 0 || field[0] != sign {
		return 0, false
	}
	body := field[1:]
	if i := strings.IndexByte(body, ','); i >= 0 {
		body = body[:i]
	}
	n, err := strconv.Atoi(body)
	if err != nil {
		return 0, false
	}
	return n, true
}

// ResolveInlineAnchor maps a (path, line, side) request against the unified diff
// text of that file into a fully-classified InlineAnchor ready to post on either
// flavor: From/To and FileType pick the side, LineType records ADDED/REMOVED/
// CONTEXT so Data Center anchors correctly. side is "new" (default when empty)
// or "old". It errors when line is not a commentable line on that side, listing
// the available ranges so the caller can correct the number.
func ResolveInlineAnchor(diff, path string, line int, side string) (*InlineAnchor, error) {
	if side == "" {
		side = DiffSideNew
	}
	if side != DiffSideNew && side != DiffSideOld {
		return nil, cerrors.New(cerrors.CategoryUsage, "BAD_SIDE",
			fmt.Sprintf("--side must be %q or %q, got %q", DiffSideNew, DiffSideOld, side))
	}

	var available []int
	for _, dl := range parseUnifiedDiff(diff) {
		num := dl.new
		if side == DiffSideOld {
			num = dl.old
		}
		if num == 0 {
			continue
		}
		available = append(available, num)
		if num != line {
			continue
		}
		a := &InlineAnchor{Path: path, Line: line, LineType: dl.kind}
		if side == DiffSideNew {
			a.To = line
			a.FileType = "TO"
		} else {
			a.From = line
			a.FileType = "FROM"
		}
		return a, nil
	}

	hint := fmt.Sprintf("commentable %s-side lines for %s: %s", side, path, formatRanges(available))
	if len(available) == 0 {
		hint = fmt.Sprintf("no %s-side lines are part of the diff for %s", side, path)
	}
	return nil, cerrors.New(cerrors.CategoryUsage, "INLINE_LINE_NOT_IN_DIFF",
		fmt.Sprintf("line %d is not part of the diff for %s on the %s side", line, path, side)).
		WithHint(hint + ". Use `pr diff --line-numbers --path " + path + "` to read the right number, or pass --side old for a removed line.")
}

// formatRanges compresses a list of line numbers into "a-b, c, d-e".
func formatRanges(nums []int) string {
	if len(nums) == 0 {
		return "(none)"
	}
	uniq := make([]int, 0, len(nums))
	seen := map[int]bool{}
	for _, n := range nums {
		if !seen[n] {
			seen[n] = true
			uniq = append(uniq, n)
		}
	}
	// parseUnifiedDiff emits side numbers in ascending order, but sort defensively.
	for i := 1; i < len(uniq); i++ {
		for j := i; j > 0 && uniq[j-1] > uniq[j]; j-- {
			uniq[j-1], uniq[j] = uniq[j], uniq[j-1]
		}
	}
	var parts []string
	start, prev := uniq[0], uniq[0]
	flush := func() {
		if start == prev {
			parts = append(parts, strconv.Itoa(start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for _, n := range uniq[1:] {
		if n == prev+1 {
			prev = n
			continue
		}
		flush()
		start, prev = n, n
	}
	flush()
	return strings.Join(parts, ", ")
}

// AnnotateDiffWithLineNumbers prefixes each unified-diff line with its old and
// new file line numbers (an "old new" gutter), leaving file and hunk headers
// intact. It lets an agent read the exact number to pass to `comment add
// --inline <path>:<line>` instead of counting hunk offsets by hand.
func AnnotateDiffWithLineNumbers(diff string) string {
	var b strings.Builder
	oldNo, newNo := 0, 0
	inHunk := false
	gutter := func(o, n int) string {
		os, ns := "", ""
		if o > 0 {
			os = strconv.Itoa(o)
		}
		if n > 0 {
			ns = strconv.Itoa(n)
		}
		return fmt.Sprintf("%6s %6s ", os, ns)
	}
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		// strings.Split leaves a trailing "" for a final newline; don't emit a
		// spurious gutter line for it.
		if i == len(lines)-1 && line == "" {
			break
		}
		switch {
		case strings.HasPrefix(line, "@@"):
			oldNo, newNo, inHunk = parseHunkHeader(line)
			b.WriteString(gutter(0, 0))
			b.WriteString(line)
		case !inHunk || strings.HasPrefix(line, "\\"):
			b.WriteString(gutter(0, 0))
			b.WriteString(line)
		case strings.HasPrefix(line, "+"):
			b.WriteString(gutter(0, newNo))
			b.WriteString(line)
			newNo++
		case strings.HasPrefix(line, "-"):
			b.WriteString(gutter(oldNo, 0))
			b.WriteString(line)
			oldNo++
		default:
			b.WriteString(gutter(oldNo, newNo))
			b.WriteString(line)
			oldNo++
			newNo++
		}
		b.WriteByte('\n')
	}
	return b.String()
}
