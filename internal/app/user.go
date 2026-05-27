package app

import (
	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	"github.com/spf13/cobra"
)

// newWhoamiCmd remains a top-level convenience alias for `user me`. The
// stand-alone `whoami` is the universal Unix idiom and predates the `user`
// subtree, so the CLI keeps it.
func newWhoamiCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Short:   "Print the user the configured credentials authenticate as",
		Example: "  bitbucket-cli whoami",
		Args:    cobra.NoArgs,
		RunE:    runWhoami(s),
	}
}

func runWhoami(s *appState) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := cmdContext(s)
		defer cancel()
		client, err := s.newClient(ctx)
		if err != nil {
			return err
		}
		user, err := client.CurrentUser(ctx)
		if err != nil {
			return err
		}
		return s.emit(user)
	}
}

// newUserCmd is the discovery entry point for `--reviewer` / `--author`
// identifiers. On Bitbucket Cloud the API only exposes users via workspace
// membership, so `user list --workspace <ws>` is the canonical path; Data
// Center has a global `/users` endpoint and `--workspace` is ignored.
func newUserCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "user",
		Short:   "Discover Bitbucket users (workspace members on Cloud / global users on DC)",
		Aliases: []string{"users"},
	}
	cmd.AddCommand(newUserListCmd(s), newUserGetCmd(s), newUserMeCmd(s))
	return cmd
}

func newUserListCmd(s *appState) *cobra.Command {
	var (
		workspace, query string
		limit            int
		all              bool
		cursor           string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users (workspace members on Cloud / global users on DC)",
		Long: "List Bitbucket users.\n\n" +
			"Cloud: --workspace is required (Cloud has no global user list; the API exposes\n" +
			"  members of a specific workspace).\n" +
			"DC:    --workspace is ignored; the global /users endpoint is used.\n\n" +
			"Use --query to filter by display-name substring.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws := defaultWorkspace(s, workspace)
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.User], error) {
				return client.ListUsers(ctx, apiclient.UserListOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Workspace: ws, Query: query,
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
	f.StringVar(&workspace, "workspace", "", "workspace slug (Cloud only; ignored on DC)")
	f.StringVar(&query, "query", "", "filter by display-name substring")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newUserGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <selector>",
		Short: "Show details of a single user (UUID/account_id on Cloud; slug/username on DC)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			u, err := client.GetUser(ctx, args[0])
			if err != nil {
				return err
			}
			return s.emit(u)
		},
	}
	return cmd
}

func newUserMeCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:     "me",
		Short:   "Print the user the configured credentials authenticate as (alias for whoami)",
		Aliases: []string{"current"},
		Args:    cobra.NoArgs,
		RunE:    runWhoami(s),
	}
}
