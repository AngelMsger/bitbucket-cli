package app

import "testing"

func TestPickRemotePrefersUpstream(t *testing.T) {
	// In a fork workflow both remotes exist; upstream is the canonical repo and
	// is usually the more accurate base, so it wins over origin.
	if got := pickRemote("", []string{"origin", "upstream"}); got != "upstream" {
		t.Errorf("pickRemote = %q, want upstream", got)
	}
}

func TestPickRemoteExplicitWins(t *testing.T) {
	if got := pickRemote("origin", []string{"origin", "upstream"}); got != "origin" {
		t.Errorf("explicit --remote origin should win, got %q", got)
	}
	if got := pickRemote("fork", []string{"origin"}); got != "fork" {
		t.Errorf("explicit --remote should be used verbatim, got %q", got)
	}
}

func TestPickRemoteFallbacks(t *testing.T) {
	if got := pickRemote("", []string{"origin"}); got != "origin" {
		t.Errorf("only origin present -> origin, got %q", got)
	}
	if got := pickRemote("", []string{"whatever"}); got != "whatever" {
		t.Errorf("no origin/upstream -> first remote, got %q", got)
	}
	if got := pickRemote("", nil); got != "origin" {
		t.Errorf("no remotes detected -> origin default, got %q", got)
	}
	if got := pickRemote("auto", []string{"origin", "upstream"}); got != "upstream" {
		t.Errorf("--remote auto should resolve like empty, got %q", got)
	}
}

func TestPRFetchCommandsWithBase(t *testing.T) {
	cmds := prFetchCommands("upstream", 42, "main")
	if len(cmds) != 2 {
		t.Fatalf("want 2 commands (source + base), got %d: %+v", len(cmds), cmds)
	}
	wantSource := "git fetch upstream refs/pull-requests/42/from:refs/remotes/upstream/pr/42"
	if cmds[0].Trace != wantSource {
		t.Errorf("source fetch = %q, want %q", cmds[0].Trace, wantSource)
	}
	if cmds[1].Trace != "git fetch upstream main" {
		t.Errorf("base fetch = %q, want 'git fetch upstream main'", cmds[1].Trace)
	}
}

func TestPRFetchCommandsWithoutBase(t *testing.T) {
	cmds := prFetchCommands("origin", 7, "")
	if len(cmds) != 1 {
		t.Fatalf("want 1 command when base unknown, got %d", len(cmds))
	}
	if cmds[0].Trace != "git fetch origin refs/pull-requests/7/from:refs/remotes/origin/pr/7" {
		t.Errorf("unexpected source fetch: %q", cmds[0].Trace)
	}
}

func TestReviewDiffCommand(t *testing.T) {
	// Triple-dot diffs against the merge-base, so this shows exactly the PR's
	// changes regardless of how far the base branch has advanced.
	if got := reviewDiffCommand("upstream", 42, "main"); got != "git diff upstream/main...upstream/pr/42" {
		t.Errorf("reviewDiffCommand = %q", got)
	}
	if got := reviewDiffCommand("origin", 42, ""); got != "" {
		t.Errorf("no base -> empty diff command, got %q", got)
	}
}
