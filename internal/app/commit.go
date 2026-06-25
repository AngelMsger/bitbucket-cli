package app

import (
	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	"github.com/spf13/cobra"
)

// newCommitCmd builds the `commit` subtree.
func newCommitCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Query commits in a repository",
	}
	cmd.AddCommand(newCommitGetCmd(s), newCommitListCmd(s), newCommitCompareCmd(s))
	return cmd
}

func newCommitGetCmd(s *appState) *cobra.Command {
	var repoArg string
	cmd := &cobra.Command{
		Use:   "get <hash>",
		Short: "Show a single commit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := resolveRepoRef(repoArg, defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			c, err := client.GetCommit(ctx, ref, args[0])
			if err != nil {
				return err
			}
			return s.emit(c)
		},
	}
	cmd.Flags().StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func newCommitListCmd(s *appState) *cobra.Command {
	var (
		repoArg, branch, path, since, until string
		limit                               int
		all                                 bool
		cursor                              string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List commits in a repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ref, err := resolveRepoRef(repoArg, defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Commit], error) {
				return client.ListCommits(ctx, apiclient.ListCommitsOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Repo:     ref, Branch: branch, Path: path, Since: since, Until: until,
				})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	f := cmd.Flags()
	f.StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	f.StringVar(&branch, "branch", "", "branch or ref to walk from")
	f.StringVar(&path, "path", "", "filter to commits touching this path")
	f.StringVar(&since, "since", "", "earliest commit date")
	f.StringVar(&until, "until", "", "latest commit date")
	addListFlags(cmd, &limit, &all, &cursor)
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func newCommitCompareCmd(s *appState) *cobra.Command {
	var repoArg, from, to string
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "List the commits between two refs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ref, err := resolveRepoRef(repoArg, defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			res, err := client.CompareCommits(ctx, apiclient.CompareCommitsReq{Repo: ref, From: from, To: to})
			if err != nil {
				return err
			}
			return s.emitList(res.Items, pageInfo{Next: res.Next, HasMore: res.Next != ""})
		},
	}
	f := cmd.Flags()
	f.StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	f.StringVar(&from, "from", "", "exclude commits reachable from this ref")
	f.StringVar(&to, "to", "", "include commits reachable from this ref")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
