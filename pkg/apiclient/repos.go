package apiclient

import (
	"context"
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
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
	res := ListResult[Repository]{Next: nextOffsetToken(raw.dcPage)}
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
		"name":        defaultStr(req.Name, req.Slug),
		"scmId":       "git",
		"description": req.Description,
		"public":      !req.Private,
	}
	return
}

// ForkRepository forks a repository. Data Center may infer the caller's
// personal project; Cloud requires an explicit destination workspace.
func (c *apiClient) ForkRepository(ctx context.Context, req ForkRepoReq) (*Repository, error) {
	if err := c.validateForkRepo(req); err != nil {
		return nil, err
	}
	method, path, payload := c.buildForkRepo(req)

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

// buildForkRepo renders the fork request for either backend.
//
// Data Center overloads POST on the repository's own path to mean "fork this";
// an empty body forks into the caller's personal project under the same name.
// Cloud has a dedicated /forks sub-resource and requires workspace.slug.
func (c *apiClient) buildForkRepo(req ForkRepoReq) (method, path string, payload any) {
	method = "POST"
	path = c.forkRepoPath(req.Source)
	body := map[string]any{}

	if c.flavor == FlavorCloud {
		if req.Name != "" {
			body["name"] = req.Name
		}
		if req.Workspace != "" {
			body["workspace"] = map[string]any{"slug": req.Workspace}
		}
		return method, path, body
	}

	if req.Name != "" {
		body["name"] = req.Name
	}
	if req.Workspace != "" {
		body["project"] = map[string]any{"key": req.Workspace}
	}
	return method, path, body
}

// validateForkRepo keeps live execution and DescribeWrite on the same
// flavor-aware validation path. Data Center can infer the caller's personal
// project; Cloud deliberately cannot and requires an explicit workspace.
func (c *apiClient) validateForkRepo(req ForkRepoReq) error {
	if err := checkRepoRef(req.Source); err != nil {
		return err
	}
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		if sup := c.supportFor(CapRepoForkImplicitTarget); !sup.Supported() {
			return cerrors.New(cerrors.CategoryUsage, "REPO_FORK_NO_WORKSPACE",
				"a destination workspace is required to fork a repository on Bitbucket Cloud: "+sup.Reason).
				WithHint("Discover a writable workspace, then pass it with --into.").
				WithNextSteps(
					"bitbucket-cli workspace list   # discover available workspaces",
					"bitbucket-cli repo fork <workspace>/<repo> --into <workspace>",
				)
		}
		return nil
	}
	if c.flavor == FlavorCloud && strings.EqualFold(workspace, strings.TrimSpace(req.Source.Workspace)) &&
		strings.TrimSpace(req.Name) == "" {
		return cerrors.New(cerrors.CategoryUsage, "REPO_FORK_NAME_REQUIRED",
			"--name is required when forking into the source repository's Cloud workspace").
			WithHint("Choose a distinct fork name so its generated slug does not collide with the source repository.").
			WithNextSteps(
				"bitbucket-cli repo fork <workspace>/<repo> --into <workspace> --name <new-name>",
			)
	}
	return nil
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
