package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestFileFormat pins the bytes of config.yaml. The engine is shared; the
// shape is bitbucket-cli's, with the default workspace written first, and
// existing files must keep reading and writing the same way.
func TestFileFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	in := File{
		CurrentContext: "prod",
		Contexts: []NamedContext{
			{Name: "default", BaseURL: "https://dc.example.com/bitbucket", Flavor: FlavorDataCenter,
				Auth: AuthConfig{Scheme: SchemePAT}},
			{Name: "prod", BaseURL: "https://api.bitbucket.org", Flavor: FlavorAuto, DetectedFlavor: FlavorCloud,
				Auth: AuthConfig{Scheme: SchemeBasic, Username: "alice@example.com",
					CredentialURL: "https://id.atlassian.com/manage-profile/security/api-tokens"}},
		},
		Defaults: Defaults{Format: "table", PageSize: 50, Timeout: 90 * time.Second, MaxRetries: 5,
			Workspace: "TEAM", ReadOnly: true},
	}
	if err := WriteFile(dir, in); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(ConfigFilePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	const want = `current_context: prod
contexts:
    - name: default
      server: https://dc.example.com/bitbucket
      flavor: datacenter
      auth:
        scheme: pat
    - name: prod
      server: https://api.bitbucket.org
      flavor: auto
      detected_flavor: cloud
      auth:
        credential_url: https://id.atlassian.com/manage-profile/security/api-tokens
        scheme: basic
        username: alice@example.com
defaults:
    workspace: TEAM
    format: table
    page_size: 50
    timeout: 1m30s
    max_retries: 5
    read_only: true
`
	if string(raw) != want {
		t.Errorf("config.yaml changed:\n%s", raw)
	}
	got, ok, err := ReadFile(dir)
	if err != nil || !ok || !reflect.DeepEqual(got, in) {
		t.Errorf("read back %+v, %v, %v", got, ok, err)
	}
}

// TestSchema pins bitbucket-cli's environment variables, defaults and scheme
// inference.
func TestSchema(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	load := func() *Resolved {
		t.Helper()
		r, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env")})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load()
	want := Config{Flavor: FlavorAuto, Auth: AuthConfig{Scheme: SchemePAT},
		Defaults: Defaults{Format: "json", PageSize: 25, Timeout: 30 * time.Second, MaxRetries: 3}}
	if !reflect.DeepEqual(r.Config, want) || r.Sources[FieldAuthScheme] != "default" {
		t.Fatalf("defaults = %+v (%v)", r.Config, r.Sources)
	}

	t.Setenv("BITBUCKET_SERVER", "https://bitbucket.example.com")
	t.Setenv("BITBUCKET_FLAVOR", FlavorDataCenter)
	t.Setenv("BITBUCKET_USERNAME", "alice")
	t.Setenv("BITBUCKET_CREDENTIAL_URL", "https://help.example.com/token")
	t.Setenv("BITBUCKET_DEFAULT_WORKSPACE", "PROJ")
	t.Setenv("BITBUCKET_FORMAT", "table")
	t.Setenv("BITBUCKET_CLI_READ_ONLY", "yes")
	t.Setenv("BITBUCKET_API_TOKEN", "api")
	r = load()
	if r.Config.BaseURL != "https://bitbucket.example.com" || r.Config.Flavor != FlavorDataCenter ||
		r.Config.Auth.Username != "alice" || r.Config.Auth.Scheme != SchemeBasic ||
		r.Config.Auth.CredentialURL != "https://help.example.com/token" || r.Config.Defaults.Workspace != "PROJ" ||
		r.Config.Defaults.Format != "table" || !r.Config.Defaults.ReadOnly || r.Secrets != (Secrets{APIToken: "api"}) {
		t.Fatalf("environment = %+v %+v", r.Config, r.Secrets)
	}
	t.Setenv("BITBUCKET_PASSWORD", "pw")
	if r = load(); r.Config.Auth.Scheme != SchemeBasic || r.Secrets.Password != "pw" {
		t.Errorf("a password selects basic: %+v %+v", r.Config.Auth, r.Secrets)
	}
	t.Setenv("BITBUCKET_PERSONAL_ACCESS_TOKEN", "legacy")
	if r = load(); r.Config.Auth.Scheme != SchemePAT || r.Secrets.PAT != "legacy" {
		t.Errorf("a token wins over a password: %+v %+v", r.Config.Auth, r.Secrets)
	}
	t.Setenv("BITBUCKET_TOKEN", "token")
	if r = load(); r.Secrets.PAT != "token" {
		t.Errorf("BITBUCKET_TOKEN wins over its legacy alias: %q", r.Secrets.PAT)
	}
	t.Setenv("BITBUCKET_AUTH_SCHEME", SchemeBasic)
	if r = load(); r.Config.Auth.Scheme != SchemeBasic {
		t.Errorf("an explicit scheme wins: %q", r.Config.Auth.Scheme)
	}
	if _, err := NormalizeServiceURL("bitbucket.example.com"); cerrors.AsCLIError(err).Code != "BAD_BASE_URL" {
		t.Errorf("a bare host is BAD_BASE_URL: %v", err)
	}

	writeMultiContextFile(t, dir)
	t.Setenv("BITBUCKET_SERVER", "")
	t.Setenv("BITBUCKET_CONTEXT", "beta")
	if r = load(); r.ActiveContext != "beta" || r.ContextSource != ContextSourceEnv || !r.ContextSelectedExplicitly() {
		t.Errorf("BITBUCKET_CONTEXT selects a context: %q %q", r.ActiveContext, r.ContextSource)
	}
}

// TestAtlassianRules pins the Cloud/Data Center hooks: Cloud defaults to
// basic auth, the flavor is checked, and a detected flavor does not survive
// a change of server or flavor.
func TestAtlassianRules(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	writeFile(t, ConfigFilePath(dir), `contexts:
  - name: cloud
    server: https://API.bitbucket.org/
    detected_flavor: cloud
`)
	load := func(flags FlagValues) *Resolved {
		t.Helper()
		r, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env"), Flags: flags})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load(FlagValues{})
	if r.Config.Auth.Scheme != SchemeBasic || r.Sources[FieldAuthScheme] != "default" || r.Config.DetectedFlavor != FlavorCloud {
		t.Errorf("Cloud default = %+v (%v)", r.Config, r.Sources)
	}
	if got := StoredContext(NamedContext{Name: "c", BaseURL: "https://bitbucket.org"}, Defaults{}); got.Auth.Scheme != SchemeBasic {
		t.Errorf("a stored Cloud context resolves basic: %q", got.Auth.Scheme)
	}
	if r = load(FlagValues{AuthScheme: SchemePAT}); r.Config.Auth.Scheme != SchemePAT {
		t.Errorf("an explicit scheme stays: %q", r.Config.Auth.Scheme)
	}
	if r = load(FlagValues{BaseURL: "https://api.bitbucket.org"}); r.Config.DetectedFlavor != FlavorCloud ||
		r.Config.CredentialBaseURL != "https://API.bitbucket.org/" {
		t.Errorf("another spelling keeps the detected flavor and the stored lookup URL: %+v", r.Config)
	}
	if r = load(FlagValues{BaseURL: "https://dc.example.com"}); r.Config.DetectedFlavor != "" || r.Config.Auth.Scheme != SchemePAT {
		t.Errorf("another server drops the detected flavor: %+v", r.Config)
	}

	file, _, err := ReadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	resolved := load(FlagValues{Flavor: FlavorDataCenter})
	plan, err := PlanServiceContext(file, "cloud", resolved, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if nc, _ := plan.File.Context("cloud"); nc.Flavor != FlavorDataCenter || nc.DetectedFlavor != "" {
		t.Errorf("a flavor change clears the detected flavor: %+v", nc)
	}
	resolved.Config.Flavor = "server"
	if _, err := PlanServiceContext(file, "cloud", resolved, false, false); cerrors.AsCLIError(err).Code != "BAD_FLAVOR" {
		t.Errorf("unknown flavor: %v", err)
	}
	if err := ValidateService(Config{BaseURL: "https://x.example.com", Auth: AuthConfig{Scheme: "oauth"}}); cerrors.AsCLIError(err).Hint != "Use one of: pat, basic." {
		t.Errorf("bad scheme: %+v", cerrors.AsCLIError(err))
	}
}

func TestReadFileLegacyFlatFormat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, ConfigFilePath(dir),
		"server: https://legacy.example.com\nflavor: datacenter\nauth:\n  scheme: pat\n")
	got, exists, err := ReadFile(dir)
	if err != nil || !exists {
		t.Fatalf("ReadFile: exists=%v err=%v", exists, err)
	}
	if len(got.Contexts) != 1 || got.Contexts[0].Name != DefaultContextName {
		t.Fatalf("legacy file should yield one default context: %+v", got.Contexts)
	}
	if got.CurrentContext != DefaultContextName {
		t.Errorf("CurrentContext = %q, want default", got.CurrentContext)
	}
	if got.Contexts[0].BaseURL != "https://legacy.example.com" {
		t.Errorf("BaseURL = %q", got.Contexts[0].BaseURL)
	}
}

func TestLoadLegacyFlatUnchanged(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	writeFile(t, ConfigFilePath(dir), "server: https://file.example.com\nflavor: datacenter\n")
	got, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.BaseURL != "https://file.example.com" || got.Config.Flavor != FlavorDataCenter {
		t.Errorf("legacy load mismatch: %+v", got.Config)
	}
	if got.Sources[fieldServer] != "file" {
		t.Errorf("server source = %q, want file", got.Sources[fieldServer])
	}
	if got.ActiveContext != DefaultContextName {
		t.Errorf("ActiveContext = %q, want default", got.ActiveContext)
	}
}

func writeMultiContextFile(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, ConfigFilePath(dir), `current_context: alpha
contexts:
  - name: alpha
    server: https://alpha.example.com
    flavor: datacenter
    auth: {scheme: pat}
  - name: beta
    server: https://beta.example.com
    flavor: cloud
    auth: {scheme: basic, username: bob}
defaults:
  format: json
`)
}

// TestResolveConfigDir covers the "prefer new, fall back to legacy" rule used
// when --config was not supplied. It overrides $HOME with a temp directory so
// the real user's config is never touched.
func TestResolveConfigDir(t *testing.T) {
	withFakeHome := func(t *testing.T) string {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		return home
	}

	t.Run("neither exists → new path", func(t *testing.T) {
		home := withFakeHome(t)
		dir, err := ResolveConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".angelmsger", "bitbucket")
		if dir != want {
			t.Errorf("dir = %q, want %q", dir, want)
		}
		if legacy, err := LegacyConfigDir(); err != nil || legacy != filepath.Join(home, ".bitbucket") {
			t.Errorf("legacy dir = %q, %v", legacy, err)
		}
	})

	t.Run("only legacy has config.yaml → legacy path", func(t *testing.T) {
		home := withFakeHome(t)
		legacy := filepath.Join(home, ".bitbucket")
		if err := os.MkdirAll(legacy, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(legacy, "config.yaml"), "server: https://x")
		dir, err := ResolveConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		if dir != legacy {
			t.Errorf("dir = %q, want %q (legacy fallback)", dir, legacy)
		}
	})

	t.Run("new has config.yaml → new path wins", func(t *testing.T) {
		home := withFakeHome(t)
		newDir := filepath.Join(home, ".angelmsger", "bitbucket")
		if err := os.MkdirAll(newDir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(newDir, "config.yaml"), "server: https://y")
		legacy := filepath.Join(home, ".bitbucket")
		if err := os.MkdirAll(legacy, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(legacy, "config.yaml"), "server: https://legacy")
		dir, err := ResolveConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		if dir != newDir {
			t.Errorf("dir = %q, want %q (new path)", dir, newDir)
		}
	})

	t.Run("legacy dir without config.yaml is ignored", func(t *testing.T) {
		home := withFakeHome(t)
		legacy := filepath.Join(home, ".bitbucket")
		if err := os.MkdirAll(legacy, 0o700); err != nil {
			t.Fatal(err)
		}
		dir, err := ResolveConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".angelmsger", "bitbucket")
		if dir != want {
			t.Errorf("dir = %q, want %q", dir, want)
		}
	})
}
