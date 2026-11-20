package app

import (
	"strings"
	"testing"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

func TestPickRemotePrefersUpstream(t *testing.T) {
	if got := pickRemote("", []string{"origin", "upstream"}); got != "upstream" {
		t.Errorf("pickRemote = %q, want upstream", got)
	}
}

func TestPickRemoteExplicitWins(t *testing.T) {
	if got := pickRemote("origin", []string{"origin", "upstream"}); got != "origin" {
		t.Errorf("explicit --remote origin should win, got %q", got)
	}
	if got := pickRemote("fork", []string{"origin"}); got != "fork" {
		t.Errorf("explicit --remote should be used verbatim, got %q", got)
	}
}

func TestPickRemoteFallbacks(t *testing.T) {
	if got := pickRemote("", []string{"origin"}); got != "origin" {
		t.Errorf("only origin present -> origin, got %q", got)
	}
	if got := pickRemote("", []string{"whatever"}); got != "whatever" {
		t.Errorf("no origin/upstream -> first remote, got %q", got)
	}
	if got := pickRemote("", nil); got != "origin" {
		t.Errorf("no remotes detected -> origin default, got %q", got)
	}
	if got := pickRemote("auto", []string{"origin", "upstream"}); got != "upstream" {
		t.Errorf("--remote auto should resolve like empty, got %q", got)
	}
}

func TestParseGitRemoteVerbose(t *testing.T) {
	raw := `origin	git@bb.corp.internal:~alice/demo.git (fetch)
origin	git@bb.corp.internal:~alice/demo.git (push)
upstream	https://bb.corp.internal/scm/PROJ/demo.git (fetch)
upstream	https://bb.corp.internal/scm/PROJ/demo.git (push)
`
	got := parseGitRemoteVerbose(raw)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	if got[0].Name != "origin" || got[0].URL != "git@bb.corp.internal:~alice/demo.git" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Name != "upstream" || got[1].URL != "https://bb.corp.internal/scm/PROJ/demo.git" {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestSelectVerifiedRemoteForkAutoSelectsDestination(t *testing.T) {
	remotes := []gitRemote{
		{Name: "origin", URL: "git@bb.corp.internal:~alice/demo.git"},
		{Name: "upstream", URL: "ssh://git@bb.corp.internal:7999/PROJ/demo.git"},
	}
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}
	src := apiclient.RepoRef{Workspace: "~alice", Slug: "demo"}

	name, u, err := selectVerifiedRemote("", false, remotes, true, dest, src, "https://bb.corp.internal", false)
	if err != nil {
		t.Fatal(err)
	}
	if name != "upstream" || u != "ssh://bb.corp.internal:7999/PROJ/demo.git" {
		t.Fatalf("got (%q, %q), want upstream sanitized destination remote", name, u)
	}
}

func TestSelectVerifiedRemoteRejectsSourceOnlyFork(t *testing.T) {
	remotes := []gitRemote{
		{Name: "origin", URL: "git@bb.corp.internal:~alice/demo.git"},
	}
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}
	src := apiclient.RepoRef{Workspace: "~alice", Slug: "demo"}

	for _, explicit := range []string{"", "origin"} {
		_, _, err := selectVerifiedRemote(explicit, false, remotes, true, dest, src, "https://bb.corp.internal", false)
		if err == nil {
			t.Fatalf("explicit=%q: expected GIT_REMOTE_MISMATCH for source-only fork", explicit)
		}
		ce := cerrors.AsCLIError(err)
		if ce.Code != "GIT_REMOTE_MISMATCH" || ce.Category != cerrors.CategoryUsage {
			t.Fatalf("explicit=%q: got %s/%s", explicit, ce.Category, ce.Code)
		}
		if !strings.Contains(ce.Hint, "destination repository PROJ/demo") {
			t.Errorf("explicit=%q: hint should explain destination repository requirement: %q", explicit, ce.Hint)
		}
	}
}

func TestSelectVerifiedRemoteRedactsCredentialsInOutputAndErrors(t *testing.T) {
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}

	// Success path: returned remoteURL must not leak embedded userinfo or query tokens.
	goodRemotes := []gitRemote{
		{Name: "origin", URL: "https://alice:top-secret-token@bb.corp.internal/scm/PROJ/demo.git?token=leak"},
	}
	name, u, err := selectVerifiedRemote("", false, goodRemotes, true, dest, apiclient.RepoRef{}, "https://bb.corp.internal", false)
	if err != nil {
		t.Fatal(err)
	}
	if name != "origin" || u != "https://bb.corp.internal/scm/PROJ/demo.git" {
		t.Fatalf("got (%q, %q), want sanitized URL", name, u)
	}
	if strings.Contains(u, "top-secret-token") || strings.Contains(u, "leak") {
		t.Fatalf("remote_url leaked credentials: %q", u)
	}

	// Mismatch error path: neither message nor hint may leak credentials from any remote.
	badRemotes := []gitRemote{
		{Name: "origin", URL: "https://bob:secret-origin@bb.corp.internal/scm/OTHER/repo.git?api_key=1"},
		{Name: "backup", URL: "https://token-backup@mirror.internal/scm/OTHER/repo.git"},
	}
	_, _, err = selectVerifiedRemote("origin", false, badRemotes, true, dest, apiclient.RepoRef{}, "https://bb.corp.internal", false)
	if err == nil {
		t.Fatal("expected GIT_REMOTE_MISMATCH")
	}
	ce := cerrors.AsCLIError(err)
	combined := ce.Message + " " + ce.Hint
	for _, secret := range []string{"secret-origin", "api_key", "token-backup"} {
		if strings.Contains(combined, secret) {
			t.Errorf("error leaked %q: %s", secret, combined)
		}
	}
}

func TestSelectVerifiedRemoteRejectsPortContextAndSSHAlias(t *testing.T) {
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}

	cases := []struct {
		name    string
		remote  string
		baseURL string
	}{
		{
			name:    "different DC context path",
			remote:  "https://bb.corp.internal/dc2/scm/PROJ/demo.git",
			baseURL: "https://bb.corp.internal/dc1",
		},
		{
			name:    "different HTTPS port",
			remote:  "https://bb.corp.internal:9443/scm/PROJ/demo.git",
			baseURL: "https://bb.corp.internal:8443",
		},
		{
			name:    "unverified SSH alias",
			remote:  "git@bb-alias:PROJ/demo.git",
			baseURL: "https://bb.corp.internal",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := selectVerifiedRemote("", false, []gitRemote{{Name: "origin", URL: tc.remote}}, true, dest, apiclient.RepoRef{}, tc.baseURL, false)
			if err == nil || cerrors.AsCLIError(err).Code != "GIT_REMOTE_MISMATCH" {
				t.Fatalf("expected GIT_REMOTE_MISMATCH, got %v", err)
			}
		})
	}
}

func TestSelectVerifiedRemotePrefersUpstreamWhenMultipleMatchDestination(t *testing.T) {
	remotes := []gitRemote{
		{Name: "origin", URL: "https://bb.corp.internal/scm/proj/demo.git"},
		{Name: "upstream", URL: "ssh://git@bb.corp.internal:7999/PROJ/demo.git"},
	}
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}

	name, _, err := selectVerifiedRemote("", false, remotes, true, dest, apiclient.RepoRef{}, "https://bb.corp.internal", false)
	if err != nil {
		t.Fatal(err)
	}
	if name != "upstream" {
		t.Fatalf("got %q, want upstream", name)
	}
}

func TestSelectVerifiedRemoteRejectsWrongRepository(t *testing.T) {
	remotes := []gitRemote{
		{Name: "origin", URL: "https://bb.corp.internal/scm/OTHER/unrelated.git"},
	}
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}

	_, _, err := selectVerifiedRemote("", false, remotes, true, dest, apiclient.RepoRef{}, "https://bb.corp.internal", false)
	if err == nil {
		t.Fatal("expected GIT_REMOTE_MISMATCH")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "GIT_REMOTE_MISMATCH" || ce.Category != cerrors.CategoryUsage {
		t.Fatalf("got %s/%s, want usage/GIT_REMOTE_MISMATCH", ce.Category, ce.Code)
	}
	if !strings.Contains(ce.Hint, "PROJ/demo") || !strings.Contains(ce.Hint, "OTHER/unrelated") {
		t.Errorf("hint should name both expected and detected remotes: %q", ce.Hint)
	}
}

func TestSelectVerifiedRemoteRejectsWrongHost(t *testing.T) {
	remotes := []gitRemote{
		{Name: "origin", URL: "git@github.com:team/demo.git"},
	}
	dest := apiclient.RepoRef{Workspace: "team", Slug: "demo"}

	_, _, err := selectVerifiedRemote("", false, remotes, true, dest, apiclient.RepoRef{}, "https://api.bitbucket.org", true)
	if err == nil {
		t.Fatal("expected GIT_REMOTE_MISMATCH for github.com remote against Bitbucket Cloud")
	}
	if cerrors.AsCLIError(err).Code != "GIT_REMOTE_MISMATCH" {
		t.Fatalf("got %v", err)
	}
}

func TestSelectVerifiedRemoteExplicitMismatchAndForceRemote(t *testing.T) {
	remotes := []gitRemote{
		{Name: "mirror", URL: "https://mirror.internal/scm/MIRROR/demo.git"},
	}
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}

	_, _, err := selectVerifiedRemote("mirror", false, remotes, true, dest, apiclient.RepoRef{}, "https://bb.corp.internal", false)
	if err == nil || cerrors.AsCLIError(err).Code != "GIT_REMOTE_MISMATCH" {
		t.Fatalf("expected GIT_REMOTE_MISMATCH for mismatched explicit remote, got %v", err)
	}

	name, u, err := selectVerifiedRemote("mirror", true, remotes, true, dest, apiclient.RepoRef{}, "https://bb.corp.internal", false)
	if err != nil {
		t.Fatalf("--force-remote should allow mirror remote: %v", err)
	}
	if name != "mirror" || u != remotes[0].URL {
		t.Fatalf("got (%q, %q), want mirror", name, u)
	}
}

func TestSelectVerifiedRemoteOutsideGitRepoAllowsPrintOnly(t *testing.T) {
	dest := apiclient.RepoRef{Workspace: "PROJ", Slug: "demo"}
	name, u, err := selectVerifiedRemote("", false, nil, false, dest, apiclient.RepoRef{}, "https://bb.corp.internal", false)
	if err != nil {
		t.Fatalf("outside git repo should default to origin for print-only: %v", err)
	}
	if name != "origin" || u != "" {
		t.Fatalf("got (%q, %q), want (origin, \"\")", name, u)
	}
}

func TestPRFetchCommandsWithBase(t *testing.T) {
	cmds := prFetchCommands("upstream", 42, "main")
	if len(cmds) != 2 {
		t.Fatalf("want 2 commands (source + base), got %d: %+v", len(cmds), cmds)
	}
	wantSource := "git fetch upstream refs/pull-requests/42/from:refs/remotes/upstream/pr/42"
	if cmds[0].Trace != wantSource {
		t.Errorf("source fetch = %q, want %q", cmds[0].Trace, wantSource)
	}
	if cmds[1].Trace != "git fetch upstream main" {
		t.Errorf("base fetch = %q, want 'git fetch upstream main'", cmds[1].Trace)
	}
}

func TestPRFetchCommandsWithoutBase(t *testing.T) {
	cmds := prFetchCommands("origin", 7, "")
	if len(cmds) != 1 {
		t.Fatalf("want 1 command when base unknown, got %d", len(cmds))
	}
	if cmds[0].Trace != "git fetch origin refs/pull-requests/7/from:refs/remotes/origin/pr/7" {
		t.Errorf("unexpected source fetch: %q", cmds[0].Trace)
	}
}

func TestReviewDiffCommand(t *testing.T) {
	if got := reviewDiffCommand("upstream", 42, "main"); got != "git diff upstream/main...upstream/pr/42" {
		t.Errorf("reviewDiffCommand = %q", got)
	}
	if got := reviewDiffCommand("origin", 42, ""); got != "" {
		t.Errorf("no base -> empty diff command, got %q", got)
	}
}
