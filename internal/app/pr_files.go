package app

import (
	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
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
	return &cobra.Command{
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
			return s.emitList(res.Items, pageInfo{})
		},
	}
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
