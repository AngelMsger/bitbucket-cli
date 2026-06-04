package app

import (
	"fmt"

	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newPRFilesCmd lists changed files (diffstat) on a PR — pure metadata for
// budgeting context before pulling per-file diffs.
func newPRFilesCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "files <workspace>/<repo>/<id>",
		Short: "List changed files in a PR (diffstat: path / status / added / removed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			res, err := client.ListPRFiles(ctx, ref, id)
			if err != nil {
				return err
			}
			return s.emitList(res.Items, pageInfo{Next: res.Next, HasMore: res.Next != ""})
		},
	}
}

// newPRThreadsCmd regroups PR comments into per-file inline threads.
func newPRThreadsCmd(s *appState) *cobra.Command {
	var (
		unresolved bool
		commentID  int
	)
	cmd := &cobra.Command{
		Use:   "threads <workspace>/<repo>/<id>",
		Short: "List PR review threads grouped by file and anchor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			res, err := client.ListPRThreads(ctx, ref, id)
			if err != nil {
				return err
			}
			items := res.Items
			// --comment selects the single thread containing that comment id
			// (root or any reply); it takes precedence over --unresolved.
			if commentID != 0 {
				for _, t := range items {
					if threadHasComment(t, commentID) {
						return s.emitList([]apiclient.Thread{t}, pageInfo{})
					}
				}
				return cerrors.New(cerrors.CategoryNotFound, "COMMENT_NOT_FOUND",
					fmt.Sprintf("no thread on this PR contains comment %d", commentID)).
					WithNextSteps("bitbucket-cli pr threads " + args[0])
			}
			if unresolved {
				filtered := items[:0]
				for _, t := range items {
					if t.Resolved {
						continue
					}
					filtered = append(filtered, t)
				}
				items = filtered
			}
			return s.emitList(items, pageInfo{})
		},
	}
	cmd.Flags().BoolVar(&unresolved, "unresolved", false, "only threads that are not resolved")
	cmd.Flags().IntVar(&commentID, "comment", 0, "only the thread containing this comment id (root or reply)")
	return cmd
}

// threadHasComment reports whether thread t contains a comment with the given id
// anywhere in its reply tree.
func threadHasComment(t apiclient.Thread, id int) bool {
	for _, c := range t.Comments {
		if c.ID == id {
			return true
		}
	}
	return false
}

// newPRStatusCmd aggregates "is this PR ready to merge?": PR detail + merge
// check + reviewer states + CI build statuses.
func newPRStatusCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "status <workspace>/<repo>/<id>",
		Short: "Show merge readiness: mergeable, conflicts, reviewers, CI builds",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			st, err := client.GetPRStatus(ctx, ref, id)
			if err != nil {
				return err
			}
			return s.emit(st)
		},
	}
}
