package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentIDs(t *testing.T) {
	t.Parallel()
	want := []string{
		"claude-code", "codex", "cursor", "agents", "gemini", "github-copilot",
		"opencode", "continue", "windsurf", "grok", "pi", "kilo", "roo",
	}
	ids := agentIDs()
	if len(ids) != len(want) {
		t.Fatalf("agentIDs() = %v (%d), want %d entries", ids, len(ids), len(want))
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("missing agent id %q", id)
		}
	}
}

func TestAgentDests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id              string
		wantHomeSuffix  string
		wantProjectPath string
	}{
		{"claude-code", filepath.Join(".claude", "skills", "bitbucket"), filepath.Join(".claude", "skills", "bitbucket")},
		{"codex", filepath.Join(".codex", "skills", "bitbucket"), filepath.Join(".agents", "skills", "bitbucket")},
		{"cursor", filepath.Join(".cursor", "skills", "bitbucket"), filepath.Join(".cursor", "skills", "bitbucket")},
		{"agents", filepath.Join(".agents", "skills", "bitbucket"), filepath.Join(".agents", "skills", "bitbucket")},
		{"gemini", filepath.Join(".gemini", "skills", "bitbucket"), filepath.Join(".gemini", "skills", "bitbucket")},
		{"github-copilot", filepath.Join(".copilot", "skills", "bitbucket"), filepath.Join(".agents", "skills", "bitbucket")},
		{"opencode", filepath.Join(".config", "opencode", "skills", "bitbucket"), filepath.Join(".opencode", "skills", "bitbucket")},
		{"continue", filepath.Join(".continue", "skills", "bitbucket"), filepath.Join(".continue", "skills", "bitbucket")},
		{"windsurf", filepath.Join(".codeium", "windsurf", "skills", "bitbucket"), filepath.Join(".windsurf", "skills", "bitbucket")},
		{"grok", filepath.Join(".grok", "skills", "bitbucket"), filepath.Join(".grok", "skills", "bitbucket")},
		{"pi", filepath.Join(".pi", "agent", "skills", "bitbucket"), filepath.Join(".pi", "skills", "bitbucket")},
		{"kilo", filepath.Join(".kilocode", "skills", "bitbucket"), filepath.Join(".kilocode", "skills", "bitbucket")},
		{"roo", filepath.Join(".roo", "skills", "bitbucket"), filepath.Join(".roo", "skills", "bitbucket")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			spec, ok := agentByID(tc.id)
			if !ok {
				t.Fatalf("agentSpec %q missing", tc.id)
			}
			projectPath, err := agentDest(spec, true)
			if err != nil {
				t.Fatal(err)
			}
			if projectPath != tc.wantProjectPath {
				t.Fatalf("project dest = %q, want %q", projectPath, tc.wantProjectPath)
			}
			homePath, err := agentDest(spec, false)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(homePath, tc.wantHomeSuffix) {
				t.Fatalf("home dest %q does not end with %q", homePath, tc.wantHomeSuffix)
			}
		})
	}
}
