package config

import (
	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/agent-cli-kit/reuse"
	"github.com/angelmsger/bitbucket-cli/pkg/constants"
)

// ReuseContexts describes every stored context to agent-cli-kit/reuse with
// only its persisted fields and built-in defaults, so no environment identity
// leaks into a source.
func ReuseContexts(file File) []reuse.Context {
	out := make([]reuse.Context, 0, len(file.Contexts))
	for _, c := range file.Contexts {
		cfg := StoredContext(c, file.Defaults)
		out = append(out, reuse.Context{Name: c.Name, Service: ReuseService(cfg), KeyBase: c.BaseURL,
			Scheme: cfg.Auth.Scheme, Username: c.Auth.Username})
	}
	return out
}

// ReuseService identifies a service for reuse: the complete normalized URL and
// the deployment flavor it is known to have. An unusable URL identifies nothing.
func ReuseService(cfg Config) string {
	base, err := NormalizeServiceURL(cfg.BaseURL)
	if err != nil || base == "" {
		return ""
	}
	return base + "\x00" + ReuseFlavor(cfg)
}

// ReuseFlavor is the flavor that scopes a service: the configured one, or the
// detected one while the configured flavor is auto. A context that was never
// detected stays auto and matches only another undetected auto context.
func ReuseFlavor(cfg Config) string {
	if cfg.Flavor != "" && cfg.Flavor != FlavorAuto {
		return cfg.Flavor
	}
	if cfg.DetectedFlavor == FlavorCloud || cfg.DetectedFlavor == FlavorDataCenter {
		return cfg.DetectedFlavor
	}
	return FlavorAuto
}

// SchemeSupplied reports whether the invocation supplied the scheme by flag,
// environment or .env, including one an environment credential implies.
func SchemeSupplied(sources map[string]string) bool {
	switch sources[fieldAuthScheme] {
	case "flag", "env", "dotenv":
		return true
	}
	return false
}

// reuseStep is the `auth reuse` step that goes before any sign-in step for the
// named context, or "" when an offline look at the file finds nothing reuse
// could adopt. A runtime username, or a service override that differs from the
// stored context, means reuse would change nothing or be refused.
func reuseStep(cfg Config, sources map[string]string, file File, name string) string {
	stored, ok := file.Context(name)
	if !ok {
		return ""
	}
	storedCfg := StoredContext(stored, file.Defaults)
	if ReuseService(storedCfg) != ReuseService(cfg) || storedCfg.Auth.Scheme != cfg.Auth.Scheme {
		return ""
	}
	target := reuse.Context{Name: stored.Name, Service: ReuseService(storedCfg), KeyBase: stored.BaseURL,
		Scheme: storedCfg.Auth.Scheme, Username: stored.Auth.Username}
	runtimeUser := ""
	switch sources[fieldAuthUsername] {
	case "flag", "env", "dotenv":
		runtimeUser = cfg.Auth.Username
	}
	if !reuse.MayReuse(target, ReuseContexts(file), runtimeUser, SchemeSupplied(sources)) {
		return ""
	}
	step := constants.AppName + " --use-context " + kit.QuoteArg(stored.Name) + " auth reuse"
	if sources[fieldAuthScheme] == "flag" {
		step += " --auth-scheme " + cfg.Auth.Scheme
	}
	return step
}
