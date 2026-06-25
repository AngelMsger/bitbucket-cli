package app

import (
	"os"
	"strconv"
	"strings"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// newCommentCmd builds the `comment` subtree (PR comments).
func newCommentCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Read and write pull-request comments",
	}
	cmd.AddCommand(newCommentListCmd(s), newCommentAddCmd(s),
		newCommentUpdateCmd(s), newCommentDeleteCmd(s), newCommentResolveCmd(s))
	return cmd
}

func newCommentListCmd(s *appState) *cobra.Command {
	var (
		prArg      string
		limit      int
		all        bool
		cursor     string
		unresolved bool
		tasks      bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List comments on a PR",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ref, id, err := resolvePRRef(prArg, apiclient.RepoRef{})
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Comment], error) {
				return client.ListPRComments(ctx, apiclient.ListPRCommentsOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Repo:     ref, PRID: id,
				})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			if unresolved || tasks {
				filtered := items[:0]
				for _, c := range items {
					if unresolved && c.Resolved {
						continue
					}
					if tasks && !c.Task {
						continue
					}
					filtered = append(filtered, c)
				}
				items = filtered
			}
			return s.emitList(items, info)
		},
	}
	cmd.Flags().StringVar(&prArg, "pr", "", "<workspace>/<repo>/<id> or PR URL")
	cmd.Flags().BoolVar(&unresolved, "unresolved", false, "only comments whose thread is not resolved")
	cmd.Flags().BoolVar(&tasks, "tasks", false, "only comments that are actionable tasks")
	addListFlags(cmd, &limit, &all, &cursor)
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}

func newCommentAddCmd(s *appState) *cobra.Command {
	var (
		prArg, content, contentFile, inline, side string
		replyTo                                   int
		dryRun                                    bool
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a comment on a PR (general or inline)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ref, id, err := resolvePRRef(prArg, apiclient.RepoRef{})
			if err != nil {
				return err
			}
			if contentFile != "" {
				b, err := os.ReadFile(contentFile)
				if err != nil {
					return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_FILE",
						"could not read --content-file")
				}
				content = string(b)
			}
			if strings.TrimSpace(content) == "" {
				return cerrors.New(cerrors.CategoryUsage, "COMMENT_NO_BODY",
					"comment content must not be empty")
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			var anchor *apiclient.InlineAnchor
			if inline != "" {
				if side != "" && side != apiclient.DiffSideNew && side != apiclient.DiffSideOld {
					return cerrors.New(cerrors.CategoryUsage, "BAD_SIDE",
						"--side must be new (post-change) or old (pre-change)")
				}
				parts := strings.SplitN(inline, ":", 2)
				if len(parts) != 2 {
					return cerrors.New(cerrors.CategoryUsage, "BAD_INLINE",
						"--inline must be <path>:<line>")
				}
				line, perr := strconv.Atoi(parts[1])
				if perr != nil {
					return cerrors.Wrap(perr, cerrors.CategoryUsage, "BAD_INLINE",
						"--inline line is not numeric")
				}
				// Resolve the anchor against the file's own structured diff so the
				// line lands on the requested side (new = post-change by default, old
				// = pre-change) with the right ADDED/REMOVED/CONTEXT classification —
				// rather than trusting a bare number that silently anchors to the
				// wrong side. GetPRFileDiffs understands both unified-diff text and
				// Data Center's JSON hunk model, so this works whichever the server
				// returns (a JSON body the CLI cannot read fails loudly as
				// DIFF_PARSE_FAILED instead of masquerading as a bad line number).
				files, derr := client.GetPRFileDiffs(ctx, ref, id, parts[0])
				if derr != nil {
					return derr
				}
				fd := apiclient.FindFileDiff(files, parts[0])
				if fd == nil {
					return cerrors.New(cerrors.CategoryUsage, "INLINE_FILE_NOT_IN_DIFF",
						"file "+parts[0]+" is not part of this PR's diff").
						WithHint("check the path against `pr files`, or drop --inline and reference path:line in a general comment")
				}
				anchor, err = apiclient.ResolveInlineAnchor(fd, line, side)
				if err != nil {
					return err
				}
			}
			req := apiclient.AddPRCommentReq{
				Repo: ref, PRID: id, Content: content, Inline: anchor, ReplyTo: replyTo,
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			cm, err := client.AddPRComment(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(cm)
		},
	}
	f := cmd.Flags()
	f.StringVar(&prArg, "pr", "", "<workspace>/<repo>/<id> or PR URL")
	f.StringVar(&content, "content", "", "comment body (Markdown); literal \\n \\t \\r are decoded to real newlines/tabs (use --content-file for exact bytes)")
	f.StringVar(&contentFile, "content-file", "", "read content from this file (sent as exact bytes, no escape decoding)")
	f.StringVar(&inline, "inline", "", "inline anchor as <path>:<line> (line is the new/post-change file line unless --side old)")
	f.StringVar(&side, "side", "new", "diff side the --inline line refers to: new (post-change) or old (pre-change/removed)")
	f.IntVar(&replyTo, "reply-to", 0, "reply to this comment ID")
	f.BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}

func newCommentUpdateCmd(s *appState) *cobra.Command {
	var prArg, content string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <comment-id>",
		Short: "Edit a comment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, prID, err := resolvePRRef(prArg, apiclient.RepoRef{})
			if err != nil {
				return err
			}
			id, perr := strconv.Atoi(args[0])
			if perr != nil {
				return cerrors.Wrap(perr, cerrors.CategoryUsage, "BAD_ID", "comment ID must be numeric")
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			req := apiclient.UpdatePRCommentReq{
				Repo: ref, PRID: prID, ID: id, Content: content,
			}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			cm, err := client.UpdatePRComment(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(cm)
		},
	}
	cmd.Flags().StringVar(&prArg, "pr", "", "<workspace>/<repo>/<id> or PR URL")
	cmd.Flags().StringVar(&content, "content", "", "new content (Markdown); literal \\n \\t \\r are decoded to real newlines/tabs")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("pr")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func newCommentResolveCmd(s *appState) *cobra.Command {
	var prArg string
	var unresolve, dryRun bool
	cmd := &cobra.Command{
		Use:   "resolve <comment-id>",
		Short: "Resolve (or reopen) a comment thread",
		Long: "Mark a PR comment thread resolved, or reopen it with --unresolve.\n" +
			"On Data Center this is also how a task (a BLOCKER-severity comment) is\n" +
			"completed or reopened. Bitbucket Cloud's separate task objects are not\n" +
			"covered — this resolves comment threads, the `resolved` field surfaced\n" +
			"by `comment list` and `pr threads`.",
		Example: "  bitbucket-cli comment resolve 42 --pr myws/myrepo/7\n" +
			"  bitbucket-cli comment resolve 42 --pr myws/myrepo/7 --unresolve",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, prID, err := resolvePRRef(prArg, apiclient.RepoRef{})
			if err != nil {
				return err
			}
			id, perr := strconv.Atoi(args[0])
			if perr != nil {
				return cerrors.Wrap(perr, cerrors.CategoryUsage, "BAD_ID", "comment ID must be numeric")
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			req := apiclient.ResolvePRCommentReq{Repo: ref, PRID: prID, ID: id, Resolve: !unresolve}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			cm, err := client.ResolvePRComment(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(cm)
		},
	}
	cmd.Flags().StringVar(&prArg, "pr", "", "<workspace>/<repo>/<id> or PR URL")
	cmd.Flags().BoolVar(&unresolve, "unresolve", false, "reopen the thread instead of resolving it")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}

func newCommentDeleteCmd(s *appState) *cobra.Command {
	var prArg string
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <comment-id>...",
		Short: "Delete one or more comments",
		Long: "Delete a PR comment. Pass several comment IDs to delete them in one run,\n" +
			"or a single '-' to read newline-separated IDs from stdin. --yes (or\n" +
			"--dry-run) is required and applies to the whole batch.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, prID, err := resolvePRRef(prArg, apiclient.RepoRef{})
			if err != nil {
				return err
			}
			items, err := collectBatchArgs(args, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if !dryRun && !yes {
				return cerrors.New(cerrors.CategoryUsage, "NEEDS_YES",
					"pass --yes to confirm deletion (or --dry-run to preview)")
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			return runBatch(s, items, func(arg string) (any, error) {
				id, perr := strconv.Atoi(arg)
				if perr != nil {
					return nil, cerrors.Wrap(perr, cerrors.CategoryUsage, "BAD_ID", "comment ID must be numeric")
				}
				req := apiclient.DeletePRCommentReq{Repo: ref, PRID: prID, ID: id}
				if dryRun {
					return client.DescribeWrite(ctx, req)
				}
				if derr := client.DeletePRComment(ctx, req); derr != nil {
					return nil, derr
				}
				return map[string]any{"deleted": id}, nil
			})
		},
	}
	cmd.Flags().StringVar(&prArg, "pr", "", "<workspace>/<repo>/<id> or PR URL")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}
