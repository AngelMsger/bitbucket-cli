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
