package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

func TestListMyPRsMyVoteFilterAndStampingDataCenter(t *testing.T) {
	calls := 0
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/rest/api/1.0/dashboard/pull-requests" {
			t.Fatalf("unexpected N+1 request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-AUSERNAME", "alice")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"isLastPage":    false,
			"nextPageStart": 25,
			"values": []any{
				map[string]any{
					"id": 1, "title": "Unvoted PR", "state": "OPEN", "updatedDate": 1700000000000,
					"fromRef":   map[string]any{"displayId": "feat-1", "latestCommit": "aaa1111", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
					"toRef":     map[string]any{"displayId": "main", "latestCommit": "bbb2222", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
					"reviewers": []any{map[string]any{"user": map[string]any{"name": "alice", "slug": "alice"}, "role": "REVIEWER", "approved": false, "status": "UNAPPROVED"}},
				},
				map[string]any{
					"id": 2, "title": "Approved PR", "state": "OPEN", "updatedDate": 1700000010000,
					"fromRef":   map[string]any{"displayId": "feat-2", "latestCommit": "ccc3333", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
					"toRef":     map[string]any{"displayId": "main", "latestCommit": "bbb2222", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
					"reviewers": []any{map[string]any{"user": map[string]any{"name": "alice", "slug": "alice"}, "role": "REVIEWER", "approved": true, "status": "APPROVED"}},
				},
				map[string]any{
					"id": 3, "title": "Needs work PR", "state": "OPEN", "updatedDate": 1700000020000,
					"fromRef":   map[string]any{"displayId": "feat-3", "latestCommit": "ddd4444", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
					"toRef":     map[string]any{"displayId": "main", "latestCommit": "bbb2222", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
					"reviewers": []any{map[string]any{"user": map[string]any{"name": "alice", "slug": "alice"}, "role": "REVIEWER", "approved": false, "status": "NEEDS_WORK"}},
				},
			},
		})
	}))

	// Unfiltered: stamps my_vote, source.commit, updated_at in a single HTTP call.
	all, err := c.ListMyPRs(context.Background(), MyPRListOpts{Role: "REVIEWER", State: "OPEN"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 HTTP call (no N+1), got %d", calls)
	}
	if all.Next != "25" {
		t.Fatalf("Next = %q, want 25", all.Next)
	}
	if len(all.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3", len(all.Items))
	}
	wantVotes := []string{"unvoted", "approved", "needs_work"}
	for i, want := range wantVotes {
		if all.Items[i].MyVote != want {
			t.Errorf("item[%d].MyVote = %q, want %q", i, all.Items[i].MyVote, want)
		}
		if all.Items[i].Source.Commit == "" || all.Items[i].UpdatedAt == "" {
			t.Errorf("item[%d] missing source.commit or updated_at: %+v", i, all.Items[i])
		}
	}

	// Filter by unvoted preserves next cursor.
	unvoted, err := c.ListMyPRs(context.Background(), MyPRListOpts{Role: "REVIEWER", State: "OPEN", MyVote: "unvoted"})
	if err != nil {
		t.Fatal(err)
	}
	if len(unvoted.Items) != 1 || unvoted.Items[0].ID != 1 || unvoted.Next != "25" {
		t.Fatalf("unvoted filter = %+v (next=%q), want ID 1 with next=25", unvoted.Items, unvoted.Next)
	}

	// Cross-flavor filter: changes_requested matches DC needs_work.
	cr, err := c.ListMyPRs(context.Background(), MyPRListOpts{Role: "REVIEWER", State: "OPEN", MyVote: "changes_requested"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cr.Items) != 1 || cr.Items[0].ID != 3 || cr.Items[0].MyVote != "needs_work" {
		t.Fatalf("changes_requested filter on DC = %+v, want ID 3 (needs_work)", cr.Items)
	}

	// Invalid --my-vote rejected before HTTP.
	prevCalls := calls
	_, err = c.ListMyPRs(context.Background(), MyPRListOpts{MyVote: "bogus"})
	if err == nil || cerrors.AsCLIError(err).Code != "BAD_MY_VOTE" {
		t.Fatalf("expected BAD_MY_VOTE, got %v", err)
	}
	if calls != prevCalls {
		t.Fatalf("invalid --my-vote should fail before HTTP")
	}
}

func TestListMyPRsMyVoteFilterAndStampingCloud(t *testing.T) {
	var gotFields string
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/2.0/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"uuid": "{alice-uuid}", "nickname": "alice"})
		case "/2.0/repositories/ws":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"values": []any{map[string]any{"slug": "demo", "workspace": map[string]any{"slug": "ws"}}},
			})
		case "/2.0/repositories/ws/demo/pullrequests":
			gotFields = r.URL.Query().Get("fields")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"values": []any{
					map[string]any{
						"id": 10, "title": "Approved Cloud PR", "state": "OPEN", "updated_on": "2026-04-01T10:00:00Z",
						"source":      map[string]any{"branch": map[string]any{"name": "f1"}, "commit": map[string]any{"hash": "1111111"}},
						"destination": map[string]any{"branch": map[string]any{"name": "main"}, "repository": map[string]any{"full_name": "ws/demo"}},
						"participants": []any{
							map[string]any{"user": map[string]any{"uuid": "{alice-uuid}"}, "role": "REVIEWER", "approved": true, "state": "approved"},
						},
					},
					map[string]any{
						"id": 11, "title": "CR Cloud PR", "state": "OPEN", "updated_on": "2026-04-01T11:00:00Z",
						"source":      map[string]any{"branch": map[string]any{"name": "f2"}, "commit": map[string]any{"hash": "2222222"}},
						"destination": map[string]any{"branch": map[string]any{"name": "main"}, "repository": map[string]any{"full_name": "ws/demo"}},
						"participants": []any{
							map[string]any{"user": map[string]any{"uuid": "{alice-uuid}"}, "role": "REVIEWER", "approved": false, "state": "changes_requested"},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected Cloud path: %s", r.URL.Path)
		}
	}))

	// Cross-flavor filter: needs_work matches Cloud changes_requested.
	res, err := c.ListMyPRs(context.Background(), MyPRListOpts{
		Role: "REVIEWER", State: "OPEN", Workspace: "ws", MyVote: "needs_work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotFields, "+values.participants") {
		t.Errorf("Cloud inbox should request +values.participants, got fields=%q", gotFields)
	}
	if len(res.Items) != 1 || res.Items[0].ID != 11 || res.Items[0].MyVote != "changes_requested" {
		t.Fatalf("Cloud filtered inbox = %+v, want ID 11 (changes_requested)", res.Items)
	}
}

func TestGetPRStatusMyReviewDataCenter(t *testing.T) {
	// Vote at t=1700000010000, new commit at t=1700000020000 -> updated_after_my_vote=true, 1 unresolved root thread.
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/1.0/application-properties":
			w.Header().Set("X-AUSERNAME", "alice")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "8.19.0"})
		case "/rest/api/1.0/users/alice":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "alice", "slug": "alice", "displayName": "Alice", "type": "NORMAL"})
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/7":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "title": "Feature", "state": "OPEN", "updatedDate": 1700000020000,
				"fromRef": map[string]any{"displayId": "feat", "latestCommit": "newhead123", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
				"toRef":   map[string]any{"displayId": "main", "latestCommit": "base000000", "repository": map[string]any{"slug": "demo", "project": map[string]any{"key": "PROJ"}}},
				"reviewers": []any{
					map[string]any{
						"user": map[string]any{"name": "alice", "slug": "alice"}, "role": "REVIEWER",
						"approved": true, "status": "APPROVED", "lastReviewedCommit": "oldhead000",
					},
				},
			})
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/7/merge":
			_ = json.NewEncoder(w).Encode(map[string]any{"canMerge": true, "conflicted": false})
		case "/rest/build-status/1.0/commits/newhead123":
			_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}, "isLastPage": true})
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/7/commits":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"isLastPage": true,
				"values": []any{
					map[string]any{"id": "newhead123", "message": "fix", "authorTimestamp": 1700000020000},
				},
			})
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/7/activities":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"isLastPage": true,
				"values": []any{
					map[string]any{"action": "APPROVED", "createdDate": 1700000010000, "user": map[string]any{"name": "alice", "slug": "alice"}},
					map[string]any{"action": "COMMENTED", "createdDate": 1700000005000, "user": map[string]any{"name": "bob", "slug": "bob"}, "comment": map[string]any{"id": 101, "text": "Open thread", "state": "OPEN"}},
					map[string]any{"action": "COMMENTED", "createdDate": 1700000006000, "user": map[string]any{"name": "bob", "slug": "bob"}, "comment": map[string]any{"id": 102, "text": "Resolved thread", "state": "RESOLVED"}},
				},
			})
		default:
			t.Fatalf("unexpected DC status path: %s", r.URL.Path)
		}
	}))

	st, err := c.GetPRStatus(context.Background(), RepoRef{Workspace: "PROJ", Slug: "demo"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if st.MyReview == nil {
		t.Fatal("expected my_review to be populated")
	}
	if st.MyReview.Vote != "approved" {
		t.Errorf("vote = %q, want approved", st.MyReview.Vote)
	}
	if st.MyReview.VotedAt != "2023-11-14T22:13:30Z" {
		t.Errorf("voted_at = %q, want 2023-11-14T22:13:30Z", st.MyReview.VotedAt)
	}
	if st.MyReview.LatestCommitAt != "2023-11-14T22:13:40Z" {
		t.Errorf("latest_commit_at = %q, want 2023-11-14T22:13:40Z", st.MyReview.LatestCommitAt)
	}
	if st.MyReview.UpdatedAfterMyVote == nil || !*st.MyReview.UpdatedAfterMyVote {
		t.Errorf("updated_after_my_vote = %v, want true", st.MyReview.UpdatedAfterMyVote)
	}
	if st.MyReview.UnresolvedThreadCount == nil || *st.MyReview.UnresolvedThreadCount != 1 {
		t.Errorf("unresolved_thread_count = %v, want 1", st.MyReview.UnresolvedThreadCount)
	}
}

func TestGetPRStatusMyReviewCloudNotUpdated(t *testing.T) {
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/2.0/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"uuid": "{alice}", "nickname": "alice", "type": "user"})
		case "/2.0/repositories/ws/demo/pullrequests/9":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 9, "title": "Clean PR", "state": "OPEN", "updated_on": "2026-04-01T12:00:00Z",
				"source":      map[string]any{"branch": map[string]any{"name": "feat"}, "commit": map[string]any{"hash": "abc1234"}},
				"destination": map[string]any{"branch": map[string]any{"name": "main"}},
				"participants": []any{
					map[string]any{
						"user": map[string]any{"uuid": "{alice}"}, "role": "REVIEWER",
						"approved": false, "state": "changes_requested", "participated_on": "2026-04-01T12:00:00Z",
					},
				},
			})
		case "/2.0/repositories/ws/demo/pullrequests/9/statuses":
			_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}})
		case "/2.0/repositories/ws/demo/pullrequests/9/commits":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"values": []any{map[string]any{"hash": "abc1234", "date": "2026-04-01T10:00:00Z"}},
			})
		case "/2.0/repositories/ws/demo/pullrequests/9/activity":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"values": []any{
					map[string]any{
						"changes_requested": map[string]any{
							"date": "2026-04-01T12:00:00Z",
							"user": map[string]any{"uuid": "{alice}"},
						},
					},
				},
			})
		case "/2.0/repositories/ws/demo/pullrequests/9/comments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"values": []any{
					map[string]any{"id": 1, "content": map[string]any{"raw": "Fixed"}, "resolution": map[string]any{"type": "comment_resolution"}},
					map[string]any{"id": 2, "content": map[string]any{"raw": "Still open"}},
					map[string]any{"id": 3, "content": map[string]any{"raw": "Reply"}, "parent": map[string]any{"id": 2}},
				},
			})
		default:
			t.Fatalf("unexpected Cloud status path: %s", r.URL.Path)
		}
	}))

	st, err := c.GetPRStatus(context.Background(), RepoRef{Workspace: "ws", Slug: "demo"}, 9)
	if err != nil {
		t.Fatal(err)
	}
	if st.MyReview == nil {
		t.Fatal("expected my_review")
	}
	if st.MyReview.Vote != "changes_requested" {
		t.Errorf("vote = %q, want changes_requested", st.MyReview.Vote)
	}
	if st.MyReview.VotedAt != "2026-04-01T12:00:00Z" {
		t.Errorf("voted_at = %q, want 2026-04-01T12:00:00Z", st.MyReview.VotedAt)
	}
	if st.MyReview.LatestCommitAt != "2026-04-01T10:00:00Z" {
		t.Errorf("latest_commit_at = %q, want 2026-04-01T10:00:00Z", st.MyReview.LatestCommitAt)
	}
	if st.MyReview.UpdatedAfterMyVote == nil || *st.MyReview.UpdatedAfterMyVote {
		t.Errorf("updated_after_my_vote = %v, want false", st.MyReview.UpdatedAfterMyVote)
	}
	if st.MyReview.UnresolvedThreadCount == nil || *st.MyReview.UnresolvedThreadCount != 1 {
		t.Errorf("unresolved_thread_count = %v, want 1", st.MyReview.UnresolvedThreadCount)
	}
}

func TestCommentAuthorAndAIAttributionMetadata(t *testing.T) {
	dcHuman := mapDCComment(1, dcComment{
		ID: 1, Text: "Looks good",
		Author: dcUser{Name: "alice", Slug: "alice", Type: "NORMAL"},
	})
	dcBotAI := mapDCComment(1, dcComment{
		ID: 2, Text: "[[AI]](https://angelmsger.github.io/bitbucket-cli/) Nil deref on empty slice.",
		Author: dcUser{Name: "ci-bot", Slug: "ci-bot", Type: "SERVICE"},
	})
	cloudHuman := mapCloudComment(1, cloudComment{
		ID: 3, Content: struct {
			Raw  string `json:"raw"`
			HTML string `json:"html"`
		}{Raw: "Plain text mentioning [[AI]] without link"},
		User: cloudUser{UUID: "{u-1}", Type: "user"},
	})
	cloudApp := mapCloudComment(1, cloudComment{
		ID: 4, Content: struct {
			Raw  string `json:"raw"`
			HTML string `json:"html"`
		}{Raw: "[[AI]](https://example.com) Auto check"},
		User: cloudUser{UUID: "{bot-1}", Type: "app"},
	})
	unknownAuthor := mapCloudComment(1, cloudComment{
		ID: 5, Content: struct {
			Raw  string `json:"raw"`
			HTML string `json:"html"`
		}{Raw: "Named bot-helper without platform type"},
		User: cloudUser{UUID: "{u-2}", Nickname: "bot-helper"},
	})

	comments := []Comment{dcHuman, dcBotAI, cloudHuman, cloudApp, unknownAuthor}

	// Without current user: is_self is nil (omitted in JSON).
	rawNoSelf, _ := json.Marshal(comments[0])
	if strings.Contains(string(rawNoSelf), "is_self") {
		t.Errorf("expected is_self omitted when identity unknown, got %s", rawNoSelf)
	}

	if dcHuman.AuthorKind != AuthorKindHuman || dcHuman.IsAIAttributed {
		t.Errorf("dcHuman = %+v", dcHuman)
	}
	if dcBotAI.AuthorKind != AuthorKindBot || !dcBotAI.IsAIAttributed {
		t.Errorf("dcBotAI = %+v", dcBotAI)
	}
	if cloudHuman.AuthorKind != AuthorKindHuman || cloudHuman.IsAIAttributed {
		t.Errorf("cloudHuman = %+v", cloudHuman)
	}
	if cloudApp.AuthorKind != AuthorKindBot || !cloudApp.IsAIAttributed {
		t.Errorf("cloudApp = %+v", cloudApp)
	}
	if unknownAuthor.AuthorKind != AuthorKindUnknown {
		t.Errorf("unknownAuthor.AuthorKind = %q, want unknown (never guess from name)", unknownAuthor.AuthorKind)
	}

	// With current user: is_self stamped accurately.
	EnrichCommentsWithSelf(comments, &User{Slug: "alice", Name: "alice"})
	if comments[0].IsSelf == nil || !*comments[0].IsSelf {
		t.Errorf("expected comments[0].IsSelf = true, got %v", comments[0].IsSelf)
	}
	if comments[1].IsSelf == nil || *comments[1].IsSelf {
		t.Errorf("expected comments[1].IsSelf = false, got %v", comments[1].IsSelf)
	}
}

func TestSubmitPRReviewValidationBlocksZeroSideEffects(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		t.Run(string(flavor), func(t *testing.T) {
			writes := 0
			c := newWriteTestClient(t, flavor, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					t.Errorf("unexpected write during failed Phase 1: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/pull-requests/1") || strings.HasSuffix(r.URL.Path, "/pullrequests/1"):
					if flavor == FlavorCloud {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"id": 1, "state": "OPEN",
							"source": map[string]any{"commit": map[string]any{"hash": "deadbeef1234"}},
						})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"id": 1, "state": "OPEN",
							"fromRef": map[string]any{"latestCommit": "deadbeef1234"},
						})
					}
				case strings.Contains(r.URL.Path, "/diff"):
					w.Header().Set("Content-Type", "text/plain")
					_, _ = w.Write([]byte("--- a/src/app.go\n+++ b/src/app.go\n@@ -10,2 +10,3 @@\n line10\n+line11\n line12\n"))
				default:
					t.Fatalf("unexpected GET %s", r.URL.Path)
				}
			}))

			repo := RepoRef{Workspace: "PROJ", Slug: "demo"}

			// 1. Stale expected-source-commit -> PR_HEAD_MOVED (conflict), zero writes.
			_, err := c.SubmitPRReview(context.Background(), SubmitPRReviewReq{
				Repo:                 repo,
				PRID:                 1,
				Verdict:              ReviewVerdictApprove,
				ExpectedSourceCommit: "0000000old",
			})
			if err == nil {
				t.Fatal("expected PR_HEAD_MOVED error")
			}
			ce := cerrors.AsCLIError(err)
			if ce.Code != "PR_HEAD_MOVED" || ce.Category != cerrors.CategoryConflict {
				t.Fatalf("got %s/%s, want conflict/PR_HEAD_MOVED", ce.Category, ce.Code)
			}
			if writes != 0 {
				t.Fatalf("writes = %d, want 0", writes)
			}

			// 2. Invalid inline line number alongside a valid general comment -> fails in Phase 1, zero writes.
			_, err = c.SubmitPRReview(context.Background(), SubmitPRReviewReq{
				Repo:                 repo,
				PRID:                 1,
				Verdict:              ReviewVerdictRequestChanges,
				ExpectedSourceCommit: "deadbeef1234",
				Comments: []ReviewCommentInput{
					{Content: "Valid summary comment"},
					{Content: "Bad inline line", Path: "src/app.go", Line: 999, Side: "new"},
				},
			})
			if err == nil {
				t.Fatal("expected invalid inline anchor error")
			}
			if writes != 0 {
				t.Fatalf("writes after invalid inline = %d, want 0", writes)
			}
		})
	}
}

func TestPlanAndSubmitPRReviewHappyPathAndPartialFailure(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		t.Run(string(flavor), func(t *testing.T) {
			var writeOrder []string
			failSecondComment := false
			nextID := 100

			c := newWriteTestClient(t, flavor, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					switch {
					case r.URL.Path == "/rest/api/1.0/application-properties":
						w.Header().Set("X-AUSERNAME", "alice")
						_ = json.NewEncoder(w).Encode(map[string]any{"version": "8.19.0"})
					case r.URL.Path == "/rest/api/1.0/users/alice":
						_ = json.NewEncoder(w).Encode(map[string]any{"name": "alice", "slug": "alice"})
					case strings.HasSuffix(r.URL.Path, "/pull-requests/1") || strings.HasSuffix(r.URL.Path, "/pullrequests/1"):
						if flavor == FlavorCloud {
							_ = json.NewEncoder(w).Encode(map[string]any{
								"id": 1, "state": "OPEN",
								"source": map[string]any{"commit": map[string]any{"hash": "cafe1234abcd"}},
							})
						} else {
							_ = json.NewEncoder(w).Encode(map[string]any{
								"id": 1, "state": "OPEN",
								"fromRef": map[string]any{"latestCommit": "cafe1234abcd"},
							})
						}
					case strings.Contains(r.URL.Path, "/diff"):
						w.Header().Set("Content-Type", "text/plain")
						_, _ = w.Write([]byte("--- a/src/app.go\n+++ b/src/app.go\n@@ -10,2 +10,3 @@\n line10\n+line11\n line12\n"))
					default:
						t.Fatalf("unexpected GET %s", r.URL.Path)
					}
					return
				}

				writeOrder = append(writeOrder, r.Method+" "+r.URL.Path)
				if strings.HasSuffix(r.URL.Path, "/comments") {
					if failSecondComment && len(writeOrder) == 2 {
						http.Error(w, `{"errors":[{"message":"boom"}]}`, http.StatusInternalServerError)
						return
					}
					nextID++
					if flavor == FlavorCloud {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"id": nextID, "content": map[string]any{"raw": "posted"},
						})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"id": nextID, "text": "posted", "state": "OPEN",
						})
					}
					return
				}
				// Vote endpoint.
				_, _ = w.Write([]byte(`{}`))
			}))

			req := SubmitPRReviewReq{
				Repo:                 RepoRef{Workspace: "PROJ", Slug: "demo"},
				PRID:                 1,
				Verdict:              ReviewVerdictRequestChanges,
				ExpectedSourceCommit: "cafe1234",
				Comments: []ReviewCommentInput{
					{Content: "Summary comment"},
					{Content: "Inline comment", Path: "src/app.go", Line: 11, Side: "new"},
					{Content: "Reply comment", ReplyTo: 50},
				},
			}

			// PlanPRReview works through NewReadOnly and sends zero writes.
			plan, err := NewReadOnly(c).PlanPRReview(context.Background(), req)
			if err != nil {
				t.Fatalf("PlanPRReview: %v", err)
			}
			if len(writeOrder) != 0 {
				t.Fatalf("PlanPRReview sent writes: %v", writeOrder)
			}
			if !plan.Validated || len(plan.Comments) != 3 || plan.Vote == nil {
				t.Fatalf("unexpected plan: %+v", plan)
			}
			if plan.Comments[0].Kind != "general" || plan.Comments[1].Kind != "inline" || plan.Comments[2].Kind != "reply" {
				t.Fatalf("unexpected planned comment kinds: %+v", plan.Comments)
			}

			// Live submit posts 3 comments then casts vote.
			res, err := c.SubmitPRReview(context.Background(), req)
			if err != nil {
				t.Fatalf("SubmitPRReview: %v", err)
			}
			if len(res.PostedComments) != 3 || !res.VoteApplied || res.FailedItem != nil {
				t.Fatalf("unexpected result: %+v", res)
			}
			if len(writeOrder) != 4 {
				t.Fatalf("expected 4 writes (3 comments + 1 vote), got %v", writeOrder)
			}

			// Mid-flight failure on comment[1]: stops before comment[2] and vote, reports posted_comments[0].
			writeOrder = nil
			failSecondComment = true
			partial, err := c.SubmitPRReview(context.Background(), req)
			if err == nil {
				t.Fatal("expected partial failure error")
			}
			ce := cerrors.AsCLIError(err)
			if ce.Code != "REVIEW_PARTIAL_FAILURE" || ce.Retryable {
				t.Fatalf("expected non-retryable REVIEW_PARTIAL_FAILURE, got %s (retryable=%v)", ce.Code, ce.Retryable)
			}
			if partial == nil || len(partial.PostedComments) != 1 || partial.VoteApplied {
				t.Fatalf("unexpected partial result: %+v", partial)
			}
			if partial.FailedItem == nil || partial.FailedItem.Kind != "comment" || partial.FailedItem.Index != 1 {
				t.Fatalf("unexpected failed_item: %+v", partial.FailedItem)
			}
		})
	}
}
