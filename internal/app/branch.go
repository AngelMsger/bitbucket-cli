package app

import (
	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newBranchCmd builds the `branch` subtree.
func newBranchCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "List and manage repository branches",
	}
	cmd.AddCommand(newBranchListCmd(s), newBranchGetCmd(s), newBranchCreateCmd(s), newBranchDeleteCmd(s))
	return cmd
}

func newBranchListCmd(s *appState) *cobra.Command {
	var (
		repoArg, query, sort string
		limit                int
		all                  bool
		cursor               string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List branches in a repository",
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
			fetch := func(c string) (apiclient.ListResult[apiclient.Branch], error) {
				return client.ListBranches(ctx, apiclient.BranchListOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Repo:     ref, Query: query, Sort: sort,
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
	f.StringVar(&query, "query", "", "filter by name substring")
	f.StringVar(&sort, "sort", "", "sort key")
	addListFlags(cmd, &limit, &all, &cursor)
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func newBranchGetCmd(s *appState) *cobra.Command {
	var repoArg string
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show a single branch",
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
			b, err := client.GetBranch(ctx, ref, args[0])
			if err != nil {
				return err
			}
			return s.emit(b)
		},
	}
	cmd.Flags().StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func newBranchCreateCmd(s *appState) *cobra.Command {
	var repoArg, fromRef string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a branch from a starting ref",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := resolveRepoRef(repoArg, defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			req := apiclient.CreateBranchReq{Repo: ref, Name: args[0], FromRef: fromRef}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			b, err := client.CreateBranch(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(b)
		},
	}
	cmd.Flags().StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	cmd.Flags().StringVar(&fromRef, "from-ref", "", "starting ref (branch name or commit hash)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without sending")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("from-ref")
	return cmd
}

func newBranchDeleteCmd(s *appState) *cobra.Command {
	var repoArg string
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a branch",
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
			req := apiclient.DeleteBranchReq{Repo: ref, Name: args[0]}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			if !yes {
				return cerrors.New(cerrors.CategoryUsage, "NEEDS_YES",
					"pass --yes to confirm deletion (or --dry-run to preview)")
			}
			if err := client.DeleteBranch(ctx, req); err != nil {
				return err
			}
			return s.emit(map[string]any{"deleted": args[0]})
		},
	}
	cmd.Flags().StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}
