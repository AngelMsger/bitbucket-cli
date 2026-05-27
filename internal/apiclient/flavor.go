package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/angelmsger/bitbucket-cli/internal/transport"
)

// NormalizeBaseURL trims trailing slashes from a base URL.
func NormalizeBaseURL(raw string) string {
	return strings.TrimRight(raw, "/")
}

// Detect probes baseURL to determine the Bitbucket flavor:
//  1. Hostname shortcut. `*.bitbucket.org` or `api.bitbucket.org` → Cloud.
//  2. Cloud probe: GET https://api.bitbucket.org/2.0/user.
//  3. Cloud probe at the configured base: <base>/2.0/user.
//  4. Data Center probe: <base>/rest/api/1.0/application-properties.
func Detect(ctx context.Context, tc *transport.Client, baseURL string) (Flavor, error) {
	if isBitbucketCloudHost(baseURL) {
		return FlavorCloud, nil
	}
	base := NormalizeBaseURL(baseURL)
	if probeOK(ctx, tc, base+"/2.0/user") {
		return FlavorCloud, nil
	}
	if probeOK(ctx, tc, base+"/rest/api/1.0/application-properties") {
		return FlavorDataCenter, nil
	}
	return FlavorAuto, cerrors.New(cerrors.CategoryNetwork, "DETECT_FAILED",
		"could not determine the Bitbucket flavor; neither the Cloud nor the Data Center API responded").
		WithNextSteps("Set the flavor explicitly with --flavor cloud|datacenter.",
			"bitbucket-cli doctor")
}

// isBitbucketCloudHost reports whether rawURL points at a Bitbucket Cloud host.
func isBitbucketCloudHost(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	host := ""
	if err == nil && parsed.Host != "" {
		host = parsed.Hostname()
	} else {
		s := strings.TrimSpace(rawURL)
		s = strings.TrimPrefix(s, "//")
		if i := strings.IndexAny(s, "/?#"); i >= 0 {
			s = s[:i]
		}
		host = s
	}
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "bitbucket.org" || host == "api.bitbucket.org" ||
		strings.HasSuffix(host, ".bitbucket.org")
}

// probeOK reports whether endpoint speaks a JSON REST API. 200/401/403 with
// JSON content type are all accepted (401/403 means auth missing but server is
// the right kind).
func probeOK(ctx context.Context, client *transport.Client, endpoint string) bool {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(ctx, req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return true
	}
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return strings.Contains(resp.Header.Get("Content-Type"), "json")
}

// Ping verifies connectivity and credentials against the configured flavor.
func (c *apiClient) Ping(ctx context.Context) (ServerInfo, error) {
	info := ServerInfo{Flavor: c.flavor, BaseURL: c.baseURL}
	if _, err := c.CurrentUser(ctx); err != nil {
		return info, err
	}
	info.Reachable = true
	return info, nil
}
