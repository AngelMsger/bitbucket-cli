package apiclient

import (
	"context"
	"fmt"
	"net/url"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// A write changes remote state, so a failure after the request was sent can
// still have applied it. The family's write-outcome rule (cerrors.FromWrite)
// keeps such a failure from reading as retryable: only a definite refusal
// passes through; anything else says the outcome is unknown and points at the
// read below, so an agent reconciles before it repeats the write. Each target
// names what the write changes and a bounded, read-only command that shows it.

// doWriteJSON sends a write and classifies its failure with the write rule.
func (c *apiClient) doWriteJSON(ctx context.Context, method, path string, query url.Values, body, out any, target cerrors.WriteTarget) error {
	return cerrors.FromWrite(c.doJSON(ctx, method, path, query, body, out), target)
}

func repoName(r RepoRef) string { return r.Workspace + "/" + r.Slug }

func prName(r RepoRef, id int) string { return fmt.Sprintf("%s/%d", repoName(r), id) }

func repoWriteTarget(r RepoRef) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: "repository " + repoName(r),
		ReadSteps:   []string{"bitbucket-cli repo get " + cerrors.ShellArg(repoName(r))},
	}
}

func forkWriteTarget(req ForkRepoReq) cerrors.WriteTarget {
	into := req.Workspace
	if into == "" {
		// Data Center forks into the caller's personal project, "~<username>".
		return cerrors.WriteTarget{
			Description: "fork of repository " + repoName(req.Source) + " in your personal project",
			ReadSteps:   []string{"bitbucket-cli whoami", "bitbucket-cli repo list --workspace '~<username>' --limit 25"},
		}
	}
	return cerrors.WriteTarget{
		Description: "fork of repository " + repoName(req.Source) + " in " + into,
		ReadSteps:   []string{"bitbucket-cli repo list --workspace " + cerrors.ShellArg(into) + " --limit 25"},
	}
}

func createPRWriteTarget(req CreatePRReq) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: fmt.Sprintf("new pull request from %s in %s", req.Source, repoName(req.Repo)),
		// --state ALL: the new PR may already be merged or declined by the
		// time it is looked for.
		ReadSteps: []string{"bitbucket-cli pr list --repo " + cerrors.ShellArg(repoName(req.Repo)) +
			" --source " + cerrors.ShellArg(req.Source) + " --state ALL --limit 25"},
	}
}

func prWriteTarget(r RepoRef, id int) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: "pull request " + prName(r, id),
		ReadSteps:   []string{"bitbucket-cli pr get " + cerrors.ShellArg(prName(r, id))},
	}
}

// prReviewWriteTarget is for a vote (approve, request changes): pr status
// shows each reviewer's current vote.
func prReviewWriteTarget(r RepoRef, id int) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: "your review on pull request " + prName(r, id),
		ReadSteps:   []string{"bitbucket-cli pr status " + cerrors.ShellArg(prName(r, id))},
	}
}

func commentWriteTarget(r RepoRef, prID, id int) cerrors.WriteTarget {
	description := "comment on pull request " + prName(r, prID)
	if id != 0 {
		description = fmt.Sprintf("comment %d on pull request %s", id, prName(r, prID))
	}
	return cerrors.WriteTarget{
		Description: description,
		ReadSteps:   []string{"bitbucket-cli comment list --pr " + cerrors.ShellArg(prName(r, prID)) + " --limit 25"},
	}
}

func branchWriteTarget(r RepoRef, name string) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: "branch " + name + " in " + repoName(r),
		ReadSteps: []string{"bitbucket-cli branch get " + cerrors.ShellArg(name) +
			" --repo " + cerrors.ShellArg(repoName(r))},
	}
}
