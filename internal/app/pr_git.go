package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/angelmsger/bitbucket-cli/internal/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
	"github.com/spf13/cobra"
)

// newPRFetchCmd prints (or runs) the `git fetch` invocation needed to bring a
// PR's source ref into the local repository. Default is print-only; --exec
// shells out to git inside the current working directory.
func newPRFetchCmd(s *appState) *cobra.Command {
	var (
		execIt bool
		remote string
	)
	cmd := &cobra.Command{
		Use:   "fetch <workspace>/<repo>/<id>",
		Short: "Print (or run, with --exec) git fetch for a PR's source ref",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			cmds := fetchCommands(remote, id)
			return runOrPrintGit(s, cmds, execIt)
		},
	}
	cmd.Flags().BoolVar(&execIt, "exec", false, "actually run `git fetch` in the current working directory")
	cmd.Flags().StringVar(&remote, "remote", "origin", "git remote name to fetch from")
	return cmd
}

// newPRCheckoutCmd prints (or runs) the git command sequence that brings the
// PR locally and switches to it (creating a `pr/<id>` branch).
func newPRCheckoutCmd(s *appState) *cobra.Command {
	var (
		execIt    bool
		remote    string
		branchFmt string
	)
	cmd := &cobra.Command{
		Use:   "checkout <workspace>/<repo>/<id>",
		Short: "Print (or run, with --exec) git fetch + checkout for a PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			localBranch := fmt.Sprintf(branchFmt, id)
			cmds := append(fetchCommands(remote, id), gitCmd{
				Args:  []string{"checkout", localBranch},
				Trace: fmt.Sprintf("git checkout %s", localBranch),
			})
			return runOrPrintGit(s, cmds, execIt)
		},
	}
	cmd.Flags().BoolVar(&execIt, "exec", false, "actually run the git commands in the current working directory")
	cmd.Flags().StringVar(&remote, "remote", "origin", "git remote name to fetch from")
	cmd.Flags().StringVar(&branchFmt, "branch", "pr/%d",
		"local branch name format; %d is replaced with the PR id")
	return cmd
}

type gitCmd struct {
	Args  []string
	Trace string
}

// fetchCommands returns the git commands that mirror Bitbucket's PR refspec
// (`refs/pull-requests/<id>/from`) into a local `pr/<id>` ref. Bitbucket Cloud
// and Data Center use the same refspec, so no flavor branching is needed.
func fetchCommands(remote string, id int) []gitCmd {
	refspec := fmt.Sprintf("refs/pull-requests/%d/from:refs/remotes/%s/pr/%d", id, remote, id)
	return []gitCmd{{
		Args:  []string{"fetch", remote, refspec},
		Trace: fmt.Sprintf("git fetch %s %s", remote, refspec),
	}}
}

// runOrPrintGit either emits a JSON envelope describing the commands (when
// execIt is false) or runs them in sequence, streaming stderr to the user.
func runOrPrintGit(s *appState, cmds []gitCmd, execIt bool) error {
	commands := make([]string, 0, len(cmds))
	for _, c := range cmds {
		commands = append(commands, c.Trace)
	}
	if !execIt {
		return s.emit(map[string]any{
			"commands": commands,
			"executed": false,
			"hint":     "re-run with --exec to actually run these (must be inside a git checkout).",
		})
	}
	if err := ensureGitWorktree(); err != nil {
		return err
	}
	var ran []string
	for _, c := range cmds {
		cmd := exec.Command("git", c.Args...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return cerrors.Wrap(err, cerrors.CategoryInternal, "GIT_EXEC",
				fmt.Sprintf("%s failed", c.Trace))
		}
		ran = append(ran, c.Trace)
	}
	return s.emit(map[string]any{
		"commands": ran,
		"executed": true,
	})
}

// ensureGitWorktree fails with a usage-class error when the current working
// directory is not inside a git checkout, so --exec users get a clean message
// instead of an opaque exec failure.
func ensureGitWorktree() error {
	out, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return cerrors.New(cerrors.CategoryUsage, "NOT_A_GIT_WORKTREE",
			"--exec must be run inside a git checkout of the repository").
			WithHint("cd into a local clone of the repo, then re-run with --exec.")
	}
	return nil
}
