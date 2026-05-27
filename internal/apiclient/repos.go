package apiclient

import (
	"context"
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListRepositories lists repositories in a workspace (Cloud) or project (DC).
func (c *apiClient) ListRepositories(ctx context.Context, opt RepoListOpts) (ListResult[Repository], error) {
	if opt.Workspace == "" {
		return ListResult[Repository]{}, cerrors.New(cerrors.CategoryUsage, "REPO_NO_WORKSPACE",
			"a workspace (Cloud) or project key (Data Center) is required to list repositories").
			WithNextSteps(
				"bitbucket-cli workspace list   # discover available workspaces / projects",
				"Pass --workspace <slug>",
				"Set BITBUCKET_DEFAULT_WORKSPACE in your env or config",
			)
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	if opt.Role != "" && c.flavor == FlavorCloud {
		q.Set("role", opt.Role)
	}
	if opt.Query != "" && c.flavor == FlavorCloud {
		q.Set("q", opt.Query)
	}
	if opt.Sort != "" && c.flavor == FlavorCloud {
		q.Set("sort", opt.Sort)
	}

	path := c.reposPath(opt.Workspace)
	if c.flavor == FlavorCloud {
		// follow absolute next-URL if provided
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		var raw cloudRepoList
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[Repository]{}, err
		}
		res := ListResult[Repository]{Next: cloudNextCursor(raw.Next)}
		for _, r := range raw.Values {
			res.Items = append(res.Items, *mapCloudRepo(r))
		}
		return res, nil
	}
	var raw dcRepoList
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Repository]{}, err
	}
	res := ListResult[Repository]{Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage)}
	for _, r := range raw.Values {
		res.Items = append(res.Items, *mapDCRepo(r))
	}
	return res, nil
}

// GetRepository fetches a single repository.
func (c *apiClient) GetRepository(ctx context.Context, ref RepoRef) (*Repository, error) {
	if err := checkRepoRef(ref); err != nil {
		return nil, err
	}
	path := c.repoPath(ref)
	if c.flavor == FlavorCloud {
		var raw cloudRepo
		if err := c.getJSON(ctx, path, nil, &raw); err != nil {
			return nil, err
		}
		return mapCloudRepo(raw), nil
	}
	var raw dcRepo
	if err := c.getJSON(ctx, path, nil, &raw); err != nil {
		return nil, err
	}
	return mapDCRepo(raw), nil
}

// CreateRepository creates a repository. Cloud and DC use different payloads.
func (c *apiClient) CreateRepository(ctx context.Context, req CreateRepoReq) (*Repository, error) {
	if req.Workspace == "" || req.Slug == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "REPO_BAD_REQ",
			"workspace and slug are required")
	}
	method, path, payload := c.buildCreateRepo(req)
	if c.flavor == FlavorCloud {
		var raw cloudRepo
		if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
			return nil, err
		}
		return mapCloudRepo(raw), nil
	}
	var raw dcRepo
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	return mapDCRepo(raw), nil
}

func (c *apiClient) buildCreateRepo(req CreateRepoReq) (method, path string, payload any) {
	method = "POST"
	if c.flavor == FlavorCloud {
		// Cloud uses PUT to the target path with a slug embedded.
		method = "POST"
		path = c.apiBase() + "/repositories/" + url.PathEscape(req.Workspace) + "/" + url.PathEscape(req.Slug)
		// Cloud actually expects PUT for create; falling back to POST is invalid.
		method = "PUT"
		payload = map[string]any{
			"scm":         "git",
			"name":        defaultStr(req.Name, req.Slug),
			"description": req.Description,
			"is_private":  req.Private,
		}
		return
	}
	path = c.reposPath(req.Workspace)
	payload = map[string]any{
		"name":          defaultStr(req.Name, req.Slug),
		"scmId":         "git",
		"description":   req.Description,
		"public":        !req.Private,
	}
	return
}

// DeleteRepository deletes a repository.
func (c *apiClient) DeleteRepository(ctx context.Context, req DeleteRepoReq) error {
	if err := checkRepoRef(req.Repo); err != nil {
		return err
	}
	return c.doJSON(ctx, "DELETE", c.repoPath(req.Repo), nil, nil, nil)
}

// checkRepoRef validates a RepoRef has both workspace and slug set.
func checkRepoRef(ref RepoRef) error {
	if strings.TrimSpace(ref.Workspace) == "" || strings.TrimSpace(ref.Slug) == "" {
		return cerrors.New(cerrors.CategoryUsage, "REPO_BAD_REF",
			"a repository reference needs both workspace/project and slug").
			WithHint("Pass <workspace>/<repo> or a Bitbucket repository URL.")
	}
	return nil
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
