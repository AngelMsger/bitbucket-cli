package app

import (
	"fmt"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
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
		Use:   "threads <workspace>/<repo>/<id>...",
		Short: "List PR review threads grouped by file and anchor",
		Long: "List PR review threads grouped by file and anchor. Pass several PR references\n" +
			"or a single '-' to read newline-separated references from stdin. With more\n" +
			"than one reference, output is a {items, has_more} aggregate with per-PR\n" +
			"ok / result / error entries and a non-zero exit code if any lookup failed.",
		Example: "  bitbucket-cli pr threads myws/myrepo/7 --unresolved\n" +
			"  bitbucket-cli pr threads myws/myrepo/7 myws/myrepo/8 --unresolved\n" +
			"  bitbucket-cli pr inbox --fields ref | jq -r '.items[].ref' | bitbucket-cli pr threads - --unresolved",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			refs, err := collectBatchArgs(args, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if len(refs) > 1 && commentID != 0 {
				return cerrors.New(cerrors.CategoryUsage, "PR_THREADS_BATCH_COMMENT",
					"--comment cannot be combined with multiple PR references").
					WithNextSteps("Pass a single PR reference when using --comment")
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			// Best-effort single identity lookup per invocation so every returned
			// comment carries `is_self` without magnifying request counts.
			me, _ := client.CurrentUser(ctx)

			fetchThreads := func(rawRef string) ([]apiclient.Thread, error) {
				ref, id, err := resolvePRRef(rawRef, apiclient.RepoRef{})
				if err != nil {
					return nil, err
				}
				res, err := client.ListPRThreads(ctx, ref, id)
				if err != nil {
					return nil, err
				}
				items := res.Items
				apiclient.EnrichThreadsWithSelf(items, me)
				if unresolved {
					filtered := make([]apiclient.Thread, 0, len(items))
					for _, t := range items {
						if t.Resolved {
							continue
						}
						filtered = append(filtered, t)
					}
					items = filtered
				}
				if items == nil {
					items = []apiclient.Thread{}
				}
				return items, nil
			}

			if len(refs) == 1 {
				ref, id, err := resolvePRRef(refs[0], apiclient.RepoRef{})
				if err != nil {
					return err
				}
				res, err := client.ListPRThreads(ctx, ref, id)
				if err != nil {
					return err
				}
				items := res.Items
				apiclient.EnrichThreadsWithSelf(items, me)
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
						WithNextSteps("bitbucket-cli pr threads " + refs[0])
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
			}

			return runBatchRead(s, refs, func(arg string) (any, error) {
				return fetchThreads(arg)
			})
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
// check + reviewer states + CI build statuses + caller review summary.
func newPRStatusCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "status <workspace>/<repo>/<id>...",
		Short: "Show merge readiness: mergeable, conflicts, reviewers, CI builds, my_review",
		Long: "Show merge readiness and incremental review summary for one or more PRs.\n" +
			"Pass several PR references or a single '-' to read newline-separated\n" +
			"references from stdin. With more than one reference, output is a\n" +
			"{items, has_more} aggregate with per-PR ok / result / error entries.",
		Example: "  bitbucket-cli pr status myws/myrepo/7\n" +
			"  bitbucket-cli pr inbox --fields ref | jq -r '.items[].ref' | bitbucket-cli pr status -",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			refs, err := collectBatchArgs(args, cmd.InOrStdin())
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			return runBatchRead(s, refs, func(arg string) (any, error) {
				ref, id, err := resolvePRRef(arg, apiclient.RepoRef{})
				if err != nil {
					return nil, err
				}
				return client.GetPRStatus(ctx, ref, id)
			})
		},
	}
}
