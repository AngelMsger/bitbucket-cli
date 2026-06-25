package app

import (
	"context"

	"github.com/angelmsger/bitbucket-cli/internal/config"
	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	"github.com/spf13/cobra"
)

// enumComplete registers a fixed set of completion values for a flag. Errors
// from registration are ignored: completion is a convenience, never required.
func enumComplete(cmd *cobra.Command, flag string, values ...string) {
	_ = cmd.RegisterFlagCompletionFunc(flag,
		cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp))
}

// completeContextNames suggests context names from the config file. It is
// best-effort: any failure yields no suggestions.
func completeContextNames(s *appState) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if s.resolved == nil {
			if err := s.load(); err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}
		file, _, err := config.ReadFile(s.cfgDir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return file.ContextNames(), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeRepoSlugs returns a completion function that suggests live
// repository slugs under the default workspace. Best-effort.
func completeRepoSlugs(s *appState) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if s.resolved == nil {
			if err := s.load(); err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}
		ws := s.cfg().Defaults.Workspace
		if ws == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
		defer cancel()
		client, err := s.newClient(ctx)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		res, err := client.ListRepositories(ctx, apiclient.RepoListOpts{
			ListOpts:  apiclient.ListOpts{Limit: 50},
			Workspace: ws,
		})
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		out := make([]string, 0, len(res.Items))
		for _, r := range res.Items {
			out = append(out, r.Slug+"\t"+r.Name)
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}
