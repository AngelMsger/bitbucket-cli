package gittransport

import "testing"

func TestParseRemoteRepo(t *testing.T) {
	tests := []struct {
		raw       string
		wantHost  string
		wantWS    string
		wantSlug  string
		wantMatch bool
	}{
		{
			raw:       "git@bitbucket.org:team/demo.git",
			wantHost:  "bitbucket.org",
			wantWS:    "team",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "ssh://git@bb.corp.internal:7999/PROJ/demo.git",
			wantHost:  "bb.corp.internal",
			wantWS:    "PROJ",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "ssh://git@bb.corp.internal:7999/scm/proj/demo.git",
			wantHost:  "bb.corp.internal",
			wantWS:    "proj",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "https://bb.corp.internal/scm/PROJ/demo.git",
			wantHost:  "bb.corp.internal",
			wantWS:    "PROJ",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "https://bb.corp.internal/bitbucket/scm/PROJ/demo.git",
			wantHost:  "bb.corp.internal",
			wantWS:    "PROJ",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "https://bb.corp.internal/projects/PROJ/repos/demo/browse",
			wantHost:  "bb.corp.internal",
			wantWS:    "PROJ",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "https://bb.corp.internal/users/alice/repos/demo",
			wantHost:  "bb.corp.internal",
			wantWS:    "~alice",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "git@bb.corp.internal:~alice/demo.git",
			wantHost:  "bb.corp.internal",
			wantWS:    "~alice",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "http://127.0.0.1:41234/scm/PROJ/demo.git",
			wantHost:  "127.0.0.1",
			wantWS:    "PROJ",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "/local/path/repo",
			wantMatch: false,
		},
	}

	for _, tc := range tests {
		got, ok := ParseRemoteRepo(tc.raw)
		if ok != tc.wantMatch {
			t.Errorf("ParseRemoteRepo(%q) ok=%v, want %v (got=%+v)", tc.raw, ok, tc.wantMatch, got)
			continue
		}
		if !ok {
			continue
		}
		if got.Host != tc.wantHost || got.Workspace != tc.wantWS || got.Slug != tc.wantSlug {
			t.Errorf("ParseRemoteRepo(%q) = %+v, want host=%q ws=%q slug=%q",
				tc.raw, got, tc.wantHost, tc.wantWS, tc.wantSlug)
		}
	}
}

func TestRemoteRepoMatchesHostAndRepo(t *testing.T) {
	cloud, ok := ParseRemoteRepo("git@bitbucket.org:team/demo.git")
	if !ok {
		t.Fatal("parse failed")
	}
	if !cloud.MatchesHost("https://api.bitbucket.org", true) {
		t.Error("bitbucket.org should match Cloud api.bitbucket.org")
	}
	if !cloud.MatchesRepo("TEAM", "demo") {
		t.Error("expected case-insensitive repo match")
	}

	gh, _ := ParseRemoteRepo("git@github.com:team/demo.git")
	if gh.MatchesHost("https://api.bitbucket.org", true) {
		t.Error("github.com must not match bitbucket.org")
	}

	alias, _ := ParseRemoteRepo("git@bb-corp:PROJ/demo.git")
	if !alias.MatchesHost("https://bitbucket.corp.internal", false) {
		t.Error("short SSH alias without dots should be allowed")
	}
}
