package apiclient

import (
	"context"
	"strconv"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListPRComments lists comments on a PR.
func (c *apiClient) ListPRComments(ctx context.Context, opt ListPRCommentsOpts) (ListResult[Comment], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[Comment]{}, err
	}
	if opt.PRID == 0 {
		return ListResult[Comment]{}, cerrors.New(cerrors.CategoryUsage, "COMMENT_NO_PR",
			"a PR ID is required")
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	path := c.prPath(opt.Repo, opt.PRID) + "/comments"

	if c.flavor == FlavorCloud {
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		var raw cloudCommentList
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[Comment]{}, err
		}
		res := ListResult[Comment]{Next: cloudNextCursor(raw.Next)}
		for _, cm := range raw.Values {
			if cm.Deleted {
				continue
			}
			res.Items = append(res.Items, mapCloudComment(opt.PRID, cm))
		}
		return res, nil
	}
	// Data Center: PR activity stream filtered by comment action.
	var raw dcActivityList
	if err := c.getJSON(ctx, c.prPath(opt.Repo, opt.PRID)+"/activities", q, &raw); err != nil {
		return ListResult[Comment]{}, err
	}
	res := ListResult[Comment]{Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage)}
	for _, a := range raw.Values {
		if a.Comment == nil {
			continue
		}
		res.Items = append(res.Items, mapDCComment(opt.PRID, *a.Comment))
	}
	return res, nil
}

// AddPRComment creates a PR comment (general or inline).
func (c *apiClient) AddPRComment(ctx context.Context, req AddPRCommentReq) (*Comment, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	method, path, payload := c.buildAddPRComment(req)
	if c.flavor == FlavorCloud {
		var raw cloudComment
		if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
			return nil, err
		}
		cm := mapCloudComment(req.PRID, raw)
		return &cm, nil
	}
	var raw dcComment
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	cm := mapDCComment(req.PRID, raw)
	return &cm, nil
}

func (c *apiClient) buildAddPRComment(req AddPRCommentReq) (method, path string, payload any) {
	method = "POST"
	path = c.prPath(req.Repo, req.PRID) + "/comments"
	if c.flavor == FlavorCloud {
		body := map[string]any{
			"content": map[string]string{"raw": req.Content},
		}
		if req.Inline != nil {
			il := map[string]any{"path": req.Inline.Path}
			if req.Inline.To > 0 {
				il["to"] = req.Inline.To
			}
			if req.Inline.From > 0 {
				il["from"] = req.Inline.From
			}
			if req.Inline.Line > 0 && req.Inline.To == 0 && req.Inline.From == 0 {
				il["to"] = req.Inline.Line
			}
			body["inline"] = il
		}
		if req.ReplyTo > 0 {
			body["parent"] = map[string]int{"id": req.ReplyTo}
		}
		payload = body
		return
	}
	// Data Center
	body := map[string]any{"text": req.Content}
	if req.Inline != nil {
		anchor := map[string]any{
			"path":     req.Inline.Path,
			"lineType": "CONTEXT",
			"fileType": "TO",
		}
		if req.Inline.Line > 0 {
			anchor["line"] = req.Inline.Line
		}
		body["anchor"] = anchor
	}
	if req.ReplyTo > 0 {
		body["parent"] = map[string]int{"id": req.ReplyTo}
	}
	payload = body
	return
}

// UpdatePRComment edits an existing PR comment.
func (c *apiClient) UpdatePRComment(ctx context.Context, req UpdatePRCommentReq) (*Comment, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	path := c.prPath(req.Repo, req.PRID) + "/comments/" + strconv.Itoa(req.ID)
	if c.flavor == FlavorCloud {
		body := map[string]any{"content": map[string]string{"raw": req.Content}}
		var raw cloudComment
		if err := c.doJSON(ctx, "PUT", path, nil, body, &raw); err != nil {
			return nil, err
		}
		cm := mapCloudComment(req.PRID, raw)
		return &cm, nil
	}
	// DC requires fetching the current version first.
	var current dcComment
	if err := c.getJSON(ctx, path, nil, &current); err != nil {
		return nil, err
	}
	body := map[string]any{"text": req.Content, "version": current.Version}
	var raw dcComment
	if err := c.doJSON(ctx, "PUT", path, nil, body, &raw); err != nil {
		return nil, err
	}
	cm := mapDCComment(req.PRID, raw)
	return &cm, nil
}

// DeletePRComment removes a PR comment.
func (c *apiClient) DeletePRComment(ctx context.Context, req DeletePRCommentReq) error {
	if err := checkRepoRef(req.Repo); err != nil {
		return err
	}
	path := c.prPath(req.Repo, req.PRID) + "/comments/" + strconv.Itoa(req.ID)
	if c.flavor == FlavorCloud {
		return c.doJSON(ctx, "DELETE", path, nil, nil, nil)
	}
	// DC requires version=N as a query param.
	var current dcComment
	if err := c.getJSON(ctx, path, nil, &current); err != nil {
		return err
	}
	return c.doJSON(ctx, "DELETE", path+"?version="+strconv.Itoa(current.Version), nil, nil, nil)
}
