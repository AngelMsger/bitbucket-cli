package apiclient

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/angelmsger/bitbucket-cli/pkg/transport"
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

// assertCoversMethods fails unless names is exactly the method set of iface,
// so a table test cannot silently skip a method added to the interface.
func assertCoversMethods(t *testing.T, iface reflect.Type, names []string) {
	t.Helper()
	var want []string
	for i := range iface.NumMethod() {
		want = append(want, iface.Method(i).Name)
	}
	got := slices.Sorted(slices.Values(names))
	if !slices.Equal(got, want) {
		t.Fatalf("table covers %v, %s declares %v", got, iface.Name(), want)
	}
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
		{"ForkRepository", func() error {
			_, err := c.ForkRepository(ctx, ForkRepoReq{Source: repo, Workspace: "target"})
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
	var names []string
	for _, tc := range cases {
		names = append(names, tc.name)
	}
	assertCoversMethods(t, reflect.TypeFor[Writer](), names)

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
			if !strings.Contains(ce.Message, `"`+tc.name+`"`) {
				t.Errorf("%s: message %q does not name the operation", tc.name, ce.Message)
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

// readCall is one call a recordingReader received.
type readCall struct {
	method string
	args   []any
}

// errCannedRead is returned alongside every canned read result, so delegation
// tests also see errors pass through unchanged.
var errCannedRead = errors.New("canned read error")

// recordingReader answers every Reader method with a fixed result and records
// the call. It embeds Client only to satisfy NewReadOnly's parameter type: a
// write that reached it would panic on the nil embedded interface.
type recordingReader struct {
	Client
	calls []readCall
}

func (r *recordingReader) record(method string, args ...any) {
	r.calls = append(r.calls, readCall{method: method, args: args})
}

func (r *recordingReader) Flavor() Flavor {
	r.record("Flavor")
	return FlavorCloud
}

func (r *recordingReader) BaseURL() string {
	r.record("BaseURL")
	return "BaseURL"
}

func (r *recordingReader) Ping(ctx context.Context) (ServerInfo, error) {
	r.record("Ping", ctx)
	return ServerInfo{}, errCannedRead
}

func (r *recordingReader) CurrentUser(ctx context.Context) (*User, error) {
	r.record("CurrentUser", ctx)
	return &User{}, errCannedRead
}

func (r *recordingReader) ListWorkspaces(ctx context.Context, opt WorkspaceListOpts) (ListResult[Workspace], error) {
	r.record("ListWorkspaces", ctx, opt)
	return ListResult[Workspace]{Next: "ListWorkspaces"}, errCannedRead
}

func (r *recordingReader) GetWorkspace(ctx context.Context, slug string) (*Workspace, error) {
	r.record("GetWorkspace", ctx, slug)
	return &Workspace{}, errCannedRead
}

func (r *recordingReader) ListUsers(ctx context.Context, opt UserListOpts) (ListResult[User], error) {
	r.record("ListUsers", ctx, opt)
	return ListResult[User]{Next: "ListUsers"}, errCannedRead
}

func (r *recordingReader) GetUser(ctx context.Context, selector string) (*User, error) {
	r.record("GetUser", ctx, selector)
	return &User{}, errCannedRead
}

func (r *recordingReader) ListTags(ctx context.Context, opt TagListOpts) (ListResult[Tag], error) {
	r.record("ListTags", ctx, opt)
	return ListResult[Tag]{Next: "ListTags"}, errCannedRead
}

func (r *recordingReader) GetTag(ctx context.Context, repo RepoRef, name string) (*Tag, error) {
	r.record("GetTag", ctx, repo, name)
	return &Tag{}, errCannedRead
}

func (r *recordingReader) ListRepositories(ctx context.Context, opt RepoListOpts) (ListResult[Repository], error) {
	r.record("ListRepositories", ctx, opt)
	return ListResult[Repository]{Next: "ListRepositories"}, errCannedRead
}

func (r *recordingReader) GetRepository(ctx context.Context, ref RepoRef) (*Repository, error) {
	r.record("GetRepository", ctx, ref)
	return &Repository{}, errCannedRead
}

func (r *recordingReader) ListPRs(ctx context.Context, opt PRListOpts) (ListResult[PullRequest], error) {
	r.record("ListPRs", ctx, opt)
	return ListResult[PullRequest]{Next: "ListPRs"}, errCannedRead
}

func (r *recordingReader) ListMyPRs(ctx context.Context, opt MyPRListOpts) (ListResult[PullRequest], error) {
	r.record("ListMyPRs", ctx, opt)
	return ListResult[PullRequest]{Next: "ListMyPRs"}, errCannedRead
}

func (r *recordingReader) GetPR(ctx context.Context, opt GetPROpts) (*PullRequest, error) {
	r.record("GetPR", ctx, opt)
	return &PullRequest{}, errCannedRead
}

func (r *recordingReader) GetPRDiff(ctx context.Context, repo RepoRef, id int) (string, error) {
	r.record("GetPRDiff", ctx, repo, id)
	return "GetPRDiff", errCannedRead
}

func (r *recordingReader) ListPRCommits(ctx context.Context, opt PRListOpts) (ListResult[Commit], error) {
	r.record("ListPRCommits", ctx, opt)
	return ListResult[Commit]{Next: "ListPRCommits"}, errCannedRead
}

func (r *recordingReader) ListPRActivity(ctx context.Context, opt PRListOpts) (ListResult[Activity], error) {
	r.record("ListPRActivity", ctx, opt)
	return ListResult[Activity]{Next: "ListPRActivity"}, errCannedRead
}

func (r *recordingReader) ListPRComments(ctx context.Context, opt ListPRCommentsOpts) (ListResult[Comment], error) {
	r.record("ListPRComments", ctx, opt)
	return ListResult[Comment]{Next: "ListPRComments"}, errCannedRead
}

func (r *recordingReader) ListBranches(ctx context.Context, opt BranchListOpts) (ListResult[Branch], error) {
	r.record("ListBranches", ctx, opt)
	return ListResult[Branch]{Next: "ListBranches"}, errCannedRead
}

func (r *recordingReader) GetBranch(ctx context.Context, repo RepoRef, name string) (*Branch, error) {
	r.record("GetBranch", ctx, repo, name)
	return &Branch{}, errCannedRead
}

func (r *recordingReader) GetCommit(ctx context.Context, repo RepoRef, hash string) (*Commit, error) {
	r.record("GetCommit", ctx, repo, hash)
	return &Commit{}, errCannedRead
}

func (r *recordingReader) ListCommits(ctx context.Context, opt ListCommitsOpts) (ListResult[Commit], error) {
	r.record("ListCommits", ctx, opt)
	return ListResult[Commit]{Next: "ListCommits"}, errCannedRead
}

func (r *recordingReader) CompareCommits(ctx context.Context, req CompareCommitsReq) (ListResult[Commit], error) {
	r.record("CompareCommits", ctx, req)
	return ListResult[Commit]{Next: "CompareCommits"}, errCannedRead
}

func (r *recordingReader) ListFiles(ctx context.Context, opt FileListOpts) (ListResult[FileEntry], error) {
	r.record("ListFiles", ctx, opt)
	return ListResult[FileEntry]{Next: "ListFiles"}, errCannedRead
}

func (r *recordingReader) GetFile(ctx context.Context, opt FileGetOpts) (*FileContent, error) {
	r.record("GetFile", ctx, opt)
	return &FileContent{}, errCannedRead
}

func (r *recordingReader) Tree(ctx context.Context, opt TreeOpts) (ListResult[FileEntry], error) {
	r.record("Tree", ctx, opt)
	return ListResult[FileEntry]{Next: "Tree"}, errCannedRead
}

func (r *recordingReader) ListPRFiles(ctx context.Context, repo RepoRef, id int) (ListResult[Diffstat], error) {
	r.record("ListPRFiles", ctx, repo, id)
	return ListResult[Diffstat]{Next: "ListPRFiles"}, errCannedRead
}

func (r *recordingReader) GetPRDiffByPath(ctx context.Context, repo RepoRef, id int, path string) (string, error) {
	r.record("GetPRDiffByPath", ctx, repo, id, path)
	return "GetPRDiffByPath", errCannedRead
}

func (r *recordingReader) GetPRFileDiffs(ctx context.Context, repo RepoRef, id int, path string) ([]FileDiff, error) {
	r.record("GetPRFileDiffs", ctx, repo, id, path)
	return []FileDiff{{}}, errCannedRead
}

func (r *recordingReader) ListPRThreads(ctx context.Context, repo RepoRef, id int) (ListResult[Thread], error) {
	r.record("ListPRThreads", ctx, repo, id)
	return ListResult[Thread]{Next: "ListPRThreads"}, errCannedRead
}

func (r *recordingReader) CheckPRMerge(ctx context.Context, repo RepoRef, id int) (*MergeCheck, error) {
	r.record("CheckPRMerge", ctx, repo, id)
	return &MergeCheck{}, errCannedRead
}

func (r *recordingReader) ListCommitStatuses(ctx context.Context, repo RepoRef, hash string) (ListResult[BuildStatus], error) {
	r.record("ListCommitStatuses", ctx, repo, hash)
	return ListResult[BuildStatus]{Next: "ListCommitStatuses"}, errCannedRead
}

func (r *recordingReader) ListPRStatuses(ctx context.Context, repo RepoRef, id int) (ListResult[BuildStatus], error) {
	r.record("ListPRStatuses", ctx, repo, id)
	return ListResult[BuildStatus]{Next: "ListPRStatuses"}, errCannedRead
}

func (r *recordingReader) GetPRStatus(ctx context.Context, repo RepoRef, id int) (*PRStatus, error) {
	r.record("GetPRStatus", ctx, repo, id)
	return &PRStatus{}, errCannedRead
}

func (r *recordingReader) DescribeWrite(ctx context.Context, op any) (WriteRequestPlan, error) {
	r.record("DescribeWrite", ctx, op)
	return WriteRequestPlan{}, errCannedRead
}

type delegationKey struct{}

// TestReadOnlyDelegatesEveryRead calls every Reader method through the
// wrapper and asserts the inner client received exactly that one call with the
// same arguments, and that its results and error came back unchanged.
func TestReadOnlyDelegatesEveryRead(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), delegationKey{}, "read-only delegation")
	page := ListOpts{Limit: 7, Cursor: "c1"}
	repo := RepoRef{Workspace: "WS", Slug: "repo"}
	// argsFor builds fresh arguments on every call, so the expected values are
	// independent of the ones handed to the wrapper: a wrapper that mutated an
	// argument in place before forwarding it would otherwise change the
	// expectation along with it.
	argsFor := func() map[string][]any {
		return map[string][]any{
			"Flavor":             nil,
			"BaseURL":            nil,
			"Ping":               {ctx},
			"CurrentUser":        {ctx},
			"ListWorkspaces":     {ctx, WorkspaceListOpts{ListOpts: page, Role: "role", Query: "query"}},
			"GetWorkspace":       {ctx, "slug"},
			"ListUsers":          {ctx, UserListOpts{ListOpts: page, Workspace: "workspace", Query: "query"}},
			"GetUser":            {ctx, "selector"},
			"ListTags":           {ctx, TagListOpts{ListOpts: page, Repo: repo, Query: "query"}},
			"GetTag":             {ctx, repo, "name"},
			"ListRepositories":   {ctx, RepoListOpts{ListOpts: page, Workspace: "workspace", Role: "role"}},
			"GetRepository":      {ctx, repo},
			"ListPRs":            {ctx, PRListOpts{ListOpts: page, Repo: repo, State: "state"}},
			"ListMyPRs":          {ctx, MyPRListOpts{ListOpts: page, Role: "role", State: "state"}},
			"GetPR":              {ctx, GetPROpts{Repo: repo, ID: 7}},
			"GetPRDiff":          {ctx, repo, 7},
			"ListPRCommits":      {ctx, PRListOpts{ListOpts: page, Repo: repo, State: "state"}},
			"ListPRActivity":     {ctx, PRListOpts{ListOpts: page, Repo: repo, State: "state"}},
			"ListPRComments":     {ctx, ListPRCommentsOpts{ListOpts: page, Repo: repo, PRID: 7}},
			"ListBranches":       {ctx, BranchListOpts{ListOpts: page, Repo: repo, Query: "query"}},
			"GetBranch":          {ctx, repo, "name"},
			"GetCommit":          {ctx, repo, "hash"},
			"ListCommits":        {ctx, ListCommitsOpts{ListOpts: page, Repo: repo, Branch: "branch"}},
			"CompareCommits":     {ctx, CompareCommitsReq{ListOpts: page, Repo: repo, From: "from"}},
			"ListFiles":          {ctx, FileListOpts{ListOpts: page, Repo: repo, Ref: "ref"}},
			"GetFile":            {ctx, FileGetOpts{Repo: repo, Ref: "ref", Path: "path"}},
			"Tree":               {ctx, TreeOpts{Repo: repo, Ref: "ref", Path: "path"}},
			"ListPRFiles":        {ctx, repo, 7},
			"GetPRDiffByPath":    {ctx, repo, 7, "path"},
			"GetPRFileDiffs":     {ctx, repo, 7, "path"},
			"ListPRThreads":      {ctx, repo, 7},
			"CheckPRMerge":       {ctx, repo, 7},
			"ListCommitStatuses": {ctx, repo, "hash"},
			"ListPRStatuses":     {ctx, repo, 7},
			"GetPRStatus":        {ctx, repo, 7},
			"DescribeWrite":      {ctx, DeleteBranchReq{Repo: repo, Name: "feature/x"}},
		}
	}
	assertCoversMethods(t, reflect.TypeFor[Reader](), slices.Collect(maps.Keys(argsFor())))

	call := func(c Client, method string, args []any) []any {
		in := make([]reflect.Value, len(args))
		for i, a := range args {
			in[i] = reflect.ValueOf(a)
		}
		var out []any
		for _, v := range reflect.ValueOf(c).MethodByName(method).Call(in) {
			out = append(out, v.Interface())
		}
		return out
	}

	for method := range argsFor() {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			inner := &recordingReader{}
			got := call(NewReadOnly(inner), method, argsFor()[method])
			expected := argsFor()[method]
			if want := call(&recordingReader{}, method, expected); !reflect.DeepEqual(got, want) {
				t.Errorf("results = %#v, want the inner client's %#v", got, want)
			}
			want := []readCall{{method: method, args: expected}}
			if !reflect.DeepEqual(inner.calls, want) {
				t.Errorf("inner client received %#v, want %#v", inner.calls, want)
			}
		})
	}
}
