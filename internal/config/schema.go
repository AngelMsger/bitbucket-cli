package config

import (
	"path/filepath"
	"strconv"

	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/bitbucket-cli/pkg/constants"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// EnvNoContextHint opts out of the runtime multi-context notice.
const EnvNoContextHint = "BITBUCKET_CLI_NO_CONTEXT_HINT"

// fields names each setting's environment variables and built-in default.
// Service fields are the team presets a setup load keeps; the user name, the
// default workspace and secrets are personal.
var fields = []kit.Field{
	{Key: fieldServer, Env: []string{"BITBUCKET_SERVER"}, Service: true},
	{Key: fieldFlavor, Env: []string{"BITBUCKET_FLAVOR"}, Default: FlavorAuto, Service: true},
	// A cached probe result, dropped when the server or flavor changes.
	{Key: fieldDetectedFlavor, ResetBy: []string{fieldServer, fieldFlavor}},
	{Key: fieldCredentialURL, Env: []string{"BITBUCKET_CREDENTIAL_URL"}, Service: true},
	{Key: fieldAuthScheme, Env: []string{"BITBUCKET_AUTH_SCHEME"}, Default: SchemePAT, Service: true},
	{Key: fieldAuthUsername, Env: []string{"BITBUCKET_USERNAME"}},
	{Key: fieldWorkspace, Env: []string{"BITBUCKET_DEFAULT_WORKSPACE"}},
	{Key: fieldFormat, Env: []string{"BITBUCKET_FORMAT"}, Default: constants.DefaultFormat, Service: true},
	{Key: fieldPageSize, Default: strconv.Itoa(constants.DefaultPageSize)},
	{Key: fieldTimeout, Default: constants.DefaultTimeout.String()},
	{Key: fieldMaxRetries, Default: strconv.Itoa(constants.DefaultMaxRetries)},
	{Key: fieldReadOnly, Env: []string{"BITBUCKET_CLI_READ_ONLY"}, Service: true},
	{Key: fieldSkipReviewers, Env: []string{"BITBUCKET_CLI_SKIP_DEFAULT_REVIEWERS"}},
	// A secret in the environment selects its scheme: a token wins over a
	// password, a password over an API token. BITBUCKET_TOKEN wins over its
	// legacy alias.
	{Key: fieldPAT, Env: []string{"BITBUCKET_TOKEN", "BITBUCKET_PERSONAL_ACCESS_TOKEN"}, Implies: SchemePAT},
	{Key: fieldPassword, Env: []string{"BITBUCKET_PASSWORD"}, Implies: SchemeBasic},
	{Key: fieldAPIToken, Env: []string{"BITBUCKET_API_TOKEN"}, Implies: SchemeBasic},
}

func init() {
	kit.Register(&kit.Schema{
		Fields: fields,
		Paths: kit.Paths{
			Dir:    filepath.Join(constants.ConfigParentDirName, constants.ConfigDirName),
			File:   constants.ConfigFileName,
			Legacy: constants.LegacyConfigDirName,
		},
		Words: kit.Words{
			App:          constants.AppName,
			ContextFlag:  "--use-context",
			ContextEnv:   "BITBUCKET_CONTEXT",
			NoHintEnv:    EnvNoContextHint,
			ListContexts: "config get-contexts",
			UseContext:   "config use-context",
			SetContext:   "config set-context",
			AuthGuide:    "auth guide",
			AuthLogin:    "auth login",
		},
		Schemes: []string{SchemePAT, SchemeBasic},
		// Cloud defaults to basic auth (an email and an API token) unless a
		// scheme was chosen.
		AfterMerge: func(v kit.Values, sources map[string]string) {
			if sources[fieldAuthScheme] == "default" && setupFlavor(configFromMap(v)) == FlavorCloud {
				v[fieldAuthScheme] = SchemeBasic
			}
		},
		Validate: func(v kit.Values) error {
			switch v[fieldFlavor] {
			case "", FlavorAuto, FlavorCloud, FlavorDataCenter:
				return nil
			}
			return cerrors.New(cerrors.CategoryConfig, "BAD_FLAVOR", "flavor must be auto, cloud or datacenter")
		},
	})
}
