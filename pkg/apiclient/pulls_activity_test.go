package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListPRActivityIncludesPRRefWithoutChangingCloudUpdateKind(t *testing.T) {
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []any{
				map[string]any{"update": map[string]any{
					"state": "DECLINED", "date": "2026-09-03T10:00:00Z",
					"author": map[string]any{"uuid": "{alice}"},
				}},
			},
		})
	}))

	res, err := c.ListPRActivity(context.Background(), PRListOpts{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, Query: "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("got %d activities; want 1", len(res.Items))
	}
	activity := res.Items[0]
	if activity.Kind != "update" || activity.State != "DECLINED" {
		t.Fatalf("kind/state = %q/%q; want update/DECLINED", activity.Kind, activity.State)
	}
	if activity.PullRequest == nil || activity.PullRequest.Ref != "ws/repo/42" {
		t.Fatalf("pull_request = %+v; want ws/repo/42", activity.PullRequest)
	}
}
