package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// cloudAllPRStates are the states enumerated when the caller asks for ALL:
// the Cloud PR endpoints default to OPEN when no `state` param is sent, so
// "ALL" must be spelled out as repeated params (the server ORs them).
var cloudAllPRStates = []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}

// addCloudStateParams writes the `state` query param(s) for a normalized
// (upper-cased, non-empty) state, expanding ALL into every PR state.
func addCloudStateParams(q url.Values, state string) {
	if state == "ALL" {
		for _, s := range cloudAllPRStates {
			q.Add("state", s)
		}
		return
	}
	q.Set("state", state)
}

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
		} else {
			state := strings.ToUpper(strings.TrimSpace(opt.State))
			if state != "" {
				addCloudStateParams(q, state)
			}
			filter, err := c.cloudPRFilter(ctx, opt)
			if err != nil {
				return ListResult[PullRequest]{}, err
			}
			if filter != "" {
				q.Set("q", filter)
			}
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
	if opt.Author != "" || opt.Reviewer != "" {
		if !opt.FilterAll {
			sup := c.supportFor(CapPRListUserFilters)
			example := "--author <username>"
			if opt.Author == "" {
				example = "--reviewer <username>"
			}
			return ListResult[PullRequest]{}, cerrors.New(cerrors.CategoryUsage, "PR_USER_FILTER_REQUIRES_ALL",
				"Data Center requires --all for --author or --reviewer: "+sup.Reason).
				WithHint("Use --all to opt into a complete scan of this repository before client-side filtering.").
				WithNextSteps("bitbucket-cli pr list --repo <project>/<repo> " + example + " --all")
		}
		return c.listAllDCFilteredPRs(ctx, opt)
	}
	return c.listDCPrPage(ctx, opt)
}

func (c *apiClient) cloudPRFilter(ctx context.Context, opt PRListOpts) (string, error) {
	if strings.TrimSpace(opt.Author) == "" && strings.TrimSpace(opt.Reviewer) == "" {
		return opt.Query, nil
	}
	parts := make([]string, 0, 3)
	if opt.Query != "" {
		parts = append(parts, "("+opt.Query+")")
	}
	for _, filter := range []struct {
		field    string
		selector string
	}{{"author.uuid", opt.Author}, {"reviewers.uuid", opt.Reviewer}} {
		if strings.TrimSpace(filter.selector) == "" {
			continue
		}
		user, err := c.GetUser(ctx, filter.selector)
		if err != nil {
			return "", err
		}
		uuid := strings.Trim(strings.TrimSpace(user.UUID), "{}")
		if uuid == "" {
			return "", cerrors.New(cerrors.CategoryInternal, "NO_USER_SELECTOR",
				"Bitbucket Cloud did not return a UUID for user "+filter.selector)
		}
		parts = append(parts, filter.field+"="+strconv.Quote(uuid))
	}
	return strings.Join(parts, " AND "), nil
}

func (c *apiClient) listAllDCFilteredPRs(ctx context.Context, opt PRListOpts) (ListResult[PullRequest], error) {
	var items []PullRequest
	cursor := opt.Cursor
	for {
		pageOpt := opt
		pageOpt.Cursor = cursor
		pageOpt.Author = ""
		pageOpt.Reviewer = ""
		pageOpt.FilterAll = false
		page, err := c.listDCPrPage(ctx, pageOpt)
		if err != nil {
			return ListResult[PullRequest]{}, err
		}
		for _, pr := range page.Items {
			if prMatchesUsers(pr, opt.Author, opt.Reviewer) {
				items = append(items, pr)
			}
		}
		if page.Next == "" {
			return ListResult[PullRequest]{Items: items}, nil
		}
		cursor = page.Next
	}
}

func prMatchesUsers(pr PullRequest, author, reviewer string) bool {
	if author != "" && !userMatchesSelector(pr.Author, author) {
		return false
	}
	if reviewer == "" {
		return true
	}
	for _, participant := range pr.Reviewers {
		if userMatchesSelector(participant.User, reviewer) {
			return true
		}
	}
	return false
}

func userMatchesSelector(user User, selector string) bool {
	selector = strings.ToLower(strings.Trim(strings.TrimSpace(selector), "{}"))
	for _, value := range []string{user.AccountID, user.UUID, user.Name, user.Slug} {
		if strings.ToLower(strings.Trim(strings.TrimSpace(value), "{}")) == selector {
			return true
		}
	}
	return false
}

func (c *apiClient) listDCPrPage(ctx context.Context, opt PRListOpts) (ListResult[PullRequest], error) {
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	path := c.prsPath(opt.Repo)
	// Data Center: the repo endpoint accepts state=ALL natively; omitting the
	// param would make the server default to OPEN.
	if opt.State != "" {
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
	res := ListResult[PullRequest]{Next: nextOffsetToken(raw.dcPage)}
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

// GetPRDiff returns the unified-diff text of a PR. When the server answers with
// a JSON hunk model (common on Data Center) it is rendered back to unified-diff
// text so the output is identical regardless of the wire format.
func (c *apiClient) GetPRDiff(ctx context.Context, repo RepoRef, id int) (string, error) {
	if err := checkRepoRef(repo); err != nil {
		return "", err
	}
	return c.fetchDiffText(ctx, c.prPath(repo, id)+"/diff", nil)
}

// GetPRFileDiffs returns the structured diff model for a PR, scoped to a single
// file when path is non-empty. It is the source of truth for inline-anchor
// resolution: it understands both unified-diff text and Data Center's JSON hunk
// model, so anchors resolve correctly whichever the server returned.
func (c *apiClient) GetPRFileDiffs(ctx context.Context, repo RepoRef, id int, path string) ([]FileDiff, error) {
	if err := checkRepoRef(repo); err != nil {
		return nil, err
	}
	endpoint, query := c.prDiffEndpoint(repo, id, path)
	body, ct, err := c.getDiffBody(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}
	return parseDiff(body, ct)
}

// fetchDiffText fetches a diff endpoint and returns display-ready unified-diff
// text, normalizing a JSON hunk model into text when needed.
func (c *apiClient) fetchDiffText(ctx context.Context, endpoint string, query url.Values) (string, error) {
	body, ct, err := c.getDiffBody(ctx, endpoint, query)
	if err != nil {
		return "", err
	}
	if !isJSONDiff(ct, body) {
		return body, nil
	}
	files, perr := parseDCJSONDiff(body)
	if perr != nil {
		return "", errDiffParse(ct, body, perr)
	}
	return RenderUnifiedDiff(files), nil
}

// prDiffEndpoint builds the diff endpoint (and query) for a PR, scoped to a file
// path when given. Cloud passes the path as a query parameter; Data Center puts
// it in the URL.
func (c *apiClient) prDiffEndpoint(repo RepoRef, id int, path string) (string, url.Values) {
	base := c.prPath(repo, id) + "/diff"
	if strings.TrimSpace(path) == "" {
		return base, nil
	}
	if c.flavor == FlavorCloud {
		q := url.Values{}
		q.Set("path", path)
		return base, q
	}
	return base + "/" + escapePath(path), nil
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
	res := ListResult[Commit]{Next: nextOffsetToken(raw.dcPage)}
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
	prRef := &ActivityPullRequestRef{
		ID: prID, Ref: normalizedPRRef(opt.Repo, prID), Repository: opt.Repo,
	}
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	// Cloud's path is `/activity`; Data Center uses `/activities`.
	path := c.prPath(opt.Repo, prID) + "/activity"
	if c.flavor != FlavorCloud {
		path = c.prPath(opt.Repo, prID) + "/activities"
	}
	if c.flavor == FlavorCloud {
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		// Cloud returns a heterogeneous list; decode loosely as a Values slice.
		var raw struct {
			Values []struct {
				Update *struct {
					State  string    `json:"state"`
					Date   string    `json:"date"`
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
				res.Items = append(res.Items, Activity{Kind: "comment", Actor: cm.Author, When: v.Comment.CreatedOn, PullRequest: prRef, Comment: &cm})
			case v.Approval != nil:
				res.Items = append(res.Items, Activity{Kind: "approval", Actor: mapCloudUser(v.Approval.User), When: v.Approval.Date, PullRequest: prRef, Approved: true})
			case v.Update != nil:
				res.Items = append(res.Items, Activity{Kind: "update", Actor: mapCloudUser(v.Update.Author), When: v.Update.Date, PullRequest: prRef, State: v.Update.State})
			}
		}
		return res, nil
	}
	var raw dcActivityList
	if err := c.getJSON(ctx, path, q, &raw); err != nil {
		return ListResult[Activity]{}, err
	}
	res := ListResult[Activity]{Next: nextOffsetToken(raw.dcPage)}
	for _, a := range raw.Values {
		entry := Activity{
			Kind:        strings.ToLower(a.Action),
			Actor:       mapDCUser(a.User),
			When:        epochToISO(a.CreatedDate),
			PullRequest: prRef,
		}
		if a.Comment != nil {
			cm := mapDCComment(prID, *a.Comment)
			// DC hoists an inline comment's anchor onto the activity.
			if cm.Inline == nil {
				cm.Inline = inlineFromDCAnchor(a.CommentAnchor)
			}
			entry.Kind = "comment"
			entry.Comment = &cm
			entry.System = isDCSystemComment(a.CommentAction, a.Comment.Text)
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
