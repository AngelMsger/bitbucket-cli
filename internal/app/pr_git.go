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

// newPRFetchCmd prints (or runs) the `git fetch` invocations needed to bring a
// PR's source ref AND its base branch into the local repository. Default is
// print-only; --exec shells out to git inside the current working directory.
func newPRFetchCmd(s *appState) *cobra.Command {
	var (
		execIt bool
		remote string
		base   string
	)
	cmd := &cobra.Command{
		Use:   "fetch <workspace>/<repo>/<id>",
		Short: "Print (or run, with --exec) git fetch for a PR's source ref and base branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			baseBranch, baseCommit, err := resolvePRBase(s, ref, id, base)
			if err != nil {
				return err
			}
			chosen := pickRemote(remote, listGitRemotes())
			cmds := prFetchCommands(chosen, id, baseBranch)
			return runOrPrintGit(s, cmds, execIt, reviewExtras(chosen, id, baseBranch, baseCommit))
		},
	}
	cmd.Flags().BoolVar(&execIt, "exec", false, "actually run `git fetch` in the current working directory")
	cmd.Flags().StringVar(&remote, "remote", "", "git remote to fetch from (default: upstream if present, else origin)")
	cmd.Flags().StringVar(&base, "base", "", "base branch to fetch (default: the PR's destination branch, looked up via the API)")
	return cmd
}

// newPRCheckoutCmd prints (or runs) the git command sequence that brings the
// PR and its base locally and switches to the PR (creating a `pr/<id>` branch).
func newPRCheckoutCmd(s *appState) *cobra.Command {
	var (
		execIt    bool
		remote    string
		base      string
		branchFmt string
	)
	cmd := &cobra.Command{
		Use:   "checkout <workspace>/<repo>/<id>",
		Short: "Print (or run, with --exec) git fetch (source + base) + checkout for a PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			baseBranch, baseCommit, err := resolvePRBase(s, ref, id, base)
			if err != nil {
				return err
			}
			chosen := pickRemote(remote, listGitRemotes())
			localBranch := fmt.Sprintf(branchFmt, id)
			cmds := append(prFetchCommands(chosen, id, baseBranch), gitCmd{
				Args:  []string{"checkout", localBranch},
				Trace: fmt.Sprintf("git checkout %s", localBranch),
			})
			return runOrPrintGit(s, cmds, execIt, reviewExtras(chosen, id, baseBranch, baseCommit))
		},
	}
	cmd.Flags().BoolVar(&execIt, "exec", false, "actually run the git commands in the current working directory")
	cmd.Flags().StringVar(&remote, "remote", "", "git remote to fetch from (default: upstream if present, else origin)")
	cmd.Flags().StringVar(&base, "base", "", "base branch to fetch (default: the PR's destination branch, looked up via the API)")
	cmd.Flags().StringVar(&branchFmt, "branch", "pr/%d",
		"local branch name format; %d is replaced with the PR id")
	return cmd
}

type gitCmd struct {
	Args  []string
	Trace string
}

// resolvePRBase determines the PR's base (destination) branch and tip commit.
// An explicit --base skips the API call (keeping the command usable offline);
// otherwise it looks the PR up and reads its destination branch.
func resolvePRBase(s *appState, ref apiclient.RepoRef, id int, override string) (branch, commit string, err error) {
	if override != "" {
		return override, "", nil
	}
	ctx, cancel := cmdContext(s)
	defer cancel()
	client, err := s.newClient(ctx)
	if err != nil {
		return "", "", err
	}
	pr, err := client.GetPR(ctx, apiclient.GetPROpts{Repo: ref, ID: id, Scope: apiclient.PRScopeSummary})
	if err != nil {
		return "", "", err
	}
	return pr.Destination.Branch, pr.Destination.Commit, nil
}

// pickRemote chooses the git remote to fetch a PR and its base from. An explicit
// non-empty choice (other than "auto") wins. Otherwise prefer "upstream" — in a
// fork workflow that is the canonical repository the PR was opened against, and
// it carries the authoritative base branch and the `refs/pull-requests/*` refs,
// so it is usually more accurate than a personal "origin" fork. Falls back to
// "origin", then the first available remote, then "origin".
func pickRemote(explicit string, remotes []string) string {
	if explicit != "" && explicit != "auto" {
		return explicit
	}
	has := func(name string) bool {
		for _, r := range remotes {
			if r == name {
				return true
			}
		}
		return false
	}
	switch {
	case has("upstream"):
		return "upstream"
	case has("origin"):
		return "origin"
	case len(remotes) > 0:
		return remotes[0]
	default:
		return "origin"
	}
}

// listGitRemotes returns the configured git remotes, or nil when the current
// directory is not a git checkout (best-effort; callers fall back to a default).
func listGitRemotes() []string {
	out, err := exec.Command("git", "remote").Output()
	if err != nil {
		return nil
	}
	var remotes []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			remotes = append(remotes, line)
		}
	}
	return remotes
}

// prFetchCommands returns the git commands that mirror Bitbucket's PR source
// refspec (`refs/pull-requests/<id>/from`) into a local `pr/<id>` ref and bring
// the base branch up to date so a local diff has the correct base. Bitbucket
// Cloud and Data Center use the same refspec, so no flavor branching is needed.
func prFetchCommands(remote string, id int, baseBranch string) []gitCmd {
	refspec := fmt.Sprintf("refs/pull-requests/%d/from:refs/remotes/%s/pr/%d", id, remote, id)
	cmds := []gitCmd{{
		Args:  []string{"fetch", remote, refspec},
		Trace: fmt.Sprintf("git fetch %s %s", remote, refspec),
	}}
	if baseBranch != "" {
		// Refresh the base branch so the local merge-base reflects what the PR is
		// actually diffed against, rather than a stale checkout.
		cmds = append(cmds, gitCmd{
			Args:  []string{"fetch", remote, baseBranch},
			Trace: fmt.Sprintf("git fetch %s %s", remote, baseBranch),
		})
	}
	return cmds
}

// reviewDiffCommand is the merge-base diff that shows exactly the PR's changes.
// The triple-dot form diffs against the merge-base of the base branch and the
// PR head, so it stays correct even as the base branch advances.
func reviewDiffCommand(remote string, id int, baseBranch string) string {
	if baseBranch == "" {
		return ""
	}
	return fmt.Sprintf("git diff %s/%s...%s/pr/%d", remote, baseBranch, remote, id)
}

// reviewExtras builds the metadata fields added to the fetch/checkout output so
// the agent knows the resolved remote, source ref, base, and the exact diff
// command to run for a correct local review.
func reviewExtras(remote string, id int, baseBranch, baseCommit string) map[string]any {
	extras := map[string]any{
		"remote":     remote,
		"source_ref": fmt.Sprintf("refs/remotes/%s/pr/%d", remote, id),
	}
	if baseBranch != "" {
		extras["base_branch"] = baseBranch
		extras["base_ref"] = fmt.Sprintf("%s/%s", remote, baseBranch)
		extras["review_diff"] = reviewDiffCommand(remote, id, baseBranch)
		if baseCommit != "" {
			extras["base_commit"] = baseCommit
		}
	}
	return extras
}

// runOrPrintGit either emits a JSON envelope describing the commands (when
// execIt is false) or runs them in sequence, streaming stderr to the user.
// extras are merged into the emitted JSON either way.
func runOrPrintGit(s *appState, cmds []gitCmd, execIt bool, extras map[string]any) error {
	commands := make([]string, 0, len(cmds))
	for _, c := range cmds {
		commands = append(commands, c.Trace)
	}
	emit := func(out map[string]any) error {
		for k, v := range extras {
			out[k] = v
		}
		return s.emit(out)
	}
	if !execIt {
		return emit(map[string]any{
			"commands": commands,
			"executed": false,
			"hint":     "re-run with --exec to actually run these (must be inside a git checkout).",
		})
	}
	if s.readOnly() {
		return cerrors.New(cerrors.CategoryPermission, "READONLY_BLOCKED",
			"--exec blocked: read-only mode is enabled").
			WithHint("Drop --exec to just print the git command, or re-run with --allow-writes.").
			WithNextSteps(
				"Re-run without --exec (the command will be printed but not run)",
				"Add --allow-writes to permit this invocation",
				"unset BITBUCKET_CLI_READ_ONLY",
			)
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
	return emit(map[string]any{
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
