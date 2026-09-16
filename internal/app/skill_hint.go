package app

import (
	"os"

	"github.com/angelmsger/bitbucket-cli/internal/output"
	"github.com/angelmsger/bitbucket-cli/pkg/constants"
	"github.com/spf13/cobra"
)

const (
	// envSkillLoaded carries the version of the Skill loaded into the agent context.
	envSkillLoaded = "BITBUCKET_CLI_SKILL"
	// envNoSkillHint opts out of the nudge entirely.
	envNoSkillHint = "BITBUCKET_CLI_NO_SKILL_HINT"
)

// maybeSkillHint nudges an agent that is shelling out to this CLI without the
// companion Skill loaded. The Skill carries the canonical usage, safety modes,
// and env setup, so inferring commands without it loses maintained behaviour.
//
// It is deliberately quiet: it writes a single structured _notice to stderr
// (stdout stays clean machine output) and stays silent when
//   - a human is at the terminal (stderr is a TTY),
//   - the loaded Skill version matches the embedded version, or it is opted out,
//   - the command is a setup/meta command (skill / config / auth / completion /
//     help) or a non-runnable command group, where the hint is just noise.
func maybeSkillHint(cmd *cobra.Command) {
	if os.Getenv(envNoSkillHint) != "" {
		return
	}
	load := currentSkillLoadState()
	if load.Status == "current" {
		return
	}
	if stderrIsTTY() {
		return
	}
	if !cmd.Runnable() || skillHintSkip(cmd) {
		return
	}
	message := "The companion Skill 'bitbucket' is the source of truth for driving this CLI " +
		"(usage recipes, safety modes, env setup). If you are an agent, load it before composing commands."
	var nextSteps []string
	if load.Loaded {
		message = "The loaded companion Skill 'bitbucket' does not match this CLI. Refresh it before composing commands."
		nextSteps = []string{
			constants.AppName + " skill install",
			"reload the agent context so it loads the refreshed Skill",
		}
	} else {
		nextSteps = []string{
			constants.AppName + " skill status",
			constants.AppName + " skill install",
			"reload the agent context so it loads the installed Skill",
		}
	}
	output.EmitNotice(os.Stderr, map[string]any{"_notice": map[string]any{
		"skill": map[string]any{
			"name":             "bitbucket",
			"status":           load.Status,
			"loaded_version":   load.Version,
			"embedded_version": embeddedSkillVersion(),
			"message":          message,
			"check":            constants.AppName + " skill status",
			"install":          constants.AppName + " skill install",
			"next_steps":       nextSteps,
			"silence":          "set " + envNoSkillHint + "=1 to suppress",
		},
	}})
}

// skillHintSkip reports whether the command (or any ancestor) is a setup/meta
// command where the discovery nudge would be noise.
func skillHintSkip(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "skill", "config", "auth", "completion", "help", "__complete", "__completeNoDesc":
			return true
		}
	}
	return false
}
