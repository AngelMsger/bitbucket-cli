package apiclient

import (
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// sampleDiff: hunk starts at old 258 / new 258.
//
//	258 258  a := 1        (context)
//	259 259  b := 2        (context)
//	260   -  old := 3      (removed)  -> old 260
//	    260 +  newx := 3    (added)    -> new 260
//	    261 +  extra := 4   (added)    -> new 261
//	261 262  c := 5        (context)
//	262 263  return nil    (context)
const sampleDiff = `diff --git a/src/server.go b/src/server.go
index 1111111..2222222 100644
--- a/src/server.go
+++ b/src/server.go
@@ -258,5 +258,6 @@ func handle() error {
 	a := 1
 	b := 2
-	old := 3
+	newx := 3
+	extra := 4
 	c := 5
 	return nil
`

func TestResolveInlineAnchorNewSideAdded(t *testing.T) {
	a, err := ResolveInlineAnchor(sampleDiff, "src/server.go", 261, DiffSideNew)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.To != 261 || a.From != 0 {
		t.Errorf("from/to = %d/%d, want 0/261", a.From, a.To)
	}
	if a.FileType != "TO" || a.LineType != "ADDED" {
		t.Errorf("fileType/lineType = %q/%q, want TO/ADDED", a.FileType, a.LineType)
	}
}

func TestResolveInlineAnchorNewSideContext(t *testing.T) {
	a, err := ResolveInlineAnchor(sampleDiff, "src/server.go", 262, DiffSideNew)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.To != 262 || a.FileType != "TO" || a.LineType != "CONTEXT" {
		t.Errorf("got to=%d fileType=%q lineType=%q, want 262/TO/CONTEXT", a.To, a.FileType, a.LineType)
	}
}

func TestResolveInlineAnchorOldSideRemoved(t *testing.T) {
	a, err := ResolveInlineAnchor(sampleDiff, "src/server.go", 260, DiffSideOld)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.From != 260 || a.To != 0 {
		t.Errorf("from/to = %d/%d, want 260/0", a.From, a.To)
	}
	if a.FileType != "FROM" || a.LineType != "REMOVED" {
		t.Errorf("fileType/lineType = %q/%q, want FROM/REMOVED", a.FileType, a.LineType)
	}
}

func TestResolveInlineAnchorDefaultSideIsNew(t *testing.T) {
	a, err := ResolveInlineAnchor(sampleDiff, "src/server.go", 260, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// new-side line 260 is the first added line (newx).
	if a.To != 260 || a.LineType != "ADDED" {
		t.Errorf("got to=%d lineType=%q, want 260/ADDED", a.To, a.LineType)
	}
}

func TestResolveInlineAnchorLineNotInDiff(t *testing.T) {
	_, err := ResolveInlineAnchor(sampleDiff, "src/server.go", 999, DiffSideNew)
	if err == nil {
		t.Fatal("expected an error for a line outside the diff")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "INLINE_LINE_NOT_IN_DIFF" {
		t.Errorf("code = %q, want INLINE_LINE_NOT_IN_DIFF", ce.Code)
	}
	// new-side commentable lines are 258..263.
	if !strings.Contains(ce.Hint, "258-263") {
		t.Errorf("hint = %q, want it to list range 258-263", ce.Hint)
	}
}

func TestResolveInlineAnchorBadSide(t *testing.T) {
	_, err := ResolveInlineAnchor(sampleDiff, "src/server.go", 260, "sideways")
	if err == nil {
		t.Fatal("expected an error for an invalid side")
	}
	if cerrors.AsCLIError(err).Code != "BAD_SIDE" {
		t.Errorf("code = %q, want BAD_SIDE", cerrors.AsCLIError(err).Code)
	}
}

func TestAnnotateDiffWithLineNumbers(t *testing.T) {
	out := AnnotateDiffWithLineNumbers(sampleDiff)
	// The added "extra := 4" line should carry new-file number 261 and no old number.
	if !strings.Contains(out, "261 +") && !strings.Contains(out, "261 ") {
		t.Errorf("annotated output missing new line 261 for the added line:\n%s", out)
	}
	// The removed "old := 3" line should carry old-file number 260.
	if !strings.Contains(out, "260") {
		t.Errorf("annotated output missing old line 260 for the removed line:\n%s", out)
	}
	// Hunk header should survive.
	if !strings.Contains(out, "@@ -258,5 +258,6 @@") {
		t.Errorf("annotated output dropped the hunk header:\n%s", out)
	}
}

// Serialization: a resolved added-line anchor must produce TO-side payloads on
// both flavors (DC must NOT hardcode CONTEXT for an added line).
func TestBuildAddPRCommentResolvedAnchorPayloads(t *testing.T) {
	anchor := &InlineAnchor{Path: "src/server.go", Line: 261, To: 261, FileType: "TO", LineType: "ADDED"}
	req := AddPRCommentReq{Repo: RepoRef{Workspace: "ws", Slug: "repo"}, PRID: 42, Content: "x", Inline: anchor}

	cloud := &apiClient{flavor: FlavorCloud}
	_, _, body := cloud.buildAddPRComment(req)
	il := body.(map[string]any)["inline"].(map[string]any)
	if il["to"] != 261 {
		t.Errorf("cloud inline.to = %v, want 261", il["to"])
	}
	if _, hasFrom := il["from"]; hasFrom {
		t.Errorf("cloud inline must not set from for a new-side anchor: %v", il)
	}

	dc := &apiClient{flavor: FlavorDataCenter}
	_, _, body = dc.buildAddPRComment(req)
	an := body.(map[string]any)["anchor"].(map[string]any)
	if an["lineType"] != "ADDED" || an["fileType"] != "TO" || an["line"] != 261 {
		t.Errorf("dc anchor = %v, want lineType=ADDED fileType=TO line=261", an)
	}
}

func TestBuildAddPRCommentRemovedAnchorDC(t *testing.T) {
	anchor := &InlineAnchor{Path: "src/server.go", Line: 260, From: 260, FileType: "FROM", LineType: "REMOVED"}
	req := AddPRCommentReq{Repo: RepoRef{Workspace: "ws", Slug: "repo"}, PRID: 42, Content: "x", Inline: anchor}

	dc := &apiClient{flavor: FlavorDataCenter}
	_, _, body := dc.buildAddPRComment(req)
	an := body.(map[string]any)["anchor"].(map[string]any)
	if an["lineType"] != "REMOVED" || an["fileType"] != "FROM" || an["line"] != 260 {
		t.Errorf("dc anchor = %v, want lineType=REMOVED fileType=FROM line=260", an)
	}

	cloud := &apiClient{flavor: FlavorCloud}
	_, _, body = cloud.buildAddPRComment(req)
	il := body.(map[string]any)["inline"].(map[string]any)
	if il["from"] != 260 {
		t.Errorf("cloud inline.from = %v, want 260", il["from"])
	}
	if _, hasTo := il["to"]; hasTo {
		t.Errorf("cloud inline must not set to for an old-side anchor: %v", il)
	}
}
