package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/bitbucket-cli/internal/config"
	"github.com/angelmsger/bitbucket-cli/pkg/constants"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

func TestDoctorCredentialRecoveryStatus(t *testing.T) {
	t.Parallel()
	err := cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_STORE_INACCESSIBLE", "hidden").
		WithRecovery(cerrors.Recovery{Action: "retry_current_command", Scope: "host"})
	if got := diagnosticStatus(err); got != "inaccessible" {
		t.Fatalf("diagnosticStatus() = %q, want inaccessible", got)
	}
	recoveryScope := diagnosticRecoveryScope(err)
	if recoveryScope != "host" {
		t.Fatalf("diagnosticRecoveryScope() = %q, want host", recoveryScope)
	}
	report := doctorReport{Checks: []doctorCheck{{RecoveryScope: recoveryScope}}}
	if !reportNeedsHostRetry(report) {
		t.Fatal("reportNeedsHostRetry() = false, want true")
	}
}

func TestCompanionSkillDoctorCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(envSkillLoaded, "")
	dir := filepath.Join(home, ".codex", "skills", "bitbucket")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	version := strings.TrimPrefix(embeddedSkillVersion(), "v")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nversion: "+version+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := companionSkillDoctorCheck(); got.Status != "installed_not_loaded" || !got.OK {
		t.Fatalf("installed check = %+v", got)
	}
	t.Setenv(envSkillLoaded, "0.0.0")
	if got := companionSkillDoctorCheck(); got.Status != "reload_required" {
		t.Fatalf("reload check = %+v", got)
	}
	t.Setenv(envSkillLoaded, version)
	if got := companionSkillDoctorCheck(); got.Status != "current" || !got.OK {
		t.Fatalf("loaded check = %+v", got)
	}
}

// keychainAnswer is a keychain backend whose reads all return err.
type keychainAnswer struct{ err error }

func (k keychainAnswer) Get(string, string) (string, error) { return "", k.err }
func (k keychainAnswer) Set(string, string, string) error   { return k.err }
func (k keychainAnswer) Delete(string, string) error        { return k.err }

// doctor lists reuse when a stored login could replace a missing one, but not
// when the store itself cannot be read: that is recovered on the host.
func TestDoctorOffersReuseForAMissingLogin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	step := "bitbucket-cli --use-context 'team' auth reuse"
	cfg := config.Config{BaseURL: "https://bitbucket.example.test", Flavor: "datacenter", Auth: config.AuthConfig{Scheme: "pat"}, ReuseStep: step}
	for answer, want := range map[error][]string{keyring.ErrNotFound: {step}, errors.New("keychain locked"): nil} {
		s := &appState{resolved: &config.Resolved{Config: cfg}, store: credstore.NewStoreWithBackend(t.TempDir(), constants.KeychainService, keychainAnswer{answer})}
		if report := runDoctor(s, true); report.Healthy || !reflect.DeepEqual(report.NextSteps, want) {
			t.Fatalf("%v: next steps %v, want %v", answer, report.NextSteps, want)
		}
	}
}
