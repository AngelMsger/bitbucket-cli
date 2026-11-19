package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// ListMyPRs is the "PRs involving me" cross-repository view. Data Center has a
// single dashboard endpoint that supports it natively; Cloud has no global
// reviewer index, so the CLI uses different strategies per role on Cloud.
func (c *apiClient) ListMyPRs(ctx context.Context, opt MyPRListOpts) (ListResult[PullRequest], error) {
	myVote, err := NormalizeMyVoteFilter(opt.MyVote)
	if err != nil {
		return ListResult[PullRequest]{}, err
	}
	role := normalizeInboxRole(opt.Role)
	state := normalizeInboxState(opt.State)
	if c.flavor == FlavorCloud {
		return c.cloudListMyPRs(ctx, opt, role, state, myVote)
	}
	return c.dcListDashboardPRs(ctx, opt, role, state, myVote)
}

func normalizeInboxRole(r string) string {
	switch strings.ToUpper(strings.TrimSpace(r)) {
	case "", "REVIEWER":
		return "REVIEWER"
	case "AUTHOR":
		return "AUTHOR"
	case "PARTICIPANT":
		return "PARTICIPANT"
	case "ANY", "ALL":
		return ""
	}
	return strings.ToUpper(r)
}

func normalizeInboxState(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "OPEN":
		return "OPEN"
	case "MERGED":
		return "MERGED"
	case "DECLINED":
		return "DECLINED"
	case "ALL":
		return "ALL"
	}
	return strings.ToUpper(s)
}

// dcListDashboardPRs hits the single Data Center dashboard endpoint:
// GET /rest/api/1.0/dashboard/pull-requests?role=REVIEWER&state=OPEN. The
// endpoint scopes by the authenticated user and stamps X-AUSERNAME on the
// response, so my_vote is resolved without N+1 requests.
func (c *apiClient) dcListDashboardPRs(ctx context.Context, opt MyPRListOpts, role, state, myVote string) (ListResult[PullRequest], error) {
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	if role != "" {
		q.Set("role", role)
	}
	if state != "ALL" {
		q.Set("state", state)
	}
	if opt.ClosedSinceSeconds > 0 {
		q.Set("closedSince", strconv.FormatInt(opt.ClosedSinceSeconds, 10))
		q.Set("order", "CLOSED_DATE")
	}
	var raw dcPRList
	username, err := c.getJSONWithHeader(ctx, "/rest/api/1.0/dashboard/pull-requests", q, "X-AUSERNAME", &raw)
	if err != nil {
		return ListResult[PullRequest]{}, err
	}
	var me *User
	if u := strings.TrimSpace(username); u != "" {
		me = &User{Name: u, Slug: u}
	} else if len(raw.Values) > 0 || myVote != "" {
		resolved, uerr := c.CurrentUser(ctx)
		if uerr != nil {
			if myVote != "" {
				return ListResult[PullRequest]{}, uerr
			}
		} else {
			me = resolved
		}
	}
	out := ListResult[PullRequest]{
		Next: nextOffsetToken(raw.dcPage),
	}
	for _, p := range raw.Values {
		repo := RepoRef{
			Workspace: p.ToRef.Repository.Project.Key,
			Slug:      p.ToRef.Repository.Slug,
		}
		pr := *mapDCPR(repo, p)
		if me != nil {
			pr.MyVote, _, _ = resolveUserVote(&pr, me)
		}
		if myVote != "" && !voteMatchesFilter(pr.MyVote, myVote) {
			continue
		}
		out.Items = append(out.Items, pr)
	}
	return out, nil
}

// cloudListMyPRs routes by role:
//   - AUTHOR: a single global call to /2.0/pullrequests/{selected_user} —
//     covers every workspace the user can see, no fan-out.
//   - REVIEWER / PARTICIPANT: requires opt.Workspace; iterates repos under
//     that workspace and filters PRs server-side with `q=reviewers.uuid="..."`.
//     Cloud has no global "PRs awaiting my review" index, so cross-workspace
//     queries would have to enumerate the user's workspaces — left to a
//     future iteration; the CLI surfaces a clear usage error for now.
func (c *apiClient) cloudListMyPRs(ctx context.Context, opt MyPRListOpts, role, state, myVote string) (ListResult[PullRequest], error) {
	if opt.ClosedSinceSeconds > 0 {
		sup := c.supportFor(CapPRInboxClosedSince)
		return ListResult[PullRequest]{}, cerrors.New(cerrors.CategoryUsage, "INBOX_CLOSED_SINCE_UNSUPPORTED",
			"--closed-since is not available on Bitbucket Cloud: "+sup.Reason).
			WithHint("Scope Cloud activity by repository or pass explicit PR refs to `pr activity`.")
	}
	me, err := c.CurrentUser(ctx)
	if err != nil {
		return ListResult[PullRequest]{}, err
	}
	selector := firstNonEmpty(me.UUID, me.Slug, me.Name)
	if selector == "" {
		return ListResult[PullRequest]{}, cerrors.New(cerrors.CategoryInternal, "NO_USER_SELECTOR",
			"could not derive the current user's identifier from the /user response")
	}

	if role == "AUTHOR" {
		q := url.Values{}
		addCloudStateParams(q, state)
		q.Set("pagelen", itoa(c.limitOf(opt.ListOpts)))
		// Ask Cloud to include participant vote records on the list response so
		// my_vote is populated without per-PR GETs.
		q.Set("fields", "+values.participants")
		path := c.apiBase() + "/pullrequests/" + url.PathEscape(selector)
		if cloudFollowURL(opt.Cursor) {
			path = opt.Cursor
			q = nil
		}
		var raw cloudPRList
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return ListResult[PullRequest]{}, err
		}
		res := ListResult[PullRequest]{Next: cloudNextCursor(raw.Next)}
		for _, p := range raw.Values {
			pr := *mapCloudPR(repoFromCloudPR(p), p)
			pr.MyVote, _, _ = resolveUserVote(&pr, me)
			if myVote != "" && !voteMatchesFilter(pr.MyVote, myVote) {
				continue
			}
			res.Items = append(res.Items, pr)
		}
		return res, nil
	}

	if opt.Workspace == "" {
		return ListResult[PullRequest]{}, cerrors.New(cerrors.CategoryUsage, "INBOX_NO_WORKSPACE",
			"Bitbucket Cloud requires --workspace for `pr inbox --role reviewer`, participant, or any").
			WithHint("Cloud has no global reviewer index. Pass --workspace <ws> to scope "+
				"the search to one workspace.").
			WithNextSteps(
				"bitbucket-cli workspace list   # discover available workspaces",
				"bitbucket-cli pr inbox --workspace <ws> --role reviewer",
			)
	}

	// Fan-out: list every repo in the workspace, then per-repo q-filtered PR query.
	uuidFilter := strings.Trim(me.UUID, "{}")
	if uuidFilter == "" {
		uuidFilter = selector
	}
	qFilter := `reviewers.uuid="` + uuidFilter + `"`
	switch role {
	case "PARTICIPANT":
		qFilter = `participants.uuid="` + uuidFilter + `"`
	case "":
		qFilter = `(author.uuid="` + uuidFilter + `" OR reviewers.uuid="` + uuidFilter +
			`" OR participants.uuid="` + uuidFilter + `")`
	}
	if state != "ALL" {
		qFilter = qFilter + ` AND state="` + state + `"`
	}

	var aggregated []PullRequest
	repoCursor := ""
	for {
		repos, err := c.ListRepositories(ctx, RepoListOpts{
			ListOpts:  ListOpts{Limit: 100, Cursor: repoCursor},
			Workspace: opt.Workspace,
		})
		if err != nil {
			return ListResult[PullRequest]{}, err
		}
		for _, r := range repos.Items {
			repoRef := RepoRef{Workspace: opt.Workspace, Slug: r.Slug}
			prs, err := c.cloudListRepoPRsForInbox(ctx, repoRef, state, qFilter)
			if err != nil {
				// 403/404 on a particular repo (permissions / archived) is
				// expected when the user spans many workspaces; skip rather
				// than abort the whole listing.
				continue
			}
			for _, pr := range prs {
				pr.MyVote, _, _ = resolveUserVote(&pr, me)
				if myVote != "" && !voteMatchesFilter(pr.MyVote, myVote) {
					continue
				}
				aggregated = append(aggregated, pr)
			}
		}
		if repos.Next == "" {
			break
		}
		repoCursor = repos.Next
	}
	return ListResult[PullRequest]{Items: aggregated}, nil
}

// cloudListRepoPRsForInbox fetches matching PRs in one Cloud repository with
// `+values.participants` so reviewer vote state is available without N+1 GETs.
func (c *apiClient) cloudListRepoPRsForInbox(ctx context.Context, repo RepoRef, state, query string) ([]PullRequest, error) {
	var all []PullRequest
	cursor := ""
	for {
		path := c.prsPath(repo)
		q := url.Values{}
		if cloudFollowURL(cursor) {
			path = cursor
			q = nil
		} else {
			q.Set("pagelen", "50")
			if state != "" {
				addCloudStateParams(q, state)
			}
			if query != "" {
				q.Set("q", query)
			}
			q.Set("fields", "+values.participants")
		}
		var raw cloudPRList
		if err := c.getJSON(ctx, path, q, &raw); err != nil {
			return nil, err
		}
		for _, p := range raw.Values {
			all = append(all, *mapCloudPR(repo, p))
		}
		next := cloudNextCursor(raw.Next)
		if next == "" {
			break
		}
		cursor = next
	}
	return all, nil
}

// repoFromCloudPR derives the destination RepoRef from a Cloud PR record. The
// repository's full_name is "<workspace>/<repo-slug>"; if it is empty (some
// older PRs omit it) the function returns a zero ref and the caller falls
// back to whatever it had.
func repoFromCloudPR(p cloudPR) RepoRef {
	full := p.Destination.Repository.FullName
	if full == "" {
		full = p.Source.Repository.FullName
	}
	if i := strings.Index(full, "/"); i > 0 {
		return RepoRef{Workspace: full[:i], Slug: full[i+1:]}
	}
	return RepoRef{}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
