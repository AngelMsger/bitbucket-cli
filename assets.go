// Package bitbucketcli is the module root. It exists only to embed packaged
// assets — the companion `bitbucket` Skill — into the CLI binary, so that
// `bitbucket-cli skill install` can deploy a version-matched copy regardless
// of how the binary itself was installed (npm, go install, prebuilt, source).
package bitbucketcli

import "embed"

// SkillFS holds the companion Skill, rooted at "skills/bitbucket".
//
//go:embed all:skills/bitbucket
var SkillFS embed.FS

// SkillRoot is the path within SkillFS at which the Skill is rooted.
const SkillRoot = "skills/bitbucket"
