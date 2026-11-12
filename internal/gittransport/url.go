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
