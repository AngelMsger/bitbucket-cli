// Package constants holds project-wide constants and build-time metadata.
package constants

import "time"

// Build-time metadata, injected via -ldflags. See Makefile.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

const (
	// AppName is the binary / command name.
	AppName = "bitbucket-cli"

	// EnvPrefix is the environment variable prefix for all settings.
	EnvPrefix = "BITBUCKET_"

	// ConfigDirName is the per-user config directory under $HOME.
	ConfigDirName = ".bitbucket"

	// ConfigFileName is the YAML config file within ConfigDirName.
	ConfigFileName = "config.yaml"

	// CredentialsFileName is the fallback secret store when no keychain is available.
	CredentialsFileName = "credentials"

	// KeychainService is the service name used for OS keychain entries.
	KeychainService = "bitbucket-cli"
)

// Defaults for runtime behaviour.
const (
	DefaultFormat     = "json"
	DefaultPageSize   = 25
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 3
	// MaxPageSize caps a single API page request.
	MaxPageSize = 250
)

// UserAgent identifies the CLI to the Bitbucket server.
func UserAgent() string {
	return AppName + "/" + Version
}
