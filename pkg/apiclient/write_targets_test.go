package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

var writeTestRepo = RepoRef{Workspace: "PROJ", Slug: "repo"}

// A write that fails after it was sent may have been applied, so its error must
// never read as retryable: it says the outcome is unknown and names the read
// that shows the remote state. Before the kit's write rule these errors carried
// the transport's retryable:true, inviting an agent to repeat the write.
func TestWriteFailuresAreUnknownOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		flavor Flavor
		write  func(Client) error
		target cerrors.WriteTarget
	}{
		{"add comment", FlavorDataCenter, func(c Client) error {
			_, err := c.AddPRComment(context.Background(), AddPRCommentReq{Repo: writeTestRepo, PRID: 7, Content: "hi"})
			return err
		}, commentWriteTarget(writeTestRepo, 7, 0)},
		{"approve", FlavorDataCenter, func(c Client) error {
			return c.ApprovePR(context.Background(), ApprovePRReq{Repo: writeTestRepo, ID: 7, Approve: true})
		}, prReviewWriteTarget(writeTestRepo, 7)},
		{"delete branch", FlavorDataCenter, func(c Client) error {
			return c.DeleteBranch(context.Background(), DeleteBranchReq{Repo: writeTestRepo, Name: "feature/x"})
		}, branchWriteTarget(writeTestRepo, "feature/x")},
		{"create repository", FlavorCloud, func(c Client) error {
			_, err := c.CreateRepository(context.Background(), CreateRepoReq{Workspace: "PROJ", Slug: "repo"})
			return err
		}, repoWriteTarget(writeTestRepo)},
		{"fork repository", FlavorCloud, func(c Client) error {
			_, err := c.ForkRepository(context.Background(), ForkRepoReq{Source: writeTestRepo, Workspace: "team"})
			return err
		}, forkWriteTarget(ForkRepoReq{Source: writeTestRepo, Workspace: "team"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newWriteTestClient(t, tc.flavor, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			ce := cerrors.AsCLIError(tc.write(c))
			if ce.Category != cerrors.CategoryServer || ce.HTTPStatus != 503 || ce.Retryable {
				t.Fatalf("got category %s, status %d, retryable %v; want a non-retryable server 503",
					ce.Category, ce.HTTPStatus, ce.Retryable)
			}
			if !strings.HasPrefix(ce.Message, "Write outcome is unknown for "+tc.target.Description) {
				t.Fatalf("message %q does not name the write target", ce.Message)
			}
			if len(ce.NextSteps) == 0 || ce.NextSteps[0] != tc.target.ReadSteps[0] {
				t.Fatalf("next steps %q, want the target's read %q", ce.NextSteps, tc.target.ReadSteps)
			}
		})
	}
}

// A definite refusal shows nothing was written, so it keeps its own code and
// guidance.
func TestWriteRefusalPassesThrough(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	ce := cerrors.AsCLIError(c.ApprovePR(context.Background(), ApprovePRReq{Repo: writeTestRepo, ID: 7, Approve: true}))
	if ce.Category != cerrors.CategoryNotFound || strings.Contains(ce.Message, "Write outcome") {
		t.Fatalf("got %s %q; want the 404 unchanged", ce.Category, ce.Message)
	}
}

func TestWriteAcknowledgedButUndecodable(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":`))
	}))
	_, err := c.AddPRComment(context.Background(), AddPRCommentReq{Repo: writeTestRepo, PRID: 7, Content: "hi"})
	if ce := cerrors.AsCLIError(err); ce.Code != cerrors.CodeWriteSucceededResponseInvalid || ce.Retryable {
		t.Fatalf("got %s retryable=%v; want %s, not retryable", ce.Code, ce.Retryable, cerrors.CodeWriteSucceededResponseInvalid)
	}
}

// Cloud's resolve endpoint does not return the comment, so the client reads it
// back. When that read fails the resolve still happened: the error says so and
// must not invite a second resolve.
func TestCloudResolveReadBackFailureKeepsTheWrite(t *testing.T) {
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	_, err := c.ResolvePRComment(context.Background(), ResolvePRCommentReq{Repo: writeTestRepo, PRID: 7, ID: 9, Resolve: true})
	ce := cerrors.AsCLIError(err)
	if ce.Code != cerrors.CodeWriteSucceededReadFailed || ce.Retryable ||
		!strings.HasPrefix(ce.Message, "Write succeeded for comment 9 on pull request PROJ/repo/7") {
		t.Fatalf("got %s retryable=%v %q; want %s", ce.Code, ce.Retryable, ce.Message, cerrors.CodeWriteSucceededReadFailed)
	}
}

// A merge that succeeded must never read as retryable because its source-branch
// cleanup failed. The cleanup keeps its own outcome: a refused delete kept the
// branch, a failed one leaves it unknown, and both name the branch read.
func TestMergeBranchCleanupKeepsTheDeleteOutcome(t *testing.T) {
	for name, tc := range map[string]struct {
		status   int
		code     string
		category cerrors.Category
	}{
		"refused": {http.StatusForbidden, "PR_MERGED_BRANCH_KEPT", cerrors.CategoryPermission},
		"unknown": {http.StatusServiceUnavailable, "PR_MERGED_BRANCH_DELETE_UNKNOWN", cerrors.CategoryServer},
	} {
		t.Run(name, func(t *testing.T) {
			fromRef := map[string]any{
				"displayId":  "feature",
				"repository": map[string]any{"slug": "repo", "project": map[string]any{"key": "FORK"}},
			}
			c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodGet:
					_ = json.NewEncoder(w).Encode(map[string]any{"id": 3, "version": 2, "fromRef": fromRef})
				case http.MethodPost:
					_ = json.NewEncoder(w).Encode(map[string]any{"id": 3, "version": 3, "state": "MERGED", "fromRef": fromRef})
				default:
					w.WriteHeader(tc.status)
				}
			}))
			pr, err := c.MergePR(context.Background(), MergePRReq{Repo: writeTestRepo, ID: 3, CloseSourceBranch: true})
			ce := cerrors.AsCLIError(err)
			if pr == nil || ce.Code != tc.code || ce.Category != tc.category || ce.HTTPStatus != tc.status || ce.Retryable {
				t.Fatalf("got pr=%v %s/%s status %d retryable=%v; want the merged PR and a non-retryable %s %s",
					pr != nil, ce.Code, ce.Category, ce.HTTPStatus, ce.Retryable, tc.category, tc.code)
			}
			if !strings.HasPrefix(ce.Message, "Write succeeded for pull request PROJ/repo/3") ||
				len(ce.NextSteps) == 0 || ce.NextSteps[0] != "bitbucket-cli branch get 'feature' --repo 'FORK/repo'" {
				t.Fatalf("message %q, next steps %q; want the merge reported done and the branch read", ce.Message, ce.NextSteps)
			}
		})
	}
}

// The transport returns a write's redirect instead of following it, so a
// redirect (here to a login page) is an unknown outcome with its status.
// Before, a 302 was followed with a GET and an approve that landed on a login
// page read as approved; a 307 sent the approve again.
func TestRedirectedWriteIsAnUnknownOutcome(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var landed int
			c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/login" {
					landed++
					w.Header().Set("Content-Type", "text/html")
					_, _ = w.Write([]byte("<html>sign in</html>"))
					return
				}
				http.Redirect(w, r, "/login", status)
			}))
			err := c.ApprovePR(context.Background(), ApprovePRReq{Repo: writeTestRepo, ID: 7, Approve: true})
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Retryable || ce.HTTPStatus != status || landed != 0 ||
				!strings.HasPrefix(ce.Message, "Write outcome is unknown for your review on pull request") ||
				!strings.Contains(ce.Hint, "answered it with a redirect") {
				t.Fatalf("got %+v after %d login page requests; want an unknown outcome with the %d", ce, landed, status)
			}
		})
	}
}

// A branch is not found only once every substring match has been read. The
// read after an uncertain branch create or delete is branch get, which used
// to give up after the first 50 matches.
func TestGetBranchFollowsEveryPage(t *testing.T) {
	var starts []string
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		start := r.URL.Query().Get("start")
		starts = append(starts, start)
		if start == "" || start == "0" {
			_, _ = w.Write([]byte(`{"values":[{"id":"refs/heads/feature-a","displayId":"feature-a"}],"isLastPage":false,"nextPageStart":50}`))
			return
		}
		_, _ = w.Write([]byte(`{"values":[{"id":"refs/heads/feature","displayId":"feature"}],"isLastPage":true}`))
	}))
	b, err := c.GetBranch(context.Background(), writeTestRepo, "feature")
	if err != nil || b.Name != "feature" || len(starts) != 2 {
		t.Fatalf("got %+v, %v after pages %q; want feature from the second page", b, err, starts)
	}
}

// Data Center lists PRs from a branch as outgoing from the branch's own
// repository, so an uncertain cross-fork create is read in the fork. Cloud
// keeps the PR in its destination repository.
func TestCreatePRReadFindsCrossForkPRs(t *testing.T) {
	req := CreatePRReq{Repo: writeTestRepo, Source: "feature", SourceRepo: "FORK/repo", Destination: "main"}
	for flavor, want := range map[Flavor]string{
		FlavorDataCenter: "bitbucket-cli pr list --repo 'FORK/repo' --source 'feature' --state ALL --limit 25",
		FlavorCloud:      "bitbucket-cli pr list --repo 'PROJ/repo' --source 'feature' --state ALL --limit 25",
	} {
		if got := createPRWriteTarget(flavor, req).ReadSteps[0]; got != want {
			t.Errorf("%s read %q, want %q", flavor, got, want)
		}
	}
}

// Cloud filters pr list --source and --target server-side; both flags used to
// be ignored there, so the read after an uncertain create listed every PR.
func TestCloudPRListFiltersBranches(t *testing.T) {
	var q string
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	}))
	if _, err := c.ListPRs(context.Background(), PRListOpts{Repo: writeTestRepo, Source: "feature", Target: "main"}); err != nil {
		t.Fatal(err)
	}
	if q != `source.branch.name="feature" AND destination.branch.name="main"` {
		t.Fatalf("q = %q", q)
	}
}
