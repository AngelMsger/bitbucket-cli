// Git transport keeps token access inside the companion and preserves Git's
// streams/exit status. --fallback permits ordinary Git for unrelated remotes.
package app

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/angelmsger/bitbucket-cli/internal/auth"
	"github.com/angelmsger/bitbucket-cli/internal/config"
	"github.com/angelmsger/bitbucket-cli/internal/gittransport"
	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/spf13/cobra"
)

type gitExit struct{ code int }

func (e *gitExit) Error() string { return fmt.Sprintf("git exited with status %d", e.code) }

func newGitCmd(s *appState) *cobra.Command {
	var fallback, dryRun bool
	cmd := &cobra.Command{
		Use:     "git [--fallback] [--dry-run] -- <git arguments>",
		Short:   "Run Git over HTTPS using the existing Bitbucket login",
		Long:    "Authenticate clone, fetch, ls-remote or push with the selected Bitbucket login.\nSSH URLs are translated to the repository's advertised HTTPS URL for this call only.\nNo token or URL rewrite is saved in Git configuration. Git output and exit status are preserved.\nPass Git's arguments after --. Recursive submodules and multiple remotes are not supported.\nGit -c accepts only core.quotepath and protocol.version.\nWithout an explicit context, an unambiguous matching stored service may be selected.\n--fallback uses ordinary Git only for unmatched URLs or an absent credential; rejected credentials remain errors.",
		Example: "  bitbucket-cli git -- clone ssh://git@git.example.com/TEAM/repo.git\n  bitbucket-cli git -- -C repo fetch upstream\n  bitbucket-cli git -- ls-remote https://git.example.com/scm/TEAM/repo.git",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() != 0 {
				return gitUsage("Put Git arguments after --")
			}
			c, err := gittransport.Parse(args)
			if err != nil {
				return gitUsage(err.Error())
			}
			raw, err := c.URL()
			if err != nil {
				return gitUsage(err.Error())
			}
			selected, ref, err := s.gitService(raw)
			if err != nil {
				return err
			}
			// Unmatched fallback belongs to ordinary Git, not this service's posture.
			if (selected != nil || !fallback) && !dryRun && c.Operation != "ls-remote" && s.readOnly() {
				return gitReadOnly()
			}
			var a *gittransport.Authentication
			if selected != nil {
				a, err = selected.gitAuthentication(cmd, raw, ref)
				if err != nil && !(fallback && missingGitCredential(err)) {
					return err
				}
			}
			if a == nil && !fallback {
				return cerrors.New(cerrors.CategoryConfig, "GIT_SERVICE_UNMATCHED", "No authenticated Bitbucket service matches this Git URL").WithNextSteps("bitbucket-cli config get-contexts", "Select the matching service with --use-context, or use --fallback for ordinary Git.")
			}
			if dryRun {
				target := raw
				if a != nil {
					target = a.HTTPS
				}
				return s.emit(map[string]any{"command": append([]string{"git"}, args...), "url": target, "authenticated": a != nil, "writes": c.Operation != "ls-remote"})
			}
			code, err := gittransport.Run(cmd.Context(), c, a, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "GIT_START_FAILED", "Could not start Git").WithNextSteps("git --version")
			}
			if code != 0 {
				return &gitExit{code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fallback, "fallback", false, "use ordinary Git when no matching service or credential is configured")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "resolve and print the public transport plan without executing Git")
	return cmd
}
func gitUsage(message string) error {
	return cerrors.New(cerrors.CategoryUsage, "GIT_ARGUMENTS", message).WithNextSteps("bitbucket-cli git --help")
}
func gitReadOnly() error {
	return cerrors.New(cerrors.CategoryPermission, "READONLY_BLOCKED", "Git would change local or remote state").WithNextSteps("Preview with --dry-run, or pass --allow-writes for this invocation.")
}
func missingGitCredential(err error) bool {
	code := cerrors.AsCLIError(err).Code
	return code == "AUTH_NO_TOKEN" || code == "AUTH_NO_BASIC"
}
func gitCloud(cfg config.Config) bool {
	base, _ := url.Parse(cfg.BaseURL)
	known := base != nil && (base.Hostname() == "api.bitbucket.org" || base.Hostname() == "bitbucket.org")
	return cfg.Flavor == config.FlavorCloud || cfg.DetectedFlavor == config.FlavorCloud || known
}
func (s *appState) gitAuthentication(cmd *cobra.Command, raw string, ref apiclient.RepoRef) (*gittransport.Authentication, error) {
	cfg := s.cfg()
	cred, err := auth.Resolve(cfg, s.resolved.Secrets, s.store)
	if err != nil {
		return nil, err
	}
	ctx, cancel := cmdContext(s)
	defer cancel()
	client, err := s.newClient(ctx)
	if err != nil {
		return nil, err
	}
	repo, err := client.GetRepository(ctx, ref)
	if err != nil {
		return nil, err
	}
	target, err := gittransport.HTTPS(repo.CloneHTTPS, cfg.BaseURL, gitCloud(cfg))
	if err != nil {
		return nil, cerrors.New(cerrors.CategoryPermission, "GIT_UNTRUSTED_URL", err.Error()).WithNextSteps("Verify the repository's HTTPS clone URL and the selected Bitbucket service.")
	}
	header := gitHeader(cred, gitCloud(cfg))
	if strings.ContainsAny(header, "\r\n\x00") {
		return nil, gitUsage("Invalid Git credential")
	}
	return &gittransport.Authentication{Original: raw, HTTPS: target, Header: header, Secret: cred.Secret}, nil
}

// URL matching happens before any credential lookup. Explicit selectors never
// fall through to another account; automatic selection requires one identity.
func (s *appState) gitService(raw string) (*appState, apiclient.RepoRef, error) {
	match := func(state *appState) (apiclient.RepoRef, bool) {
		cfg := state.cfg()
		project, name, ok := gittransport.Repository(raw, cfg.BaseURL, gitCloud(cfg))
		return apiclient.RepoRef{Workspace: project, Slug: name}, ok
	}
	if ref, ok := match(s); ok {
		return s, ref, nil
	}
	if s.resolved.ContextSelectedExplicitly() || s.gflags.baseURL != "" || s.gflags.authScheme != "" || s.gflags.flavor != "" || s.resolved.Sources[config.FieldServer] == "env" || s.resolved.Sources[config.FieldServer] == "dotenv" {
		return nil, apiclient.RepoRef{}, nil
	}
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		return nil, apiclient.RepoRef{}, err
	}
	var selected *appState
	var ref apiclient.RepoRef
	identity := ""
	for _, named := range file.Contexts {
		candidate := &appState{gflags: s.gflags, cfgDir: s.cfgDir, store: s.store,
			resolved: &config.Resolved{Config: config.StoredContext(named, file.Defaults), ActiveContext: named.Name}}
		candidate.gflags.useContext = named.Name
		found, ok := match(candidate)
		if !ok {
			continue
		}
		cfg := candidate.cfg()
		normalized, err := config.NormalizeServiceURL(cfg.BaseURL)
		if err != nil {
			return nil, ref, err
		}
		key := normalized + "\x00" + cfg.Auth.Scheme
		if cfg.Auth.Scheme == auth.SchemeBasic {
			key += "\x00" + cfg.Auth.Username
		}
		if selected != nil && identity != key {
			return nil, ref, cerrors.New(cerrors.CategoryConfig, "GIT_CONTEXT_AMBIGUOUS", "Several Bitbucket identities match this Git URL").WithNextSteps("bitbucket-cli config get-contexts", "Choose the intended identity with --use-context.")
		}
		selected = candidate
		ref = found
		identity = key
	}
	return selected, ref, nil
}

// Cloud API-token email identities differ from the static Git token identity.
func gitHeader(cred auth.Credential, cloud bool) string {
	if !cloud {
		return cred.Header()
	}
	user := "x-token-auth"
	if cred.Scheme == auth.SchemeBasic {
		user = "x-bitbucket-api-token-auth"
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+cred.Secret))
}
