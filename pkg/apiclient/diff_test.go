package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

// dcJSONDiff is Bitbucket Data Center's JSON hunk model for the same change the
// unified-diff `sampleDiff` describes: a hunk at old/new 258 with one removed
// line (old 260) and two added lines (new 260, 261). It reproduces the wire
// format that broke inline comments — note the `destination` line numbers that
// previously leaked through `pr diff --line-numbers` as raw JSON.
const dcJSONDiff = `{
  "diffs": [
    {
      "source": {"toString": "src/server.go"},
      "destination": {"toString": "src/server.go"},
      "hunks": [
        {
          "sourceLine": 258, "sourceSpan": 5,
          "destinationLine": 258, "destinationSpan": 6,
          "segments": [
            {"type": "CONTEXT", "lines": [
              {"source": 258, "destination": 258, "line": "\ta := 1"},
              {"source": 259, "destination": 259, "line": "\tb := 2"}
            ]},
            {"type": "REMOVED", "lines": [
              {"source": 260, "destination": 260, "line": "\told := 3"}
            ]},
            {"type": "ADDED", "lines": [
              {"source": 261, "destination": 260, "line": "\tnewx := 3"},
              {"source": 261, "destination": 261, "line": "\textra := 4"}
            ]},
            {"type": "CONTEXT", "lines": [
              {"source": 261, "destination": 262, "line": "\tc := 5"},
              {"source": 262, "destination": 263, "line": "\treturn nil"}
            ]}
          ]
        }
      ]
    }
  ]
}`

func dcClient(t *testing.T, handler http.Handler) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{
		Flavor:    FlavorDataCenter,
		BaseURL:   srv.URL,
		Transport: transport.New(transport.Options{}),
	})
}

func diffHandler(contentType, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diff") {
			http.NotFound(w, r)
			return
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write([]byte(body))
	})
}

// The headline regression: a Data Center instance that returns a JSON hunk model
// must still resolve an inline anchor (previously every line failed with
// "no new-side lines are part of the diff").
func TestGetPRFileDiffsResolvesJSONAnchor(t *testing.T) {
	c := dcClient(t, diffHandler("application/json;charset=UTF-8", dcJSONDiff))
	files, err := c.GetPRFileDiffs(context.Background(), RepoRef{Workspace: "PROJ", Slug: "repo"}, 1, "src/server.go")
	if err != nil {
		t.Fatalf("GetPRFileDiffs: %v", err)
	}
	fd := FindFileDiff(files, "src/server.go")
	if fd == nil {
		t.Fatal("file not found in parsed JSON diff")
	}

	// new-side added line 261 ("extra := 4")
	a, err := ResolveInlineAnchor(fd, 261, DiffSideNew)
	if err != nil {
		t.Fatalf("resolve new 261: %v", err)
	}
	if a.To != 261 || a.FileType != "TO" || a.LineType != "ADDED" {
		t.Errorf("new 261 -> to=%d fileType=%q lineType=%q, want 261/TO/ADDED", a.To, a.FileType, a.LineType)
	}

	// old-side removed line 260 ("old := 3")
	r, err := ResolveInlineAnchor(fd, 260, DiffSideOld)
	if err != nil {
		t.Fatalf("resolve old 260: %v", err)
	}
	if r.From != 260 || r.FileType != "FROM" || r.LineType != "REMOVED" {
		t.Errorf("old 260 -> from=%d fileType=%q lineType=%q, want 260/FROM/REMOVED", r.From, r.FileType, r.LineType)
	}

	// new-side context line 263 ("return nil")
	ctxLine, err := ResolveInlineAnchor(fd, 263, DiffSideNew)
	if err != nil {
		t.Fatalf("resolve new 263: %v", err)
	}
	if ctxLine.LineType != "CONTEXT" {
		t.Errorf("new 263 lineType = %q, want CONTEXT", ctxLine.LineType)
	}
}

// `pr diff` against a JSON-serving server must render readable unified-diff text,
// not leak raw JSON.
func TestGetPRDiffRendersJSONToText(t *testing.T) {
	c := dcClient(t, diffHandler("application/json", dcJSONDiff))
	out, err := c.GetPRDiff(context.Background(), RepoRef{Workspace: "PROJ", Slug: "repo"}, 1)
	if err != nil {
		t.Fatalf("GetPRDiff: %v", err)
	}
	if strings.Contains(out, "\"destination\"") || strings.Contains(out, "\"hunks\"") {
		t.Fatalf("rendered diff still contains raw JSON:\n%s", out)
	}
	for _, want := range []string{"--- a/src/server.go", "+++ b/src/server.go", "@@ -258,5 +258,6 @@", "-\told := 3", "+\tnewx := 3", "+\textra := 4"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered diff missing %q:\n%s", want, out)
		}
	}
	// And the rendered text must annotate cleanly, so `pr diff --line-numbers`
	// produces a real gutter even from a JSON source.
	ann := AnnotateDiffWithLineNumbers(out)
	if !strings.Contains(ann, "@@ -258,5 +258,6 @@") {
		t.Errorf("annotation dropped the hunk header:\n%s", ann)
	}
	if !strings.Contains(ann, "261 +\textra := 4") {
		t.Errorf("annotation missing new-line 261 gutter for the added line:\n%s", ann)
	}
}

// A JSON body the CLI cannot decode must surface as DIFF_PARSE_FAILED (a parse /
// tool-compatibility error), never as INLINE_LINE_NOT_IN_DIFF blaming the line.
func TestGetPRFileDiffsMalformedJSONFailsLoudly(t *testing.T) {
	c := dcClient(t, diffHandler("application/json", `{"diffs": [ this is not valid json `))
	_, err := c.GetPRFileDiffs(context.Background(), RepoRef{Workspace: "PROJ", Slug: "repo"}, 1, "src/server.go")
	if err == nil {
		t.Fatal("expected a parse error for malformed JSON")
	}
	if code := cerrors.AsCLIError(err).Code; code != "DIFF_PARSE_FAILED" {
		t.Errorf("code = %q, want DIFF_PARSE_FAILED", code)
	}
}

// The Cloud / text path must pass through unchanged.
func TestGetPRDiffTextPassthrough(t *testing.T) {
	text := "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n"
	c := dcClient(t, diffHandler("text/plain", text))
	out, err := c.GetPRDiff(context.Background(), RepoRef{Workspace: "PROJ", Slug: "repo"}, 1)
	if err != nil {
		t.Fatalf("GetPRDiff: %v", err)
	}
	if out != text {
		t.Errorf("text diff was altered:\ngot  %q\nwant %q", out, text)
	}
}

// Content-type may be missing or mislabeled; a body starting with '{' is sniffed
// as JSON since a unified diff never does.
func TestIsJSONDiffSniffsBody(t *testing.T) {
	cases := []struct {
		ct, body string
		want     bool
	}{
		{"application/json", "{}", true},
		{"", "  {\"diffs\":[]}", true},
		{"text/plain", "--- a/x\n+++ b/x\n", false},
		{"", "diff --git a/x b/x\n", false},
		{"application/json", "diff --git a/x b/x", true}, // header wins
	}
	for _, tc := range cases {
		if got := isJSONDiff(tc.ct, tc.body); got != tc.want {
			t.Errorf("isJSONDiff(%q, %q) = %v, want %v", tc.ct, tc.body, got, tc.want)
		}
	}
}

// A removed line whose content starts with "-- " (or an added line with "++ ")
// must be parsed as a body line, not mistaken for a --- / +++ file header.
func TestParseUnifiedDashDashContentLine(t *testing.T) {
	diff := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n--- removed flag\n+++ added flag\n"
	fd := parseUnifiedFile(diff, "x")
	lines := fd.allLines()
	if len(lines) != 2 {
		t.Fatalf("got %d body lines, want 2: %+v", len(lines), lines)
	}
	if lines[0].Kind != "REMOVED" || lines[0].Text != "-- removed flag" {
		t.Errorf("line 0 = %+v, want REMOVED '-- removed flag'", lines[0])
	}
	if lines[1].Kind != "ADDED" || lines[1].Text != "++ added flag" {
		t.Errorf("line 1 = %+v, want ADDED '++ added flag'", lines[1])
	}
}

func TestParseDCJSONDiffMapping(t *testing.T) {
	files, err := parseDCJSONDiff(dcJSONDiff)
	if err != nil {
		t.Fatalf("parseDCJSONDiff: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	lines := files[0].allLines()
	// Spot-check the two added lines carry only a new number, the removed line
	// only an old number, and a context line carries both.
	var added, removed, context int
	for _, ln := range lines {
		switch ln.Kind {
		case "ADDED":
			added++
			if ln.Old != 0 {
				t.Errorf("added line has old number %d, want 0", ln.Old)
			}
		case "REMOVED":
			removed++
			if ln.New != 0 {
				t.Errorf("removed line has new number %d, want 0", ln.New)
			}
		case "CONTEXT":
			context++
			if ln.Old == 0 || ln.New == 0 {
				t.Errorf("context line missing a number: old=%d new=%d", ln.Old, ln.New)
			}
		}
	}
	if added != 2 || removed != 1 || context != 4 {
		t.Errorf("counts added/removed/context = %d/%d/%d, want 2/1/4", added, removed, context)
	}
}

// JSON-sourced and text-sourced diffs of the same change must agree on the set
// of commentable lines — the model is the single source of truth.
func TestJSONAndTextAgreeOnCommentableLines(t *testing.T) {
	jsonFiles, err := parseDCJSONDiff(dcJSONDiff)
	if err != nil {
		t.Fatalf("parseDCJSONDiff: %v", err)
	}
	textFile := parseUnifiedFile(sampleDiff, "src/server.go")

	jsonNew := FormatLineRanges(CommentableLines(&jsonFiles[0], DiffSideNew))
	textNew := FormatLineRanges(CommentableLines(textFile, DiffSideNew))
	if jsonNew != textNew {
		t.Errorf("new-side commentable lines differ: json=%q text=%q", jsonNew, textNew)
	}
	if jsonNew != "258-263" {
		t.Errorf("new-side commentable = %q, want 258-263", jsonNew)
	}
}
