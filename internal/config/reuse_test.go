package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/angelmsger/bitbucket-cli/internal/auth" // registers the auth schemes
	"github.com/angelmsger/bitbucket-cli/internal/config"
)

func TestLoadOffersReuseBeforeSignIn(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"BITBUCKET_SERVER", "BITBUCKET_FLAVOR", "BITBUCKET_USERNAME", "BITBUCKET_PASSWORD", "BITBUCKET_API_TOKEN", "BITBUCKET_TOKEN", "BITBUCKET_PERSONAL_ACCESS_TOKEN", "BITBUCKET_CONTEXT", "BITBUCKET_AUTH_SCHEME"} {
		t.Setenv(k, "")
	}
	file := config.File{CurrentContext: "team", Contexts: []config.NamedContext{
		{Name: "personal", BaseURL: "https://bitbucket.example.test", Flavor: "datacenter", Auth: config.AuthConfig{Scheme: "basic", Username: "me"}},
		{Name: "team", BaseURL: "https://bitbucket.example.test", Flavor: "datacenter", Auth: config.AuthConfig{Scheme: "pat"}},
	}}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	load := func(opt config.LoadOptions) *config.Resolved {
		t.Helper()
		opt.ConfigDir, opt.DotenvPath = dir, filepath.Join(t.TempDir(), "absent.env")
		r, err := config.Load(opt)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load(config.LoadOptions{})
	if r.Config.ReuseStep != "bitbucket-cli --use-context 'team' auth reuse" {
		t.Fatalf("reuse step %q", r.Config.ReuseStep)
	}
	g, err := config.Guide(r.Config, r.Sources)
	if err != nil || g.NextSteps[0] != r.Config.ReuseStep || !strings.Contains(g.Instructions[0], "auth reuse") {
		t.Fatalf("guide %+v %v", g, err)
	}
	if r = load(config.LoadOptions{Context: "personal"}); r.Config.ReuseStep != "" {
		t.Fatalf("a context with a basic identity was offered reuse: %q", r.Config.ReuseStep)
	}
	if r = load(config.LoadOptions{Flags: config.FlagValues{AuthScheme: "pat"}}); r.Config.ReuseStep != "" {
		t.Fatalf("a supplied scheme without same-scheme sources was offered reuse: %q", r.Config.ReuseStep)
	}
	if r = load(config.LoadOptions{Flags: config.FlagValues{BaseURL: "https://other.example.test"}}); r.Config.ReuseStep != "" {
		t.Fatalf("an override was offered reuse: %q", r.Config.ReuseStep)
	}
	if r = load(config.LoadOptions{Flags: config.FlagValues{Flavor: "cloud"}}); r.Config.ReuseStep != "" {
		t.Fatalf("a flavor override was offered reuse: %q", r.Config.ReuseStep)
	}
}

// The setup plan offers reuse for the scheme the preset names, since that
// scheme is the context's own afterwards rather than a limit on reuse.
func TestPlanServiceContextLeadsWithReuse(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"BITBUCKET_SERVER", "BITBUCKET_FLAVOR", "BITBUCKET_USERNAME", "BITBUCKET_CONTEXT", "BITBUCKET_AUTH_SCHEME"} {
		t.Setenv(k, "")
	}
	r, err := config.Load(config.LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env"), Setup: true, Context: "team",
		Flags: config.FlagValues{BaseURL: "https://bitbucket.example.test", AuthScheme: "pat", Flavor: "datacenter"}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := config.PlanServiceContext(config.File{}, "team", r, false, false)
	if err != nil || len(plan.NextSteps) == 0 || plan.NextSteps[0] != "bitbucket-cli --use-context 'team' auth reuse" {
		t.Fatalf("plan %+v %v", plan, err)
	}
}

// The service a context reuses within is its URL plus the flavor it is known
// to have: a detected flavor stands for auto.
func TestReuseServiceScopesByKnownFlavor(t *testing.T) {
	base := config.Config{BaseURL: "https://bitbucket.example.test/"}
	detected := base
	detected.Flavor, detected.DetectedFlavor = "auto", "datacenter"
	explicit := base
	explicit.Flavor = "datacenter"
	undetected := base
	undetected.Flavor = "auto"
	if config.ReuseService(detected) != config.ReuseService(explicit) {
		t.Fatal("a detected flavor does not match the same explicit flavor")
	}
	if config.ReuseService(undetected) == config.ReuseService(explicit) || config.ReuseService(undetected) != config.ReuseService(base) {
		t.Fatal("an undetected auto flavor matched a known one, or an empty flavor differs from auto")
	}
	if config.ReuseService(config.Config{BaseURL: "https://user:pw@bitbucket.example.test"}) != "" {
		t.Fatal("a credential-bearing URL identified a service")
	}
}
