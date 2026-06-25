package apiclient

import (
	"context"
	"net/url"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// ListWorkspaces returns every workspace (Cloud) / project (DC) the
// authenticated user can see. This is the discovery entry point that backs
// `bitbucket-cli workspace list` — any other command's `--workspace <slug>`
// argument refers to one of the values returned here.
//
//	Cloud: GET /2.0/workspaces?role=...&q=name~"..."
//	DC:    GET /rest/api/1.0/projects?name=...&permission=PROJECT_READ
func (c *apiClient) ListWorkspaces(ctx context.Context, opt WorkspaceListOpts) (ListResult[Workspace], error) {
	limit := c.limitOf(opt.ListOpts)
	if c.flavor == FlavorCloud {
		q := url.Values{}
		q.Set("pagelen", itoa(limit))
		if opt.Role != "" {
			q.Set("role", opt.Role)
		}
		if opt.Query != "" {
			q.Set("q", `name~"`+opt.Query+`"`)
		}
		path := c.apiBase() + "/workspaces"
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		var raw struct {
			Values []struct {
				Slug      string `json:"slug"`
				Name      string `json:"name"`
				UUID      string `json:"uuid"`
				Type      string `json:"type"`
				CreatedOn string `json:"created_on"`
				Links     struct {
					HTML struct {
						Href string `json:"href"`
					} `json:"html"`
				} `json:"links"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[Workspace]{}, err
		}
		res := ListResult[Workspace]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			res.Items = append(res.Items, Workspace{
				Slug:      v.Slug,
				Name:      v.Name,
				UUID:      v.UUID,
				Type:      v.Type,
				URL:       v.Links.HTML.Href,
				CreatedAt: v.CreatedOn,
			})
		}
		return res, nil
	}
	// Data Center
	q := c.queryWithLimit(opt.Cursor, limit)
	if opt.Query != "" {
		q.Set("name", opt.Query)
	}
	var raw struct {
		Values []struct {
			Key         string `json:"key"`
			Name        string `json:"name"`
			ID          int    `json:"id"`
			Description string `json:"description"`
			Public      bool   `json:"public"`
			Type        string `json:"type"`
			Links       struct {
				Self []struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"links"`
		} `json:"values"`
		Size       int  `json:"size"`
		Limit      int  `json:"limit"`
		Start      int  `json:"start"`
		IsLastPage bool `json:"isLastPage"`
	}
	if err := c.getJSON(ctx, "/rest/api/1.0/projects", q, &raw); err != nil {
		return ListResult[Workspace]{}, err
	}
	res := ListResult[Workspace]{
		Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage),
	}
	for _, v := range raw.Values {
		ws := Workspace{
			Slug:        v.Key,
			Name:        v.Name,
			Type:        v.Type,
			Description: v.Description,
		}
		if len(v.Links.Self) > 0 {
			ws.URL = v.Links.Self[0].Href
		}
		res.Items = append(res.Items, ws)
	}
	return res, nil
}

// GetWorkspace returns a single workspace / project by slug or key.
//
//	Cloud: GET /2.0/workspaces/{slug}
//	DC:    GET /rest/api/1.0/projects/{key}
func (c *apiClient) GetWorkspace(ctx context.Context, slug string) (*Workspace, error) {
	if slug == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "WORKSPACE_NO_SLUG",
			"a workspace slug (or DC project key) is required")
	}
	if c.flavor == FlavorCloud {
		var raw struct {
			Slug      string `json:"slug"`
			Name      string `json:"name"`
			UUID      string `json:"uuid"`
			Type      string `json:"type"`
			CreatedOn string `json:"created_on"`
			Links     struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		}
		if err := c.getJSON(ctx, c.apiBase()+"/workspaces/"+url.PathEscape(slug), nil, &raw); err != nil {
			return nil, err
		}
		return &Workspace{
			Slug:      raw.Slug,
			Name:      raw.Name,
			UUID:      raw.UUID,
			Type:      raw.Type,
			URL:       raw.Links.HTML.Href,
			CreatedAt: raw.CreatedOn,
		}, nil
	}
	var raw struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Public      bool   `json:"public"`
		Type        string `json:"type"`
		Links       struct {
			Self []struct {
				Href string `json:"href"`
			} `json:"self"`
		} `json:"links"`
	}
	if err := c.getJSON(ctx, "/rest/api/1.0/projects/"+url.PathEscape(slug), nil, &raw); err != nil {
		return nil, err
	}
	ws := &Workspace{
		Slug:        raw.Key,
		Name:        raw.Name,
		Type:        raw.Type,
		Description: raw.Description,
	}
	if len(raw.Links.Self) > 0 {
		ws.URL = raw.Links.Self[0].Href
	}
	return ws, nil
}
