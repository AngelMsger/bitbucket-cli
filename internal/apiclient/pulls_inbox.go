package apiclient

import (
	"context"
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListMyPRs is the "PRs involving me" cross-repository view. Data Center has a
// single dashboard endpoint that supports it natively; Cloud has no global
// reviewer index, so the CLI uses different strategies per role on Cloud.
func (c *apiClient) ListMyPRs(ctx context.Context, opt MyPRListOpts) (ListResult[PullRequest], error) {
	role := normalizeInboxRole(opt.Role)
	state := normalizeInboxState(opt.State)
	if c.flavor == FlavorCloud {
		return c.cloudListMyPRs(ctx, opt, role, state)
	}
	return c.dcListDashboardPRs(ctx, opt, role, state)
}

func normalizeInboxRole(r string) string {
	switch strings.ToUpper(strings.TrimSpace(r)) {
	case "", "REVIEWER":
		return "REVIEWER"
	case "AUTHOR":
		return "AUTHOR"
	case "PARTICIPANT":
		return "PARTICIPANT"
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
// endpoint scopes by the authenticated user — no UUID handoff needed.
func (c *apiClient) dcListDashboardPRs(ctx context.Context, opt MyPRListOpts, role, state string) (ListResult[PullRequest], error) {
	limit := c.limitOf(opt.ListOpts)
	q := c.queryWithLimit(opt.Cursor, limit)
	q.Set("role", role)
	if state != "ALL" {
		q.Set("state", state)
	}
	var raw dcPRList
	if err := c.getJSON(ctx, "/rest/api/1.0/dashboard/pull-requests", q, &raw); err != nil {
		return ListResult[PullRequest]{}, err
	}
	out := ListResult[PullRequest]{
		Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage),
	}
	for _, p := range raw.Values {
		repo := RepoRef{
			Workspace: p.ToRef.Repository.Project.Key,
			Slug:      p.ToRef.Repository.Slug,
		}
		out.Items = append(out.Items, *mapDCPR(repo, p))
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
func (c *apiClient) cloudListMyPRs(ctx context.Context, opt MyPRListOpts, role, state string) (ListResult[PullRequest], error) {
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
		if state != "ALL" {
			q.Set("state", state)
		}
		q.Set("pagelen", itoa(c.limitOf(opt.ListOpts)))
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
			res.Items = append(res.Items, *mapCloudPR(repoFromCloudPR(p), p))
		}
		return res, nil
	}

	if opt.Workspace == "" {
		return ListResult[PullRequest]{}, cerrors.New(cerrors.CategoryUsage, "INBOX_NO_WORKSPACE",
			"Bitbucket Cloud requires --workspace for `pr inbox --role reviewer` (and --role participant)").
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
	q := `reviewers.uuid="` + uuidFilter + `"`
	if role == "PARTICIPANT" {
		q = `participants.uuid="` + uuidFilter + `"`
	}
	if state != "ALL" {
		q = q + ` AND state="` + state + `"`
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
			page, err := c.ListPRs(ctx, PRListOpts{
				ListOpts: ListOpts{Limit: 50},
				Repo:     repoRef, State: state, Query: q,
			})
			if err != nil {
				// 403/404 on a particular repo (permissions / archived) is
				// expected when the user spans many workspaces; skip rather
				// than abort the whole listing.
				continue
			}
			aggregated = append(aggregated, page.Items...)
		}
		if repos.Next == "" {
			break
		}
		repoCursor = repos.Next
	}
	return ListResult[PullRequest]{Items: aggregated}, nil
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
