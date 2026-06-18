package cliflags

import (
	"reflect"
	"testing"
)

func testInfo() FlagInfo {
	return FlagInfo{
		Known:   map[string]bool{"user-id": true, "user-name": true, "limit": true, "format": true, "max-items": true, "dry-run": true},
		Numeric: map[string]bool{"limit": true, "max-items": true},
	}
}

func TestKebab(t *testing.T) {
	cases := map[string]string{
		"userId":    "user-id",
		"user_name": "user-name",
		"UserName":  "user-name",
		"user-id":   "user-id",
		"format":    "format",
		"maxItems":  "max-items",
	}
	for in, want := range cases {
		if got := kebab(in); got != want {
			t.Errorf("kebab(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	info := testInfo()
	cases := []struct {
		name     string
		in       []string
		wantOut  []string
		wantKind []string
	}{
		{"flag-name camel", []string{"--userId", "7"}, []string{"--user-id", "7"}, []string{"flag-name"}},
		{"flag-name snake", []string{"--user_name=bob"}, []string{"--user-name=bob"}, []string{"flag-name"}},
		{"flag-name with eq", []string{"--userId=7"}, []string{"--user-id=7"}, []string{"flag-name"}},
		{"sticky int", []string{"--limit100"}, []string{"--limit", "100"}, []string{"sticky-value"}},
		{"sticky camel int", []string{"--maxItems50"}, []string{"--max-items", "50"}, []string{"sticky-value"}},
		{"sticky on non-numeric left alone", []string{"--format2"}, []string{"--format2"}, nil},
		{"already canonical", []string{"--limit", "5"}, []string{"--limit", "5"}, nil},
		{"unknown flag untouched", []string{"--bogus"}, []string{"--bogus"}, nil},
		{"short flag untouched", []string{"-v"}, []string{"-v"}, nil},
		{"after double dash untouched", []string{"--", "--userId"}, []string{"--", "--userId"}, nil},
		{"completion bypass", []string{"__complete", "--userId"}, []string{"__complete", "--userId"}, nil},
		{"positional untouched", []string{"page", "get", "123"}, []string{"page", "get", "123"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, corr := Normalize(tc.in, info)
			if !reflect.DeepEqual(out, tc.wantOut) {
				t.Errorf("out = %v; want %v", out, tc.wantOut)
			}
			var kinds []string
			for _, c := range corr {
				kinds = append(kinds, c.Kind)
			}
			if !reflect.DeepEqual(kinds, tc.wantKind) {
				t.Errorf("correction kinds = %v; want %v", kinds, tc.wantKind)
			}
		})
	}
}

// TestNormalizeNoFalsePositive guards the headline safety property: a token
// that does not normalize to a known flag is never rewritten.
func TestNormalizeNoFalsePositive(t *testing.T) {
	out, corr := Normalize([]string{"--statusRocket"}, testInfo())
	if len(corr) != 0 || !reflect.DeepEqual(out, []string{"--statusRocket"}) {
		t.Errorf("unknown camel flag should pass through: out=%v corr=%v", out, corr)
	}
}

func TestInterpretEscapesValue(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		want        string
		wantChanged bool
	}{
		{"literal newlines", `a\n\nb`, "a\n\nb", true},
		{"tab and cr", `a\tb\rc`, "a\tb\rc", true},
		{"no backslash untouched", "plain text", "plain text", false},
		{"unknown escape preserved", `match \d+ digits`, `match \d+ digits`, false},
		{"escaped backslash collapses", `a\\nb`, `a\nb`, true},
		{"trailing lone backslash kept", `path\`, `path\`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, changed := interpretEscapes(tc.in)
			if got != tc.want || changed != tc.wantChanged {
				t.Errorf("interpretEscapes(%q) = (%q, %v); want (%q, %v)",
					tc.in, got, changed, tc.want, tc.wantChanged)
			}
		})
	}
}

func TestInterpretEscapes(t *testing.T) {
	cases := []struct {
		name     string
		in       []string
		wantOut  []string
		wantCorr int
		wantFlag string
	}{
		{"space form content", []string{"--content", `a\n\nb`}, []string{"--content", "a\n\nb"}, 1, "--content"},
		{"eq form description", []string{`--description=x\ny`}, []string{"--description=x\ny"}, 1, "--description"},
		{"message decoded", []string{"--message", `line1\nline2`}, []string{"--message", "line1\nline2"}, 1, "--message"},
		{"content-file is not a body flag", []string{"--content-file", `a\nb`}, []string{"--content-file", `a\nb`}, 0, ""},
		{"non-body flag untouched", []string{"--limit", `5\n`}, []string{"--limit", `5\n`}, 0, ""},
		{"body flag without escapes untouched", []string{"--content", "plain"}, []string{"--content", "plain"}, 0, ""},
		{"after double dash untouched", []string{"--", "--content", `a\nb`}, []string{"--", "--content", `a\nb`}, 0, ""},
		{"completion bypass", []string{"__complete", "--content", `a\nb`}, []string{"__complete", "--content", `a\nb`}, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, corr := InterpretEscapes(tc.in, BodyFlags)
			if !reflect.DeepEqual(out, tc.wantOut) {
				t.Errorf("out = %q; want %q", out, tc.wantOut)
			}
			if len(corr) != tc.wantCorr {
				t.Fatalf("corrections = %d; want %d (%v)", len(corr), tc.wantCorr, corr)
			}
			if tc.wantCorr > 0 {
				if corr[0].Kind != "escape" {
					t.Errorf("kind = %q; want escape", corr[0].Kind)
				}
				if corr[0].Flag != tc.wantFlag {
					t.Errorf("flag = %q; want %q", corr[0].Flag, tc.wantFlag)
				}
				if corr[0].Detail == "" {
					t.Errorf("detail should be non-empty")
				}
			}
		})
	}
}
