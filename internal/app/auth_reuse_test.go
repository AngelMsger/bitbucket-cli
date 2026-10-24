package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/agent-cli-kit/reuse"
	"github.com/angelmsger/bitbucket-cli/internal/auth"
	"github.com/angelmsger/bitbucket-cli/internal/config"
	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	"github.com/angelmsger/bitbucket-cli/pkg/constants"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// The decision itself is agent-cli-kit/reuse's and tested there. These tests
// cover bitbucket-cli's wiring: how contexts and their flavor are described,
// how the destination is rewritten, the account verification reports, and the
// native store, verification and command.

const reuseBase = "https://service.example.test/deploy"

// existingAccount is the account the fixture's accepted credentials act as.
var existingAccount = reuse.Account{ID: "{existing}", Name: "Existing User"}

// fakeStore answers lookups from keychain entries keyed like the real store.
type fakeStore map[string]string

func (f fakeStore) lookup(key string) credstore.Lookup {
	if v, ok := f[key]; ok {
		return credstore.Lookup{Secret: v, Keychain: credstore.Hit}
	}
	return credstore.Lookup{Keychain: credstore.Miss, File: credstore.Miss}
}

func (f fakeStore) resolve(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
	l := f.lookup(auth.AccountKey(cfg.BaseURL, cfg.Auth.Scheme))
	if !l.Found() {
		return auth.Credential{}, l, cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_NOT_VISIBLE_OR_MISSING", "missing")
	}
	cred := auth.Credential{Scheme: cfg.Auth.Scheme, Username: cfg.Auth.Username, Secret: l.Secret}
	return cred, l, cred.Validate()
}

func unauthorized() *cerrors.CLIError {
	return cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401)
}

// reuseFixture writes contexts (the last is the destination) and returns
// services over a fake store that holds the basic secret "test-secret", which
// verifies only with the username "existing-user".
func reuseFixture(t *testing.T, contexts ...config.NamedContext) (*appState, fakeStore, reuseServices) {
	t.Helper()
	dir := t.TempDir()
	if len(contexts) == 0 {
		contexts = []config.NamedContext{
			{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
			{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
		}
	}
	file := config.File{CurrentContext: contexts[0].Name, Contexts: contexts}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	target := contexts[len(contexts)-1]
	s := &appState{cfgDir: dir, resolved: &config.Resolved{Config: config.StoredContext(target, file.Defaults), ActiveContext: target.Name}}
	store := fakeStore{auth.AccountKey(reuseBase, "basic"): "test-secret"}
	services := reuseServices{Read: config.ReadFile, Write: config.WriteFile, Resolve: store.resolve, Lookup: store.lookup,
		Verify: func(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
			if cred.Secret == "test-secret" && cred.Username == "existing-user" {
				return existingAccount, nil
			}
			return reuse.Account{}, unauthorized()
		}}
	return s, store, services
}

func targetOf(t *testing.T, s *appState) config.NamedContext {
	t.Helper()
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := file.Context(s.resolved.ActiveContext)
	return c
}

func TestAuthReusePreviewApplyAndIdempotence(t *testing.T) {
	s, _, services := reuseFixture(t)
	before, _, _ := config.ReadFile(s.cfgDir)
	preview, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !preview.Changed || preview.State != "available" || !preview.Verified || preview.Scheme != "basic" || preview.Account != existingAccount.ID {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("preview changed config")
	}
	applied, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || applied.State != "reused" || applied.SourceContext != "personal" || applied.Username != "existing-user" || applied.AccountName != existingAccount.Name {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	if got := targetOf(t, s); got.Auth.Username != "existing-user" || after.CurrentContext != before.CurrentContext || !reflect.DeepEqual(after.Contexts[0], before.Contexts[0]) {
		t.Fatalf("did not preserve the source or the current context: %+v", after)
	}
	services.Write = func(string, config.File) error { t.Fatal("repeated write"); return nil }
	services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
		t.Fatal("a basic identity must remain unchanged without a lookup")
		return auth.Credential{}, credstore.Lookup{}, nil
	}
	repeated, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || repeated.Changed || repeated.State != "unchanged" {
		t.Fatalf("repeat: %+v %v", repeated, err)
	}
	raw, _ := json.Marshal(applied)
	if strings.Contains(string(raw), "test-secret") {
		t.Fatal("secret reached output")
	}
	if len(applied.NextSteps) != 1 || applied.NextSteps[0] != "bitbucket-cli --use-context 'team' whoami" {
		t.Fatalf("next steps: %v", applied.NextSteps)
	}
}

// A destination without a login of its own adopts a context of the other
// scheme on the same service, in either direction.
func TestAuthReuseAdoptsAnotherSchemesContext(t *testing.T) {
	t.Run("pat source", func(t *testing.T) {
		s, store, services := reuseFixture(t,
			config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat"}},
			config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
		)
		store[auth.AccountKey(reuseBase, "pat")] = "pat-secret"
		services.Verify = func(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
			if cred.Scheme == "pat" && cred.Secret == "pat-secret" {
				return existingAccount, nil
			}
			return reuse.Account{}, unauthorized()
		}
		r, err := reuseAuthentication(s, reuseOptions{}, services)
		if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "pat" || r.Username != "" || r.Account != existingAccount.ID {
			t.Fatalf("got %+v %v", r, err)
		}
		if got := targetOf(t, s); got.Auth.Scheme != "pat" || got.Auth.Username != "" {
			t.Fatalf("destination %+v", got)
		}
	})
	t.Run("basic source", func(t *testing.T) {
		s, _, services := reuseFixture(t,
			config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
			config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat"}},
		)
		r, err := reuseAuthentication(s, reuseOptions{}, services)
		if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "basic" || r.Username != "existing-user" {
			t.Fatalf("got %+v %v", r, err)
		}
		if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" {
			t.Fatalf("destination %+v", got)
		}
	})
}

// A pat context whose username is only a label adopts the basic secret stored
// for its service, which no context points at any more, paired with the label.
func TestAuthReuseLabelledPatAdoptsStoredSecret(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat", Username: "existing-user"}},
	)
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "" || r.Scheme != "basic" || r.Username != "existing-user" || !strings.Contains(r.Reason, "stored") {
		t.Fatalf("got %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" {
		t.Fatalf("destination %+v", got)
	}
}

// A working login is kept; an explicit source replaces it only once it no
// longer works.
func TestAuthReuseKeepsAWorkingLogin(t *testing.T) {
	s, store, services := reuseFixture(t,
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat", Username: "existing-user"}},
	)
	store[auth.AccountKey(reuseBase, "pat")] = "working-pat"
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (reuse.Account, error) {
		if cred.Secret == "working-pat" {
			return existingAccount, nil
		}
		return verify(ctx, cfg, cred)
	}
	for _, opts := range []reuseOptions{{}, {FromScheme: "basic"}} {
		r, err := reuseAuthentication(s, opts, services)
		if err != nil || r.State != "unchanged" || !r.Verified || r.Changed {
			t.Fatalf("%+v: %+v %v", opts, r, err)
		}
	}
	store[auth.AccountKey(reuseBase, "pat")] = "revoked-pat"
	r, err := reuseAuthentication(s, reuseOptions{FromScheme: "basic"}, services)
	if err != nil || r.State != "reused" || r.Scheme != "basic" || r.Username != "existing-user" {
		t.Fatalf("explicit after revocation: %+v %v", r, err)
	}
}

func TestAuthReuseMatchesCompleteServiceBeforeCredentialAccess(t *testing.T) {
	for _, kind := range []string{"host", "path", "protocol", "port", "flavor"} {
		t.Run(kind, func(t *testing.T) {
			personal := config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}}
			// The same host with another protocol, path or flavor shares the
			// destination's credential key, so stored secrets are off limits.
			sharesKey := true
			switch kind {
			case "host":
				personal.BaseURL, sharesKey = "https://other.example.test/deploy", false
			case "path":
				personal.BaseURL = "https://service.example.test/other"
			case "protocol":
				personal.BaseURL = "http://service.example.test/deploy"
			case "port":
				personal.BaseURL, sharesKey = "https://service.example.test:8443/deploy", false
			case "flavor":
				personal.Flavor = "cloud"
			}
			s, _, services := reuseFixture(t, personal, config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}})
			resolve := services.Resolve
			services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
				if cfg.Auth.Username == "existing-user" {
					t.Fatal("read another service's credential")
				}
				return resolve(cfg)
			}
			result, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
			if err != nil || result.Changed || result.State != "unavailable" {
				t.Fatalf("mismatch: %+v %v", result, err)
			}
			_, err = reuseAuthentication(s, reuseOptions{From: "personal", DryRun: true}, services)
			if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_MISMATCH" {
				t.Fatalf("explicit mismatch: %v", err)
			}
			if sharesKey {
				services.Lookup = func(string) credstore.Lookup {
					t.Fatal("read a stored secret whose key another service shares")
					return credstore.Lookup{}
				}
				_, err = reuseAuthentication(s, reuseOptions{FromScheme: "pat", DryRun: true}, services)
				if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_MISMATCH" {
					t.Fatalf("stored secret under a shared key: %v", err)
				}
			}
		})
	}
}

// A scheme the invocation supplies keeps reuse within that scheme.
func TestAuthReuseSuppliedSchemeStaysWithinIt(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat"}},
	)
	s.resolved.Sources = map[string]string{config.FieldAuthScheme: "flag"}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unavailable" || r.Changed {
		t.Fatalf("pinned: %+v %v", r, err)
	}
	for _, opts := range []reuseOptions{{From: "personal"}, {FromScheme: "basic"}} {
		_, err = reuseAuthentication(s, opts, services)
		if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_MISMATCH" {
			t.Fatalf("pinned explicit %+v: %v", opts, err)
		}
	}
}

func TestAuthReusePreservesDestinationIdentity(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "other-user"}},
	)
	services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
		t.Fatal("must not replace an existing identity")
		return auth.Credential{}, credstore.Lookup{}, nil
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unchanged" || r.Changed {
		t.Fatalf("existing identity: %+v %v", r, err)
	}
}

func TestAuthReuseAmbiguityAndExplicitSelection(t *testing.T) {
	second := "https://SERVICE.example.test:443/deploy"
	s, store, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "second", BaseURL: second, Auth: config.AuthConfig{Scheme: "basic", Username: "second-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
	)
	store[auth.AccountKey(second, "basic")] = "second-secret"
	services.Verify = func(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
		return reuse.Account{ID: "{" + cred.Username + "}"}, nil
	}
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	ce := cerrors.AsCLIError(err)
	if err == nil || ce.Code != "AUTH_REUSE_AMBIGUOUS" {
		t.Fatalf("ambiguity: %v", err)
	}
	details, _ := ce.Details.(map[string]any)
	candidates, _ := details["candidates"].([]reuse.Candidate)
	if len(candidates) != 2 || candidates[0].Account == "" || candidates[0].Account == candidates[1].Account {
		t.Fatalf("candidates: %+v", ce.Details)
	}
	r, err := reuseAuthentication(s, reuseOptions{From: "SECOND"}, services)
	if err != nil || r.SourceContext != "second" || r.State != "reused" || r.Account != "{second-user}" {
		t.Fatalf("explicit choice: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.BaseURL != second || got.Auth.Username != "second-user" {
		t.Fatalf("destination does not resolve the chosen key: %+v", got)
	}
}

// A stored basic secret pairs with every username recorded on the service;
// --from-scheme with --from-context chooses the pairing.
func TestAuthReuseStoredSecretPairingNeedsAChoice(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "x", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat", Username: "user-a"}},
		config.NamedContext{Name: "y", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat", Username: "user-b"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat"}},
	)
	services.Verify = func(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
		if cred.Scheme == "basic" && cred.Secret == "test-secret" {
			return reuse.Account{ID: "{" + cred.Username + "}"}, nil
		}
		return reuse.Account{}, unauthorized()
	}
	_, err := reuseAuthentication(s, reuseOptions{}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_AMBIGUOUS" {
		t.Fatalf("ambiguity: %v", err)
	}
	if steps := strings.Join(cerrors.AsCLIError(err).NextSteps, "\n"); !strings.Contains(steps, "--from-scheme basic") {
		t.Fatalf("no --from-scheme step: %s", steps)
	}
	r, err := reuseAuthentication(s, reuseOptions{FromScheme: "basic", From: "x"}, services)
	if err != nil || r.State != "reused" || r.Username != "user-a" || r.SourceContext != "" || r.Account != "{user-a}" {
		t.Fatalf("explicit pairing: %+v %v", r, err)
	}
}

// Credentials of one account are one login, whatever their scheme; the
// destination's scheme wins. Different accounts need a choice.
func TestAuthReuseCountsAccountsNotCredentials(t *testing.T) {
	for _, sameAccount := range []bool{true, false} {
		s, store, services := reuseFixture(t,
			config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat", Username: "existing-user"}},
			config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
		)
		store[auth.AccountKey(reuseBase, "pat")] = "pat-secret"
		verify := services.Verify
		services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (reuse.Account, error) {
			if cred.Secret != "pat-secret" {
				return verify(ctx, cfg, cred)
			}
			if sameAccount {
				return existingAccount, nil
			}
			return reuse.Account{ID: "{service-account}"}, nil
		}
		r, err := reuseAuthentication(s, reuseOptions{}, services)
		if !sameAccount {
			if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_AMBIGUOUS" {
				t.Fatalf("different accounts: %+v %v", r, err)
			}
			continue
		}
		if err != nil || r.State != "reused" || r.Scheme != "basic" || r.Username != "existing-user" || r.Account != existingAccount.ID {
			t.Fatalf("one account: %+v %v", r, err)
		}
	}
}

func TestAuthReuseDuplicateAliasesDoNotRequireChoice(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "alias", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
	)
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !r.Changed {
		t.Fatalf("same native credential: %+v %v", r, err)
	}
}

func TestAuthReuseRetainsOperationalFailures(t *testing.T) {
	for _, kind := range []string{"store", "network", "permission", "identity"} {
		t.Run(kind, func(t *testing.T) {
			s, _, services := reuseFixture(t)
			before, _, _ := config.ReadFile(s.cfgDir)
			failure := cerrors.New(cerrors.CategoryNetwork, "NETWORK_ERROR", "network unavailable")
			switch kind {
			case "store":
				failure = cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_STORE_INACCESSIBLE", "host keychain unavailable")
				services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
					return auth.Credential{}, credstore.Lookup{Keychain: credstore.Failed}, failure
				}
			case "permission":
				failure = cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "access denied")
			case "identity":
				failure = cerrors.New(cerrors.CategoryAuth, "AUTH_IDENTITY_UNAVAILABLE", "authenticated identity could not be established")
			}
			if kind != "store" {
				services.Verify = func(context.Context, config.Config, auth.Credential) (reuse.Account, error) {
					return reuse.Account{}, failure
				}
			}
			_, err := reuseAuthentication(s, reuseOptions{}, services)
			if !errors.Is(err, failure) {
				t.Fatalf("lost diagnostic: %v", err)
			}
			if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
				t.Fatal("failure mutated config")
			}
		})
	}
}

func TestAuthReuseExpiredIdentityIsUnavailable(t *testing.T) {
	s, _, services := reuseFixture(t)
	services.Verify = func(context.Context, config.Config, auth.Credential) (reuse.Account, error) {
		return reuse.Account{}, unauthorized()
	}
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || r.State != "unavailable" || r.Changed {
		t.Fatalf("expired credential: %+v %v", r, err)
	}
}

func TestAuthReuseSelfConfigurationAndConcurrentChange(t *testing.T) {
	s, _, services := reuseFixture(t)
	s.resolved.Config.Defaults.ReadOnly = true
	if r, err := reuseAuthentication(s, reuseOptions{}, services); err != nil || r.State != "reused" {
		t.Fatalf("native self-configuration should remain available: %+v %v", r, err)
	}

	s, _, services = reuseFixture(t)
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (reuse.Account, error) {
		f, _, _ := config.ReadFile(s.cfgDir)
		f.CurrentContext = "changed-by-user"
		if err := config.WriteFile(s.cfgDir, f); err != nil {
			return reuse.Account{}, err
		}
		return verify(ctx, cfg, cred)
	}
	_, err := reuseAuthentication(s, reuseOptions{}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_CONFIG_CHANGED" {
		t.Fatalf("concurrent change: %v", err)
	}

	s, store, services := reuseFixture(t)
	before, _, _ := config.ReadFile(s.cfgDir)
	resolve := services.Resolve
	calls := 0
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		calls++
		if calls > 1 {
			store[auth.AccountKey(reuseBase, "basic")] = "rotated-secret"
		}
		return resolve(cfg)
	}
	_, err = reuseAuthentication(s, reuseOptions{}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_CREDENTIAL_CHANGED" {
		t.Fatalf("credential rotation: %v", err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("rotation changed destination")
	}
}

func TestAuthReuseRejectsTargetOverridesAndUnknownSource(t *testing.T) {
	s, _, services := reuseFixture(t)
	s.resolved.Config.BaseURL = "https://service.example.test/other"
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_TARGET_MISMATCH" {
		t.Fatal(err)
	}
	s.resolved.Config.BaseURL = reuseBase
	_, err = reuseAuthentication(s, reuseOptions{From: "missing", DryRun: true}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_NOT_FOUND" {
		t.Fatal(err)
	}
}

// Sources are described by their persisted fields and the built-in default
// scheme, never by an environment secret.
func TestAuthReuseUsesPersistedDefaultsWithoutEnvironmentIdentity(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: "https://SERVICE.example.test:443/deploy", Auth: config.AuthConfig{Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase},
	)
	s.resolved.Secrets = config.Secrets{PAT: "environment-secret"}
	store := fakeStore{auth.AccountKey("https://SERVICE.example.test:443/deploy", "pat"): "stored-token"}
	services.Resolve, services.Lookup = store.resolve, store.lookup
	services.Verify = func(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
		if cred.Secret != "stored-token" {
			return reuse.Account{}, unauthorized()
		}
		return existingAccount, nil
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "pat" {
		t.Fatalf("stored defaults: %+v %v", r, err)
	}
}

// The native path verifies with the API client against both flavors, reports
// the canonical account and treats a token and a password of that account as
// one login.
func TestAuthReuseCommandVerifiesNativeStoredCredential(t *testing.T) {
	for flavor, account := range map[string]string{"cloud": "{cloud-uuid}", "datacenter": "existing-user"} {
		t.Run(flavor, func(t *testing.T) {
			keyring.MockInit()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				user, password, basic := r.BasicAuth()
				token := r.Header.Get("Authorization") == "Bearer retained-token"
				if r.Method != http.MethodGet || !(token || basic && user == "existing-user" && password == "retained-secret") {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-AUSERNAME", "existing-user")
				_, _ = w.Write([]byte(`{"uuid":"{cloud-uuid}","account_id":"cloud-account","username":"existing-user","nickname":"existing","display_name":"Existing User","name":"existing-user","slug":"existing-user","displayName":"Existing User"}`))
			}))
			defer server.Close()
			base := server.URL + "/deploy"
			s, _, _ := reuseFixture(t,
				config.NamedContext{Name: "personal", BaseURL: base, Flavor: flavor, Auth: config.AuthConfig{Scheme: "pat", Username: "existing-user"}},
				config.NamedContext{Name: "team", BaseURL: base, Flavor: flavor, Auth: config.AuthConfig{Scheme: "basic"}},
			)
			s.store = auth.NewStore(s.cfgDir)
			for _, credential := range []auth.Credential{
				{Scheme: "pat", Secret: "retained-token"},
				{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"},
			} {
				if _, err := auth.Save(base, credential, s.store); err != nil {
					t.Fatal(err)
				}
			}
			preview, err := reuseAuthentication(s, reuseOptions{DryRun: true}, s.reuseServices())
			if err != nil || preview.State != "available" || preview.Scheme != "basic" || preview.Account != account || preview.AccountName != "Existing User" {
				t.Fatalf("preview: %+v %v", preview, err)
			}
			command := newAuthReuseCmd(s)
			if err := command.RunE(command, nil); err != nil {
				t.Fatal(err)
			}
			if requests == 0 {
				t.Fatal("did not verify with the native API client")
			}
			after, _, _ := config.ReadFile(s.cfgDir)
			target, _ := after.Context("team")
			actual, err := auth.Resolve(config.StoredContext(target, after.Defaults), config.Secrets{}, auth.NewStore(s.cfgDir))
			if err != nil || actual.Username != "existing-user" || actual.Secret != "retained-secret" {
				t.Fatalf("native reuse unavailable after reload: %+v %v", actual, err)
			}
		})
	}
}

func TestAuthReusePreservesLegacyStoreKeyUnderEquivalentOverride(t *testing.T) {
	keyring.MockInit()
	legacy := "https://SERVICE.example.test:443/deploy/"
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: legacy, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
	)
	store := auth.NewStore(s.cfgDir)
	credential := auth.Credential{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"}
	if _, err := auth.Save(legacy, credential, store); err != nil {
		t.Fatal(err)
	}
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		return auth.ResolveLookup(cfg, config.Secrets{}, store)
	}
	services.Lookup = store.Lookup
	services.Verify = func(context.Context, config.Config, auth.Credential) (reuse.Account, error) {
		return existingAccount, nil
	}
	if _, err := reuseAuthentication(s, reuseOptions{}, services); err != nil {
		t.Fatal(err)
	}
	fresh, err := config.Load(config.LoadOptions{ConfigDir: s.cfgDir, Context: "team", DotenvPath: filepath.Join(t.TempDir(), "absent"), Flags: config.FlagValues{BaseURL: reuseBase}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := auth.Resolve(fresh.Config, config.Secrets{}, auth.NewStore(s.cfgDir))
	if err != nil || got.Secret != credential.Secret || got.Username != credential.Username {
		t.Fatalf("equivalent override lost native credential: %v", err)
	}
	if fresh.Config.BaseURL != reuseBase {
		t.Fatal("changed request destination")
	}
	if err := auth.ForgetForConfig(fresh.Config, fresh.Config.Auth.Scheme, store); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(auth.AccountKey(legacy, credential.Scheme)); err == nil {
		t.Fatal("logout left the original credential active")
	}
	fresh.Config.BaseURL = "https://service.example.test/other"
	_, err = auth.Resolve(fresh.Config, config.Secrets{}, store)
	if err == nil || cerrors.AsCLIError(err).Code != "CREDENTIAL_SERVICE_MISMATCH" {
		t.Fatalf("foreign deployment reused stored key: %v", err)
	}
}

func TestAuthReusePreservesWorkingTokenWithoutUsername(t *testing.T) {
	other := "https://SERVICE.example.test:443/deploy/"
	s, store, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: other, Auth: config.AuthConfig{Scheme: "pat", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "pat"}},
	)
	store[auth.AccountKey(reuseBase, "pat")] = "destination-secret"
	store[auth.AccountKey(other, "pat")] = "other-source-secret"
	before, _, _ := config.ReadFile(s.cfgDir)
	services.Verify = func(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
		if cred.Secret != "destination-secret" {
			t.Fatal("attempted to switch an already valid destination login")
		}
		return existingAccount, nil
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.Changed || !r.Verified || r.State != "unchanged" {
		t.Fatalf("existing token: %+v %v", r, err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("modified a valid token-only context")
	}
}

func TestAuthReuseMatchesPreviouslyDetectedFlavor(t *testing.T) {
	s, _, services := reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Flavor: "auto", DetectedFlavor: "datacenter", Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Flavor: "datacenter", Auth: config.AuthConfig{Scheme: "basic"}},
	)
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (reuse.Account, error) {
		if cfg.Flavor != "datacenter" {
			t.Fatal("ignored verified deployment flavor")
		}
		return verify(ctx, cfg, cred)
	}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" {
		t.Fatalf("detected flavor: %+v %v", r, err)
	}
}

// The canonical account follows the deployment: Cloud's UUID (its Atlassian
// account ID without one) and Data Center's username.
func TestReuseAccountIsCanonicalPerFlavor(t *testing.T) {
	for _, tc := range []struct {
		flavor apiclient.Flavor
		user   apiclient.User
		want   reuse.Account
	}{
		{apiclient.FlavorCloud, apiclient.User{UUID: "{u}", AccountID: "a", Name: "n", DisplayName: "D"}, reuse.Account{ID: "{u}", Name: "D"}},
		{apiclient.FlavorCloud, apiclient.User{AccountID: "a", Name: "n"}, reuse.Account{ID: "a", Name: "n"}},
		{apiclient.FlavorDataCenter, apiclient.User{UUID: "{u}", Name: "n", DisplayName: "D"}, reuse.Account{ID: "n", Name: "D"}},
	} {
		if got := reuseAccount(&tc.user, tc.flavor); got != tc.want {
			t.Errorf("%s %+v: got %+v, want %+v", tc.flavor, tc.user, got, tc.want)
		}
	}
}

// A rejected credential leads with reuse when a stored login could replace it;
// other errors, and reuse or login themselves, keep their steps.
func TestRejectedCredentialRecoveryLeadsWithReuse(t *testing.T) {
	step := "bitbucket-cli --use-context 'team' auth reuse"
	s := &appState{resolved: &config.Resolved{Config: config.Config{BaseURL: reuseBase, ReuseStep: step}}}
	ce := unauthorized()
	withReuseRecovery(ce, s, &cobra.Command{Use: "whoami"})
	if len(ce.NextSteps) < 2 || ce.NextSteps[0] != step {
		t.Fatalf("next steps %v", ce.NextSteps)
	}
	for _, c := range []struct {
		err *cerrors.CLIError
		cmd string
	}{
		{cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "denied").WithHTTPStatus(403), "whoami"},
		{unauthorized(), "reuse"},
		{unauthorized(), "login"},
	} {
		steps := append([]string(nil), c.err.NextSteps...)
		withReuseRecovery(c.err, s, &cobra.Command{Use: c.cmd})
		if !reflect.DeepEqual(steps, c.err.NextSteps) {
			t.Fatalf("%s %s gained a reuse step: %v", c.err.Code, c.cmd, c.err.NextSteps)
		}
	}
}

// A source-selection failure recovers through the context listing. That
// command's name differs across the CLI family, so resolve what each error
// advertises against this command tree instead of trusting its spelling.
func TestAuthReuseSourceRecoveryNamesARealCommand(t *testing.T) {
	failures := map[string]error{}

	s, _, services := reuseFixture(t)
	_, failures["AUTH_REUSE_SOURCE_NOT_FOUND"] = reuseAuthentication(s, reuseOptions{From: "missing", DryRun: true}, services)

	s, _, services = reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "second", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: "second-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
	)
	services.Verify = func(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
		return reuse.Account{ID: cred.Username}, nil
	}
	_, failures["AUTH_REUSE_AMBIGUOUS"] = reuseAuthentication(s, reuseOptions{DryRun: true}, services)

	s, _, services = reuseFixture(t,
		config.NamedContext{Name: "personal", BaseURL: "https://other.example.test/deploy", Auth: config.AuthConfig{Scheme: "basic", Username: "existing-user"}},
		config.NamedContext{Name: "team", BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic"}},
	)
	_, failures["AUTH_REUSE_SOURCE_MISMATCH"] = reuseAuthentication(s, reuseOptions{From: "personal", DryRun: true}, services)

	for code, err := range failures {
		if err == nil || cerrors.AsCLIError(err).Code != code {
			t.Fatalf("%s: got %v", code, err)
		}
		steps := cerrors.AsCLIError(err).NextSteps
		if len(steps) == 0 {
			t.Fatalf("%s: no recovery command", code)
		}
		words := strings.Fields(steps[0])
		if words[0] != constants.AppName {
			t.Fatalf("%s: recovery %q is not a %s command", code, steps[0], constants.AppName)
		}
		cmd, rest, findErr := NewRootCmd().Find(words[1:])
		if findErr != nil || len(rest) != 0 || !cmd.Runnable() {
			t.Errorf("%s: recovery %q does not resolve to a command (unresolved %v, %v)", code, steps[0], rest, findErr)
		}
	}
}
