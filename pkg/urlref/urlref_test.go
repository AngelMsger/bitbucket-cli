package urlref

import "testing"

func TestParseBaseURLAndContextPaths(t *testing.T) {
	tests := []struct {
		raw      string
		wantBase string
		wantWS   string
		wantSlug string
		wantPR   int
		wantURL  bool
	}{
		{
			raw:      "https://bitbucket.org/team/demo/pull-requests/42",
			wantBase: "https://bitbucket.org",
			wantWS:   "team",
			wantSlug: "demo",
			wantPR:   42,
			wantURL:  true,
		},
		{
			raw:      "https://bb.corp.internal:8443/projects/PROJ/repos/demo/pull-requests/7",
			wantBase: "https://bb.corp.internal:8443",
			wantWS:   "PROJ",
			wantSlug: "demo",
			wantPR:   7,
			wantURL:  true,
		},
		{
			raw:      "https://bb.corp.internal:8443/dc1/projects/PROJ/repos/demo/pull-requests/9",
			wantBase: "https://bb.corp.internal:8443/dc1",
			wantWS:   "PROJ",
			wantSlug: "demo",
			wantPR:   9,
			wantURL:  true,
		},
		{
			raw:      "https://bb.corp.internal/bitbucket/users/alice/repos/demo/pull-requests/3",
			wantBase: "https://bb.corp.internal/bitbucket",
			wantWS:   "~alice",
			wantSlug: "demo",
			wantPR:   3,
			wantURL:  true,
		},
		{
			raw:      "team/demo/12",
			wantBase: "",
			wantWS:   "team",
			wantSlug: "demo",
			wantPR:   12,
			wantURL:  false,
		},
	}

	for _, tc := range tests {
		got := Parse(tc.raw)
		if got.IsURL != tc.wantURL || got.BaseURL != tc.wantBase ||
			got.Workspace != tc.wantWS || got.Slug != tc.wantSlug || got.PRID != tc.wantPR {
			t.Errorf("Parse(%q) = %+v, want url=%v base=%q ws=%q slug=%q pr=%d",
				tc.raw, got, tc.wantURL, tc.wantBase, tc.wantWS, tc.wantSlug, tc.wantPR)
		}
	}
}
