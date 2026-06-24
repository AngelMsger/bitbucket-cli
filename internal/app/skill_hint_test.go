package app

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestSkillHintSkip checks which commands suppress the discovery nudge: setup /
// meta commands and their children, but not real working commands like pr merge.
func TestSkillHintSkip(t *testing.T) {
	root := newRootCmd()
	find := func(path ...string) *cobra.Command {
		t.Helper()
		cur := root
		for _, name := range path {
			var match *cobra.Command
			for _, c := range cur.Commands() {
				if c.Name() == name {
					match = c
					break
				}
			}
			if match == nil {
				t.Fatalf("command %q not found under %q", name, cur.Name())
			}
			cur = match
		}
		return cur
	}

	for _, tc := range []struct {
		path []string
		skip bool
	}{
		{[]string{"skill", "status"}, true},
		{[]string{"skill", "install"}, true},
		{[]string{"config", "init"}, true},
		{[]string{"pr", "merge"}, false},
		{[]string{"pr", "create"}, false},
		{[]string{"repo", "get"}, false},
	} {
		if got := skillHintSkip(find(tc.path...)); got != tc.skip {
			t.Errorf("skillHintSkip(%v) = %v; want %v", tc.path, got, tc.skip)
		}
	}
}
