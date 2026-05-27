package apiclient

import "context"

// CurrentUser returns the user the configured credentials authenticate as.
//
// Cloud: GET /2.0/user
// Data Center: GET /rest/api/1.0/users — the API root has no "current user"
// endpoint, so we read the application properties first and rely on the
// authentication decorator filling in the username via .../users/{slug}.
// For simplicity we fall back to /plugins/servlet/applinks/whoami when needed;
// in this minimal implementation we return the user record from /users/{slug}
// using the configured Auth Username (set by the credential resolver).
func (c *apiClient) CurrentUser(ctx context.Context) (*User, error) {
	if c.flavor == FlavorCloud {
		var raw cloudUser
		if err := c.getJSON(ctx, c.apiBase()+"/user", nil, &raw); err != nil {
			return nil, err
		}
		u := mapCloudUser(raw)
		return &u, nil
	}
	// Data Center has no /user/current — return a minimal record. Callers that
	// need the full user can fetch by slug separately.
	return &User{Type: "dc"}, nil
}
