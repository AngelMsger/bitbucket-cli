package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

type cloudDefaultReviewer struct {
	User cloudUser `json:"user"`
}

type cloudDefaultReviewerList struct {
	Values []cloudDefaultReviewer `json:"values"`
	Next   string                 `json:"next"`
}

// dcDefaultReviewerItem covers both response shapes seen across Data Center
// versions: older servers return users directly, while newer API schemas wrap
// reviewers in the matching pull-request condition.
type dcDefaultReviewerItem struct {
	Name      string   `json:"name"`
	Reviewers []dcUser `json:"reviewers"`
}

func (c *apiClient) resolveDefaultReviewers(ctx context.Context, req CreatePRReq) ([]string, error) {
	if c.flavor == FlavorCloud {
		return c.cloudEffectiveDefaultReviewers(ctx, req.Repo)
	}
	return c.dcEffectiveDefaultReviewers(ctx, req)
}

func (c *apiClient) cloudEffectiveDefaultReviewers(ctx context.Context, repo RepoRef) ([]string, error) {
	path := c.repoPath(repo) + "/effective-default-reviewers"
	result := []string{}
	seen := map[string]bool{}
	for path != "" {
		var page cloudDefaultReviewerList
		if err := c.getJSON(ctx, path, nil, &page); err != nil {
			return nil, err
		}
		for _, reviewer := range page.Values {
			id := strings.TrimSpace(reviewer.User.UUID)
			if id != "" && !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
		path = page.Next
	}
	return result, nil
}

func (c *apiClient) dcEffectiveDefaultReviewers(ctx context.Context, req CreatePRReq) ([]string, error) {
	targetRepo, err := c.dcGetRepoRaw(ctx, req.Repo)
	if err != nil {
		return nil, err
	}
	if targetRepo.ID == 0 {
		return nil, cerrors.New(cerrors.CategoryParse, "PR_TARGET_REPO_NO_ID",
			"the target repository response did not include its Data Center ID")
	}

	sourceRepoRef := req.Repo
	sourceRepo := targetRepo
	if req.SourceRepo != "" {
		sourceRepoRef, err = parseRepoSpec(req.SourceRepo)
		if err != nil {
			return nil, err
		}
		sourceRepo, err = c.dcGetRepoRaw(ctx, sourceRepoRef)
		if err != nil {
			return nil, err
		}
		if sourceRepo.ID == 0 {
			return nil, cerrors.New(cerrors.CategoryParse, "PR_SOURCE_REPO_NO_ID",
				"the source repository response did not include its Data Center ID")
		}
	}

	targetRefID := branchRefID(req.Destination)
	if targetRefID == "" {
		var defaultBranch struct {
			ID string `json:"id"`
		}
		if err := c.getJSON(ctx, c.repoPath(req.Repo)+"/default-branch", nil, &defaultBranch); err != nil {
			return nil, err
		}
		targetRefID = strings.TrimSpace(defaultBranch.ID)
		if targetRefID == "" {
			return nil, cerrors.New(cerrors.CategoryParse, "PR_TARGET_BRANCH_MISSING",
				"the target repository response did not include a default branch")
		}
	}

	path := "/rest/default-reviewers/latest/projects/" + url.PathEscape(req.Repo.Workspace) +
		"/repos/" + url.PathEscape(req.Repo.Slug) + "/reviewers"
	q := url.Values{}
	q.Set("sourceRepoId", strconv.Itoa(sourceRepo.ID))
	q.Set("targetRepoId", strconv.Itoa(targetRepo.ID))
	q.Set("sourceRefId", branchRefID(req.Source))
	q.Set("targetRefId", targetRefID)

	var raw []dcDefaultReviewerItem
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return nil, err
	}
	result := []string{}
	seen := map[string]bool{}
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	for _, item := range raw {
		appendName(item.Name)
		for _, reviewer := range item.Reviewers {
			appendName(reviewer.Name)
		}
	}
	return result, nil
}

func (c *apiClient) dcGetRepoRaw(ctx context.Context, repo RepoRef) (*dcRepo, error) {
	var raw dcRepo
	if err := c.getJSON(ctx, c.repoPath(repo), nil, &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func branchRefID(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "refs/") {
		return name
	}
	return "refs/heads/" + name
}
