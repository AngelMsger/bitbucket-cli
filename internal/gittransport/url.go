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
	Scheme      string
	Host        string
	Port        string
	ContextPath string
	Workspace   string
	Slug        string
}

// ParseRemoteRepo extracts the normalized scheme, host, port, optional Data
// Center HTTP context path, workspace/project, and repo slug from common
// Bitbucket Cloud and Data Center git remote URLs:
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
	ctxPath, ws, slug, ok := parseBitbucketRepoPath(u.Path)
	if !ok {
		return RemoteRepo{}, false
	}
	return RemoteRepo{
		Scheme:      scheme,
		Host:        strings.ToLower(u.Hostname()),
		Port:        u.Port(),
		ContextPath: ctxPath,
		Workspace:   ws,
		Slug:        slug,
	}, true
}

// MatchesDeployment reports whether r belongs to the same Bitbucket deployment
// as baseURL. Hostnames must match strictly (mapping Cloud `api.bitbucket.org`
// to `bitbucket.org`), with no unverified SSH host-alias bypass. For HTTP(S)
// remotes, the effective port and Data Center context path must also match so
// distinct installations sharing a hostname are never conflated.
func (r RemoteRepo) MatchesDeployment(baseURL string, cloud bool) bool {
	if strings.TrimSpace(baseURL) == "" {
		return true
	}
	b, err := url.Parse(baseURL)
	if err != nil || b.Hostname() == "" {
		return false
	}
	expectedHost := normalizeDeploymentHost(b.Hostname(), cloud)
	if !strings.EqualFold(r.Host, expectedHost) {
		return false
	}
	expectedCtx := ""
	if !cloud {
		expectedCtx = strings.Trim(b.Path, "/")
	}
	switch r.Scheme {
	case "http", "https":
		if effectiveHTTPPort(r.Scheme, r.Port) != effectiveHTTPPort(strings.ToLower(b.Scheme), b.Port()) {
			return false
		}
		return strings.Trim(r.ContextPath, "/") == expectedCtx
	case "ssh":
		if r.ContextPath != "" && strings.Trim(r.ContextPath, "/") != expectedCtx {
			return false
		}
		return true
	default:
		return false
	}
}

// SameDeployment reports whether two Bitbucket HTTP(S) base URLs (for example,
// a web PR URL's deployment base and a configured API server base URL) refer to
// the same Bitbucket installation. On Bitbucket Cloud, `api.bitbucket.org` and
// `bitbucket.org` map to the same deployment on the default HTTPS port.
func SameDeployment(urlA, urlB string, cloud bool) bool {
	a, err := url.Parse(strings.TrimSpace(urlA))
	if err != nil || a.Hostname() == "" {
		return false
	}
	b, err := url.Parse(strings.TrimSpace(urlB))
	if err != nil || b.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(normalizeDeploymentHost(a.Hostname(), cloud), normalizeDeploymentHost(b.Hostname(), cloud)) {
		return false
	}
	if effectiveHTTPPort(strings.ToLower(a.Scheme), a.Port()) != effectiveHTTPPort(strings.ToLower(b.Scheme), b.Port()) {
		return false
	}
	if cloud {
		return true
	}
	return strings.Trim(a.Path, "/") == strings.Trim(b.Path, "/")
}

func normalizeDeploymentHost(host string, cloud bool) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if cloud && h == "api.bitbucket.org" {
		return "bitbucket.org"
	}
	return h
}

func effectiveHTTPPort(scheme, port string) string {
	if port != "" {
		return port
	}
	if scheme == "http" {
		return "80"
	}
	return "443"
}

// MatchesRepo reports whether r identifies the given workspace/project and slug
// (case-insensitive, matching Bitbucket's clone-URL semantics).
func (r RemoteRepo) MatchesRepo(workspace, slug string) bool {
	if workspace == "" || slug == "" {
		return false
	}
	return strings.EqualFold(r.Workspace, workspace) && strings.EqualFold(r.Slug, slug)
}

// SanitizeRemoteURL removes embedded credentials (userinfo / tokens), query
// parameters, and fragments before a git remote URL is printed to stdout or
// included in diagnostics.
func SanitizeRemoteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "<redacted-remote-url>"
		}
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	// SCP-style SSH remote: [user[:pass]@]host:path
	// Locate the last '@' before the first '/' (if any), then split host:path at
	// the first ':' after that '@' so user:password@host:path is redacted too.
	prefix := raw
	if slash := strings.IndexByte(raw, '/'); slash >= 0 {
		prefix = raw[:slash]
	}
	if at := strings.LastIndexByte(prefix, '@'); at >= 0 {
		if colon := strings.IndexByte(raw[at+1:], ':'); colon >= 0 {
			user := raw[:at]
			host := raw[at+1 : at+1+colon]
			cleanPath := raw[at+1+colon+1:]
			if host == "" {
				return "<redacted-remote-url>"
			}
			if idx := strings.IndexAny(cleanPath, "?#"); idx >= 0 {
				cleanPath = cleanPath[:idx]
			}
			if user == "git" {
				return "git@" + host + ":" + cleanPath
			}
			return host + ":" + cleanPath
		}
	}
	// Local filesystem path or opaque remote without credentials.
	if idx := strings.IndexAny(raw, "?#"); idx >= 0 {
		raw = raw[:idx]
	}
	return raw
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

func parseBitbucketRepoPath(path string) (contextPath, workspace, slug string, ok bool) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", "", "", false
	}
	parts := strings.Split(trimmed, "/")

	// Data Center browse / clone paths:
	//   [context...]/projects/<KEY>/repos/<slug>[/...]
	//   [context...]/users/<user>/repos/<slug>[/...]
	for i := 0; i+3 < len(parts); i++ {
		if strings.EqualFold(parts[i], "projects") && strings.EqualFold(parts[i+2], "repos") {
			ws, sl := parts[i+1], strings.TrimSuffix(parts[i+3], ".git")
			if safePart(ws) && safePart(sl) {
				return strings.Join(parts[:i], "/"), ws, sl, true
			}
			return "", "", "", false
		}
		if strings.EqualFold(parts[i], "users") && strings.EqualFold(parts[i+2], "repos") {
			u, sl := parts[i+1], strings.TrimSuffix(parts[i+3], ".git")
			if safePart(u) && safePart(sl) {
				if !strings.HasPrefix(u, "~") {
					u = "~" + u
				}
				return strings.Join(parts[:i], "/"), u, sl, true
			}
			return "", "", "", false
		}
	}

	// Data Center clone path: [context...]/scm/<KEY>/<slug>[.git]
	for i := 0; i+2 < len(parts); i++ {
		if strings.EqualFold(parts[i], "scm") && i+3 == len(parts) {
			ws, sl := parts[i+1], strings.TrimSuffix(parts[i+2], ".git")
			if safePart(ws) && safePart(sl) {
				return strings.Join(parts[:i], "/"), ws, sl, true
			}
			return "", "", "", false
		}
	}

	// Standard two-segment path: /<workspace-or-project>/<slug>[.git]
	if len(parts) == 2 {
		ws, sl := parts[0], strings.TrimSuffix(parts[1], ".git")
		if safePart(ws) && safePart(sl) {
			return "", ws, sl, true
		}
	}
	return "", "", "", false
}
