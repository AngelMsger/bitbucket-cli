package apiclient

import (
	"context"
	"fmt"
	"net/url"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// ApprovePR toggles an approval on a PR.
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

// RequestPRChanges casts or withdraws the authenticated user's change request.
func (c *apiClient) RequestPRChanges(ctx context.Context, req RequestChangesReq) error {
	method, path, payload, err := c.buildRequestPRChanges(ctx, req)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, method, path, nil, payload, nil)
}

// Preview and execution share the same participant-state guard.
func (c *apiClient) buildRequestPRChanges(ctx context.Context, req RequestChangesReq) (method, path string, payload any, err error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return "", "", nil, err
	}
	if c.flavor == FlavorDataCenter {
		me, err := c.CurrentUser(ctx)
		if err != nil {
			return "", "", nil, err
		}
		status := "NEEDS_WORK"
		if !req.Request {
			if err := c.requireDCChangeRequest(ctx, req, me.Slug); err != nil {
				return "", "", nil, err
			}
			status = "UNAPPROVED"
		}
		return "PUT", c.prPath(req.Repo, req.ID) + "/participants/" + url.PathEscape(me.Slug),
			map[string]any{"status": status}, nil
	}
	method = "POST"
	if !req.Request {
		method = "DELETE"
	}
	return method, c.prPath(req.Repo, req.ID) + "/request-changes", nil, nil
}

func (c *apiClient) requireDCChangeRequest(ctx context.Context, req RequestChangesReq, userSlug string) error {
	pr, err := c.GetPR(ctx, GetPROpts{Repo: req.Repo, ID: req.ID, Scope: PRScopeFull})
	if err != nil {
		return err
	}
	needsWork, otherState := false, false
	for _, group := range [][]Participant{pr.Reviewers, pr.Participants} {
		for _, participant := range group {
			if !userMatchesSelector(participant.User, userSlug) {
				continue
			}
			if participant.State == "needs_work" && !participant.Approved {
				needsWork = true
			} else {
				otherState = true
			}
		}
	}
	if !needsWork || otherState {
		return cerrors.New(cerrors.CategoryConflict, "PR_NO_CHANGE_REQUEST",
			"No confirmed Needs Work vote by the current user can be withdrawn.").
			WithHint("The current vote was preserved. Read the participant state before retrying.").
			WithNextSteps(fmt.Sprintf("bitbucket-cli pr get %s --scope full --fields reviewers,participants", normalizedPRRef(req.Repo, req.ID)))
	}
	return nil
}
