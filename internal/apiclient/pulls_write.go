package apiclient

import (
	"context"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// CreatePR opens a new pull request.
func (c *apiClient) CreatePR(ctx context.Context, req CreatePRReq) (*PullRequest, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	if req.Source == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "PR_NO_SOURCE",
			"source branch is required").
			WithNextSteps("Pass --source <branch>")
	}
	method, path, payload, err := c.buildCreatePR(req)
	if err != nil {
		return nil, err
	}
	if c.flavor == FlavorCloud {
		var raw cloudPR
		if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
			return nil, err
		}
		return mapCloudPR(req.Repo, raw), nil
	}
	var raw dcPR
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	return mapDCPR(req.Repo, raw), nil
}

func (c *apiClient) buildCreatePR(req CreatePRReq) (method, path string, payload any, err error) {
	method = "POST"
	if c.flavor == FlavorCloud {
		path = c.prsPath(req.Repo)
		body := map[string]any{
			"title":       req.Title,
			"description": req.Description,
			"source": map[string]any{
				"branch": map[string]string{"name": req.Source},
			},
		}
		if req.SourceRepo != "" {
			body["source"].(map[string]any)["repository"] = map[string]string{"full_name": req.SourceRepo}
		}
		if req.Destination != "" {
			body["destination"] = map[string]any{
				"branch": map[string]string{"name": req.Destination},
			}
		}
		if req.CloseSourceBranch {
			body["close_source_branch"] = true
		}
		reviewers := make([]map[string]string, 0, len(req.Reviewers))
		for _, r := range req.Reviewers {
			reviewers = append(reviewers, map[string]string{"uuid": r})
		}
		if len(reviewers) > 0 {
			body["reviewers"] = reviewers
		}
		payload = body
		return
	}
	// Data Center
	path = c.prsPath(req.Repo)
	// DC has no close-source-branch property at PR creation. The emulation only
	// exists at merge time (delete the source branch after a successful merge),
	// so honouring it here is impossible — reject rather than silently drop it.
	if req.CloseSourceBranch {
		err = cerrors.New(cerrors.CategoryUsage, "PR_CREATE_CLOSE_SOURCE_DC",
			"Bitbucket Data Center cannot set close-source-branch when opening a PR").
			WithHint("Pass --close-source-branch to `pr merge` instead; it deletes the source branch after the merge.")
		return
	}
	// fromRef points at the source repository. For a same-repo PR that is the
	// target repo; for a cross-fork PR it is the fork named by --source-repo.
	fromRepo := req.Repo
	if req.SourceRepo != "" {
		fork, perr := parseRepoSpec(req.SourceRepo)
		if perr != nil {
			err = perr
			return
		}
		fromRepo = fork
		// DC matches cross-fork by comparing fromRef/toRef repositories, so the
		// target ref must be spelled out explicitly — it cannot default.
		if req.Destination == "" {
			err = cerrors.New(cerrors.CategoryUsage, "PR_FORK_NEEDS_TARGET",
				"a cross-fork PR requires an explicit target branch").
				WithNextSteps("Pass --target <branch> (the upstream destination branch)")
			return
		}
	}
	body := map[string]any{
		"title":       req.Title,
		"description": req.Description,
		"state":       "OPEN",
		"open":        true,
		"closed":      false,
		"fromRef": map[string]any{
			"id": "refs/heads/" + req.Source,
			"repository": map[string]any{
				"slug":    fromRepo.Slug,
				"project": map[string]string{"key": fromRepo.Workspace},
			},
		},
	}
	if req.Destination != "" {
		body["toRef"] = map[string]any{
			"id": "refs/heads/" + req.Destination,
			"repository": map[string]any{
				"slug":    req.Repo.Slug,
				"project": map[string]string{"key": req.Repo.Workspace},
			},
		}
	}
	reviewers := make([]map[string]any, 0, len(req.Reviewers))
	for _, r := range req.Reviewers {
		reviewers = append(reviewers, map[string]any{"user": map[string]string{"name": r}})
	}
	if len(reviewers) > 0 {
		body["reviewers"] = reviewers
	}
	payload = body
	return
}

// parseRepoSpec splits a "<workspace>/<repo>" spec into a RepoRef. It is used
// for cross-repo references (e.g. --source-repo) where the value is a bare spec
// rather than a resolved RepoRef.
func parseRepoSpec(spec string) (RepoRef, error) {
	ws, slug, ok := strings.Cut(spec, "/")
	if !ok || ws == "" || slug == "" || strings.Contains(slug, "/") {
		return RepoRef{}, cerrors.Newf(cerrors.CategoryUsage, "BAD_REPO_SPEC",
			"expected <workspace>/<repo>, got %q", spec)
	}
	return RepoRef{Workspace: ws, Slug: slug}, nil
}

// UpdatePR edits a PR's title/description/reviewers.
func (c *apiClient) UpdatePR(ctx context.Context, req UpdatePRReq) (*PullRequest, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	method, path, payload, err := c.buildUpdatePR(ctx, req)
	if err != nil {
		return nil, err
	}
	if c.flavor == FlavorCloud {
		var raw cloudPR
		if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
			return nil, err
		}
		return mapCloudPR(req.Repo, raw), nil
	}
	var raw dcPR
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	return mapDCPR(req.Repo, raw), nil
}

func (c *apiClient) buildUpdatePR(ctx context.Context, req UpdatePRReq) (method, path string, payload any, err error) {
	method = "PUT"
	path = c.prPath(req.Repo, req.ID)
	body := map[string]any{}
	if req.Title != "" {
		body["title"] = req.Title
	}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if c.flavor == FlavorCloud {
		if req.Reviewers != nil {
			reviewers := make([]map[string]string, 0, len(req.Reviewers))
			for _, r := range req.Reviewers {
				reviewers = append(reviewers, map[string]string{"uuid": r})
			}
			body["reviewers"] = reviewers
		}
		payload = body
		return
	}
	// Data Center guards updates with an optimistic lock: the PUT must carry the
	// PR's current version or the server rejects it. Fetch it first.
	cur, gerr := c.dcGetPRRaw(ctx, req.Repo, req.ID)
	if gerr != nil {
		err = gerr
		return
	}
	body["version"] = cur.Version
	if req.Reviewers != nil {
		reviewers := make([]map[string]any, 0, len(req.Reviewers))
		for _, r := range req.Reviewers {
			reviewers = append(reviewers, map[string]any{"user": map[string]string{"name": r}})
		}
		body["reviewers"] = reviewers
	}
	payload = body
	return
}

// DeclinePR closes an open PR without merging.
func (c *apiClient) DeclinePR(ctx context.Context, req DeclinePRReq) (*PullRequest, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	path := c.prPath(req.Repo, req.ID) + "/decline"
	if c.flavor == FlavorCloud {
		var raw cloudPR
		if err := c.doJSON(ctx, "POST", path, nil, map[string]string{"message": req.Message}, &raw); err != nil {
			return nil, err
		}
		return mapCloudPR(req.Repo, raw), nil
	}
	// DC requires the PR version; do a GET-then-POST.
	cur, err := c.dcGetPRRaw(ctx, req.Repo, req.ID)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"version": cur.Version}
	var raw dcPR
	if err := c.doJSON(ctx, "POST", path+"?version="+strconv.Itoa(cur.Version), nil, body, &raw); err != nil {
		return nil, err
	}
	return mapDCPR(req.Repo, raw), nil
}

func (c *apiClient) dcGetPRRaw(ctx context.Context, repo RepoRef, id int) (*dcPR, error) {
	var raw dcPR
	if err := c.getJSON(ctx, c.prPath(repo, id), nil, &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

// MergePR merges a PR.
func (c *apiClient) MergePR(ctx context.Context, req MergePRReq) (*PullRequest, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	method, path, payload, err := c.buildMergePR(ctx, req)
	if err != nil {
		return nil, err
	}
	if c.flavor == FlavorCloud {
		var raw cloudPR
		if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
			return nil, err
		}
		return mapCloudPR(req.Repo, raw), nil
	}
	var raw dcPR
	if err := c.doJSON(ctx, method, path, nil, payload, &raw); err != nil {
		return nil, err
	}
	pr := mapDCPR(req.Repo, raw)
	// DC has no close-source-branch flag on the merge call. Honor the opt-in by
	// deleting the source branch after a successful merge. The source ref may
	// live in a fork, so resolve the repository from the PR's own fromRef.
	if req.CloseSourceBranch {
		if derr := c.dcDeleteSourceBranch(ctx, raw); derr != nil {
			return pr, cerrors.Wrap(derr, cerrors.CategoryServer, "PR_MERGED_BRANCH_KEPT",
				"PR merged, but deleting the source branch failed")
		}
	}
	return pr, nil
}

// dcDeleteSourceBranch removes the source branch of a just-merged DC PR, reading
// the branch name and (possibly forked) repository from the PR's fromRef.
func (c *apiClient) dcDeleteSourceBranch(ctx context.Context, pr dcPR) error {
	src := pr.FromRef
	repo := RepoRef{Workspace: src.Repository.Project.Key, Slug: src.Repository.Slug}
	if repo.Workspace == "" || repo.Slug == "" || src.DisplayID == "" {
		return cerrors.New(cerrors.CategoryInternal, "PR_NO_SOURCE_REF",
			"could not resolve the source branch from the merged PR")
	}
	return c.DeleteBranch(ctx, DeleteBranchReq{Repo: repo, Name: src.DisplayID})
}

func (c *apiClient) buildMergePR(ctx context.Context, req MergePRReq) (method, path string, payload any, err error) {
	method = "POST"
	path = c.prPath(req.Repo, req.ID) + "/merge"
	if c.flavor == FlavorCloud {
		strategy := normalizeCloudStrategy(req.Strategy)
		body := map[string]any{}
		if strategy != "" {
			body["merge_strategy"] = strategy
		}
		if req.Message != "" {
			body["message"] = req.Message
		}
		if req.CloseSourceBranch {
			body["close_source_branch"] = true
		}
		payload = body
		return
	}
	// DC needs the version.
	cur, gerr := c.dcGetPRRaw(ctx, req.Repo, req.ID)
	if gerr != nil {
		err = gerr
		return
	}
	body := map[string]any{"version": cur.Version}
	if s := normalizeDCStrategy(req.Strategy); s != "" {
		body["strategyId"] = s
	}
	if req.Message != "" {
		body["message"] = req.Message
	}
	payload = body
	path = path + "?version=" + strconv.Itoa(cur.Version)
	return
}

func normalizeCloudStrategy(s string) string {
	switch s {
	case "merge_commit", "merge", "":
		return "merge_commit"
	case "squash":
		return "squash"
	case "fast_forward", "ff":
		return "fast_forward"
	}
	return s
}

func normalizeDCStrategy(s string) string {
	switch s {
	case "merge_commit", "merge", "":
		return "no-ff"
	case "squash":
		return "squash"
	case "fast_forward", "ff":
		return "ff-only"
	}
	return s
}

// DescribeWrite renders the HTTP plan for a write op (for --dry-run).
func (c *apiClient) DescribeWrite(ctx context.Context, op any) (WriteRequestPlan, error) {
	switch v := op.(type) {
	case CreateRepoReq:
		m, p, body := c.buildCreateRepo(v)
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case CreatePRReq:
		m, p, body, err := c.buildCreatePR(v)
		if err != nil {
			return WriteRequestPlan{}, err
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case UpdatePRReq:
		m, p, body, err := c.buildUpdatePR(ctx, v)
		if err != nil {
			return WriteRequestPlan{}, err
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case MergePRReq:
		m, p, body, err := c.buildMergePR(ctx, v)
		if err != nil {
			return WriteRequestPlan{}, err
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case DeclinePRReq:
		return WriteRequestPlan{
			Method:  "POST",
			URL:     c.baseURL + c.prPath(v.Repo, v.ID) + "/decline",
			Payload: map[string]string{"message": v.Message},
		}, nil
	case ApprovePRReq:
		m := "POST"
		if !v.Approve {
			m = "DELETE"
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + c.prPath(v.Repo, v.ID) + "/approve"}, nil
	case CreateBranchReq:
		m, p, body := c.buildCreateBranch(v)
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case DeleteBranchReq:
		m, p := c.buildDeleteBranch(v)
		return WriteRequestPlan{Method: m, URL: c.baseURL + p}, nil
	case AddPRCommentReq:
		if err := checkRepoRef(v.Repo); err != nil {
			return WriteRequestPlan{}, err
		}
		m, p, body := c.buildAddPRComment(v)
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case UpdatePRCommentReq:
		m, p, body, err := c.buildUpdatePRComment(ctx, v)
		if err != nil {
			return WriteRequestPlan{}, err
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case DeletePRCommentReq:
		m, p, err := c.buildDeletePRComment(ctx, v)
		if err != nil {
			return WriteRequestPlan{}, err
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + p}, nil
	case ResolvePRCommentReq:
		m, p, body, err := c.buildResolvePRComment(ctx, v)
		if err != nil {
			return WriteRequestPlan{}, err
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: body}, nil
	case DeleteRepoReq:
		if err := checkRepoRef(v.Repo); err != nil {
			return WriteRequestPlan{}, err
		}
		return WriteRequestPlan{Method: "DELETE", URL: c.baseURL + c.repoPath(v.Repo)}, nil
	case RequestChangesReq:
		if err := checkRepoRef(v.Repo); err != nil {
			return WriteRequestPlan{}, err
		}
		if sup := c.supportFor(CapPRRequestChanges); !sup.Supported() {
			return WriteRequestPlan{}, cerrors.New(cerrors.CategoryUsage, "PR_REQ_CHANGES_DC",
				"pr request-changes is not available on this backend: "+sup.Reason).
				WithHint("On Data Center, decline the PR or post a comment to request changes.")
		}
		m := "POST"
		if !v.Request {
			m = "DELETE"
		}
		return WriteRequestPlan{Method: m, URL: c.baseURL + c.prPath(v.Repo, v.ID) + "/request-changes"}, nil
	}
	return WriteRequestPlan{}, cerrors.New(cerrors.CategoryInternal, "UNSUPPORTED_WRITE",
		"DescribeWrite called with an unsupported op type")
}
