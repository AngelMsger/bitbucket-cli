package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/angelmsger/bitbucket-cli/pkg/constants"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

// Client is the flavor-agnostic Bitbucket API surface. All methods return
// normalized models; flavor-specific request shapes are hidden.
type Client interface {
	Flavor() Flavor
	BaseURL() string
	Ping(ctx context.Context) (ServerInfo, error)

	CurrentUser(ctx context.Context) (*User, error)

	ListWorkspaces(ctx context.Context, opt WorkspaceListOpts) (ListResult[Workspace], error)
	GetWorkspace(ctx context.Context, slug string) (*Workspace, error)

	ListUsers(ctx context.Context, opt UserListOpts) (ListResult[User], error)
	GetUser(ctx context.Context, selector string) (*User, error)

	ListTags(ctx context.Context, opt TagListOpts) (ListResult[Tag], error)
	GetTag(ctx context.Context, repo RepoRef, name string) (*Tag, error)

	ListRepositories(ctx context.Context, opt RepoListOpts) (ListResult[Repository], error)
	GetRepository(ctx context.Context, ref RepoRef) (*Repository, error)
	CreateRepository(ctx context.Context, req CreateRepoReq) (*Repository, error)
	DeleteRepository(ctx context.Context, req DeleteRepoReq) error

	ListPRs(ctx context.Context, opt PRListOpts) (ListResult[PullRequest], error)
	ListMyPRs(ctx context.Context, opt MyPRListOpts) (ListResult[PullRequest], error)
	GetPR(ctx context.Context, opt GetPROpts) (*PullRequest, error)
	GetPRDiff(ctx context.Context, repo RepoRef, id int) (string, error)
	ListPRCommits(ctx context.Context, opt PRListOpts) (ListResult[Commit], error)
	ListPRActivity(ctx context.Context, opt PRListOpts) (ListResult[Activity], error)

	CreatePR(ctx context.Context, req CreatePRReq) (*PullRequest, error)
	UpdatePR(ctx context.Context, req UpdatePRReq) (*PullRequest, error)
	DeclinePR(ctx context.Context, req DeclinePRReq) (*PullRequest, error)
	MergePR(ctx context.Context, req MergePRReq) (*PullRequest, error)
	ApprovePR(ctx context.Context, req ApprovePRReq) error
	RequestPRChanges(ctx context.Context, req RequestChangesReq) error

	ListPRComments(ctx context.Context, opt ListPRCommentsOpts) (ListResult[Comment], error)
	AddPRComment(ctx context.Context, req AddPRCommentReq) (*Comment, error)
	UpdatePRComment(ctx context.Context, req UpdatePRCommentReq) (*Comment, error)
	DeletePRComment(ctx context.Context, req DeletePRCommentReq) error
	ResolvePRComment(ctx context.Context, req ResolvePRCommentReq) (*Comment, error)

	ListBranches(ctx context.Context, opt BranchListOpts) (ListResult[Branch], error)
	GetBranch(ctx context.Context, repo RepoRef, name string) (*Branch, error)
	CreateBranch(ctx context.Context, req CreateBranchReq) (*Branch, error)
	DeleteBranch(ctx context.Context, req DeleteBranchReq) error

	GetCommit(ctx context.Context, repo RepoRef, hash string) (*Commit, error)
	ListCommits(ctx context.Context, opt ListCommitsOpts) (ListResult[Commit], error)
	CompareCommits(ctx context.Context, req CompareCommitsReq) (ListResult[Commit], error)

	// v0.2: source browsing.
	ListFiles(ctx context.Context, opt FileListOpts) (ListResult[FileEntry], error)
	GetFile(ctx context.Context, opt FileGetOpts) (*FileContent, error)
	Tree(ctx context.Context, opt TreeOpts) (ListResult[FileEntry], error)

	// v0.2: PR file-level operations.
	ListPRFiles(ctx context.Context, repo RepoRef, id int) (ListResult[Diffstat], error)
	GetPRDiffByPath(ctx context.Context, repo RepoRef, id int, path string) (string, error)
	GetPRFileDiffs(ctx context.Context, repo RepoRef, id int, path string) ([]FileDiff, error)
	ListPRThreads(ctx context.Context, repo RepoRef, id int) (ListResult[Thread], error)

	// v0.2: PR merge readiness + CI status.
	CheckPRMerge(ctx context.Context, repo RepoRef, id int) (*MergeCheck, error)
	ListCommitStatuses(ctx context.Context, repo RepoRef, hash string) (ListResult[BuildStatus], error)
	ListPRStatuses(ctx context.Context, repo RepoRef, id int) (ListResult[BuildStatus], error)
	GetPRStatus(ctx context.Context, repo RepoRef, id int) (*PRStatus, error)

	DescribeWrite(ctx context.Context, op any) (WriteRequestPlan, error)
}

// apiClient is the single Client implementation. Per-flavor behaviour is
// selected by the flavor field and the helpers in dialect.go / mapping.go.
type apiClient struct {
	flavor   Flavor
	baseURL  string
	pageSize int
	http     *transport.Client
}

// Config configures a Client.
type Config struct {
	Flavor    Flavor
	BaseURL   string
	PageSize  int
	Transport *transport.Client
}

// New builds a Client. The transport must already carry the auth decorator.
func New(cfg Config) Client {
	ps := cfg.PageSize
	if ps <= 0 {
		ps = constants.DefaultPageSize
	}
	if ps > constants.MaxPageSize {
		ps = constants.MaxPageSize
	}
	return &apiClient{
		flavor:   cfg.Flavor,
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		pageSize: ps,
		http:     cfg.Transport,
	}
}

func (c *apiClient) Flavor() Flavor  { return c.flavor }
func (c *apiClient) BaseURL() string { return c.baseURL }

// limitOf returns the effective page size for a ListOpts.
func (c *apiClient) limitOf(opt ListOpts) int {
	if opt.Limit > 0 {
		if opt.Limit > constants.MaxPageSize {
			return constants.MaxPageSize
		}
		return opt.Limit
	}
	return c.pageSize
}

// getJSON performs a GET and decodes the JSON body into out.
func (c *apiClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out)
}

// getDiffBody performs a GET against a diff endpoint and returns the response
// body together with its Content-Type. Bitbucket serves the PR diff as either
// unified-diff text or a JSON hunk model depending on the deployment, and some
// Data Center instances ignore the Accept header entirely — so the caller must
// inspect the actual content type rather than trust a requested format. We
// advertise a preference for text but explicitly accept JSON so a JSON-only
// server still returns 200 instead of 406.
func (c *apiClient) getDiffBody(ctx context.Context, path string, query url.Values) (body, contentType string, err error) {
	endpoint := c.absEndpoint(path, query)
	req, rerr := http.NewRequest(http.MethodGet, endpoint, nil)
	if rerr != nil {
		return "", "", cerrors.Wrap(rerr, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	req.Header.Set("Accept", "text/plain, application/json")
	resp, derr := c.http.Do(ctx, req)
	if derr != nil {
		return "", "", cerrors.Wrap(derr, cerrors.CategoryNetwork, "NETWORK",
			fmt.Sprintf("request to %s failed", endpoint))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", c.httpError(resp)
	}
	raw, _ := io.ReadAll(resp.Body)
	return string(raw), resp.Header.Get("Content-Type"), nil
}

// doJSON performs an HTTP request and decodes a JSON response into out.
// Non-2xx responses are converted into structured *errors.CLIError values.
func (c *apiClient) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	endpoint := c.absEndpoint(path, query)

	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return cerrors.Wrap(err, cerrors.CategoryInternal, "ENCODE", "failed to encode request body")
		}
		reqBody = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryNetwork, "NETWORK",
			fmt.Sprintf("request to %s failed", endpoint))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.httpError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	rawResp, _ := io.ReadAll(resp.Body)
	if len(bytes.TrimSpace(rawResp)) == 0 {
		return nil
	}
	return decodeJSON(rawResp, out)
}

func (c *apiClient) absEndpoint(path string, query url.Values) string {
	endpoint := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		endpoint = c.baseURL + path
	}
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(endpoint, "?") {
			sep = "&"
		}
		endpoint += sep + query.Encode()
	}
	return endpoint
}

// decodeJSON unmarshals a server response body into out, surfacing parse errors
// with a body snippet so a shape mismatch is diagnosable.
func decodeJSON(body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return cerrors.Wrap(err, cerrors.CategoryParse, "DECODE",
			fmt.Sprintf("could not decode the server response: %v", err)).
			WithHint("The server's JSON did not match what bitbucket-cli expected; " +
				"this is likely a client bug, not a failed request.").
			WithNextSteps("Report it with this snippet: " + snippet)
	}
	return nil
}

// httpError turns a non-2xx response into a classified CLIError.
func (c *apiClient) httpError(resp *http.Response) error {
	cat := cerrors.FromHTTPStatus(resp.StatusCode)
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := fmt.Sprintf("Bitbucket returned HTTP %d", resp.StatusCode)
	if detail := extractAPIMessage(snippet); detail != "" {
		msg += ": " + detail
	}
	return cerrors.New(cat, "HTTP_"+http.StatusText(resp.StatusCode), msg).
		WithHTTPStatus(resp.StatusCode)
}

// extractAPIMessage best-effort extracts a human message from a Bitbucket JSON
// error body. Cloud returns {"error": {"message": "..."}}; Data Center returns
// {"errors": [{"message": "...", "exceptionName": "..."}]}.
func extractAPIMessage(raw []byte) string {
	var cloud struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &cloud) == nil && cloud.Error.Message != "" {
		return cloud.Error.Message
	}
	var dc struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(raw, &dc) == nil && len(dc.Errors) > 0 && dc.Errors[0].Message != "" {
		return dc.Errors[0].Message
	}
	return ""
}
