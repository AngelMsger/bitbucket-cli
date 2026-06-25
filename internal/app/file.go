package app

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// newFileCmd builds the `file` subtree: browse and read source at any ref.
func newFileCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Browse and read repository source files at any ref",
	}
	cmd.AddCommand(newFileListCmd(s), newFileGetCmd(s), newFileTreeCmd(s))
	return cmd
}

func newFileListCmd(s *appState) *cobra.Command {
	var (
		ref, path string
		limit     int
		all       bool
		cursor    string
	)
	cmd := &cobra.Command{
		Use:   "list <workspace>/<repo>",
		Short: "List entries under a repository path at a ref",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := resolveRepoRef(args[0], defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.FileEntry], error) {
				return client.ListFiles(ctx, apiclient.FileListOpts{
					ListOpts: apiclient.ListOpts{Limit: limit, Cursor: c},
					Repo:     repo, Ref: ref, Path: path,
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
	f.StringVar(&ref, "ref", "", "branch / tag / commit (default: repository default branch)")
	f.StringVar(&path, "path", "", "path within the repo to list (empty = root)")
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newFileGetCmd(s *appState) *cobra.Command {
	var (
		ref, path, rangeFlag, output string
	)
	cmd := &cobra.Command{
		Use:   "get <workspace>/<repo>",
		Short: "Read a file's raw contents at a ref",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := resolveRepoRef(args[0], defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			if strings.TrimSpace(path) == "" {
				return cerrors.New(cerrors.CategoryUsage, "FILE_NO_PATH",
					"--path is required").
					WithHint("Pass --path to identify the file to read.")
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fc, err := client.GetFile(ctx, apiclient.FileGetOpts{Repo: repo, Ref: ref, Path: path})
			if err != nil {
				return err
			}
			body := fc.Bytes
			if rangeFlag != "" {
				body, err = sliceByRange(body, rangeFlag)
				if err != nil {
					return err
				}
			}
			// --output writes raw bytes; absent it the CLI emits a JSON envelope
			// so structured consumers still see metadata.
			if output != "" {
				return writeRawOutput(output, body)
			}
			envelope := map[string]any{
				"path":      fc.Path,
				"ref":       fc.Ref,
				"size":      fc.Size,
				"encoding":  fc.Encoding,
				"truncated": fc.Truncated,
				"content":   string(body),
			}
			return s.emit(envelope)
		},
	}
	f := cmd.Flags()
	f.StringVar(&ref, "ref", "", "branch / tag / commit (default: repository default branch)")
	f.StringVar(&path, "path", "", "file path within the repo (required)")
	f.StringVar(&rangeFlag, "range", "", "1-based inclusive line range, e.g. 10:40 (client-side slicing)")
	f.StringVar(&output, "output", "", "write raw bytes to this file; `-` for stdout")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newFileTreeCmd(s *appState) *cobra.Command {
	var (
		ref, path string
		depth     int
	)
	cmd := &cobra.Command{
		Use:   "tree <workspace>/<repo>",
		Short: "Recursively list files under a path at a ref",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := resolveRepoRef(args[0], defaultWorkspace(s, ""))
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			res, err := client.Tree(ctx, apiclient.TreeOpts{Repo: repo, Ref: ref, Path: path, Depth: depth})
			if err != nil {
				return err
			}
			return s.emitList(res.Items, pageInfo{Next: res.Next, HasMore: res.Next != ""})
		},
	}
	f := cmd.Flags()
	f.StringVar(&ref, "ref", "", "branch / tag / commit (default: repository default branch)")
	f.StringVar(&path, "path", "", "subtree root (empty = repo root)")
	f.IntVar(&depth, "depth", 0, "maximum depth from --path (0 = unlimited)")
	return cmd
}

// sliceByRange returns the L1-th through L2-th lines (1-based inclusive). Out-
// of-range bounds clamp to the file's extent rather than erroring, but L1>L2
// and unparsable forms are rejected.
func sliceByRange(body []byte, spec string) ([]byte, error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 {
		return nil, cerrors.New(cerrors.CategoryUsage, "BAD_RANGE",
			"--range must be <L1>:<L2>, e.g. 10:40")
	}
	l1, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	l2, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || l1 < 1 || l2 < 1 || l1 > l2 {
		return nil, cerrors.Newf(cerrors.CategoryUsage, "BAD_RANGE",
			"invalid --range %q: must be two positive integers L1 <= L2", spec)
	}
	lines := bytes.Split(body, []byte{'\n'})
	if l1 > len(lines) {
		return nil, nil
	}
	if l2 > len(lines) {
		l2 = len(lines)
	}
	return bytes.Join(lines[l1-1:l2], []byte{'\n'}), nil
}

// writeRawOutput writes body to `-` (stdout) or a file path. Directories that
// do not exist are not created (callers should pass a leaf path).
func writeRawOutput(target string, body []byte) error {
	if target == "-" {
		if _, err := os.Stdout.Write(body); err != nil {
			return cerrors.Wrap(err, cerrors.CategoryInternal, "OUTPUT", "failed to write stdout")
		}
		return nil
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return cerrors.Wrap(err, cerrors.CategoryInternal, "OUTPUT",
			fmt.Sprintf("failed to write %s", target))
	}
	return nil
}
