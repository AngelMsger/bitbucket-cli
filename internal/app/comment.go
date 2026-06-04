package app

import (
	"os"
	"strconv"
	"strings"

	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newCommentCmd builds the `comment` subtree (PR comments).
func newCommentCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Read and write pull-request comments",
	}
	cmd.AddCommand(newCommentListCmd(s), newCommentAddCmd(s),
		newCommentUpdateCmd(s), newCommentDeleteCmd(s))
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
		prArg, content, contentFile, inline string
		replyTo                             int
		dryRun                              bool
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
			var anchor *apiclient.InlineAnchor
			if inline != "" {
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
				anchor = &apiclient.InlineAnchor{Path: parts[0], Line: line, To: line}
			}
			req := apiclient.AddPRCommentReq{
				Repo: ref, PRID: id, Content: content, Inline: anchor, ReplyTo: replyTo,
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
			cm, err := client.AddPRComment(ctx, req)
			if err != nil {
				return err
			}
			return s.emit(cm)
		},
	}
	f := cmd.Flags()
	f.StringVar(&prArg, "pr", "", "<workspace>/<repo>/<id> or PR URL")
	f.StringVar(&content, "content", "", "comment body (Markdown)")
	f.StringVar(&contentFile, "content-file", "", "read content from this file")
	f.StringVar(&inline, "inline", "", "inline anchor as <path>:<line>")
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
	cmd.Flags().StringVar(&content, "content", "", "new content (Markdown)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("pr")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func newCommentDeleteCmd(s *appState) *cobra.Command {
	var prArg string
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <comment-id>",
		Short: "Delete a comment",
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
			req := apiclient.DeletePRCommentReq{Repo: ref, PRID: prID, ID: id}
			if dryRun {
				return emitDryRun(s, client, ctx, req)
			}
			if !yes {
				return cerrors.New(cerrors.CategoryUsage, "NEEDS_YES",
					"pass --yes to confirm deletion (or --dry-run to preview)")
			}
			if err := client.DeletePRComment(ctx, req); err != nil {
				return err
			}
			return s.emit(map[string]any{"deleted": id})
		},
	}
	cmd.Flags().StringVar(&prArg, "pr", "", "<workspace>/<repo>/<id> or PR URL")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the HTTP request without sending it")
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}
