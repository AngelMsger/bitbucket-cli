package apiclient

import (
	"context"
	"net/url"
)

// GetCommit fetches a single commit by hash.
func (c *apiClient) GetCommit(ctx context.Context, repo RepoRef, hash string) (*Commit, error) {
	if err := checkRepoRef(repo); err != nil {
		return nil, err
	}
	path := c.commitsPath(repo) + "/" + url.PathEscape(hash)
	if c.flavor == FlavorCloud {
		var raw cloudCommit
		if err := c.getJSON(ctx, path, nil, &raw); err != nil {
			return nil, err
		}
		cm := mapCloudCommit(raw)
		return &cm, nil
	}
	// Data Center commit endpoint: .../commits/{hash}
	var raw dcCommit
	if err := c.getJSON(ctx, path, nil, &raw); err != nil {
		return nil, err
	}
	cm := mapDCCommit(raw)
	return &cm, nil
}

// ListCommits lists commits, optionally filtered by branch / path / date range.
func (c *apiClient) ListCommits(ctx context.Context, opt ListCommitsOpts) (ListResult[Commit], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[Commit]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	path := c.commitsPath(opt.Repo)

	if c.flavor == FlavorCloud {
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		if opt.Branch != "" {
			path += "/" + url.PathEscape(opt.Branch)
		}
		if opt.Path != "" {
			if q == nil {
				q = url.Values{}
			}
			q.Set("path", opt.Path)
		}
		var raw cloudCommitList
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[Commit]{}, err
		}
		res := ListResult[Commit]{Next: cloudNextCursor(raw.Next)}
		for _, cm := range raw.Values {
			res.Items = append(res.Items, mapCloudCommit(cm))
		}
		return res, nil
	}
	if opt.Branch != "" {
		q.Set("until", opt.Branch)
	}
	if opt.Path != "" {
		q.Set("path", opt.Path)
	}
	var raw dcCommitList
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Commit]{}, err
	}
	res := ListResult[Commit]{Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage)}
	for _, cm := range raw.Values {
		res.Items = append(res.Items, mapDCCommit(cm))
	}
	return res, nil
}

// CompareCommits returns the commits reachable from To but not From.
func (c *apiClient) CompareCommits(ctx context.Context, req CompareCommitsReq) (ListResult[Commit], error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return ListResult[Commit]{}, err
	}
	q := url.Values{}
	if c.flavor == FlavorCloud {
		// Cloud: GET /repositories/{ws}/{repo}/commits?include=<to>&exclude=<from>
		q.Set("include", req.To)
		q.Set("exclude", req.From)
		var raw cloudCommitList
		if err := c.getJSON(ctx, c.commitsPath(req.Repo), q, &raw); err != nil {
			return ListResult[Commit]{}, err
		}
		res := ListResult[Commit]{Next: cloudNextCursor(raw.Next)}
		for _, cm := range raw.Values {
			res.Items = append(res.Items, mapCloudCommit(cm))
		}
		return res, nil
	}
	// DC: GET /repos/{slug}/commits?until=<to>&since=<from>
	q.Set("until", req.To)
	q.Set("since", req.From)
	var raw dcCommitList
	if err := c.getJSON(ctx, c.commitsPath(req.Repo), q, &raw); err != nil {
		return ListResult[Commit]{}, err
	}
	res := ListResult[Commit]{Next: nextOffsetToken("", 0, len(raw.Values), raw.IsLastPage)}
	for _, cm := range raw.Values {
		res.Items = append(res.Items, mapDCCommit(cm))
	}
	return res, nil
}
