package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

func newWriteTestClient(t *testing.T, flavor Flavor, handler http.Handler) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{Flavor: flavor, BaseURL: srv.URL, Transport: transport.New(transport.Options{})})
}

// TestUpdatePRDataCenterPreservesVersionAndReviewers guards both pieces of
// state that a DC metadata PUT must round-trip: the optimistic-lock version and
// the complete reviewer set.
func TestUpdatePRDataCenterPreservesVersionAndReviewers(t *testing.T) {
	var getHit bool
	var putBody map[string]any
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getHit = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "version": 5, "title": "old",
				"reviewers": []any{
					map[string]any{"user": map[string]string{"name": "alice"}},
					map[string]any{"user": map[string]string{"name": "bob"}},
				},
			})
		case http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &putBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "version": 6, "title": "new"})
		default:
			t.Fatalf("unexpected %s", r.Method)
		}
	}))

	if _, err := c.UpdatePR(context.Background(), UpdatePRReq{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7, Title: "new",
	}); err != nil {
		t.Fatal(err)
	}
	if !getHit {
		t.Fatal("expected a version GET before the PUT")
	}
	if v, _ := putBody["version"].(float64); int(v) != 5 {
		t.Errorf("PUT version = %v; want the current version 5", putBody["version"])
	}
	if putBody["title"] != "new" {
		t.Errorf("PUT title = %v; want new", putBody["title"])
	}
	if got := reviewerNamesFromPayload(t, putBody); strings.Join(got, ",") != "alice,bob" {
		t.Errorf("PUT reviewers = %v; want existing reviewers [alice bob]", got)
	}
}

func TestUpdatePRDataCenterReplacesReviewersWhenExplicit(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("dry-run should only GET; got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 7, "version": 5,
			"reviewers": []any{map[string]any{"user": map[string]string{"name": "alice"}}},
		})
	}))
	plan, err := c.DescribeWrite(context.Background(), UpdatePRReq{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7,
		Reviewers: []string{"carol"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := plan.Payload.(map[string]any)
	if got := reviewerNamesFromPayload(t, body); strings.Join(got, ",") != "carol" {
		t.Errorf("PUT reviewers = %v; want explicit replacement [carol]", got)
	}
}

func TestUpdatePRCloudOmitsUnchangedReviewers(t *testing.T) {
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("Cloud dry-run should not send HTTP; got %s", r.Method)
	}))
	plan, err := c.DescribeWrite(context.Background(), UpdatePRReq{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, ID: 7, Description: "new",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := plan.Payload.(map[string]any)
	if _, ok := body["reviewers"]; ok {
		t.Errorf("Cloud PUT should omit unchanged reviewers; got %v", body["reviewers"])
	}
}

// TestDescribeUpdatePRDataCenter confirms --dry-run also resolves the version
// (it must hit the GET) and reports a PUT carrying it.
func TestDescribeUpdatePRDataCenter(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("dry-run should only GET; got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "version": 9})
	}))
	plan, err := c.DescribeWrite(context.Background(), UpdatePRReq{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7, Description: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := plan.Payload.(map[string]any)
	if plan.Method != http.MethodPut || body["version"] != 9 {
		t.Errorf("plan = %s version=%v; want PUT version=9", plan.Method, body["version"])
	}
}

func reviewerNamesFromPayload(t *testing.T, body map[string]any) []string {
	t.Helper()
	b, err := json.Marshal(body["reviewers"])
	if err != nil {
		t.Fatal(err)
	}
	var reviewers []struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(b, &reviewers); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		names = append(names, reviewer.User.Name)
	}
	return names
}

// TestCreatePRDataCenterFork verifies a cross-fork PR points fromRef at the fork
// while POSTing to the upstream repo's pull-requests endpoint.
func TestCreatePRDataCenterFork(t *testing.T) {
	var path string
	var body map[string]any
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "version": 0})
	}))

	if _, err := c.CreatePR(context.Background(), CreatePRReq{
		Repo:        RepoRef{Workspace: "UP", Slug: "repo"},
		Title:       "t",
		Source:      "feature",
		SourceRepo:  "FORK/repo",
		Destination: "dev",
		Reviewers:   []string{},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "/projects/UP/repos/repo/pull-requests") {
		t.Errorf("POST path = %q; want the upstream UP/repo endpoint", path)
	}
	from := body["fromRef"].(map[string]any)["repository"].(map[string]any)
	if from["slug"] != "repo" || from["project"].(map[string]any)["key"] != "FORK" {
		t.Errorf("fromRef.repository = %v; want fork FORK/repo", from)
	}
	to := body["toRef"].(map[string]any)["repository"].(map[string]any)
	if to["project"].(map[string]any)["key"] != "UP" {
		t.Errorf("toRef.repository = %v; want upstream UP/repo", to)
	}
}

func TestCreatePRCloudAddsAllEffectiveDefaultReviewers(t *testing.T) {
	var postBody map[string]any
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/effective-default-reviewers") && r.URL.Query().Get("page") == "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"values": []any{map[string]any{"user": map[string]string{"uuid": "{alice}"}}},
				"next":   "/2.0/repositories/ws/repo/effective-default-reviewers?page=2",
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/effective-default-reviewers") && r.URL.Query().Get("page") == "2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"values": []any{map[string]any{"user": map[string]string{"uuid": "{bob}"}}},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pullrequests"):
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &postBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "title": "t"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.String())
		}
	}))

	if _, err := c.CreatePR(context.Background(), CreatePRReq{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, Title: "t", Source: "feature",
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(postBody["reviewers"])
	got := string(raw)
	if !strings.Contains(got, `"uuid":"{alice}"`) || !strings.Contains(got, `"uuid":"{bob}"`) {
		t.Errorf("POST reviewers = %s; want both effective default reviewers", raw)
	}
}

func TestDescribeCreatePRDataCenterResolvesConditionalDefaults(t *testing.T) {
	var reviewerQuery string
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/1.0/projects/UP/repos/repo":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 20})
		case "/rest/api/1.0/projects/FORK/repos/repo":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 10})
		case "/rest/default-reviewers/latest/projects/UP/repos/repo/reviewers":
			reviewerQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode([]any{
				map[string]any{
					"reviewers": []any{
						map[string]string{"name": "alice"},
						map[string]string{"name": "bob"},
					},
				},
				map[string]any{"reviewers": []any{map[string]string{"name": "alice"}}},
			})
		default:
			t.Fatalf("dry-run should only resolve defaults; got %s %s", r.Method, r.URL.String())
		}
	}))

	plan, err := c.DescribeWrite(context.Background(), CreatePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, Title: "t",
		Source: "feature/x", SourceRepo: "FORK/repo", Destination: "dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantQueryParts := []string{
		"sourceRepoId=10",
		"targetRepoId=20",
		"sourceRefId=refs%2Fheads%2Ffeature%2Fx",
		"targetRefId=refs%2Fheads%2Fdev",
	}
	for _, part := range wantQueryParts {
		if !strings.Contains(reviewerQuery, part) {
			t.Errorf("default-reviewer query %q missing %q", reviewerQuery, part)
		}
	}
	body := plan.Payload.(map[string]any)
	if got := reviewerNamesFromPayload(t, body); strings.Join(got, ",") != "alice,bob" {
		t.Errorf("planned reviewers = %v; want deduplicated conditional defaults [alice bob]", got)
	}
}

func TestDescribeCreatePRDataCenterUsesDefaultTargetBranch(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/1.0/projects/UP/repos/repo":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 20})
		case "/rest/api/1.0/projects/UP/repos/repo/default-branch":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "refs/heads/main"})
		case "/rest/default-reviewers/latest/projects/UP/repos/repo/reviewers":
			if got := r.URL.Query().Get("targetRefId"); got != "refs/heads/main" {
				t.Errorf("targetRefId = %q; want refs/heads/main", got)
			}
			_ = json.NewEncoder(w).Encode([]any{map[string]string{"name": "alice"}})
		default:
			t.Fatalf("dry-run should only resolve defaults; got %s %s", r.Method, r.URL.String())
		}
	}))

	plan, err := c.DescribeWrite(context.Background(), CreatePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, Title: "t", Source: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := reviewerNamesFromPayload(t, plan.Payload.(map[string]any)); strings.Join(got, ",") != "alice" {
		t.Errorf("planned reviewers = %v; want [alice]", got)
	}
}

func TestDescribeCreatePRExplicitReviewersSkipDefaults(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		t.Run(string(flavor), func(t *testing.T) {
			c := newWriteTestClient(t, flavor, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				t.Fatalf("explicit reviewers should not pre-fetch defaults; got %s %s", r.Method, r.URL.Path)
			}))
			plan, err := c.DescribeWrite(context.Background(), CreatePRReq{
				Repo: RepoRef{Workspace: "UP", Slug: "repo"}, Title: "t", Source: "feature",
				Destination: "dev", Reviewers: []string{"carol"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := plan.Payload.(map[string]any)["reviewers"]; !ok {
				t.Fatal("explicit reviewer missing from planned payload")
			}
		})
	}
}

func TestCreatePRDefaultReviewerFailureStillPosts(t *testing.T) {
	var posted bool
	var warnings []Warning
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/effective-default-reviewers"):
			http.Error(w, `{"error":{"message":"defaults unavailable"}}`, http.StatusForbidden)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pullrequests"):
			posted = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "title": "t"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	c := New(Config{
		Flavor: FlavorCloud, BaseURL: srv.URL, Transport: transport.New(transport.Options{}),
		WarningSink: func(w Warning) { warnings = append(warnings, w) },
	})

	if _, err := c.CreatePR(context.Background(), CreatePRReq{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, Title: "t", Source: "feature",
	}); err != nil {
		t.Fatal(err)
	}
	if !posted {
		t.Fatal("expected the PR POST after the default-reviewer lookup failed")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v; want one warning", warnings)
	}
}

// TestCreatePRDataCenterForkNeedsTarget confirms a fork PR without --target is a
// usage error rather than a malformed request.
func TestCreatePRDataCenterForkNeedsTarget(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("should not send HTTP; got %s %s", r.Method, r.URL.Path)
	}))
	_, err := c.CreatePR(context.Background(), CreatePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, Title: "t",
		Source: "feature", SourceRepo: "FORK/repo", // no Destination
	})
	if err == nil {
		t.Fatal("expected an error when a fork PR omits --target")
	}
}

// TestMergePRDataCenterCloseSourceBranch verifies the opt-in deletes the source
// branch (from the PR's fromRef, honoring a fork) after a successful merge.
func TestMergePRDataCenterCloseSourceBranch(t *testing.T) {
	var deletePath string
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pull-requests/3"):
			// version pre-fetch for the merge + fromRef for the cleanup
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 3, "version": 2,
				"fromRef": map[string]any{
					"displayId":  "feature",
					"repository": map[string]any{"slug": "repo", "project": map[string]any{"key": "FORK"}},
				},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/merge"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 3, "version": 3, "state": "MERGED",
				"fromRef": map[string]any{
					"displayId":  "feature",
					"repository": map[string]any{"slug": "repo", "project": map[string]any{"key": "FORK"}},
				},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/branch-utils/"):
			deletePath = r.URL.String()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))

	if _, err := c.MergePR(context.Background(), MergePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, ID: 3, CloseSourceBranch: true,
	}); err != nil {
		t.Fatal(err)
	}
	if deletePath == "" {
		t.Fatal("expected a source-branch DELETE after merge")
	}
	if !strings.Contains(deletePath, "/projects/FORK/repos/repo/branches") || !strings.Contains(deletePath, "name=feature") {
		t.Errorf("delete path = %q; want the fork branch feature", deletePath)
	}
}

// TestMergePRDataCenterNoCloseSourceBranch confirms the source branch is left
// alone when the opt-in is absent.
func TestMergePRDataCenterNoCloseSourceBranch(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("did not expect a branch DELETE without --close-source-branch")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 3, "version": 2})
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 3, "version": 3, "state": "MERGED"})
		}
	}))
	if _, err := c.MergePR(context.Background(), MergePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, ID: 3,
	}); err != nil {
		t.Fatal(err)
	}
}
