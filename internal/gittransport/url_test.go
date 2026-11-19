package gittransport

import "testing"

func TestParseRemoteRepo(t *testing.T) {
	tests := []struct {
		raw       string
		wantHost  string
		wantPort  string
		wantCtx   string
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
			wantPort:  "7999",
			wantWS:    "PROJ",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "ssh://git@bb.corp.internal:7999/scm/proj/demo.git",
			wantHost:  "bb.corp.internal",
			wantPort:  "7999",
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
			raw:       "https://bb.corp.internal:8443/bitbucket/scm/PROJ/demo.git",
			wantHost:  "bb.corp.internal",
			wantPort:  "8443",
			wantCtx:   "bitbucket",
			wantWS:    "PROJ",
			wantSlug:  "demo",
			wantMatch: true,
		},
		{
			raw:       "https://bb.corp.internal/dc1/projects/PROJ/repos/demo/browse",
			wantHost:  "bb.corp.internal",
			wantCtx:   "dc1",
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
			wantPort:  "41234",
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
		if got.Host != tc.wantHost || got.Port != tc.wantPort || got.ContextPath != tc.wantCtx ||
			got.Workspace != tc.wantWS || got.Slug != tc.wantSlug {
			t.Errorf("ParseRemoteRepo(%q) = %+v, want host=%q port=%q ctx=%q ws=%q slug=%q",
				tc.raw, got, tc.wantHost, tc.wantPort, tc.wantCtx, tc.wantWS, tc.wantSlug)
		}
	}
}

func TestRemoteRepoMatchesDeploymentAndRepo(t *testing.T) {
	cloud, ok := ParseRemoteRepo("git@bitbucket.org:team/demo.git")
	if !ok {
		t.Fatal("parse failed")
	}
	if !cloud.MatchesDeployment("https://api.bitbucket.org", true) {
		t.Error("bitbucket.org should match Cloud api.bitbucket.org")
	}
	if !cloud.MatchesDeployment("https://api.bitbucket.org/2.0", true) {
		t.Error("bitbucket.org should match Cloud api.bitbucket.org/2.0")
	}
	if !cloud.MatchesRepo("TEAM", "demo") {
		t.Error("expected case-insensitive repo match")
	}

	cloudHTTPS, _ := ParseRemoteRepo("https://bitbucket.org/team/demo.git")
	if !cloudHTTPS.MatchesDeployment("https://api.bitbucket.org", true) {
		t.Error("Cloud HTTPS remote should match api.bitbucket.org")
	}
	if cloudHTTPS.MatchesDeployment("https://api.bitbucket.org:8443", true) {
		t.Error("mismatched Cloud HTTPS port must not match")
	}

	gh, _ := ParseRemoteRepo("git@github.com:team/demo.git")
	if gh.MatchesDeployment("https://api.bitbucket.org", true) {
		t.Error("github.com must not match bitbucket.org")
	}

	// Unverified SSH aliases must be rejected (use --force-remote explicitly).
	alias, _ := ParseRemoteRepo("git@bb-corp:PROJ/demo.git")
	if alias.MatchesDeployment("https://bitbucket.corp.internal", false) {
		t.Error("unverified SSH alias must not match another hostname")
	}

	// Data Center port and context-path isolation.
	dcCtx, _ := ParseRemoteRepo("https://bb.corp.internal:8443/dc1/scm/PROJ/demo.git")
	if !dcCtx.MatchesDeployment("https://bb.corp.internal:8443/dc1", false) {
		t.Error("matching DC host, port, and context path should match")
	}
	if dcCtx.MatchesDeployment("https://bb.corp.internal:9443/dc1", false) {
		t.Error("different DC HTTPS port must not match")
	}
	if dcCtx.MatchesDeployment("https://bb.corp.internal:8443/dc2", false) {
		t.Error("different DC context path must not match")
	}
	if dcCtx.MatchesDeployment("https://bb.corp.internal:8443", false) {
		t.Error("context-path remote must not match root DC base URL")
	}

	// Explicit default port :443 matches implicit https.
	dcDefault443, _ := ParseRemoteRepo("https://bb.corp.internal:443/scm/PROJ/demo.git")
	if !dcDefault443.MatchesDeployment("https://bb.corp.internal", false) {
		t.Error("explicit :443 should match default https port")
	}

	// DC SSH remote uses port 7999 and omits HTTP context path.
	dcSSH, _ := ParseRemoteRepo("ssh://git@bb.corp.internal:7999/PROJ/demo.git")
	if !dcSSH.MatchesDeployment("https://bb.corp.internal:8443/dc1", false) {
		t.Error("DC SSH remote on matching host should match deployment despite HTTPS port/context")
	}
}

func TestSanitizeRemoteURL(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{
			raw:  "https://alice:s3cr3t@bb.corp.internal/scm/PROJ/demo.git?token=abc#frag",
			want: "https://bb.corp.internal/scm/PROJ/demo.git",
		},
		{
			raw:  "https://x-token-auth@bitbucket.org/team/demo.git",
			want: "https://bitbucket.org/team/demo.git",
		},
		{
			raw:  "ssh://git:secret@bb.corp.internal:7999/PROJ/demo.git?k=v",
			want: "ssh://bb.corp.internal:7999/PROJ/demo.git",
		},
		{
			raw:  "git@bitbucket.org:team/demo.git",
			want: "git@bitbucket.org:team/demo.git",
		},
		{
			raw:  "alice:secret@bitbucket.org:team/demo.git?tok=1",
			want: "bitbucket.org:team/demo.git",
		},
		{
			raw:  "https://user:secret@[bad-url",
			want: "<redacted-remote-url>",
		},
		{
			raw:  "/tmp/local-repo.git",
			want: "/tmp/local-repo.git",
		},
	}
	for _, tc := range tests {
		if got := SanitizeRemoteURL(tc.raw); got != tc.want {
			t.Errorf("SanitizeRemoteURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
