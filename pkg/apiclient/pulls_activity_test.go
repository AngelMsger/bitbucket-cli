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

func TestListPRActivityClassifiesDataCenterPredefinedReviewerCommentAsSystem(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []any{
				map[string]any{
					"action": "COMMENTED", "commentAction": "ADDED", "createdDate": 1,
					"user": map[string]any{"name": "alice"},
					"comment": map[string]any{
						"id": 1, "author": map[string]any{"name": "alice"},
						"text": "User(s) Bob and Carol have been added automatically as predefined branch reviewers. ",
					},
				},
				map[string]any{
					"action": "COMMENTED", "commentAction": "ADDED", "createdDate": 2,
					"user": map[string]any{"name": "alice"},
					"comment": map[string]any{
						"id": 2, "author": map[string]any{"name": "alice"},
						"text": "Bob and Carol have been added automatically as predefined branch reviewers; please verify.",
					},
				},
			},
			"isLastPage": true,
		})
	}))

	res, err := c.ListPRActivity(context.Background(), PRListOpts{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, Query: "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("got %d activities; want 2", len(res.Items))
	}
	if !res.Items[0].System {
		t.Fatal("predefined-reviewer comment should be classified as system activity")
	}
	if res.Items[1].System {
		t.Fatal("a human comment containing similar words should not be classified as system activity")
	}
}
