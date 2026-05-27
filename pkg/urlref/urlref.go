// Package urlref parses Bitbucket repository / pull-request references.
//
// A reference may be:
//   - a Cloud URL: https://bitbucket.org/<workspace>/<repo>
//   - a Cloud PR URL: https://bitbucket.org/<workspace>/<repo>/pull-requests/<id>
//   - a Data Center URL: https://<host>/projects/<KEY>/repos/<repo>
//   - a DC PR URL: https://<host>/projects/<KEY>/repos/<repo>/pull-requests/<id>
//   - a shorthand: <workspace>/<repo> or <workspace>/<repo>/<id>
package urlref

import (
	"net/url"
	"path"
	"strconv"
	"strings"
)

// Ref is a parsed Bitbucket reference. Fields are populated best-effort.
type Ref struct {
	IsURL     bool
	Workspace string // Cloud workspace slug or DC project key
	Slug      string // repository slug
	PRID      int    // pull request numeric ID, 0 if absent
	CommitID  string // commit hash, "" if absent
}

// Parse returns a best-effort Ref. The input is never modified; whitespace is
// trimmed but the original casing is preserved.
func Parse(raw string) Ref {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Ref{}
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return parseURL(s)
	}
	parts := strings.Split(s, "/")
	switch len(parts) {
	case 2:
		return Ref{Workspace: parts[0], Slug: parts[1]}
	case 3:
		r := Ref{Workspace: parts[0], Slug: parts[1]}
		if id, err := strconv.Atoi(parts[2]); err == nil {
			r.PRID = id
		}
		return r
	}
	return Ref{}
}

func parseURL(raw string) Ref {
	u, err := url.Parse(raw)
	if err != nil {
		return Ref{IsURL: true}
	}
	r := Ref{IsURL: true}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) >= 4 && segments[0] == "projects" && segments[2] == "repos" {
		r.Workspace = segments[1]
		r.Slug = segments[3]
		findPRAndCommit(segments[4:], &r)
		return r
	}
	if len(segments) >= 2 {
		r.Workspace = segments[0]
		r.Slug = segments[1]
		findPRAndCommit(segments[2:], &r)
	}
	return r
}

func findPRAndCommit(rest []string, r *Ref) {
	for i := 0; i+1 < len(rest); i++ {
		switch rest[i] {
		case "pull-requests", "pullrequests":
			if id, err := strconv.Atoi(rest[i+1]); err == nil {
				r.PRID = id
			}
		case "commits":
			r.CommitID = path.Base(rest[i+1])
		}
	}
}
