package apiclient

import (
	"net/url"
	"strconv"
	"strings"
)

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
//   Cloud:       "pullrequests"
//   Data Center: "pull-requests"
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
//   Cloud:       /repositories/{ws}/{repo}/refs/branches
//   Data Center: /projects/{key}/repos/{repo}/branches
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
