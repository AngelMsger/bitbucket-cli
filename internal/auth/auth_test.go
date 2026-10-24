package auth

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/bitbucket-cli/internal/config"
	"github.com/angelmsger/bitbucket-cli/pkg/constants"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

// failingKeyring is a keychain backend whose every call fails with err.
type failingKeyring struct{ err error }

func (f failingKeyring) Get(string, string) (string, error) { return "", f.err }
func (f failingKeyring) Set(string, string, string) error   { return f.err }
func (f failingKeyring) Delete(string, string) error        { return f.err }

// newStoreWithKeyring returns bitbucket-cli's Store over a substitute keychain
// backend. The store's own behavior is tested in agent-cli-kit/credstore.
func newStoreWithKeyring(dir string, backend credstore.Backend) *Store {
	return credstore.NewStoreWithBackend(dir, constants.KeychainService, backend)
}

func TestMain(m *testing.M) {
	// Use an in-memory keychain for all tests in this package.
	keyring.MockInit()
	os.Exit(m.Run())
}

// TestCredentialValidate pins bitbucket-cli's scheme rules and their error
// codes and messages: pat needs only a token, basic needs a username too, and
// an unknown scheme is named without listing the accepted ones.
func TestCredentialValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cred    Credential
		code    string
		message string
	}{
		{"pat ok", Credential{Scheme: SchemePAT, Secret: "tok"}, "", ""},
		{"pat missing secret", Credential{Scheme: SchemePAT}, "AUTH_NO_TOKEN", "no Personal Access Token configured"},
		{"basic ok", Credential{Scheme: SchemeBasic, Username: "u", Secret: "p"}, "", ""},
		{"basic missing user", Credential{Scheme: SchemeBasic, Secret: "p"}, "AUTH_NO_BASIC", "basic auth requires both a username and a password/token"},
		{"basic missing secret", Credential{Scheme: SchemeBasic, Username: "u"}, "AUTH_NO_BASIC", "basic auth requires both a username and a password/token"},
		{"unknown scheme", Credential{Scheme: "oauth", Secret: "x"}, "AUTH_BAD_SCHEME", `unknown auth scheme "oauth"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cred.Validate()
			if tc.code == "" {
				if err != nil {
					t.Fatalf("Validate() err = %v, want nil", err)
				}
				return
			}
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Code != tc.code || ce.Message != tc.message || ce.Category != cerrors.CategoryConfig {
				t.Fatalf("Validate() = %+v, want config %s %q", ce, tc.code, tc.message)
			}
		})
	}
}

// TestCredentialHeader pins each scheme's wire form: pat is a Bearer token,
// basic is HTTP Basic.
func TestCredentialHeader(t *testing.T) {
	t.Parallel()
	pat := Credential{Scheme: SchemePAT, Secret: "abc"}
	if got := pat.Header(); got != "Bearer abc" {
		t.Errorf("pat header = %q", got)
	}
	basic := Credential{Scheme: SchemeBasic, Username: "alice", Secret: "pw"}
	// base64("alice:pw") == YWxpY2U6cHc=
	if got := basic.Header(); got != "Basic YWxpY2U6cHc=" {
		t.Errorf("basic header = %q", got)
	}
}

// TestAccountKey pins the keychain account every earlier release stored
// secrets under: the host and scheme, ignoring any deployment path.
func TestAccountKey(t *testing.T) {
	t.Parallel()
	for base, want := range map[string]string{
		"https://kms.example.com":         "kms.example.com:pat",
		"https://kms.example.com/deploy/": "kms.example.com:pat",
	} {
		if got := AccountKey(base, SchemePAT); got != want {
			t.Errorf("AccountKey(%q) = %q, want %q", base, got, want)
		}
	}
}

// NewStore keeps secrets under bitbucket-cli's keychain service, where every
// earlier release stored them. The store itself is tested in
// agent-cli-kit/credstore.
func TestNewStoreUsesBitbucketKeychainService(t *testing.T) {
	// Not parallel: the shared go-keyring mock's map is written here and is not
	// goroutine-safe, so the keychain-writing tests must not run concurrently.
	s := NewStore(t.TempDir())
	backend, err := s.Save("host:pat", "tok-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if backend != BackendKeychain {
		t.Errorf("backend = %q, want keychain", backend)
	}
	got, err := keyring.Get(constants.KeychainService, "host:pat")
	if err != nil || got != "tok-xyz" {
		t.Errorf("keychain %q holds %q, %v; want tok-xyz", constants.KeychainService, got, err)
	}
	if err := s.Delete("host:pat"); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePrefersTransientSecret(t *testing.T) {
	// Not parallel: writes the shared, non-goroutine-safe go-keyring mock map.
	s := NewStore(t.TempDir())
	if _, err := s.Save("kms.example.com:pat", "stored-token"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		BaseURL: "https://kms.example.com",
		Auth:    config.AuthConfig{Scheme: SchemePAT},
	}
	cred, err := Resolve(cfg, config.Secrets{PAT: "flag-token"}, s)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Secret != "flag-token" {
		t.Errorf("Secret = %q, want transient flag-token", cred.Secret)
	}
}

func TestResolveFallsBackToStore(t *testing.T) {
	// Not parallel: writes the shared, non-goroutine-safe go-keyring mock map.
	s := NewStore(t.TempDir())
	if _, err := s.Save("kms2.example.com:pat", "stored-token"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		BaseURL: "https://kms2.example.com",
		Auth:    config.AuthConfig{Scheme: SchemePAT},
	}
	cred, err := Resolve(cfg, config.Secrets{}, s)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Secret != "stored-token" {
		t.Errorf("Secret = %q, want stored-token", cred.Secret)
	}
}

// TestResolveSchemeSecrets pins which supplied secret each scheme sends: pat is
// the default and takes the PAT; basic takes the password (Data Center) when
// one is set, otherwise the API token (Cloud).
func TestResolveSchemeSecrets(t *testing.T) {
	t.Parallel()
	all := config.Secrets{PAT: "pat-tok", Password: "dc-pass", APIToken: "cloud-tok"}
	tests := []struct {
		name    string
		scheme  string
		secrets config.Secrets
		want    Credential
	}{
		{"default is pat", "", all, Credential{Scheme: SchemePAT, Username: "alice", Secret: "pat-tok"}},
		{"pat", SchemePAT, all, Credential{Scheme: SchemePAT, Username: "alice", Secret: "pat-tok"}},
		{"basic password wins", SchemeBasic, all, Credential{Scheme: SchemeBasic, Username: "alice", Secret: "dc-pass"}},
		{"basic api token", SchemeBasic, config.Secrets{APIToken: "cloud-tok"}, Credential{Scheme: SchemeBasic, Username: "alice", Secret: "cloud-tok"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Config{
				BaseURL: "https://bitbucket.example.com",
				Auth:    config.AuthConfig{Scheme: tc.scheme, Username: "alice"},
			}
			got, err := Resolve(cfg, tc.secrets, nil)
			if err != nil || got != tc.want {
				t.Fatalf("Resolve() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

// TestResolveNoCredentialErrors pins the recovery bitbucket-cli returns when
// the stored secret cannot be read: its code, product wording and steps.
func TestResolveNoCredentialErrors(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		BaseURL: "https://nocreds.example.com",
		Auth:    config.AuthConfig{Scheme: SchemePAT},
	}
	const hostRetry = "Retry the same command with access to the host user environment (home directory and OS keychain)."
	tests := []struct {
		name    string
		err     error
		code    string
		message string
		steps   []string
	}{
		{"missing", keyring.ErrNotFound, "CREDENTIAL_NOT_VISIBLE_OR_MISSING",
			"stored Bitbucket credentials are missing or not visible in this execution environment",
			[]string{hostRetry, "bitbucket-cli doctor",
				"Only if the host retry also reports missing credentials, run `bitbucket-cli config init` in the user's terminal or set BITBUCKET_* environment variables.",
				"bitbucket-cli auth guide", "Credential page: https://nocreds.example.com"}},
		{"inaccessible", errors.New("sandbox denied keychain"), "CREDENTIAL_STORE_INACCESSIBLE",
			"stored Bitbucket credentials cannot be read in this execution environment",
			[]string{hostRetry, "bitbucket-cli doctor",
				"Do not run `config init` unless the same check also fails in the host environment."}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStoreWithKeyring(t.TempDir(), failingKeyring{err: tc.err})
			_, err := Resolve(cfg, config.Secrets{}, s)
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Code != tc.code || ce.Message != tc.message {
				t.Fatalf("Resolve() error = %+v, want code %s %q", ce, tc.code, tc.message)
			}
			if !reflect.DeepEqual(ce.NextSteps, tc.steps) {
				t.Errorf("next steps = %q, want %q", ce.NextSteps, tc.steps)
			}
			if ce.Recovery == nil || ce.Recovery.Scope != "host" || ce.Retryable {
				t.Fatalf("Resolve() recovery = %+v retryable=%v, want host and false", ce.Recovery, ce.Retryable)
			}
		})
	}
}

// A context that reuse could serve lists `auth reuse` before any sign-in step:
// first for a missing identity, after the host retry for a missing secret.
func TestResolveListsReuseBeforeSignIn(t *testing.T) {
	t.Parallel()
	const step = "bitbucket-cli --use-context 'team' auth reuse"
	cfg := config.Config{BaseURL: "https://reuse.example.com", ReuseStep: step, Auth: config.AuthConfig{Scheme: SchemeBasic}}
	_, err := Resolve(cfg, config.Secrets{Password: "p"}, nil)
	if ce := cerrors.AsCLIError(err); ce.Code != "AUTH_NO_BASIC" || len(ce.NextSteps) == 0 || ce.NextSteps[0] != step {
		t.Fatalf("missing identity: %+v", ce)
	}
	cfg.Auth = config.AuthConfig{Scheme: SchemePAT}
	_, err = Resolve(cfg, config.Secrets{}, newStoreWithKeyring(t.TempDir(), failingKeyring{err: keyring.ErrNotFound}))
	ce := cerrors.AsCLIError(err)
	if ce.Code != "CREDENTIAL_NOT_VISIBLE_OR_MISSING" || len(ce.NextSteps) < 4 || ce.NextSteps[2] != step || ce.NextSteps[3] != MissingStep {
		t.Fatalf("missing secret: %+v", ce.NextSteps)
	}
}
