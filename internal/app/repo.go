package app

import (
	"fmt"

	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newRepoCmd builds the `repo` subtree.
func newRepoCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Browse and manage Bitbucket repositories",
	}
	cmd.AddCommand(newRepoListCmd(s), newRepoGetCmd(s), newRepoCloneURLCmd(s),
		newRepoCreateCmd(s), newRepoDeleteCmd(s))
	return cmd
}

func newRepoListCmd(s *appState) *cobra.Command {
	var (
		workspace, role, query, sort string
		limit                        int
		all                          bool
		cursor                       string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repositories in a workspace (Cloud) or project (Data Center)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws := defaultWorkspace(s, workspace)
			if ws == "" {
				return cerrors.New(cerrors.CategoryUsage, "REPO_NO_WORKSPACE",
					"a workspace is required").
					WithNextSteps(
						"bitbucket-cli workspace list   # discover available workspaces / projects",
						"Pass --workspace <slug>",
						"Set BITBUCKET_DEFAULT_WORKSPACE",
					)
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Repository], error) {
				return client.ListRepositories(ctx, apiclient.RepoListOpts{
					ListOpts:  apiclient.ListOpts{Limit: limit, Cursor: c},
					Workspace: ws, Role: role, Query: query, Sort: sort,
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
	f.StringVar(&workspace, "workspace", "", "workspace slug (Cloud) or project key (Data Center)")
	f.StringVar(&role, "role", "", "filter by role (Cloud only): owner|contributor|member")
	f.StringVar(&query, "query", "", "server-side filter expression (Cloud `q=`)")
	f.StringVar(&sort, "sort", "", "sort key (Cloud only)")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newRepoGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <workspace>/<repo> | <url>",
		Short: "Show a repository's details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := resolveRepoRef(args[0], defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			r, err := client.GetRepository(ctx, ref)
			if err != nil {
				return err
			}
			return s.emit(r)
		},
	}
	return cmd
}

func newRepoCloneURLCmd(s *appState) *cobra.Command {
	var protocol string
	cmd := &cobra.Command{
		Use:   "clone-url <workspace>/<repo> | <url>",
		Short: "Print the HTTPS or SSH clone URL of a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := resolveRepoRef(args[0], defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			r, err := client.GetRepository(ctx, ref)
			if err != nil {
				return err
			}
			out := r.CloneHTTPS
			if protocol == "ssh" {
				out = r.CloneSSH
			}
			if out == "" {
				return cerrors.Newf(cerrors.CategoryNotFound, "NO_CLONE_URL",
					"no %s clone URL on this repository", protocol)
			}
			fmt.Println(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&protocol, "protocol", "https", "https or ssh")
	return cmd
}

func newRepoCreateCmd(s *appState) *cobra.Command {
	var (
		workspace, name, description string
		private                      bool
		dryRun                       bool
	)
	cmd := &cobra.Command{
		Use:   "create <slug>",
		Short: "Create a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws := defaultWorkspace(s, workspace)
			if ws == "" {
				return cerrors.New(cerrors.CategoryUsage, "REPO_NO_WORKSPACE",
					"a workspace is required (--workspace)")
			}
			req := apiclient.CreateRepoReq{
				Workspace:   ws,
				Slug:        args[0],
				Name:        name,
				Description: description,
				Private:     private,
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			r, err := client.CreateRepository(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(r)
		},
	}
	f := cmd.Flags()
	f.StringVar(&workspace, "workspace", "", "workspace slug / project key")
	f.StringVar(&name, "name", "", "human-friendly name (defaults to slug)")
	f.StringVar(&description, "description", "", "repository description; literal \\n \\t \\r are decoded to real newlines/tabs")
	f.BoolVar(&private, "private", true, "make the repository private")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request instead of sending it")
	return cmd
}

func newRepoDeleteCmd(s *appState) *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <workspace>/<repo> | <url>",
		Short: "Delete a repository (irreversible)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := resolveRepoRef(args[0], defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			req := apiclient.DeleteRepoReq{Repo: ref}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			if !yes {
				return cerrors.New(cerrors.CategoryUsage, "NEEDS_YES",
					"repo delete is destructive; pass --yes to confirm (or --dry-run to preview)")
			}
			if err := client.DeleteRepository(ctx, req); err != nil {
				return err
			}
			return s.emit(map[string]any{"deleted": ref})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

// defaultWorkspace returns the workspace override, falling back to the
// config-level default (resolved from --workspace flag if added, env or YAML).
func defaultWorkspace(s *appState, override string) string {
	if override != "" {
		return override
	}
	return s.cfg().Defaults.Workspace
}
