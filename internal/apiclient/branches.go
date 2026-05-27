package apiclient

import (
	"context"
	"net/url"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListBranches lists the branches of a repository.
func (c *apiClient) ListBranches(ctx context.Context, opt BranchListOpts) (ListResult[Branch], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[Branch]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	path := c.branchesPath(opt.Repo)

	if c.flavor == FlavorCloud {
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		if opt.Query != "" {
			if q == nil {
				q = url.Values{}
			}
			q.Set("q", `name~"`+opt.Query+`"`)
		}
		if opt.Sort != "" {
			if q == nil {
				q = url.Values{}
			}
			q.Set("sort", opt.Sort)
		}
		// Fetch repo to learn default branch name (needed to flag entries).
		defaultName := ""
		if repo, err := c.GetRepository(ctx, opt.Repo); err == nil && repo != nil {
			defaultName = repo.DefaultBranch
		}
		var raw cloudBranchList
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[Branch]{}, err
		}
		res := ListResult[Branch]{Next: cloudNextCursor(raw.Next)}
		for _, b := range raw.Values {
			res.Items = append(res.Items, mapCloudBranch(b, defaultName))
		}
		return res, nil
	}
	if opt.Query != "" {
		q.Set("filterText", opt.Query)
	}
	if opt.Sort != "" {
		q.Set("orderBy", opt.Sort)
	}
	var raw dcBranchList
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Branch]{}, err
	}
	res := ListResult[Branch]{Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage)}
	for _, b := range raw.Values {
		res.Items = append(res.Items, mapDCBranch(b))
	}
	return res, nil
}

// GetBranch returns a single branch by name.
func (c *apiClient) GetBranch(ctx context.Context, repo RepoRef, name string) (*Branch, error) {
	if err := checkRepoRef(repo); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "BRANCH_NO_NAME", "branch name is required")
	}
	// Both flavors offer "filter" / "filterText" — we list with the filter and
	// pick the exact match.
	listed, err := c.ListBranches(ctx, BranchListOpts{Repo: repo, Query: name, ListOpts: ListOpts{Limit: 50}})
	if err != nil {
		return nil, err
	}
	for _, b := range listed.Items {
		if b.Name == name {
			return &b, nil
		}
	}
	return nil, cerrors.New(cerrors.CategoryNotFound, "BRANCH_NOT_FOUND",
		"branch not found").
		WithHTTPStatus(404)
}

// CreateBranch creates a new branch from a ref.
func (c *apiClient) CreateBranch(ctx context.Context, req CreateBranchReq) (*Branch, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	method, path, payload := c.buildCreateBranch(req)
	if c.flavor == FlavorCloud {
		var raw cloudBranch
		if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
			return nil, err
		}
		b := mapCloudBranch(raw, "")
		return &b, nil
	}
	var raw dcBranch
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	b := mapDCBranch(raw)
	return &b, nil
}

func (c *apiClient) buildCreateBranch(req CreateBranchReq) (method, path string, payload any) {
	method = "POST"
	if c.flavor == FlavorCloud {
		path = c.branchesPath(req.Repo)
		payload = map[string]any{
			"name":   req.Name,
			"target": map[string]string{"hash": req.FromRef},
		}
		return
	}
	// DC uses the branch-utils plugin endpoint at /branch-utils/1.0.
	path = "/rest/branch-utils/1.0/projects/" + url.PathEscape(req.Repo.Workspace) +
		"/repos/" + url.PathEscape(req.Repo.Slug) + "/branches"
	payload = map[string]any{
		"name":       req.Name,
		"startPoint": req.FromRef,
	}
	return
}

// DeleteBranch removes a branch.
func (c *apiClient) DeleteBranch(ctx context.Context, req DeleteBranchReq) error {
	if err := checkRepoRef(req.Repo); err != nil {
		return err
	}
	method, path := c.buildDeleteBranch(req)
	return c.doJSON(ctx, method, path, nil, nil, nil)
}

func (c *apiClient) buildDeleteBranch(req DeleteBranchReq) (method, path string) {
	if c.flavor == FlavorCloud {
		method = "DELETE"
		path = c.branchesPath(req.Repo) + "/" + url.PathEscape(req.Name)
		return
	}
	method = "DELETE"
	path = "/rest/branch-utils/1.0/projects/" + url.PathEscape(req.Repo.Workspace) +
		"/repos/" + url.PathEscape(req.Repo.Slug) + "/branches?name=" + url.QueryEscape(req.Name)
	return
}
