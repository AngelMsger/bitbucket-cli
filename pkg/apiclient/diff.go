package apiclient

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// This file holds the single structured diff model that both inline-anchor
// resolution and line-number annotation derive from. Bitbucket serves a PR diff
// in two wire formats — Cloud (and DC honoring `Accept: text/plain`) returns a
// unified-diff *text*; many Data Center instances return a *JSON* hunk model at
// the same endpoint regardless of Accept. The old pipeline only understood text,
// so against a JSON-serving DC every inline anchor failed with the misleading
// "no new-side lines are part of the diff" and `pr diff --line-numbers` printed
// raw JSON. We now parse whichever format the server actually returned into the
// neutral FileDiff model below (see parseDiff), keeping one source of truth.

// DiffLine is one body line of a file diff with its resolved old/new line
// numbers. A number is 0 when the line does not exist on that side (an added
// line has no old number; a removed line has no new number).
type DiffLine struct {
	Old  int    // old / source (pre-change) line number, 0 if not on the old side
	New  int    // new / destination (post-change) line number, 0 if not on the new side
	Kind string // ADDED | REMOVED | CONTEXT
	Text string // line content without the +/-/space prefix
}

// DiffHunk is one "@@ -old +new @@" hunk of a file diff.
type DiffHunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Section            string // the trailing "@@ ... @@ <section heading>" text, if any
	Lines              []DiffLine
}

// FileDiff is a single file's diff as a structured model. It is built from
// either a unified-diff text (parseUnifiedToFiles) or Bitbucket Data Center's
// JSON hunk model (parseDCJSONDiff), so downstream consumers never depend on the
// wire format the server chose.
type FileDiff struct {
	OldPath   string
	NewPath   string
	Hunks     []DiffHunk
	Binary    bool
	Truncated bool
}

// allLines flattens the file's hunks into their body lines in order.
func (f *FileDiff) allLines() []DiffLine {
	var out []DiffLine
	for _, h := range f.Hunks {
		out = append(out, h.Lines...)
	}
	return out
}

// FindFileDiff returns the FileDiff for path, matching on either side's path. A
// per-file fetch usually yields exactly one file, so a lone entry is returned
// regardless of path.
func FindFileDiff(files []FileDiff, path string) *FileDiff {
	if len(files) == 1 {
		return &files[0]
	}
	for i := range files {
		if files[i].NewPath == path || files[i].OldPath == path {
			return &files[i]
		}
	}
	return nil
}

// parseDiff turns a raw diff body into the structured model, choosing the parser
// by the response content type (falling back to sniffing the body, since a bare
// JSON object never begins a unified diff). A body that looks like JSON but does
// not decode into the expected hunk shape yields DIFF_PARSE_FAILED rather than a
// silently empty model — the difference between "this PR has no diff" and "the
// CLI could not read the diff the server sent".
func parseDiff(body, contentType string) ([]FileDiff, error) {
	if isJSONDiff(contentType, body) {
		files, err := parseDCJSONDiff(body)
		if err != nil {
			return nil, errDiffParse(contentType, body, err)
		}
		return files, nil
	}
	return parseUnifiedToFiles(body), nil
}

// isJSONDiff reports whether the diff body should be decoded as Bitbucket's JSON
// hunk model rather than unified-diff text.
func isJSONDiff(contentType, body string) bool {
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		return true
	}
	// Some DC deployments / proxies omit or mislabel the content type. A unified
	// diff never starts with '{'; a JSON hunk model always does.
	return strings.HasPrefix(strings.TrimSpace(body), "{")
}

// errDiffParse reports a diff body the CLI could not read as either unified-diff
// text or a JSON hunk model. It is deliberately a parse/tool error — not a usage
// error blaming the caller's line number — and carries a snippet so the cause is
// diagnosable.
func errDiffParse(contentType, body string, cause error) error {
	snippet := strings.TrimSpace(body)
	if len(snippet) > 200 {
		snippet = snippet[:200] + "…"
	}
	return cerrors.Wrap(cause, cerrors.CategoryParse, "DIFF_PARSE_FAILED",
		"the PR diff response was not in a recognized format (expected unified-diff text or a Bitbucket JSON hunk model)").
		WithHint(fmt.Sprintf("server returned Content-Type %q — this is a CLI/server compatibility issue, not a bad line number. "+
			"Fall back to a general PR comment (`comment add` without --inline) and reference the location as path:line in the body. First bytes: %q",
			contentType, snippet))
}

// --- Bitbucket Data Center JSON hunk model ---------------------------------

type dcDiffResponse struct {
	Diffs     []dcDiff `json:"diffs"`
	Truncated bool     `json:"truncated"`
}

type dcDiff struct {
	Source      *dcDiffPath `json:"source"`
	Destination *dcDiffPath `json:"destination"`
	Hunks       []dcHunk    `json:"hunks"`
	Binary      bool        `json:"binary"`
	Truncated   bool        `json:"truncated"`
}

type dcDiffPath struct {
	ToString string `json:"toString"`
}

type dcHunk struct {
	SourceLine      int         `json:"sourceLine"`
	SourceSpan      int         `json:"sourceSpan"`
	DestinationLine int         `json:"destinationLine"`
	DestinationSpan int         `json:"destinationSpan"`
	Segments        []dcSegment `json:"segments"`
}

type dcSegment struct {
	Type  string       `json:"type"` // ADDED | REMOVED | CONTEXT
	Lines []dcDiffJSON `json:"lines"`
}

type dcDiffJSON struct {
	Source      int    `json:"source"`
	Destination int    `json:"destination"`
	Line        string `json:"line"`
}

// parseDCJSONDiff maps Data Center's JSON diff into FileDiffs. The segment type
// is authoritative — it gives ADDED/REMOVED/CONTEXT directly, so the resolved
// anchor's LineType never falls back to the historical CONTEXT guess.
func parseDCJSONDiff(body string) ([]FileDiff, error) {
	var resp dcDiffResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, err
	}
	files := make([]FileDiff, 0, len(resp.Diffs))
	for _, d := range resp.Diffs {
		fd := FileDiff{
			OldPath:   pathOf(d.Source),
			NewPath:   pathOf(d.Destination),
			Binary:    d.Binary,
			Truncated: d.Truncated || resp.Truncated,
		}
		for _, h := range d.Hunks {
			hunk := DiffHunk{
				OldStart: h.SourceLine, OldCount: h.SourceSpan,
				NewStart: h.DestinationLine, NewCount: h.DestinationSpan,
			}
			for _, seg := range h.Segments {
				kind := strings.ToUpper(strings.TrimSpace(seg.Type))
				for _, ln := range seg.Lines {
					dl := DiffLine{Kind: kind, Text: ln.Line}
					switch kind {
					case "ADDED":
						dl.New = ln.Destination
					case "REMOVED":
						dl.Old = ln.Source
					default: // CONTEXT
						dl.Old = ln.Source
						dl.New = ln.Destination
					}
					hunk.Lines = append(hunk.Lines, dl)
				}
			}
			fd.Hunks = append(fd.Hunks, hunk)
		}
		files = append(files, fd)
	}
	return files, nil
}

func pathOf(p *dcDiffPath) string {
	if p == nil {
		return ""
	}
	return p.ToString
}

// --- Unified-diff text -> model --------------------------------------------

// parseUnifiedToFiles parses a (possibly multi-file) unified-diff text into the
// structured model.
func parseUnifiedToFiles(diff string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var hunk *DiffHunk
	oldNo, newNo := 0, 0

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			files = append(files, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			cur = &FileDiff{}
		// `---`/`+++` are file headers only outside a hunk body; once a hunk is
		// open a leading `-`/`+` is a removed/added line (whose content may itself
		// start with "-- "/"++ "), so let those fall through to the body cases.
		case hunk == nil && strings.HasPrefix(line, "--- "):
			if cur == nil {
				cur = &FileDiff{}
			}
			cur.OldPath = stripDiffPath(strings.TrimPrefix(line, "--- "))
		case hunk == nil && strings.HasPrefix(line, "+++ "):
			if cur == nil {
				cur = &FileDiff{}
			}
			cur.NewPath = stripDiffPath(strings.TrimPrefix(line, "+++ "))
		case strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch"):
			if cur != nil {
				cur.Binary = true
			}
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			if cur == nil {
				cur = &FileDiff{}
			}
			oldNo, newNo = parseHunkHeaderFull(line)
			hunk = &DiffHunk{OldStart: oldNo, NewStart: newNo, Section: hunkSection(line)}
		case hunk == nil:
			// Header noise (index, mode, similarity, etc.) before the first hunk.
			continue
		case line == "" || strings.HasPrefix(line, "\\"):
			// Trailing split artifact or "\ No newline at end of file" metadata.
			continue
		case strings.HasPrefix(line, "+"):
			hunk.Lines = append(hunk.Lines, DiffLine{New: newNo, Kind: "ADDED", Text: line[1:]})
			newNo++
		case strings.HasPrefix(line, "-"):
			hunk.Lines = append(hunk.Lines, DiffLine{Old: oldNo, Kind: "REMOVED", Text: line[1:]})
			oldNo++
		default:
			text := line
			if strings.HasPrefix(line, " ") {
				text = line[1:]
			}
			hunk.Lines = append(hunk.Lines, DiffLine{Old: oldNo, New: newNo, Kind: "CONTEXT", Text: text})
			oldNo++
			newNo++
		}
	}
	flushFile()
	return files
}

// parseUnifiedFile parses a single-file unified diff into one FileDiff, stamping
// path when the diff carried no +++/--- headers.
func parseUnifiedFile(diff, path string) *FileDiff {
	files := parseUnifiedToFiles(diff)
	if len(files) == 0 {
		return &FileDiff{NewPath: path, OldPath: path}
	}
	fd := &files[0]
	if fd.NewPath == "" {
		fd.NewPath = path
	}
	if fd.OldPath == "" {
		fd.OldPath = path
	}
	return fd
}

// stripDiffPath removes the a/ or b/ prefix from a --- / +++ header path and
// maps /dev/null to the empty string (added/deleted file).
func stripDiffPath(p string) string {
	p = strings.TrimSpace(p)
	// Drop a trailing tab + timestamp some diff producers append.
	if i := strings.IndexByte(p, '\t'); i >= 0 {
		p = p[:i]
	}
	if p == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

// parseHunkHeaderFull returns the old/new start lines from a hunk header,
// defaulting to (1,1) on a malformed header so body lines still get numbers.
func parseHunkHeaderFull(line string) (oldStart, newStart int) {
	o, n, ok := parseHunkHeader(line)
	if !ok {
		return 1, 1
	}
	return o, n
}

// hunkSection returns the section heading after the closing "@@" of a hunk
// header, e.g. "func handle() error {" — preserved for faithful rendering.
func hunkSection(line string) string {
	if i := strings.Index(line, "@@"); i >= 0 {
		rest := line[i+2:]
		if j := strings.Index(rest, "@@"); j >= 0 {
			return strings.TrimSpace(rest[j+2:])
		}
	}
	return ""
}

// --- model -> unified-diff text --------------------------------------------

// RenderUnifiedDiff renders the structured model back into a unified-diff text.
// It is used to present a JSON-sourced diff (Data Center) as the same readable
// text a Cloud diff arrives in, so `pr diff` and its --line-numbers annotation
// behave identically regardless of the wire format.
func RenderUnifiedDiff(files []FileDiff) string {
	var b strings.Builder
	for _, f := range files {
		old, new := f.OldPath, f.NewPath
		if old == "" {
			old = new
		}
		if new == "" {
			new = old
		}
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", old, new)
		if f.Binary {
			fmt.Fprintf(&b, "Binary files a/%s and b/%s differ\n", old, new)
			continue
		}
		if f.OldPath == "" {
			b.WriteString("--- /dev/null\n")
		} else {
			fmt.Fprintf(&b, "--- a/%s\n", f.OldPath)
		}
		if f.NewPath == "" {
			b.WriteString("+++ /dev/null\n")
		} else {
			fmt.Fprintf(&b, "+++ b/%s\n", f.NewPath)
		}
		for _, h := range f.Hunks {
			b.WriteString(renderHunkHeader(h))
			for _, ln := range h.Lines {
				switch ln.Kind {
				case "ADDED":
					b.WriteByte('+')
				case "REMOVED":
					b.WriteByte('-')
				default:
					b.WriteByte(' ')
				}
				b.WriteString(ln.Text)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func renderHunkHeader(h DiffHunk) string {
	oldC, newC := h.OldCount, h.NewCount
	if oldC == 0 {
		oldC = countLines(h.Lines, true)
	}
	if newC == 0 {
		newC = countLines(h.Lines, false)
	}
	header := fmt.Sprintf("@@ -%s +%s @@", rangeSpec(h.OldStart, oldC), rangeSpec(h.NewStart, newC))
	if h.Section != "" {
		header += " " + h.Section
	}
	return header + "\n"
}

func countLines(lines []DiffLine, old bool) int {
	n := 0
	for _, ln := range lines {
		if old && ln.Kind != "ADDED" {
			n++
		}
		if !old && ln.Kind != "REMOVED" {
			n++
		}
	}
	return n
}

func rangeSpec(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}
