package apiclient

import (
	"context"
	"net/url"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListUsers enumerates users the current credentials can see — the discovery
// path for `--reviewer` / `--author` style flags.
//
//   Cloud: GET /2.0/workspaces/{workspace}/members?pagelen=N&q=...
//          (Cloud has no global user index; the closest is workspace
//          membership, so opt.Workspace is required.)
//   DC:    GET /rest/api/1.0/users?filter=...&start=N&limit=M
func (c *apiClient) ListUsers(ctx context.Context, opt UserListOpts) (ListResult[User], error) {
	limit := c.limitOf(opt.ListOpts)
	if c.flavor == FlavorCloud {
		if opt.Workspace == "" {
			return ListResult[User]{}, cerrors.New(cerrors.CategoryUsage, "USER_NO_WORKSPACE",
				"Bitbucket Cloud lists users via workspace membership; --workspace is required").
				WithHint("Pick a workspace from `bitbucket-cli workspace list` and pass it as --workspace.").
				WithNextSteps(
					"bitbucket-cli workspace list",
					"bitbucket-cli user list --workspace <slug>",
				)
		}
		q := url.Values{}
		q.Set("pagelen", itoa(limit))
		if opt.Query != "" {
			q.Set("q", `user.display_name~"`+opt.Query+`"`)
		}
		path := c.apiBase() + "/workspaces/" + url.PathEscape(opt.Workspace) + "/members"
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		var raw struct {
			Values []struct {
				User cloudUser `json:"user"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[User]{}, err
		}
		res := ListResult[User]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			res.Items = append(res.Items, mapCloudUser(v.User))
		}
		return res, nil
	}
	// Data Center: global /users endpoint.
	q := c.queryWithLimit(opt.Cursor, limit)
	if opt.Query != "" {
		q.Set("filter", opt.Query)
	}
	var raw struct {
		Values     []dcUser `json:"values"`
		Size       int      `json:"size"`
		Limit      int      `json:"limit"`
		Start      int      `json:"start"`
		IsLastPage bool     `json:"isLastPage"`
	}
	if err := c.getJSON(ctx, "/rest/api/1.0/users", q, &raw); err != nil {
		return ListResult[User]{}, err
	}
	res := ListResult[User]{
		Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage),
	}
	for _, u := range raw.Values {
		res.Items = append(res.Items, mapDCUser(u))
	}
	return res, nil
}

// GetUser fetches a single user by selector.
//   Cloud: GET /2.0/users/{selector}  — selector is "{uuid}" or account_id.
//   DC:    GET /rest/api/1.0/users/{slug}
func (c *apiClient) GetUser(ctx context.Context, selector string) (*User, error) {
	if selector == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "USER_NO_SELECTOR",
			"a user selector is required (UUID/account_id on Cloud, username/slug on DC)")
	}
	if c.flavor == FlavorCloud {
		var raw cloudUser
		if err := c.getJSON(ctx, c.apiBase()+"/users/"+url.PathEscape(selector), nil, &raw); err != nil {
			return nil, err
		}
		u := mapCloudUser(raw)
		return &u, nil
	}
	var raw dcUser
	if err := c.getJSON(ctx, "/rest/api/1.0/users/"+url.PathEscape(selector), nil, &raw); err != nil {
		return nil, err
	}
	u := mapDCUser(raw)
	return &u, nil
}
