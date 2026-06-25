package apiclient

import (
	"context"
	"strconv"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
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
		// Walk the reply tree: DC nests replies inside the root comment's
		// "comments" array, so a plain map would silently drop every reply. The
		// inline anchor is hoisted to the activity (a.CommentAnchor), not the
		// comment, so it is threaded in here and applied to the root.
		flattenDCComment(opt.PRID, 0, a.CommentAnchor, *a.Comment, &res.Items)
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
		// lineType / fileType come from ResolveInlineAnchor's diff classification;
		// fall back to the historical CONTEXT/TO guess for an unresolved anchor.
		fileType := req.Inline.FileType
		if fileType == "" {
			fileType = "TO"
		}
		lineType := req.Inline.LineType
		if lineType == "" {
			lineType = "CONTEXT"
		}
		anchor := map[string]any{
			"path":     req.Inline.Path,
			"lineType": lineType,
			"fileType": fileType,
		}
		// Anchor on the side fileType names: the FROM line for the old side, the
		// TO line for the new side, falling back to Line for an unresolved anchor.
		line := req.Inline.Line
		if fileType == "FROM" && req.Inline.From > 0 {
			line = req.Inline.From
		} else if fileType == "TO" && req.Inline.To > 0 {
			line = req.Inline.To
		}
		if line > 0 {
			anchor["line"] = line
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
	method, path, payload, err := c.buildUpdatePRComment(ctx, req)
	if err != nil {
		return nil, err
	}
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

// buildUpdatePRComment assembles the HTTP call for editing a PR comment.
// On Data Center it performs a read-only GET to learn the current version
// (the GET is safe under --dry-run).
func (c *apiClient) buildUpdatePRComment(ctx context.Context, req UpdatePRCommentReq) (method, path string, payload any, err error) {
	if rerr := checkRepoRef(req.Repo); rerr != nil {
		return "", "", nil, rerr
	}
	method = "PUT"
	path = c.prPath(req.Repo, req.PRID) + "/comments/" + strconv.Itoa(req.ID)
	if c.flavor == FlavorCloud {
		payload = map[string]any{"content": map[string]string{"raw": req.Content}}
		return
	}
	// DC requires fetching the current version first.
	var current dcComment
	if gerr := c.getJSON(ctx, path, nil, &current); gerr != nil {
		err = gerr
		return
	}
	payload = map[string]any{"text": req.Content, "version": current.Version}
	return
}

// ResolvePRComment marks a comment thread resolved, or reopens it when
// req.Resolve is false. Cloud uses the dedicated resolve endpoint (POST to
// resolve, DELETE to reopen) and then re-reads the comment for a uniform
// result; Data Center PUTs the comment's state (RESOLVED / OPEN), which is the
// same transition that completes or reopens a task (severity == BLOCKER).
func (c *apiClient) ResolvePRComment(ctx context.Context, req ResolvePRCommentReq) (*Comment, error) {
	method, path, payload, err := c.buildResolvePRComment(ctx, req)
	if err != nil {
		return nil, err
	}
	if c.flavor == FlavorCloud {
		// The resolve endpoint returns a resolution object (or 204 on reopen),
		// not the comment, so re-read it for a consistent Comment result.
		if derr := c.doJSON(ctx, method, path, nil, payload, nil); derr != nil {
			return nil, derr
		}
		return c.getPRComment(ctx, req.Repo, req.PRID, req.ID)
	}
	var raw dcComment
	if derr := c.doJSON(ctx, method, path, nil, payload, &raw); derr != nil {
		return nil, derr
	}
	cm := mapDCComment(req.PRID, raw)
	return &cm, nil
}

// buildResolvePRComment assembles the HTTP call for resolving/reopening a
// comment thread. On Data Center it performs a read-only GET to learn the
// current version first (the GET is safe under --dry-run).
func (c *apiClient) buildResolvePRComment(ctx context.Context, req ResolvePRCommentReq) (method, path string, payload any, err error) {
	if rerr := checkRepoRef(req.Repo); rerr != nil {
		return "", "", nil, rerr
	}
	base := c.prPath(req.Repo, req.PRID) + "/comments/" + strconv.Itoa(req.ID)
	if c.flavor == FlavorCloud {
		method = "POST"
		if !req.Resolve {
			method = "DELETE"
		}
		path = base + "/resolve"
		return
	}
	// Data Center: PUT the comment with the new state and its current version.
	method = "PUT"
	path = base
	state := "RESOLVED"
	if !req.Resolve {
		state = "OPEN"
	}
	var current dcComment
	if gerr := c.getJSON(ctx, path, nil, &current); gerr != nil {
		err = gerr
		return
	}
	payload = map[string]any{"state": state, "version": current.Version}
	return
}

// getPRComment fetches a single PR comment (Cloud). It backs ResolvePRComment's
// re-read after the resolve endpoint, which does not return the comment itself.
func (c *apiClient) getPRComment(ctx context.Context, repo RepoRef, prID, id int) (*Comment, error) {
	path := c.prPath(repo, prID) + "/comments/" + strconv.Itoa(id)
	var raw cloudComment
	if err := c.getJSON(ctx, path, nil, &raw); err != nil {
		return nil, err
	}
	cm := mapCloudComment(prID, raw)
	return &cm, nil
}

// DeletePRComment removes a PR comment.
func (c *apiClient) DeletePRComment(ctx context.Context, req DeletePRCommentReq) error {
	method, path, err := c.buildDeletePRComment(ctx, req)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, method, path, nil, nil, nil)
}

// buildDeletePRComment assembles the HTTP call for deleting a PR comment.
// On Data Center it performs a read-only GET to learn the current version
// (the GET is safe under --dry-run).
func (c *apiClient) buildDeletePRComment(ctx context.Context, req DeletePRCommentReq) (method, path string, err error) {
	if rerr := checkRepoRef(req.Repo); rerr != nil {
		return "", "", rerr
	}
	method = "DELETE"
	path = c.prPath(req.Repo, req.PRID) + "/comments/" + strconv.Itoa(req.ID)
	if c.flavor == FlavorCloud {
		return
	}
	var current dcComment
	if gerr := c.getJSON(ctx, path, nil, &current); gerr != nil {
		err = gerr
		return
	}
	path += "?version=" + strconv.Itoa(current.Version)
	return
}
