package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/angelmsger/bitbucket-cli/pkg/transport"
	"github.com/angelmsger/bitbucket-cli/pkg/urlref"
	"github.com/spf13/cobra"
)

// buildProbeTransport returns an unauthenticated transport used for flavor
// detection and connectivity checks.
func buildProbeTransport(s *appState) *transport.Client {
	return transport.New(transport.Options{
		Timeout:    s.timeout(),
		MaxRetries: s.cfg().Defaults.MaxRetries,
	})
}

// cmdContext returns a context bounded by the configured request timeout.
func cmdContext(s *appState) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.timeout())
}

// resolveRepoRef parses a "<workspace>/<repo>" pair or a Bitbucket repository
// URL into a RepoRef. If workspaceFallback is non-empty and arg is a bare slug,
// it is used as the workspace.
func resolveRepoRef(arg, workspaceFallback string) (apiclient.RepoRef, error) {
	ref := urlref.Parse(arg)
	if ref.Workspace != "" && ref.Slug != "" {
		return apiclient.RepoRef{Workspace: ref.Workspace, Slug: ref.Slug}, nil
	}
	if strings.Contains(arg, "/") {
		parts := strings.SplitN(arg, "/", 2)
		return apiclient.RepoRef{Workspace: parts[0], Slug: parts[1]}, nil
	}
	if workspaceFallback != "" && arg != "" {
		return apiclient.RepoRef{Workspace: workspaceFallback, Slug: arg}, nil
	}
	return apiclient.RepoRef{}, cerrors.Newf(cerrors.CategoryUsage, "BAD_REPO",
		"could not resolve a repository from %q", arg).
		WithHint("Pass <workspace>/<repo> or a full Bitbucket repository URL.").
		WithNextSteps("Set --workspace or BITBUCKET_DEFAULT_WORKSPACE to omit the workspace.")
}

// resolvePRRef parses an argument that identifies a PR. It accepts:
//   - "<workspace>/<repo>/<id>"   (e.g. "myws/myrepo/42")
//   - "<workspace>/<repo>#<id>"
//   - a full Bitbucket PR URL
//   - a bare numeric ID, paired with a separately-supplied repo ref
func resolvePRRef(arg string, defaultRepo apiclient.RepoRef) (apiclient.RepoRef, int, error) {
	ref := urlref.Parse(arg)
	if ref.Workspace != "" && ref.Slug != "" && ref.PRID > 0 {
		return apiclient.RepoRef{Workspace: ref.Workspace, Slug: ref.Slug}, ref.PRID, nil
	}
	// "ws/repo#42" or "ws/repo/42"
	for _, sep := range []string{"#", "/"} {
		if i := strings.LastIndex(arg, sep); i > 0 {
			head, tail := arg[:i], arg[i+1:]
			if id, err := strconv.Atoi(tail); err == nil && id > 0 && strings.Count(head, "/") >= 1 {
				rr, err := resolveRepoRef(head, "")
				if err == nil {
					return rr, id, nil
				}
			}
		}
	}
	if id, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil && id > 0 {
		if defaultRepo.Slug == "" {
			return apiclient.RepoRef{}, 0, cerrors.New(cerrors.CategoryUsage, "PR_NO_REPO",
				"a bare PR ID needs a repository context").
				WithHint("Pass --repo <workspace>/<repo>, or a full <workspace>/<repo>/<id> form.")
		}
		return defaultRepo, id, nil
	}
	return apiclient.RepoRef{}, 0, cerrors.Newf(cerrors.CategoryUsage, "BAD_PR_REF",
		"could not resolve a pull request from %q", arg)
}

// addListFlags registers the shared pagination flags every list command takes.
func addListFlags(cmd *cobra.Command, limit *int, all *bool, cursor *string) {
	f := cmd.Flags()
	f.IntVar(limit, "limit", 0, "page size (default from config)")
	f.BoolVar(all, "all", false, "fetch every page of results")
	f.StringVar(cursor, "cursor", "", "start from this pagination cursor (the 'next' of a prior page)")
}

// pageInfo carries the pagination cursor for one page of a listing.
type pageInfo struct {
	Next    string
	HasMore bool
}

// collectPage fetches results for a list command. With all set it walks every
// page starting at cursor and returns the full set; otherwise it returns the
// single page at cursor plus the cursor for the page after it.
func collectPage[T any](fetch apiclient.FetchPage[T], cursor string, all bool) ([]T, pageInfo, error) {
	if all {
		var items []T
		c := cursor
		for {
			page, err := fetch(c)
			if err != nil {
				return items, pageInfo{}, err
			}
			items = append(items, page.Items...)
			if page.Next == "" {
				return items, pageInfo{}, nil
			}
			c = page.Next
		}
	}
	page, err := fetch(cursor)
	if err != nil {
		return nil, pageInfo{}, err
	}
	return page.Items, pageInfo{Next: page.Next, HasMore: page.Next != ""}, nil
}

// confirm reads "yes"/"y" from stdin to confirm a destructive action when
// --yes was not passed. It returns true on confirmation.
func confirm(prompt string) bool {
	// In auto / scripted contexts we never block on stdin; require --yes.
	return false
}

// emitDryRun resolves a write request into the HTTP request it would send and
// emits that plan instead of performing the write. It is the shared entry
// point for every command's `--dry-run` branch.
func emitDryRun(s *appState, client apiclient.Client, ctx context.Context, op any) error {
	plan, err := client.DescribeWrite(ctx, op)
	if err != nil {
		return err
	}
	return s.emit(plan)
}
