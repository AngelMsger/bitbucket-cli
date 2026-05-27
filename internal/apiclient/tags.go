package apiclient

import (
	"context"
	"net/url"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListTags returns the tags of a repository.
//
//	Cloud: GET /2.0/repositories/{ws}/{slug}/refs/tags
//	DC:    GET /rest/api/1.0/projects/{key}/repos/{slug}/tags
func (c *apiClient) ListTags(ctx context.Context, opt TagListOpts) (ListResult[Tag], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[Tag]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	var path string
	if c.flavor == FlavorCloud {
		path = c.repoPath(opt.Repo) + "/refs/tags"
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
		var raw struct {
			Values []struct {
				Name   string `json:"name"`
				Target struct {
					Hash    string `json:"hash"`
					Date    string `json:"date"`
					Message string `json:"message"`
				} `json:"target"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[Tag]{}, err
		}
		res := ListResult[Tag]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			res.Items = append(res.Items, Tag{
				Name:    v.Name,
				Target:  v.Target.Hash,
				Date:    v.Target.Date,
				Message: v.Target.Message,
			})
		}
		return res, nil
	}
	// Data Center
	path = c.repoPath(opt.Repo) + "/tags"
	if opt.Query != "" {
		q.Set("filterText", opt.Query)
	}
	if opt.Sort != "" {
		q.Set("orderBy", opt.Sort)
	}
	var raw struct {
		Values []struct {
			ID           string `json:"id"`
			DisplayID    string `json:"displayId"`
			Type         string `json:"type"`
			LatestCommit string `json:"latestCommit"`
			Hash         string `json:"hash"`
		} `json:"values"`
		Size       int  `json:"size"`
		Limit      int  `json:"limit"`
		Start      int  `json:"start"`
		IsLastPage bool `json:"isLastPage"`
	}
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Tag]{}, err
	}
	res := ListResult[Tag]{
		Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage),
	}
	for _, v := range raw.Values {
		res.Items = append(res.Items, Tag{
			Name:   v.DisplayID,
			Target: v.LatestCommit,
		})
	}
	return res, nil
}

// GetTag returns a single tag by name.
//
//	Cloud: GET /2.0/repositories/{ws}/{slug}/refs/tags/{name}
//	DC:    GET /rest/api/1.0/projects/{key}/repos/{slug}/tags/{name}
func (c *apiClient) GetTag(ctx context.Context, repo RepoRef, name string) (*Tag, error) {
	if err := checkRepoRef(repo); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "TAG_NO_NAME", "tag name is required")
	}
	if c.flavor == FlavorCloud {
		var raw struct {
			Name   string `json:"name"`
			Target struct {
				Hash    string `json:"hash"`
				Date    string `json:"date"`
				Message string `json:"message"`
			} `json:"target"`
		}
		path := c.repoPath(repo) + "/refs/tags/" + url.PathEscape(name)
		if err := c.getJSON(ctx, path, nil, &raw); err != nil {
			return nil, err
		}
		return &Tag{
			Name: raw.Name, Target: raw.Target.Hash,
			Date: raw.Target.Date, Message: raw.Target.Message,
		}, nil
	}
	var raw struct {
		ID           string `json:"id"`
		DisplayID    string `json:"displayId"`
		LatestCommit string `json:"latestCommit"`
	}
	if err := c.getJSON(ctx, c.repoPath(repo)+"/tags/"+url.PathEscape(name), nil, &raw); err != nil {
		return nil, err
	}
	return &Tag{Name: raw.DisplayID, Target: raw.LatestCommit}, nil
}
