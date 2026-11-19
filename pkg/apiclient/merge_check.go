package apiclient

import (
	"context"
	"strings"
	"sync"
	"time"
)

// CheckPRMerge returns the server-side pre-merge verdict.
//
//	DC:    direct endpoint, GET /pull-requests/{id}/merge
//	Cloud: no dedicated endpoint; derived from the PR's state, conflict flag
//	       on the PR detail, and reviewer required-approvals (best-effort).
func (c *apiClient) CheckPRMerge(ctx context.Context, repo RepoRef, id int) (*MergeCheck, error) {
	if err := checkRepoRef(repo); err != nil {
		return nil, err
	}
	if c.flavor != FlavorCloud {
		var raw struct {
			CanMerge   bool   `json:"canMerge"`
			Conflicted bool   `json:"conflicted"`
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
//
//	Cloud: GET /2.0/repositories/{ws}/{slug}/commit/{hash}/statuses
//	DC:    GET /rest/build-status/1.0/commits/{hash}
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
// merge check + reviewer states + CI build statuses + caller's incremental
// review summary (`my_review`). Secondary fetches run in parallel; partial
// failures degrade gracefully (the field is left nil / empty so the caller
// still sees the rest).
func (c *apiClient) GetPRStatus(ctx context.Context, repo RepoRef, id int) (*PRStatus, error) {
	pr, err := c.GetPR(ctx, GetPROpts{Repo: repo, ID: id})
	if err != nil {
		return nil, err
	}
	out := &PRStatus{PR: pr, Reviewers: pr.Reviewers}

	var (
		wg              sync.WaitGroup
		mu              sync.Mutex
		me              *User
		activities      []Activity
		activitiesOK    bool
		commits         []Commit
		cloudUnresolved *int
	)
	wg.Add(5)

	go func() {
		defer wg.Done()
		mc, err := c.CheckPRMerge(ctx, repo, id)
		if err == nil {
			mu.Lock()
			out.MergeCheck = mc
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		builds, err := c.ListPRStatuses(ctx, repo, id)
		if err == nil {
			mu.Lock()
			out.Builds = builds.Items
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		u, err := c.CurrentUser(ctx)
		if err == nil && u != nil {
			mu.Lock()
			me = u
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		acts, err := CollectAll(func(cursor string) (ListResult[Activity], error) {
			return c.ListPRActivity(ctx, PRListOpts{
				Repo:     repo,
				ListOpts: ListOpts{Limit: 100, Cursor: cursor},
				Query:    itoa(id),
			})
		}, 0)
		if err == nil {
			mu.Lock()
			activities = acts
			activitiesOK = true
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		// First page of commits (newest first on both Cloud and DC) gives
		// latest_commit_at without walking unbounded history.
		page, err := c.ListPRCommits(ctx, PRListOpts{
			Repo:     repo,
			ListOpts: ListOpts{Limit: 25},
			Query:    itoa(id),
		})
		if err == nil {
			mu.Lock()
			commits = page.Items
			mu.Unlock()
		}
	}()

	// Cloud does not include comment resolution status in /activity, so fetch
	// comments in parallel on Cloud to count unresolved root threads. On Data
	// Center, /activities already carries every root comment's current state.
	if c.flavor == FlavorCloud {
		wg.Add(1)
		go func() {
			defer wg.Done()
			comments, err := CollectAll(func(cursor string) (ListResult[Comment], error) {
				return c.ListPRComments(ctx, ListPRCommentsOpts{
					ListOpts: ListOpts{Limit: 100, Cursor: cursor},
					Repo:     repo,
					PRID:     id,
				})
			}, 0)
			if err == nil {
				n := countUnresolvedRootComments(comments)
				mu.Lock()
				cloudUnresolved = &n
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	out.MyReview = buildMyReview(c.flavor, pr, me, activities, activitiesOK, commits, cloudUnresolved)
	return out, nil
}

func countUnresolvedRootComments(comments []Comment) int {
	seen := map[int]bool{}
	n := 0
	for _, cm := range comments {
		if cm.ParentID != 0 || seen[cm.ID] {
			continue
		}
		seen[cm.ID] = true
		if !cm.Resolved {
			n++
		}
	}
	return n
}

func countDCUnresolvedThreads(activities []Activity) int {
	seen := map[int]bool{}
	n := 0
	for _, a := range activities {
		if a.System || a.Comment == nil || a.Comment.ParentID != 0 {
			continue
		}
		if seen[a.Comment.ID] {
			continue
		}
		seen[a.Comment.ID] = true
		if !a.Comment.Resolved {
			n++
		}
	}
	return n
}

func buildMyReview(flavor Flavor, pr *PullRequest, me *User, activities []Activity, activitiesOK bool, commits []Commit, cloudUnresolved *int) *MyReview {
	if pr == nil || me == nil {
		return nil
	}
	vote, participatedAt, lastReviewedCommit := resolveUserVote(pr, me)
	mr := &MyReview{Vote: vote}

	// Latest commit timestamp across returned PR commits.
	var latestCommitTime time.Time
	for _, cm := range commits {
		if t, ok := parseBitbucketTime(cm.Date); ok && t.After(latestCommitTime) {
			latestCommitTime = t
		}
	}
	if !latestCommitTime.IsZero() {
		mr.LatestCommitAt = formatRFC3339UTC(latestCommitTime)
	}

	// Unresolved thread count: reused from /activities on DC, or fetched from
	// /comments on Cloud.
	if flavor == FlavorCloud {
		mr.UnresolvedThreadCount = cloudUnresolved
	} else if activitiesOK {
		n := countDCUnresolvedThreads(activities)
		mr.UnresolvedThreadCount = &n
	}

	if vote == VoteUnvoted {
		return mr
	}

	// Find the caller's most recent matching vote event in the activity stream.
	var votedAt time.Time
	for _, a := range activities {
		if !userMatchesUser(a.Actor, *me) {
			continue
		}
		if !activityMatchesVote(a, vote) {
			continue
		}
		if t, ok := parseBitbucketTime(a.When); ok && t.After(votedAt) {
			votedAt = t
		}
	}
	// Fall back to Cloud's participant.participated_on if activity did not have a
	// matching vote timestamp (for example, Cloud change-request timestamps).
	if votedAt.IsZero() && participatedAt != "" {
		if t, ok := parseBitbucketTime(participatedAt); ok {
			votedAt = t
		}
	}
	if !votedAt.IsZero() {
		mr.VotedAt = formatRFC3339UTC(votedAt)
	}

	// Determine whether new commits or non-system review activity occurred after
	// the caller's last vote.
	if !votedAt.IsZero() {
		updated := false
		if !latestCommitTime.IsZero() && latestCommitTime.After(votedAt) {
			updated = true
		}
		if lastReviewedCommit != "" && pr.Source.Commit != "" && !commitsMatch(pr.Source.Commit, lastReviewedCommit) {
			updated = true
		}
		if !updated && activitiesOK {
			for _, a := range activities {
				if a.System {
					continue
				}
				t, ok := parseBitbucketTime(a.When)
				if !ok || !t.After(votedAt) {
					continue
				}
				switch strings.ToLower(a.Kind) {
				case "comment", "update", "rescoped":
					updated = true
				}
				if updated {
					break
				}
			}
		}
		// Fallback when neither commits nor activity were available: compare PR updated_at.
		if !updated && latestCommitTime.IsZero() && !activitiesOK {
			if prUpdated, ok := parseBitbucketTime(pr.UpdatedAt); ok && prUpdated.After(votedAt) {
				updated = true
			}
		}
		mr.UpdatedAfterMyVote = &updated
	} else if lastReviewedCommit != "" && pr.Source.Commit != "" {
		updated := !commitsMatch(pr.Source.Commit, lastReviewedCommit)
		mr.UpdatedAfterMyVote = &updated
	}

	return mr
}

func activityMatchesVote(a Activity, vote string) bool {
	kind := strings.ToLower(strings.TrimSpace(a.Kind))
	switch vote {
	case VoteApproved:
		return kind == "approval" || kind == "approved" || a.Approved
	case VoteNeedsWork, VoteChangesRequested:
		return kind == "reviewed" || kind == "changes_requested" || kind == "needs_work" ||
			strings.EqualFold(a.State, "changes_requested") || strings.EqualFold(a.State, "needs_work")
	default:
		return false
	}
}
