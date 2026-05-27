package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListPRs lists pull requests in a repository.
func (c *apiClient) ListPRs(ctx context.Context, opt PRListOpts) (ListResult[PullRequest], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[PullRequest]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	path := c.prsPath(opt.Repo)

	if c.flavor == FlavorCloud {
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		state := strings.ToUpper(strings.TrimSpace(opt.State))
		if state == "" || state == "ALL" {
			// Cloud requires a state; for "ALL" enumerate the common ones via `q`.
		}
		if state != "" && state != "ALL" {
			if q == nil {
				q = url.Values{}
			}
			q.Set("state", state)
		}
		if opt.Query != "" {
			if q == nil {
				q = url.Values{}
			}
			q.Set("q", opt.Query)
		}
		var raw cloudPRList
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[PullRequest]{}, err
		}
		res := ListResult[PullRequest]{Next: cloudNextCursor(raw.Next)}
		for _, p := range raw.Values {
			res.Items = append(res.Items, *mapCloudPR(opt.Repo, p))
		}
		return res, nil
	}
	// Data Center
	if opt.State != "" && strings.ToUpper(opt.State) != "ALL" {
		q.Set("state", strings.ToUpper(opt.State))
	}
	if opt.Source != "" {
		q.Set("at", "refs/heads/"+opt.Source)
		q.Set("direction", "OUTGOING")
	}
	if opt.Target != "" {
		q.Set("at", "refs/heads/"+opt.Target)
		q.Set("direction", "INCOMING")
	}
	var raw dcPRList
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[PullRequest]{}, err
	}
	res := ListResult[PullRequest]{Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage)}
	for _, p := range raw.Values {
		res.Items = append(res.Items, *mapDCPR(opt.Repo, p))
	}
	return res, nil
}

// GetPR fetches a single PR.
func (c *apiClient) GetPR(ctx context.Context, opt GetPROpts) (*PullRequest, error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return nil, err
	}
	path := c.prPath(opt.Repo, opt.ID)
	if c.flavor == FlavorCloud {
		var raw cloudPR
		if err := c.getJSON(ctx, path, nil, &raw); err != nil {
			return nil, err
		}
		return mapCloudPR(opt.Repo, raw), nil
	}
	var raw dcPR
	if err := c.getJSON(ctx, path, nil, &raw); err != nil {
		return nil, err
	}
	return mapDCPR(opt.Repo, raw), nil
}

// GetPRDiff returns the unified diff text of a PR.
func (c *apiClient) GetPRDiff(ctx context.Context, repo RepoRef, id int) (string, error) {
	if err := checkRepoRef(repo); err != nil {
		return "", err
	}
	if c.flavor == FlavorCloud {
		return c.getText(ctx, c.prPath(repo, id)+"/diff", nil)
	}
	// Data Center returns JSON hunks at /diff; for now request the raw
	// `application/x-patch` content via /diff.diff.
	return c.getText(ctx, c.prPath(repo, id)+".diff", nil)
}

// ListPRCommits lists the commits included in a PR.
func (c *apiClient) ListPRCommits(ctx context.Context, opt PRListOpts) (ListResult[Commit], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[Commit]{}, err
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	prID, _ := strconv.Atoi(strings.TrimSpace(strings.Trim(opt.Query, "#")))
	if prID == 0 {
		return ListResult[Commit]{}, cerrors.New(cerrors.CategoryUsage, "PR_NO_ID",
			"a PR ID is required (passed via opt.Query)")
	}
	path := c.prPath(opt.Repo, prID) + "/commits"
	if c.flavor == FlavorCloud {
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
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

// ListPRActivity returns the activity stream of a PR.
func (c *apiClient) ListPRActivity(ctx context.Context, opt PRListOpts) (ListResult[Activity], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[Activity]{}, err
	}
	prID, _ := strconv.Atoi(strings.TrimSpace(strings.Trim(opt.Query, "#")))
	if prID == 0 {
		return ListResult[Activity]{}, cerrors.New(cerrors.CategoryUsage, "PR_NO_ID",
			"a PR ID is required (passed via opt.Query)")
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	path := c.prPath(opt.Repo, prID) + "/activity"
	if c.flavor == FlavorCloud {
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		// Cloud returns a heterogeneous list; decode loosely as a Values slice.
		var raw struct {
			Values []struct {
				Update *struct {
					State string    `json:"state"`
					Date  string    `json:"date"`
					Author cloudUser `json:"author"`
				} `json:"update"`
				Approval *struct {
					Date string    `json:"date"`
					User cloudUser `json:"user"`
				} `json:"approval"`
				Comment *cloudComment `json:"comment"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[Activity]{}, err
		}
		res := ListResult[Activity]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			switch {
			case v.Comment != nil:
				cm := mapCloudComment(prID, *v.Comment)
				res.Items = append(res.Items, Activity{Kind: "comment", Actor: cm.Author, When: v.Comment.CreatedOn, Comment: &cm})
			case v.Approval != nil:
				res.Items = append(res.Items, Activity{Kind: "approval", Actor: mapCloudUser(v.Approval.User), When: v.Approval.Date, Approved: true})
			case v.Update != nil:
				res.Items = append(res.Items, Activity{Kind: "update", Actor: mapCloudUser(v.Update.Author), When: v.Update.Date, State: v.Update.State})
			}
		}
		return res, nil
	}
	var raw dcActivityList
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Activity]{}, err
	}
	res := ListResult[Activity]{Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage)}
	for _, a := range raw.Values {
		entry := Activity{
			Kind:  strings.ToLower(a.Action),
			Actor: mapDCUser(a.User),
			When:  epochToISO(a.CreatedDate),
		}
		if a.Comment != nil {
			cm := mapDCComment(prID, *a.Comment)
			entry.Kind = "comment"
			entry.Comment = &cm
		}
		switch strings.ToUpper(a.Action) {
		case "APPROVED":
			entry.Kind = "approval"
			entry.Approved = true
		case "MERGED":
			entry.Kind = "merge"
		case "DECLINED":
			entry.Kind = "decline"
		}
		res.Items = append(res.Items, entry)
	}
	return res, nil
}
