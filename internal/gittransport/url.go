// URL matching binds authentication to one configured Bitbucket deployment.
package gittransport

import (
	"fmt"
	"net/url"
	"strings"
)

// Repository extracts a ref only inside the configured deployment. SSH ports may
// differ from HTTPS; the API supplies the HTTPS endpoint rather than port guessing.
func Repository(raw, base string, cloud bool) (string, string, bool) {
	u, err := parseURL(raw)
	if err != nil {
		return "", "", false
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" {
		return "", "", false
	}
	host := b.Hostname()
	if cloud && host == "api.bitbucket.org" {
		host = "bitbucket.org"
	}
	if !strings.EqualFold(u.Hostname(), host) {
		return "", "", false
	}
	p := u.Path
	if u.Scheme == "https" {
		if httpsPort(u) != httpsPort(b) {
			return "", "", false
		}
		prefix := strings.TrimRight(b.Path, "/") + "/scm/"
		if cloud {
			prefix = "/"
		}
		if !strings.HasPrefix(p, prefix) {
			return "", "", false
		}
		p = strings.TrimPrefix(p, prefix)
	} else if u.Scheme == "ssh" {
		p = strings.TrimPrefix(p, "/")
	} else {
		return "", "", false
	}
	parts := strings.Split(p, "/")
	if len(parts) != 2 || !safePart(parts[0]) || !safePart(parts[1]) {
		return "", "", false
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), true
}

func httpsPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	return "443"
}

// HTTPS refuses redirected credentials, alternate hosts and deployment paths.
// Bitbucket may advertise a username; it is removed before Git sees the URL.
func HTTPS(raw, base string, cloud bool) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("repository did not advertise a trusted HTTPS clone URL")
	}
	if u.User != nil {
		if _, password := u.User.Password(); password {
			return "", fmt.Errorf("clone URL contains a password")
		}
		u.User = nil
	}
	clean := u.String()
	if _, _, ok := Repository(clean, base, cloud); !ok {
		return "", fmt.Errorf("HTTPS clone URL is outside the configured Bitbucket deployment")
	}
	return clean, nil
}
func parseURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		before, after, ok := strings.Cut(raw, ":")
		if !ok || !strings.Contains(before, "@") {
			return nil, fmt.Errorf("not a remote URL")
		}
		raw = "ssh://" + before + "/" + after
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, fmt.Errorf("unsafe URL")
	}
	if u.User != nil {
		if _, password := u.User.Password(); password {
			return nil, fmt.Errorf("embedded password")
		}
	}
	return u, nil
}
func safePart(part string) bool {
	return part != "" && part != "." && part != ".." && !strings.ContainsAny(part, "\\\r\n\x00?#")
}

// RemoteRepo is the normalized repository identity parsed from a git remote URL.
type RemoteRepo struct {
	Scheme    string
	Host      string
	Workspace string
	Slug      string
}

// ParseRemoteRepo extracts the normalized host, workspace/project, and repo
// slug from common Bitbucket Cloud and Data Center git remote URLs:
//   - SCP-style SSH: git@host:ws/repo.git, git@host:scm/PROJ/repo.git, git@host:~user/repo.git
//   - Explicit SSH:  ssh://git@host:7999/PROJ/repo.git, ssh://git@host/scm/PROJ/repo.git
//   - HTTPS / HTTP:  https://host/ws/repo.git, https://host[/ctx]/scm/PROJ/repo.git,
//     https://host[/ctx]/projects/PROJ/repos/repo[/...],
//     https://host[/ctx]/users/alice/repos/repo[/...]
func ParseRemoteRepo(raw string) (RemoteRepo, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RemoteRepo{}, false
	}
	u, err := parseRemoteURLPermissive(raw)
	if err != nil || u.Hostname() == "" {
		return RemoteRepo{}, false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "ssh" && scheme != "https" && scheme != "http" {
		return RemoteRepo{}, false
	}
	ws, slug, ok := parseBitbucketRepoPath(u.Path)
	if !ok {
		return RemoteRepo{}, false
	}
	return RemoteRepo{
		Scheme:    scheme,
		Host:      strings.ToLower(u.Hostname()),
		Workspace: ws,
		Slug:      slug,
	}, true
}

// MatchesHost reports whether r belongs to the same Bitbucket deployment host
// as baseURL. Short SSH config aliases without dots (e.g. `git@bb:PROJ/demo`)
// are allowed because SSH host aliases cannot be resolved without parsing
// `~/.ssh/config`.
func (r RemoteRepo) MatchesHost(baseURL string, cloud bool) bool {
	if strings.TrimSpace(baseURL) == "" {
		return true
	}
	b, err := url.Parse(baseURL)
	if err != nil || b.Hostname() == "" {
		return true
	}
	expected := strings.ToLower(b.Hostname())
	if cloud && expected == "api.bitbucket.org" {
		expected = "bitbucket.org"
	}
	if strings.EqualFold(r.Host, expected) {
		return true
	}
	if r.Scheme == "ssh" && !strings.Contains(r.Host, ".") {
		return true
	}
	return false
}

// MatchesRepo reports whether r identifies the given workspace/project and slug
// (case-insensitive, matching Bitbucket's clone-URL semantics).
func (r RemoteRepo) MatchesRepo(workspace, slug string) bool {
	if workspace == "" || slug == "" {
		return false
	}
	return strings.EqualFold(r.Workspace, workspace) && strings.EqualFold(r.Slug, slug)
}

func parseRemoteURLPermissive(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		before, after, ok := strings.Cut(raw, ":")
		if !ok || !strings.Contains(before, "@") {
			return nil, fmt.Errorf("not a remote URL")
		}
		raw = "ssh://" + before + "/" + strings.TrimPrefix(after, "/")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(raw, "\r\n\x00") {
		return nil, fmt.Errorf("unsafe URL")
	}
	return u, nil
}

func parseBitbucketRepoPath(path string) (workspace, slug string, ok bool) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")

	// Data Center browse / clone paths:
	//   [context...]/projects/<KEY>/repos/<slug>[/...]
	//   [context...]/users/<user>/repos/<slug>[/...]
	for i := 0; i+3 < len(parts); i++ {
		if strings.EqualFold(parts[i], "projects") && strings.EqualFold(parts[i+2], "repos") {
			ws, sl := parts[i+1], strings.TrimSuffix(parts[i+3], ".git")
			if safePart(ws) && safePart(sl) {
				return ws, sl, true
			}
			return "", "", false
		}
		if strings.EqualFold(parts[i], "users") && strings.EqualFold(parts[i+2], "repos") {
			u, sl := parts[i+1], strings.TrimSuffix(parts[i+3], ".git")
			if safePart(u) && safePart(sl) {
				if !strings.HasPrefix(u, "~") {
					u = "~" + u
				}
				return u, sl, true
			}
			return "", "", false
		}
	}

	// Data Center clone path: [context...]/scm/<KEY>/<slug>[.git]
	for i := 0; i+2 < len(parts); i++ {
		if strings.EqualFold(parts[i], "scm") && i+3 == len(parts) {
			ws, sl := parts[i+1], strings.TrimSuffix(parts[i+2], ".git")
			if safePart(ws) && safePart(sl) {
				return ws, sl, true
			}
			return "", "", false
		}
	}

	// Standard two-segment path: /<workspace-or-project>/<slug>[.git]
	if len(parts) == 2 {
		ws, sl := parts[0], strings.TrimSuffix(parts[1], ".git")
		if safePart(ws) && safePart(sl) {
			return ws, sl, true
		}
	}
	return "", "", false
}
