package apiclient

import (
	"context"
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
