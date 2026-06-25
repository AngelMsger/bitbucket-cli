package app

import (
	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	"github.com/spf13/cobra"
)

// newTagCmd builds the `tag` subtree — discovery for tag names referenced by
// `--ref` in file / commit commands. Read-only; tag creation is a git
// operation, not a typical CLI workflow.
func newTagCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tag",
		Short:   "List and inspect repository tags",
		Aliases: []string{"tags"},
	}
	cmd.AddCommand(newTagListCmd(s), newTagGetCmd(s))
	return cmd
}

func newTagListCmd(s *appState) *cobra.Command {
	var (
		repoArg, query, sort string
		limit                int
		all                  bool
		cursor               string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags in a repository",
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
			fetch := func(c string) (apiclient.ListResult[apiclient.Tag], error) {
				return client.ListTags(ctx, apiclient.TagListOpts{
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

func newTagGetCmd(s *appState) *cobra.Command {
	var repoArg string
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show a single tag (commit hash + date / message on Cloud)",
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
			t, err := client.GetTag(ctx, ref, args[0])
			if err != nil {
				return err
			}
			return s.emit(t)
		},
	}
	cmd.Flags().StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}
