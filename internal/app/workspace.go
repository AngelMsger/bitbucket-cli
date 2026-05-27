package app

import (
	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	"github.com/spf13/cobra"
)

// newWorkspaceCmd builds the `workspace` subtree — the discovery entry point
// for "what value should I pass to --workspace anywhere else in the CLI?".
//
// Cloud calls these workspaces; Data Center calls them projects. The CLI uses
// "workspace" as the universal term and accepts either's slug / key.
func newWorkspaceCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Short:   "List and inspect Bitbucket workspaces (Cloud) / projects (DC)",
		Aliases: []string{"workspaces", "project", "projects"},
	}
	cmd.AddCommand(newWorkspaceListCmd(s), newWorkspaceGetCmd(s))
	return cmd
}

func newWorkspaceListCmd(s *appState) *cobra.Command {
	var (
		role, query string
		limit       int
		all         bool
		cursor      string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every workspace / project the current credentials can see",
		Long: "List every Bitbucket workspace (Cloud) or project (Data Center) the\n" +
			"authenticated user can see. The `slug` field of each entry is what every\n" +
			"other command's --workspace flag expects.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Workspace], error) {
				return client.ListWorkspaces(ctx, apiclient.WorkspaceListOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Role:     role, Query: query,
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
	f.StringVar(&role, "role", "", "filter by role (Cloud only): owner | collaborator | member")
	f.StringVar(&query, "query", "", "filter by name substring")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newWorkspaceGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <slug>",
		Short: "Show details of a single workspace / project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			ws, err := client.GetWorkspace(ctx, args[0])
			if err != nil {
				return err
			}
			return s.emit(ws)
		},
	}
	return cmd
}
