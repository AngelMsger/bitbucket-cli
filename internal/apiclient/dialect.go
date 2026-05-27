package apiclient

import (
	"net/url"
	"strconv"
	"strings"
)

// urlPathEscape escapes a single path segment using net/url's rules.
func urlPathEscape(s string) string { return url.PathEscape(s) }

// dialect.go centralises per-flavor REST differences:
//   - Cloud serves REST 2.0 under /2.0
//   - Data Center serves REST 1.0 under /rest/api/1.0
//
// Cloud uses cursor pagination (response carries an absolute `next` URL).
// Data Center uses offset pagination (start / limit / isLastPage).

// apiBase is the REST root for the configured flavor.
func (c *apiClient) apiBase() string {
	if c.flavor == FlavorCloud {
		return "/2.0"
	}
	return "/rest/api/1.0"
}

// repoPath is the URL path for a repository's REST root.
// Cloud:        /2.0/repositories/{workspace}/{repo}
// Data Center:  /rest/api/1.0/projects/{project}/repos/{repo}
func (c *apiClient) repoPath(ref RepoRef) string {
	if c.flavor == FlavorCloud {
		return c.apiBase() + "/repositories/" + url.PathEscape(ref.Workspace) + "/" + url.PathEscape(ref.Slug)
	}
	return c.apiBase() + "/projects/" + url.PathEscape(ref.Workspace) + "/repos/" + url.PathEscape(ref.Slug)
}

// reposPath is the URL path for listing repositories.
// Cloud:        /2.0/repositories/{workspace}
// Data Center:  /rest/api/1.0/projects/{project}/repos
func (c *apiClient) reposPath(workspace string) string {
	if c.flavor == FlavorCloud {
		return c.apiBase() + "/repositories/" + url.PathEscape(workspace)
	}
	return c.apiBase() + "/projects/" + url.PathEscape(workspace) + "/repos"
}

// prSegment is the URL segment used for pull requests under a repository.
//
//	Cloud:       "pullrequests"
//	Data Center: "pull-requests"
func (c *apiClient) prSegment() string {
	if c.flavor == FlavorCloud {
		return "pullrequests"
	}
	return "pull-requests"
}

// prPath returns the path of a specific PR.
func (c *apiClient) prPath(ref RepoRef, id int) string {
	return c.repoPath(ref) + "/" + c.prSegment() + "/" + strconv.Itoa(id)
}

// prsPath returns the path of the PR collection.
func (c *apiClient) prsPath(ref RepoRef) string {
	return c.repoPath(ref) + "/" + c.prSegment()
}

// branchesPath returns the path of the branch collection.
//
//	Cloud:       /repositories/{ws}/{repo}/refs/branches
//	Data Center: /projects/{key}/repos/{repo}/branches
func (c *apiClient) branchesPath(ref RepoRef) string {
	if c.flavor == FlavorCloud {
		return c.repoPath(ref) + "/refs/branches"
	}
	return c.repoPath(ref) + "/branches"
}

// commitsPath returns the path of the commit collection.
func (c *apiClient) commitsPath(ref RepoRef) string {
	return c.repoPath(ref) + "/commits"
}

// queryWithLimit builds a query string with pagination params. Cloud and DC
// both accept their respective limit / page params via this helper.
func (c *apiClient) queryWithLimit(cursor string, limit int) url.Values {
	q := url.Values{}
	if c.flavor == FlavorCloud {
		q.Set("pagelen", strconv.Itoa(limit))
		// Cloud uses absolute `next` URLs; when present we follow them directly,
		// so the cursor here is unused at request build time.
		return q
	}
	// Data Center: start / limit
	start := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil {
			start = n
		}
	}
	q.Set("start", strconv.Itoa(start))
	q.Set("limit", strconv.Itoa(limit))
	return q
}

// nextOffsetToken computes the cursor for the following DC page.
func nextOffsetToken(cursor string, limit, size int, isLastPage bool) string {
	if isLastPage || limit <= 0 || size < limit {
		return ""
	}
	start := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil {
			start = n
		}
	}
	return strconv.Itoa(start + limit)
}

// cloudNextCursor extracts the cursor portion (the `page` value or the whole
// URL) from a Cloud absolute `next` link.
func cloudNextCursor(raw string) string {
	if raw == "" {
		return ""
	}
	// Cloud returns an absolute URL; we pass it through verbatim so the caller
	// can hit it as-is in the next fetch.
	return raw
}

// cloudFollowURL is true when the cursor is an absolute Cloud pagination URL.
func cloudFollowURL(cursor string) bool {
	return strings.HasPrefix(cursor, "http://") || strings.HasPrefix(cursor, "https://")
}

// --- v0.2 path helpers ---

// srcPath returns the URL for browsing source at a ref + path on Cloud
// (`/2.0/repositories/{ws}/{slug}/src/{ref}/{path}`). The ref segment is not
// percent-escaped (Bitbucket treats slashes as part of the ref segment), but
// path components are.
func (c *apiClient) srcPath(ref RepoRef, gitRef, p string) string {
	base := c.repoPath(ref) + "/src/" + gitRef
	if p == "" {
		return base
	}
	return base + "/" + escapePath(p)
}

// filesPath returns the DC URL for listing files at a ref + path
// (`/rest/api/1.0/projects/{key}/repos/{slug}/files/{path}`). The ref is
// passed as `?at=<ref>` by the caller (it's a query param on DC).
func (c *apiClient) filesPath(ref RepoRef, p string) string {
	base := c.repoPath(ref) + "/files"
	if p == "" {
		return base
	}
	return base + "/" + escapePath(p)
}

// rawPath returns the DC URL for the raw byte content of a file
// (`/rest/api/1.0/projects/{key}/repos/{slug}/raw/{path}`).
func (c *apiClient) rawPath(ref RepoRef, p string) string {
	base := c.repoPath(ref) + "/raw"
	if p == "" {
		return base
	}
	return base + "/" + escapePath(p)
}

// prDiffstatPath is the Cloud per-file change summary endpoint.
func (c *apiClient) prDiffstatPath(ref RepoRef, id int) string {
	return c.prPath(ref, id) + "/diffstat"
}

// prChangesPath is the DC counterpart to Cloud's diffstat — same shape, named
// `/changes` because DC mixes change metadata with rename detection there.
func (c *apiClient) prChangesPath(ref RepoRef, id int) string {
	return c.prPath(ref, id) + "/changes"
}

// prMergePath is the DC pre-merge check endpoint:
// `/rest/api/1.0/.../pull-requests/{id}/merge` (GET returns mergeable verdict;
// POST performs the actual merge). Cloud has no direct counterpart; the CLI
// derives mergeability from the PR state on Cloud.
func (c *apiClient) prMergePath(ref RepoRef, id int) string {
	return c.prPath(ref, id) + "/merge"
}

// commitStatusesPath is the per-commit CI / build status endpoint.
//
//	Cloud: /2.0/repositories/{ws}/{slug}/commit/{hash}/statuses
//	DC:    /rest/build-status/1.0/commits/{hash}  (NOTE: a different rest plugin)
func (c *apiClient) commitStatusesPath(ref RepoRef, hash string) string {
	if c.flavor == FlavorCloud {
		return c.repoPath(ref) + "/commit/" + hash + "/statuses"
	}
	// DC's build-status plugin lives outside the standard /rest/api/1.0 tree.
	return "/rest/build-status/1.0/commits/" + hash
}

// prStatusesPath is the Cloud-only per-PR aggregate of build statuses.
// On DC the CLI walks via the PR's toRef.latestCommit and the build-status plugin.
func (c *apiClient) prStatusesPath(ref RepoRef, id int) string {
	return c.prPath(ref, id) + "/statuses"
}

// escapePath URL-escapes a path's segments but preserves "/" separators so
// nested paths still resolve. (`url.PathEscape("a/b")` returns "a%2Fb", which
// the Bitbucket source endpoints reject as a single segment.)
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = pathSegmentEscape(s)
	}
	return strings.Join(parts, "/")
}

// pathSegmentEscape mirrors url.PathEscape but is a thin wrapper to keep the
// rule local — Bitbucket accepts the standard pchar set including `:@!$&'()*+,;=`.
func pathSegmentEscape(s string) string { return urlPathEscape(s) }
