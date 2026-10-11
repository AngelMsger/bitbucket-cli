package errors

import "testing"

// The model is tested in agent-cli-kit; this checks that bitbucket-cli's own
// guidance is what an error built through this package carries.
func TestErrorsCarryBitbucketGuidance(t *testing.T) {
	e := New(CategoryConfig, "NO_BASE_URL", "no Bitbucket server URL configured")
	if len(e.NextSteps) == 0 || e.NextSteps[0] != "bitbucket-cli config init" {
		t.Fatalf("NextSteps = %v, want bitbucket-cli's config guidance", e.NextSteps)
	}
	if ExitCode(e) != ExitConfig {
		t.Fatalf("ExitCode = %d, want %d", ExitCode(e), ExitConfig)
	}
}
