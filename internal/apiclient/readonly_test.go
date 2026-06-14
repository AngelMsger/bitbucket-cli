package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/angelmsger/bitbucket-cli/internal/transport"
)

// newReadOnlyTestClient builds a Data Center flavored client wrapped in the
// read-only enforcement layer, pointed at handler. The handler is expected to
// never fire when the client's mutating methods are exercised.
func newReadOnlyTestClient(t *testing.T, handler http.Handler) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	inner := New(Config{
		Flavor:    FlavorDataCenter,
		BaseURL:   srv.URL,
		Transport: transport.New(transport.Options{}),
	})
	return NewReadOnly(inner)
}

// TestReadOnlyBlocksEveryMutator drives every mutating method on the wrapper
// and asserts each one returns a READONLY_BLOCKED permission error before any
// HTTP request is sent.
func TestReadOnlyBlocksEveryMutator(t *testing.T) {
	t.Parallel()
	c := newReadOnlyTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("read-only wrapper sent an HTTP request: %s %s", r.Method, r.URL.Path)
	}))
	ctx := context.Background()
	repo := RepoRef{Workspace: "WS", Slug: "repo"}

	cases := []struct {
		name string
		fn   func() error
	}{
		{"CreateRepository", func() error {
			_, err := c.CreateRepository(ctx, CreateRepoReq{Workspace: "WS", Slug: "x"})
			return err
		}},
		{"DeleteRepository", func() error { return c.DeleteRepository(ctx, DeleteRepoReq{Repo: repo}) }},
		{"CreatePR", func() error {
			_, err := c.CreatePR(ctx, CreatePRReq{Repo: repo, Title: "t", Source: "feat"})
			return err
		}},
		{"UpdatePR", func() error { _, err := c.UpdatePR(ctx, UpdatePRReq{Repo: repo, ID: 1, Title: "x"}); return err }},
		{"DeclinePR", func() error { _, err := c.DeclinePR(ctx, DeclinePRReq{Repo: repo, ID: 1}); return err }},
		{"MergePR", func() error { _, err := c.MergePR(ctx, MergePRReq{Repo: repo, ID: 1}); return err }},
		{"ApprovePR", func() error { return c.ApprovePR(ctx, ApprovePRReq{Repo: repo, ID: 1, Approve: true}) }},
		{"RequestPRChanges", func() error {
			return c.RequestPRChanges(ctx, RequestChangesReq{Repo: repo, ID: 1, Request: true})
		}},
		{"AddPRComment", func() error {
			_, err := c.AddPRComment(ctx, AddPRCommentReq{Repo: repo, PRID: 1, Content: "x"})
			return err
		}},
		{"UpdatePRComment", func() error {
			_, err := c.UpdatePRComment(ctx, UpdatePRCommentReq{Repo: repo, PRID: 1, ID: 1, Content: "y"})
			return err
		}},
		{"DeletePRComment", func() error {
			return c.DeletePRComment(ctx, DeletePRCommentReq{Repo: repo, PRID: 1, ID: 1})
		}},
		{"ResolvePRComment", func() error {
			_, err := c.ResolvePRComment(ctx, ResolvePRCommentReq{Repo: repo, PRID: 1, ID: 1, Resolve: true})
			return err
		}},
		{"CreateBranch", func() error {
			_, err := c.CreateBranch(ctx, CreateBranchReq{Repo: repo, Name: "feat", FromRef: "main"})
			return err
		}},
		{"DeleteBranch", func() error { return c.DeleteBranch(ctx, DeleteBranchReq{Repo: repo, Name: "feat"}) }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.fn()
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			ce := cerrors.AsCLIError(err)
			if ce.Category != cerrors.CategoryPermission {
				t.Errorf("%s: category = %s, want permission", tc.name, ce.Category)
			}
			if ce.Code != "READONLY_BLOCKED" {
				t.Errorf("%s: code = %s, want READONLY_BLOCKED", tc.name, ce.Code)
			}
			if !strings.Contains(strings.Join(ce.NextSteps, " "), "--allow-writes") {
				t.Errorf("%s: next_steps missing --allow-writes hint: %v", tc.name, ce.NextSteps)
			}
		})
	}
}

// TestReadOnlyAllowsDescribeWrite verifies that --dry-run (DescribeWrite) is
// not blocked by the wrapper, even though the underlying op is a write.
func TestReadOnlyAllowsDescribeWrite(t *testing.T) {
	t.Parallel()
	c := newReadOnlyTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("DescribeWrite should not send HTTP: %s %s", r.Method, r.URL.Path)
	}))
	plan, err := c.DescribeWrite(context.Background(), DeleteRepoReq{
		Repo: RepoRef{Workspace: "WS", Slug: "repo"},
	})
	if err != nil {
		t.Fatalf("DescribeWrite under read-only failed: %v", err)
	}
	if plan.Method != "DELETE" {
		t.Errorf("plan.Method = %q, want DELETE", plan.Method)
	}
	if !strings.Contains(plan.URL, "/repos/repo") {
		t.Errorf("plan.URL = %q, want substring /repos/repo", plan.URL)
	}
}
