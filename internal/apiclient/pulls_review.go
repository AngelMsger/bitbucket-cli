package apiclient

import (
	"context"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ApprovePR toggles an approval on a PR.
//
//	Cloud:       POST/DELETE .../approve
//	Data Center: POST/DELETE .../approve
func (c *apiClient) ApprovePR(ctx context.Context, req ApprovePRReq) error {
	if err := checkRepoRef(req.Repo); err != nil {
		return err
	}
	method := "POST"
	if !req.Approve {
		method = "DELETE"
	}
	return c.doJSON(ctx, method, c.prPath(req.Repo, req.ID)+"/approve", nil, nil, nil)
}

// RequestPRChanges toggles a "request changes" vote. Cloud supports this via
// the request-changes endpoint; Data Center exposes the same concept via the
// reviewer status APIs but the flow differs, so we surface a clear error there.
func (c *apiClient) RequestPRChanges(ctx context.Context, req RequestChangesReq) error {
	if err := checkRepoRef(req.Repo); err != nil {
		return err
	}
	if c.flavor != FlavorCloud {
		return cerrors.New(cerrors.CategoryUsage, "PR_REQ_CHANGES_DC",
			"request-changes is only available on Bitbucket Cloud").
			WithHint("On Data Center, decline the PR or post a comment to request changes.")
	}
	method := "POST"
	if !req.Request {
		method = "DELETE"
	}
	return c.doJSON(ctx, method, c.prPath(req.Repo, req.ID)+"/request-changes", nil, nil, nil)
}
