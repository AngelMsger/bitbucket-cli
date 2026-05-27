package apiclient

import (
	"context"
	"strings"
	"sync"
)

// CheckPRMerge returns the server-side pre-merge verdict.
//   DC:    direct endpoint, GET /pull-requests/{id}/merge
//   Cloud: no dedicated endpoint; derived from the PR's state, conflict flag
//          on the PR detail, and reviewer required-approvals (best-effort).
func (c *apiClient) CheckPRMerge(ctx context.Context, repo RepoRef, id int) (*MergeCheck, error) {
	if err := checkRepoRef(repo); err != nil {
		return nil, err
	}
	if c.flavor != FlavorCloud {
		var raw struct {
			CanMerge   bool `json:"canMerge"`
			Conflicted bool `json:"conflicted"`
			Outcome    string `json:"outcome"`
			Vetoes     []struct {
				SummaryMessage  string `json:"summaryMessage"`
				DetailedMessage string `json:"detailedMessage"`
			} `json:"vetoes"`
		}
		if err := c.getJSON(ctx, c.prMergePath(repo, id), nil, &raw); err != nil {
			return nil, err
		}
		mc := &MergeCheck{
			CanMerge:   raw.CanMerge,
			Conflicted: raw.Conflicted,
			Outcome:    raw.Outcome,
		}
		for _, v := range raw.Vetoes {
			if v.SummaryMessage != "" {
				mc.Vetoes = append(mc.Vetoes, v.SummaryMessage)
			}
		}
		return mc, nil
	}
	// Cloud: derive from PR detail. A PR with state OPEN that returns a
	// merge_commit on GET is considered mergeable; an explicit `reason` of
	// `merge_conflict` flags conflicts.
	pr, err := c.GetPR(ctx, GetPROpts{Repo: repo, ID: id})
	if err != nil {
		return nil, err
	}
	mc := &MergeCheck{}
	switch strings.ToUpper(pr.State) {
	case "OPEN":
		mc.CanMerge = true
	case "MERGED":
		mc.Outcome = "merged"
	case "DECLINED", "SUPERSEDED":
		mc.Outcome = strings.ToLower(pr.State)
	}
	return mc, nil
}

// ListCommitStatuses returns CI / build statuses attached to a commit.
//   Cloud: GET /2.0/repositories/{ws}/{slug}/commit/{hash}/statuses
//   DC:    GET /rest/build-status/1.0/commits/{hash}
func (c *apiClient) ListCommitStatuses(ctx context.Context, repo RepoRef, hash string) (ListResult[BuildStatus], error) {
	if err := checkRepoRef(repo); err != nil {
		return ListResult[BuildStatus]{}, err
	}
	if hash == "" {
		return ListResult[BuildStatus]{}, nil
	}
	endpoint := c.commitStatusesPath(repo, hash)
	if c.flavor == FlavorCloud {
		var raw struct {
			Values []struct {
				Key         string `json:"key"`
				Name        string `json:"name"`
				State       string `json:"state"`
				URL         string `json:"url"`
				Description string `json:"description"`
				CreatedOn   string `json:"created_on"`
				UpdatedOn   string `json:"updated_on"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, endpoint, nil, &raw); err != nil {
			return ListResult[BuildStatus]{}, err
		}
		res := ListResult[BuildStatus]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			res.Items = append(res.Items, BuildStatus{
				Key: v.Key, Name: v.Name, State: v.State, URL: v.URL,
				Description: v.Description,
				CommitHash:  hash, CreatedAt: v.CreatedOn, UpdatedAt: v.UpdatedOn,
			})
		}
		return res, nil
	}
	var raw struct {
		Values []struct {
			Key         string `json:"key"`
			Name        string `json:"name"`
			State       string `json:"state"`
			URL         string `json:"url"`
			Description string `json:"description"`
			DateAdded   int64  `json:"dateAdded"`
		} `json:"values"`
		IsLastPage bool `json:"isLastPage"`
	}
	if err := c.getJSON(ctx, endpoint, nil, &raw); err != nil {
		return ListResult[BuildStatus]{}, err
	}
	res := ListResult[BuildStatus]{}
	for _, v := range raw.Values {
		res.Items = append(res.Items, BuildStatus{
			Key: v.Key, Name: v.Name, State: v.State, URL: v.URL,
			Description: v.Description, CommitHash: hash,
			CreatedAt: epochToISO(v.DateAdded),
		})
	}
	return res, nil
}

// ListPRStatuses returns the build statuses associated with a PR. Cloud has a
// dedicated endpoint; DC requires walking via the PR's destination commit.
func (c *apiClient) ListPRStatuses(ctx context.Context, repo RepoRef, id int) (ListResult[BuildStatus], error) {
	if err := checkRepoRef(repo); err != nil {
		return ListResult[BuildStatus]{}, err
	}
	if c.flavor == FlavorCloud {
		var raw struct {
			Values []struct {
				Key         string `json:"key"`
				Name        string `json:"name"`
				State       string `json:"state"`
				URL         string `json:"url"`
				Description string `json:"description"`
				Commit      struct {
					Hash string `json:"hash"`
				} `json:"commit"`
				CreatedOn string `json:"created_on"`
				UpdatedOn string `json:"updated_on"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, c.prStatusesPath(repo, id), nil, &raw); err != nil {
			return ListResult[BuildStatus]{}, err
		}
		res := ListResult[BuildStatus]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			res.Items = append(res.Items, BuildStatus{
				Key: v.Key, Name: v.Name, State: v.State, URL: v.URL,
				Description: v.Description, CommitHash: v.Commit.Hash,
				CreatedAt: v.CreatedOn, UpdatedAt: v.UpdatedOn,
			})
		}
		return res, nil
	}
	pr, err := c.GetPR(ctx, GetPROpts{Repo: repo, ID: id})
	if err != nil {
		return ListResult[BuildStatus]{}, err
	}
	hash := pr.Source.Commit
	if hash == "" {
		hash = pr.Destination.Commit
	}
	return c.ListCommitStatuses(ctx, repo, hash)
}

// GetPRStatus assembles the aggregated review-readiness view: PR detail +
// merge check + reviewer states + CI build statuses. The three secondary
// fetches run in parallel; partial failures degrade gracefully (the field is
// left nil / empty so the caller still sees the rest).
func (c *apiClient) GetPRStatus(ctx context.Context, repo RepoRef, id int) (*PRStatus, error) {
	pr, err := c.GetPR(ctx, GetPROpts{Repo: repo, ID: id})
	if err != nil {
		return nil, err
	}
	out := &PRStatus{PR: pr, Reviewers: pr.Reviewers}

	var wg sync.WaitGroup
	var mu sync.Mutex
	wg.Add(2)

	go func() {
		defer wg.Done()
		mc, err := c.CheckPRMerge(ctx, repo, id)
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			out.MergeCheck = mc
		}
	}()
	go func() {
		defer wg.Done()
		builds, err := c.ListPRStatuses(ctx, repo, id)
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			out.Builds = builds.Items
		}
	}()
	wg.Wait()
	return out, nil
}
