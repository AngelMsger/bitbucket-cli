package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestListPRCommentsDataCenterReplies guards the Data Center reply tree. DC does
// not emit replies as their own activity entries — it embeds a comment's replies
// in its nested "comments" array — so the parser must walk that tree, otherwise
// `comment add --reply-to` writes a reply that no read command can ever find.
func TestListPRCommentsDataCenterReplies(t *testing.T) {
	c := newResolveTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/activities") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"isLastPage": true,
			"values": []any{
				map[string]any{
					"action":        "COMMENTED",
					"commentAction": "ADDED",
					"comment": map[string]any{
						"id":    2642135,
						"text":  "root review comment",
						"state": "OPEN",
						"anchor": map[string]any{
							"path": "packages/core/src/cache/LazyValueProvider.ts", "line": 137,
							"fileType": "TO", "lineType": "CONTEXT",
						},
						"comments": []any{
							map[string]any{
								"id":    2647711,
								"text":  "first reply",
								"state": "OPEN",
								"comments": []any{
									map[string]any{"id": 2648130, "text": "nested reply", "state": "OPEN"},
								},
							},
						},
					},
				},
			},
		})
	}))

	repo := RepoRef{Workspace: "FX", Slug: "fx-corp"}

	res, err := c.ListPRComments(context.Background(), ListPRCommentsOpts{Repo: repo, PRID: 1952})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]Comment{}
	for _, cm := range res.Items {
		got[cm.ID] = cm
	}
	if len(got) != 3 {
		t.Fatalf("expected root + 2 replies = 3 comments, got %d: %+v", len(got), res.Items)
	}
	if r, ok := got[2647711]; !ok || r.ParentID != 2642135 {
		t.Errorf("reply 2647711 should have ParentID 2642135, got %+v (present=%v)", r, ok)
	}
	if r, ok := got[2648130]; !ok || r.ParentID != 2647711 {
		t.Errorf("nested reply 2648130 should have ParentID 2647711, got %+v (present=%v)", r, ok)
	}

	threads, err := c.ListPRThreads(context.Background(), repo, 1952)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads.Items) != 1 {
		t.Fatalf("expected a single thread, got %d", len(threads.Items))
	}
	inThread := map[int]bool{}
	for _, cm := range threads.Items[0].Comments {
		inThread[cm.ID] = true
	}
	for _, id := range []int{2642135, 2647711, 2648130} {
		if !inThread[id] {
			t.Errorf("thread should contain comment %d (it is on the PR), but it is missing", id)
		}
	}
}

// TestDataCenterActivityAnchorAndThreadScope mirrors the real-world PR that
// exposed two activities-stream bugs:
//
//  1. Anchor placement — DC hoists an inline comment's anchor to the activity
//     (activity.commentAnchor), not into the comment, so an inline comment read
//     back from /activities must still come out anchored.
//  2. Thread scope — each top-level comment is its own thread; independent
//     general comments must not be merged into one bucket, so `--comment <id>`
//     (threadHasComment) returns just that root and its replies.
func TestDataCenterActivityAnchorAndThreadScope(t *testing.T) {
	c := newResolveTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"isLastPage": true,
			"values": []any{
				// General root with two replies nested under it.
				map[string]any{"action": "COMMENTED", "comment": map[string]any{
					"id": 2642135, "text": "AI suggestion", "state": "OPEN",
					"comments": []any{
						map[string]any{"id": 2645582, "text": "won't fix", "state": "OPEN"},
						map[string]any{"id": 2647711, "text": "follow-up", "state": "OPEN"},
					},
				}},
				// Inline comment: anchor is on the ACTIVITY, not the comment.
				map[string]any{
					"action": "COMMENTED",
					"comment": map[string]any{
						"id": 2646853, "text": "perf test is flaky", "state": "OPEN",
					},
					"commentAnchor": map[string]any{
						"path": "packages/core/tests/cache/SingleFlight.perf.test.ts",
						"line": 17, "fileType": "TO", "lineType": "ADDED",
					},
				},
				// A second, independent general comment.
				map[string]any{"action": "COMMENTED", "comment": map[string]any{
					"id": 2647783, "text": "decline and fix", "state": "OPEN",
				}},
			},
		})
	}))

	repo := RepoRef{Workspace: "FX", Slug: "fx-corp"}

	// Bug B: the inline comment read from /activities must be anchored.
	res, err := c.ListPRComments(context.Background(), ListPRCommentsOpts{Repo: repo, PRID: 1952})
	if err != nil {
		t.Fatal(err)
	}
	var inline *Comment
	for i := range res.Items {
		if res.Items[i].ID == 2646853 {
			inline = &res.Items[i]
		}
	}
	if inline == nil || inline.Inline == nil {
		t.Fatalf("comment 2646853 should be anchored (anchor lives on activity.commentAnchor); got %+v", inline)
	}
	if inline.Inline.Path != "packages/core/tests/cache/SingleFlight.perf.test.ts" || inline.Inline.Line != 17 {
		t.Errorf("anchor mismatch: %+v", inline.Inline)
	}

	// Bug A: --comment 2647711 must scope to its root thread only.
	threads, err := c.ListPRThreads(context.Background(), repo, 1952)
	if err != nil {
		t.Fatal(err)
	}
	var target *Thread
	for i := range threads.Items {
		for _, cm := range threads.Items[i].Comments {
			if cm.ID == 2647711 {
				target = &threads.Items[i]
			}
		}
	}
	if target == nil {
		t.Fatal("no thread contains comment 2647711")
	}
	ids := map[int]bool{}
	for _, cm := range target.Comments {
		ids[cm.ID] = true
	}
	want := map[int]bool{2642135: true, 2645582: true, 2647711: true}
	if len(ids) != len(want) {
		t.Errorf("thread for 2647711 should hold exactly %v, got %v", keys(want), keys(ids))
	}
	for id := range want {
		if !ids[id] {
			t.Errorf("thread for 2647711 missing %d", id)
		}
	}
	for _, leak := range []int{2647783, 2646853} {
		if ids[leak] {
			t.Errorf("thread for 2647711 leaked unrelated comment %d", leak)
		}
	}
	// Three independent roots -> three threads (general comments not merged).
	if len(threads.Items) != 3 {
		t.Errorf("expected 3 threads (one per root), got %d", len(threads.Items))
	}
}

func keys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
