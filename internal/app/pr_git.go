package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/angelmsger/bitbucket-cli/internal/gittransport"
	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// newPRFetchCmd prints (or runs) the `git fetch` invocations needed to bring a
// PR's source ref AND its base branch into the local repository. Default is
// print-only; --exec shells out to git inside the current working directory.
func newPRFetchCmd(s *appState) *cobra.Command {
	var (
		execIt      bool
		remote      string
		base        string
		forceRemote bool
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
			if execIt && s.readOnly() {
				return readOnlyBlockedGitExec()
			}
			meta, err := resolvePRGitMeta(s, ref, id, base)
			if err != nil {
				return err
			}
			remotes, inRepo := inspectLocalGitRemotes()
			chosen, chosenURL, err := selectVerifiedRemote(
				remote, forceRemote, remotes, inRepo,
				meta.destRepo, meta.sourceRepo, meta.baseURL, meta.isCloud,
			)
			if err != nil {
				return err
			}
			cmds := prFetchCommands(chosen, id, meta.baseBranch)
			return runOrPrintGit(s, cmds, execIt, reviewExtras(chosen, chosenURL, id, meta.baseBranch, meta.baseCommit))
		},
	}
	cmd.Flags().BoolVar(&execIt, "exec", false, "actually run `git fetch` in the current working directory")
	cmd.Flags().StringVar(&remote, "remote", "", "git remote to fetch from (default: auto-select remote matching PR repository, preferring upstream then origin)")
	cmd.Flags().StringVar(&base, "base", "", "base branch to fetch (default: the PR's destination branch, looked up via the API)")
	cmd.Flags().BoolVar(&forceRemote, "force-remote", false, "bypass repository remote URL identity verification (for mirrors or custom remotes)")
	return cmd
}

// newPRCheckoutCmd prints (or runs) the git command sequence that brings the
// PR and its base locally and switches to the PR (creating a `pr/<id>` branch).
func newPRCheckoutCmd(s *appState) *cobra.Command {
	var (
		execIt      bool
		remote      string
		base        string
		branchFmt   string
		forceRemote bool
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
			if execIt && s.readOnly() {
				return readOnlyBlockedGitExec()
			}
			meta, err := resolvePRGitMeta(s, ref, id, base)
			if err != nil {
				return err
			}
			remotes, inRepo := inspectLocalGitRemotes()
			chosen, chosenURL, err := selectVerifiedRemote(
				remote, forceRemote, remotes, inRepo,
				meta.destRepo, meta.sourceRepo, meta.baseURL, meta.isCloud,
			)
			if err != nil {
				return err
			}
			localBranch := fmt.Sprintf(branchFmt, id)
			cmds := append(prFetchCommands(chosen, id, meta.baseBranch), gitCmd{
				Args:  []string{"checkout", localBranch},
				Trace: fmt.Sprintf("git checkout %s", localBranch),
			})
			return runOrPrintGit(s, cmds, execIt, reviewExtras(chosen, chosenURL, id, meta.baseBranch, meta.baseCommit))
		},
	}
	cmd.Flags().BoolVar(&execIt, "exec", false, "actually run the git commands in the current working directory")
	cmd.Flags().StringVar(&remote, "remote", "", "git remote to fetch from (default: auto-select remote matching PR repository, preferring upstream then origin)")
	cmd.Flags().StringVar(&base, "base", "", "base branch to fetch (default: the PR's destination branch, looked up via the API)")
	cmd.Flags().StringVar(&branchFmt, "branch", "pr/%d",
		"local branch name format; %d is replaced with the PR id")
	cmd.Flags().BoolVar(&forceRemote, "force-remote", false, "bypass repository remote URL identity verification (for mirrors or custom remotes)")
	return cmd
}

type gitCmd struct {
	Args  []string
	Trace string
}

type gitRemote struct {
	Name string
	URL  string
}

type prGitMeta struct {
	baseBranch string
	baseCommit string
	destRepo   apiclient.RepoRef
	sourceRepo apiclient.RepoRef
	baseURL    string
	isCloud    bool
}

// resolvePRGitMeta determines the PR's destination/source repository identities
// and base branch/commit. An explicit --base skips the API call (keeping the
// command usable offline) while still verifying the destination RepoRef from the
// PR reference.
func resolvePRGitMeta(s *appState, ref apiclient.RepoRef, id int, baseOverride string) (prGitMeta, error) {
	meta := prGitMeta{
		baseBranch: baseOverride,
		destRepo:   ref,
		baseURL:    s.cfg().BaseURL,
		isCloud:    apiclient.Flavor(s.cfg().Flavor) == apiclient.FlavorCloud,
	}
	if baseOverride != "" {
		return meta, nil
	}
	ctx, cancel := cmdContext(s)
	defer cancel()
	client, err := s.newClient(ctx)
	if err != nil {
		return prGitMeta{}, err
	}
	pr, err := client.GetPR(ctx, apiclient.GetPROpts{Repo: ref, ID: id, Scope: apiclient.PRScopeSummary})
	if err != nil {
		return prGitMeta{}, err
	}
	meta.baseBranch = pr.Destination.Branch
	meta.baseCommit = pr.Destination.Commit
	meta.baseURL = client.BaseURL()
	meta.isCloud = client.Flavor() == apiclient.FlavorCloud
	if dst := parseFullRepoName(pr.Destination.Repository); dst.Slug != "" {
		meta.destRepo = dst
	}
	if src := parseFullRepoName(pr.Source.Repository); src.Slug != "" {
		meta.sourceRepo = src
	}
	return meta, nil
}

func parseFullRepoName(full string) apiclient.RepoRef {
	ws, slug, ok := strings.Cut(strings.TrimSpace(full), "/")
	if !ok || ws == "" || slug == "" || strings.Contains(slug, "/") {
		return apiclient.RepoRef{}
	}
	return apiclient.RepoRef{Workspace: ws, Slug: slug}
}

// selectVerifiedRemote resolves and verifies the local git remote against the
// PR's destination and source repositories.
//
//   - Outside a git checkout (inRepo == false), print-only mode keeps working
//     using the explicit remote or "origin" (while --exec still fails in
//     ensureGitWorktree).
//   - Inside a git checkout without an explicit --remote, remotes matching
//     destRepo win (preferring "upstream", then "origin"); if none match
//     destRepo, remotes matching sourceRepo are considered. If no remote
//     matches either repository, GIT_REMOTE_MISMATCH is returned unless
//     forceRemote is set.
//   - Inside a git checkout with an explicit --remote, that remote must exist
//     and match destRepo or sourceRepo unless forceRemote is set.
func selectVerifiedRemote(
	explicit string,
	forceRemote bool,
	remotes []gitRemote,
	inRepo bool,
	destRepo, sourceRepo apiclient.RepoRef,
	baseURL string,
	cloud bool,
) (name, remoteURL string, err error) {
	explicit = strings.TrimSpace(explicit)
	if !inRepo {
		if explicit != "" && explicit != "auto" {
			return explicit, "", nil
		}
		return "origin", "", nil
	}

	matches := func(rawURL string, target apiclient.RepoRef) bool {
		if target.Workspace == "" || target.Slug == "" {
			return false
		}
		parsed, ok := gittransport.ParseRemoteRepo(rawURL)
		if !ok {
			return false
		}
		if !parsed.MatchesRepo(target.Workspace, target.Slug) {
			return false
		}
		return parsed.MatchesHost(baseURL, cloud)
	}

	if explicit != "" && explicit != "auto" {
		for _, r := range remotes {
			if r.Name == explicit {
				if forceRemote || matches(r.URL, destRepo) || matches(r.URL, sourceRepo) {
					return r.Name, r.URL, nil
				}
				return "", "", remoteMismatchError(
					fmt.Sprintf("git remote %q (%s) does not match PR repository %s", r.Name, r.URL, formatExpectedRepos(destRepo, sourceRepo)),
					remotes, destRepo, sourceRepo,
				)
			}
		}
		if forceRemote {
			return explicit, "", nil
		}
		return "", "", remoteMismatchError(
			fmt.Sprintf("git remote %q is not configured in this repository (expected %s)", explicit, formatExpectedRepos(destRepo, sourceRepo)),
			remotes, destRepo, sourceRepo,
		)
	}

	// Auto-selection: prefer destination repo matches, then source repo matches.
	var destMatches, srcMatches []gitRemote
	for _, r := range remotes {
		if matches(r.URL, destRepo) {
			destMatches = append(destMatches, r)
		} else if matches(r.URL, sourceRepo) {
			srcMatches = append(srcMatches, r)
		}
	}
	if len(destMatches) > 0 {
		chosen := pickPreferredRemote(destMatches)
		return chosen.Name, chosen.URL, nil
	}
	if len(srcMatches) > 0 {
		chosen := pickPreferredRemote(srcMatches)
		return chosen.Name, chosen.URL, nil
	}
	if forceRemote {
		names := make([]string, 0, len(remotes))
		for _, r := range remotes {
			names = append(names, r.Name)
		}
		chosenName := pickRemote("", names)
		for _, r := range remotes {
			if r.Name == chosenName {
				return r.Name, r.URL, nil
			}
		}
		return chosenName, "", nil
	}
	return "", "", remoteMismatchError(
		fmt.Sprintf("no git remote in this repository matches PR repository %s", formatExpectedRepos(destRepo, sourceRepo)),
		remotes, destRepo, sourceRepo,
	)
}

func pickPreferredRemote(candidates []gitRemote) gitRemote {
	for _, pref := range []string{"upstream", "origin"} {
		for _, r := range candidates {
			if r.Name == pref {
				return r
			}
		}
	}
	return candidates[0]
}

func formatExpectedRepos(dest, src apiclient.RepoRef) string {
	d := dest.Workspace + "/" + dest.Slug
	if src.Workspace != "" && src.Slug != "" &&
		(!strings.EqualFold(src.Workspace, dest.Workspace) || !strings.EqualFold(src.Slug, dest.Slug)) {
		return fmt.Sprintf("%s (or source fork %s/%s)", d, src.Workspace, src.Slug)
	}
	return d
}

func remoteMismatchError(msg string, remotes []gitRemote, dest, src apiclient.RepoRef) error {
	var found []string
	for _, r := range remotes {
		found = append(found, fmt.Sprintf("%s=%s", r.Name, r.URL))
	}
	detected := "(none)"
	if len(found) > 0 {
		detected = strings.Join(found, ", ")
	}
	hint := fmt.Sprintf("Expected repository %s; current checkout remotes: %s.",
		formatExpectedRepos(dest, src), detected)
	return cerrors.New(cerrors.CategoryUsage, "GIT_REMOTE_MISMATCH", msg).
		WithHint(hint).
		WithNextSteps(
			"cd into a local clone of "+dest.Workspace+"/"+dest.Slug+" and retry",
			"git remote -v   # inspect configured remotes",
			"Pass --remote <name> --force-remote only if fetching from an intentional mirror",
		)
}

// pickRemote chooses the git remote to fetch a PR and its base from when
// identity validation is bypassed (--force-remote) or unavailable.
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

// inspectLocalGitRemotes checks whether cwd is inside a git checkout and parses
// `git remote -v` fetch URLs.
func inspectLocalGitRemotes() ([]gitRemote, bool) {
	inOut, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(inOut)) != "true" {
		return nil, false
	}
	out, err := exec.Command("git", "remote", "-v").Output()
	if err != nil {
		return nil, true
	}
	return parseGitRemoteVerbose(string(out)), true
}

// parseGitRemoteVerbose parses `git remote -v` output and returns unique
// remotes with their fetch URLs in appearance order.
func parseGitRemoteVerbose(raw string) []gitRemote {
	var remotes []gitRemote
	seen := map[string]int{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, u := fields[0], fields[1]
		isFetch := len(fields) < 3 || fields[2] == "(fetch)"
		if idx, exists := seen[name]; exists {
			if isFetch {
				remotes[idx].URL = u
			}
			continue
		}
		seen[name] = len(remotes)
		remotes = append(remotes, gitRemote{Name: name, URL: u})
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
func reviewExtras(remote, remoteURL string, id int, baseBranch, baseCommit string) map[string]any {
	extras := map[string]any{
		"remote":     remote,
		"source_ref": fmt.Sprintf("refs/remotes/%s/pr/%d", remote, id),
	}
	if remoteURL != "" {
		extras["remote_url"] = remoteURL
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

func readOnlyBlockedGitExec() error {
	e := cerrors.ReadOnlyBlocked("--exec", cerrors.ReadOnlyMode{
		Env:        "BITBUCKET_CLI_READ_ONLY",
		ConfigFile: "~/.angelmsger/bitbucket/config.yaml",
	})
	e.NextSteps = append([]string{"Re-run without --exec to print the git commands without running them"}, e.NextSteps...)
	return e
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
		return readOnlyBlockedGitExec()
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
