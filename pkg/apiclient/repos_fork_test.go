package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

// forkRecorder captures the request a fork would send and replies with a
// plausible repository record.
type forkRecorder struct {
	method string
	path   string
	body   map[string]any
}

func newForkTestClient(t *testing.T, flavor Flavor, rec *forkRecorder, reply string) Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)

	return New(Config{
		Flavor:    flavor,
		BaseURL:   srv.URL,
		Transport: transport.New(transport.Options{}),
	})
}

const dcForkReply = `{"slug":"fx-code","name":"fx-code","project":{"key":"~ALICE"},
  "links":{"clone":[{"name":"ssh","href":"ssh://git@host:7999/~alice/fx-code.git"}]}}`

// Data Center overloads POST on the repository's own path to mean "fork this".
// An empty body forks into the caller's personal project under the same name,
// which is the case a fork-and-pull workflow hits every time.
func TestForkRepositoryDataCenterDefaultsToPersonalArea(t *testing.T) {
	t.Parallel()
	var rec forkRecorder
	c := newForkTestClient(t, FlavorDataCenter, &rec, dcForkReply)

	repo, err := c.ForkRepository(context.Background(), ForkRepoReq{
		Source: RepoRef{Workspace: "FX", Slug: "fx-code"},
	})
	if err != nil {
		t.Fatalf("ForkRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("ForkRepository returned no repository")
	}

	if rec.method != http.MethodPost {
		t.Errorf("method = %q, want POST", rec.method)
	}
	if want := "/rest/api/1.0/projects/FX/repos/fx-code"; rec.path != want {
		t.Errorf("path = %q, want %q", rec.path, want)
	}
	// Nothing is sent that was not asked for: the server picks the caller's
	// personal project rather than this client guessing its name.
	if len(rec.body) != 0 {
		t.Errorf("body = %v, want empty so the server applies its own default", rec.body)
	}
}

func TestForkRepositoryDataCenterHonoursTarget(t *testing.T) {
	t.Parallel()
	var rec forkRecorder
	c := newForkTestClient(t, FlavorDataCenter, &rec, dcForkReply)

	_, err := c.ForkRepository(context.Background(), ForkRepoReq{
		Source:    RepoRef{Workspace: "FX", Slug: "fx-code"},
		Workspace: "~alice",
		Name:      "fx-code-experiment",
	})
	if err != nil {
		t.Fatalf("ForkRepository: %v", err)
	}

	if got := rec.body["name"]; got != "fx-code-experiment" {
		t.Errorf("body.name = %v, want the requested fork name", got)
	}
	project, ok := rec.body["project"].(map[string]any)
	if !ok {
		t.Fatalf("body.project = %v, want an object naming the target project", rec.body["project"])
	}
	if got := project["key"]; got != "~alice" {
		t.Errorf("body.project.key = %v, want ~alice", got)
	}
}

func TestForkRepositoryCloudRequiresTargetWorkspace(t *testing.T) {
	t.Parallel()
	var rec forkRecorder
	c := newForkTestClient(t, FlavorCloud, &rec,
		`{"slug":"repo","full_name":"alice/repo","is_private":true}`)

	_, err := c.ForkRepository(context.Background(), ForkRepoReq{
		Source: RepoRef{Workspace: "team", Slug: "repo"},
	})
	if err == nil {
		t.Fatal("Cloud fork without a destination workspace should fail locally")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "REPO_FORK_NO_WORKSPACE" {
		t.Errorf("code = %q, want REPO_FORK_NO_WORKSPACE", ce.Code)
	}
	if len(ce.NextSteps) == 0 || ce.NextSteps[0] != "bitbucket-cli workspace list   # discover available workspaces" {
		t.Errorf("next_steps = %v, want workspace discovery first", ce.NextSteps)
	}
	if rec.method != "" {
		t.Errorf("request method = %q, want no HTTP request", rec.method)
	}
}

func TestForkRepositoryCloudSameWorkspaceRequiresName(t *testing.T) {
	t.Parallel()
	var rec forkRecorder
	c := newForkTestClient(t, FlavorCloud, &rec,
		`{"slug":"repo-copy","full_name":"team/repo-copy","is_private":true}`)

	_, err := c.ForkRepository(context.Background(), ForkRepoReq{
		Source:    RepoRef{Workspace: "team", Slug: "repo"},
		Workspace: "TEAM",
	})
	if err == nil {
		t.Fatal("Cloud fork into the source workspace without a new name should fail locally")
	}
	if got := cerrors.AsCLIError(err).Code; got != "REPO_FORK_NAME_REQUIRED" {
		t.Errorf("code = %q, want REPO_FORK_NAME_REQUIRED", got)
	}
	if rec.method != "" {
		t.Errorf("request method = %q, want no HTTP request", rec.method)
	}
}

// Cloud has a dedicated sub-resource rather than overloading POST.
func TestForkRepositoryCloudUsesForksSubresource(t *testing.T) {
	t.Parallel()
	var rec forkRecorder
	c := newForkTestClient(t, FlavorCloud, &rec,
		`{"slug":"repo","full_name":"alice/repo","is_private":true}`)

	_, err := c.ForkRepository(context.Background(), ForkRepoReq{
		Source:    RepoRef{Workspace: "team", Slug: "repo"},
		Workspace: "alice",
	})
	if err != nil {
		t.Fatalf("ForkRepository: %v", err)
	}

	if want := "/2.0/repositories/team/repo/forks"; rec.path != want {
		t.Errorf("path = %q, want %q", rec.path, want)
	}
	workspace, ok := rec.body["workspace"].(map[string]any)
	if !ok {
		t.Fatalf("body.workspace = %v, want an object naming the target workspace", rec.body["workspace"])
	}
	if got := workspace["slug"]; got != "alice" {
		t.Errorf("body.workspace.slug = %v, want alice", got)
	}
}

// A reference missing its workspace fails locally, before any request is sent.
func TestForkRepositoryRejectsAnIncompleteReference(t *testing.T) {
	t.Parallel()
	c := newForkTestClient(t, FlavorDataCenter, &forkRecorder{}, dcForkReply)

	_, err := c.ForkRepository(context.Background(), ForkRepoReq{
		Source: RepoRef{Slug: "fx-code"},
	})
	if err == nil {
		t.Fatal("a reference with no workspace should be rejected")
	}
	if got := cerrors.AsCLIError(err).Category; got != cerrors.CategoryUsage {
		t.Errorf("category = %q, want usage", got)
	}
}

// --dry-run must build its preview through the same path the live request
// uses, or the preview can drift from what would actually be sent.
func TestDescribeWriteCoversFork(t *testing.T) {
	t.Parallel()
	c := newForkTestClient(t, FlavorDataCenter, &forkRecorder{}, dcForkReply)

	plan, err := c.DescribeWrite(context.Background(), ForkRepoReq{
		Source:    RepoRef{Workspace: "FX", Slug: "fx-code"},
		Workspace: "~alice",
	})
	if err != nil {
		t.Fatalf("DescribeWrite: %v", err)
	}
	if plan.Method != http.MethodPost {
		t.Errorf("plan.Method = %q, want POST", plan.Method)
	}
	if plan.URL == "" {
		t.Error("plan.URL is empty")
	}
	payload, ok := plan.Payload.(map[string]any)
	if !ok {
		t.Fatalf("plan.Payload = %T, want a map", plan.Payload)
	}
	if _, ok := payload["project"]; !ok {
		t.Errorf("plan.Payload = %v, should carry the target project", payload)
	}
}

func TestDescribeWriteValidatesForkLikeLiveExecution(t *testing.T) {
	t.Parallel()
	c := newForkTestClient(t, FlavorCloud, &forkRecorder{}, `{}`)

	_, err := c.DescribeWrite(context.Background(), ForkRepoReq{
		Source: RepoRef{Workspace: "team", Slug: "repo"},
	})
	if err == nil {
		t.Fatal("Cloud fork dry-run without --into should fail like live execution")
	}
	if got := cerrors.AsCLIError(err).Code; got != "REPO_FORK_NO_WORKSPACE" {
		t.Errorf("code = %q, want REPO_FORK_NO_WORKSPACE", got)
	}
}
