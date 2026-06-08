package app

import (
	"fmt"
	"os"
	"strconv"

	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newPRCmd builds the `pr` subtree.
func newPRCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Drive Bitbucket pull requests (list, review, merge)",
	}
	cmd.AddCommand(
		newPRListCmd(s), newPRInboxCmd(s), newPRGetCmd(s), newPRCreateCmd(s), newPRUpdateCmd(s),
		newPRDiffCmd(s), newPRCommitsCmd(s), newPRActivityCmd(s),
		newPRFilesCmd(s), newPRThreadsCmd(s), newPRStatusCmd(s),
		newPRFetchCmd(s), newPRCheckoutCmd(s),
		newPRApproveCmd(s), newPRUnapproveCmd(s), newPRRequestChangesCmd(s),
		newPRDeclineCmd(s), newPRMergeCmd(s),
	)
	return cmd
}

func newPRListCmd(s *appState) *cobra.Command {
	var (
		repoArg, state, author, reviewer, source, target, query string
		limit                                                   int
		all                                                     bool
		cursor                                                  string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests in a repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repoArg == "" {
				return cerrors.New(cerrors.CategoryUsage, "PR_NO_REPO",
					"a repository is required").
					WithNextSteps("Pass --repo <workspace>/<repo>")
			}
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
			fetch := func(c string) (apiclient.ListResult[apiclient.PullRequest], error) {
				return client.ListPRs(ctx, apiclient.PRListOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Repo:     ref,
					State:    state, Author: author, Reviewer: reviewer,
					Source: source, Target: target, Query: query,
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
	f.StringVar(&repoArg, "repo", "", "<workspace>/<repo> or Bitbucket repo URL")
	f.StringVar(&state, "state", "OPEN", "OPEN | MERGED | DECLINED | ALL")
	f.StringVar(&author, "author", "", "filter by author username")
	f.StringVar(&reviewer, "reviewer", "", "filter by reviewer username")
	f.StringVar(&source, "source", "", "filter by source branch")
	f.StringVar(&target, "target", "", "filter by destination branch")
	f.StringVar(&query, "query", "", "server-side filter (Cloud `q=`)")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newPRInboxCmd(s *appState) *cobra.Command {
	var (
		role, state, workspace string
		limit                  int
		all                    bool
		cursor                 string
	)
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "List PRs involving me across repositories (--role reviewer by default)",
		Long: "List pull requests involving the authenticated user across every accessible\n" +
			"repository.\n\n" +
			"Data Center uses the dashboard endpoint — a single call covers every project.\n" +
			"Bitbucket Cloud has no global reviewer index, so --role reviewer (and --role\n" +
			"participant) require --workspace; --role author works globally via the user's\n" +
			"`/pullrequests/<uuid>` endpoint.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws := defaultWorkspace(s, workspace)
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.PullRequest], error) {
				return client.ListMyPRs(ctx, apiclient.MyPRListOpts{
					ListOpts:  apiclient.ListOpts{Limit: limit, Cursor: c},
					Role:      role,
					State:     state,
					Workspace: ws,
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
	f.StringVar(&role, "role", "reviewer", "reviewer | author | participant")
	f.StringVar(&state, "state", "OPEN", "OPEN | MERGED | DECLINED | ALL")
	f.StringVar(&workspace, "workspace", "",
		"Cloud workspace to scope reviewer / participant queries to (ignored on Data Center)")
	addListFlags(cmd, &limit, &all, &cursor)
	enumComplete(cmd, "role", "reviewer", "author", "participant")
	enumComplete(cmd, "state", "OPEN", "MERGED", "DECLINED", "ALL")
	return cmd
}

func newPRGetCmd(s *appState) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "get <workspace>/<repo>/<id> | <url>",
		Short: "Show a pull request",
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
			switch apiclient.PRScope(scope) {
			case apiclient.PRScopeDiff:
				diff, err := client.GetPRDiff(ctx, ref, id)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprint(os.Stdout, diff)
				return nil
			case apiclient.PRScopeCommits:
				items, info, err := collectPage(func(c string) (apiclient.ListResult[apiclient.Commit], error) {
					return client.ListPRCommits(ctx, apiclient.PRListOpts{
						Repo: ref, ListOpts: apiclient.ListOpts{Cursor: c}, Query: strconv.Itoa(id),
					})
				}, "", false)
				if err != nil {
					return err
				}
				return s.emitList(items, info)
			case apiclient.PRScopeActivity:
				items, info, err := collectPage(func(c string) (apiclient.ListResult[apiclient.Activity], error) {
					return client.ListPRActivity(ctx, apiclient.PRListOpts{
						Repo: ref, ListOpts: apiclient.ListOpts{Cursor: c}, Query: strconv.Itoa(id),
					})
				}, "", false)
				if err != nil {
					return err
				}
				return s.emitList(items, info)
			}
			pr, err := client.GetPR(ctx, apiclient.GetPROpts{Repo: ref, ID: id, Scope: apiclient.PRScope(scope)})
			if err != nil {
				return err
			}
			return s.emit(pr)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "summary",
		"summary | full | diff | commits | activity")
	return cmd
}

func newPRCreateCmd(s *appState) *cobra.Command {
	var (
		repoArg, title, description, descriptionFile string
		source, sourceRepo, destination              string
		reviewers                                    []string
		closeSourceBranch                            bool
		dryRun                                       bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Open a new pull request",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ref, err := resolveRepoRef(repoArg, defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			if descriptionFile != "" {
				b, err := os.ReadFile(descriptionFile)
				if err != nil {
					return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_FILE",
						"could not read --description-file")
				}
				description = string(b)
			}
			req := apiclient.CreatePRReq{
				Repo: ref, Title: title, Description: description,
				Source: source, SourceRepo: sourceRepo, Destination: destination,
				Reviewers: reviewers, CloseSourceBranch: closeSourceBranch,
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
			pr, err := client.CreatePR(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(pr)
		},
	}
	f := cmd.Flags()
	f.StringVar(&repoArg, "repo", "", "<workspace>/<repo>")
	f.StringVar(&title, "title", "", "PR title")
	f.StringVar(&description, "description", "", "PR description (Markdown)")
	f.StringVar(&descriptionFile, "description-file", "", "read description from this file")
	f.StringVar(&source, "source", "", "source branch")
	f.StringVar(&sourceRepo, "source-repo", "", "cross-repo source (Cloud forks): <ws>/<repo>")
	f.StringVar(&destination, "target", "", "destination branch (default: repo default)")
	f.StringSliceVar(&reviewers, "reviewer", nil, "reviewer UUID (Cloud) or username (DC); repeatable")
	f.BoolVar(&closeSourceBranch, "close-source-branch", false, "close the source branch on merge")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newPRUpdateCmd(s *appState) *cobra.Command {
	var (
		title, description string
		reviewers          []string
		dryRun             bool
	)
	cmd := &cobra.Command{
		Use:   "update <workspace>/<repo>/<id>",
		Short: "Edit a PR's title, description, or reviewers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			req := apiclient.UpdatePRReq{Repo: ref, ID: id, Title: title, Description: description}
			if cmd.Flags().Changed("reviewer") {
				req.Reviewers = reviewers
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
			pr, err := client.UpdatePR(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(pr)
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "new title")
	f.StringVar(&description, "description", "", "new description")
	f.StringSliceVar(&reviewers, "reviewer", nil, "replace reviewer list (repeatable)")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newPRDiffCmd(s *appState) *cobra.Command {
	var path string
	var lineNumbers bool
	var commentable bool
	cmd := &cobra.Command{
		Use:   "diff <workspace>/<repo>/<id>",
		Short: "Print the unified diff of a PR (use --path to scope to one file)",
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
			if commentable {
				// List the line numbers each file can carry an inline comment on, so
				// an agent picks valid `--inline <path>:<line>` anchors up front
				// instead of probing them one at a time.
				files, ferr := client.GetPRFileDiffs(ctx, ref, id, path)
				if ferr != nil {
					return ferr
				}
				return s.emit(commentableRanges(files))
			}
			var diff string
			if path != "" {
				diff, err = client.GetPRDiffByPath(ctx, ref, id, path)
			} else {
				diff, err = client.GetPRDiff(ctx, ref, id)
			}
			if err != nil {
				return err
			}
			if lineNumbers {
				// Annotate with an "old new" gutter so the agent can read the exact
				// new-file line number to pass to `comment add --inline <path>:<line>`.
				diff = apiclient.AnnotateDiffWithLineNumbers(diff)
			}
			_, _ = fmt.Fprint(os.Stdout, diff)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "restrict the diff to a single file path")
	cmd.Flags().BoolVar(&lineNumbers, "line-numbers", false, "prefix each line with its old/new file line numbers (for picking --inline lines)")
	cmd.Flags().BoolVar(&commentable, "commentable", false, "list the new/old-side line numbers each file can carry an inline comment on")
	return cmd
}

// fileCommentable reports the inline-commentable line ranges for one file.
type fileCommentable struct {
	Path string `json:"path"`
	New  string `json:"new_side"` // commentable new/post-change line ranges
	Old  string `json:"old_side"` // commentable old/pre-change line ranges
}

func commentableRanges(files []apiclient.FileDiff) []fileCommentable {
	out := make([]fileCommentable, 0, len(files))
	for i := range files {
		f := &files[i]
		p := f.NewPath
		if p == "" {
			p = f.OldPath
		}
		out = append(out, fileCommentable{
			Path: p,
			New:  apiclient.FormatLineRanges(apiclient.CommentableLines(f, apiclient.DiffSideNew)),
			Old:  apiclient.FormatLineRanges(apiclient.CommentableLines(f, apiclient.DiffSideOld)),
		})
	}
	return out
}

func newPRCommitsCmd(s *appState) *cobra.Command {
	var limit int
	var all bool
	var cursor string
	cmd := &cobra.Command{
		Use:   "commits <workspace>/<repo>/<id>",
		Short: "List commits included in a PR",
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
			fetch := func(c string) (apiclient.ListResult[apiclient.Commit], error) {
				return client.ListPRCommits(ctx, apiclient.PRListOpts{
					Repo: ref, ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Query: strconv.Itoa(id),
				})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newPRActivityCmd(s *appState) *cobra.Command {
	var limit int
	var all bool
	var cursor string
	cmd := &cobra.Command{
		Use:   "activity <workspace>/<repo>/<id>",
		Short: "List the activity timeline of a PR",
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
			fetch := func(c string) (apiclient.ListResult[apiclient.Activity], error) {
				return client.ListPRActivity(ctx, apiclient.PRListOpts{
					Repo: ref, ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Query: strconv.Itoa(id),
				})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newPRApproveCmd(s *appState) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "approve <workspace>/<repo>/<id>",
		Short: "Approve a PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return togglePRApproval(s, args[0], true, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newPRUnapproveCmd(s *appState) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "unapprove <workspace>/<repo>/<id>",
		Short: "Withdraw an approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return togglePRApproval(s, args[0], false, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func togglePRApproval(s *appState, arg string, approve, dryRun bool) error {
	ref, id, err := resolvePRRef(arg, apiclient.RepoRef{})
	if err != nil {
		return err
	}
	ctx, cancel := cmdContext(s)
	defer cancel()
	client, err := s.newClient(ctx)
	if err != nil {
		return err
	}
	req := apiclient.ApprovePRReq{Repo: ref, ID: id, Approve: approve}
	if dryRun {
		return emitDryRun(s, client, ctx, req)
	}
	if err := client.ApprovePR(ctx, req); err != nil {
		return err
	}
	return s.emit(map[string]any{"approved": approve, "pr": map[string]any{"repo": ref, "id": id}})
}

func newPRRequestChangesCmd(s *appState) *cobra.Command {
	var withdraw, dryRun bool
	cmd := &cobra.Command{
		Use:   "request-changes <workspace>/<repo>/<id>",
		Short: "Cast (or withdraw) a request-changes vote (Cloud only)",
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
			req := apiclient.RequestChangesReq{Repo: ref, ID: id, Request: !withdraw}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			if err := client.RequestPRChanges(ctx, req); err != nil {
				return err
			}
			return s.emit(map[string]any{"requested_changes": !withdraw, "pr": map[string]any{"repo": ref, "id": id}})
		},
	}
	cmd.Flags().BoolVar(&withdraw, "withdraw", false, "withdraw a previous request-changes vote")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newPRDeclineCmd(s *appState) *cobra.Command {
	var message string
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "decline <workspace>/<repo>/<id>",
		Short: "Decline (close without merging) a PR",
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
			req := apiclient.DeclinePRReq{Repo: ref, ID: id, Message: message}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			if !yes {
				return cerrors.New(cerrors.CategoryUsage, "NEEDS_YES",
					"pass --yes to confirm declining the PR (or --dry-run to preview)")
			}
			pr, err := client.DeclinePR(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(pr)
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "decline message")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm declining")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	return cmd
}

func newPRMergeCmd(s *appState) *cobra.Command {
	var (
		strategy, message string
		closeSourceBranch bool
		dryRun, yes       bool
	)
	cmd := &cobra.Command{
		Use:   "merge <workspace>/<repo>/<id>",
		Short: "Merge a PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			req := apiclient.MergePRReq{
				Repo: ref, ID: id, Strategy: strategy, Message: message,
				CloseSourceBranch: closeSourceBranch,
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
			if !yes {
				return cerrors.New(cerrors.CategoryUsage, "NEEDS_YES",
					"pass --yes to confirm the merge (or --dry-run to preview)")
			}
			pr, err := client.MergePR(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(pr)
		},
	}
	f := cmd.Flags()
	f.StringVar(&strategy, "strategy", "merge_commit", "merge_commit | squash | fast_forward")
	f.StringVar(&message, "message", "", "merge commit message")
	f.BoolVar(&closeSourceBranch, "close-source-branch", false, "close the source branch after merging")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	f.BoolVar(&yes, "yes", false, "confirm the merge")
	return cmd
}
