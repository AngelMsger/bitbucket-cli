package apiclient

import (
	"context"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// CurrentUser returns the user the configured credentials authenticate as.
//
// Cloud: GET /2.0/user returns the record directly.
//
// Data Center has no "current user" endpoint. It does stamp every authenticated
// response with an X-AUSERNAME header naming the caller, so the username is
// recovered from a cheap authenticated request and then resolved to a full
// record. The indirection earns its keep: without a username a caller cannot
// address its own personal project (~username), which is where forks live.
func (c *apiClient) CurrentUser(ctx context.Context) (*User, error) {
	if c.flavor == FlavorCloud {
		var raw cloudUser
		if err := c.getJSON(ctx, c.apiBase()+"/user", nil, &raw); err != nil {
			return nil, err
		}
		u := mapCloudUser(raw)
		return &u, nil
	}

	// Application properties is a cheap, deployment-wide endpoint that does not
	// require user-directory browsing permission. Authentication middleware still
	// stamps its response with the current username.
	username, err := c.getResponseHeader(ctx, c.apiBase()+"/application-properties", "X-AUSERNAME", nil)
	if err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, cerrors.New(cerrors.CategoryAuth, "AUTH_IDENTITY_UNAVAILABLE",
			"Bitbucket Data Center did not identify the authenticated user in the X-AUSERNAME response header").
			WithHint("Ensure the request is authenticated and that a reverse proxy is not stripping X-AUSERNAME.").
			WithNextSteps(
				"bitbucket-cli auth status",
				"bitbucket-cli doctor",
				"Check the Bitbucket or reverse-proxy response-header configuration",
			)
	}

	// The header gives a username; the record adds the display name. Failing to
	// fetch it is not fatal — the username is the part callers act on. An empty
	// record counts as a failure to resolve rather than as a resolution: some
	// deployments answer the lookup with a body carrying nothing useful.
	resolved := &User{Type: "dc", Name: username, Slug: username}
	if user, err := c.GetUser(ctx, username); err == nil && user != nil &&
		(user.Slug != "" || user.Name != "" || user.DisplayName != "") {
		resolved = user
		resolved.Type = "dc"
		if resolved.Slug == "" {
			resolved.Slug = username
		}
		if resolved.Name == "" {
			resolved.Name = username
		}
	}
	return resolved, nil
}
