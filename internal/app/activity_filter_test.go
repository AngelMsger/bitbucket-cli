package app

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

func TestResolveActivityWindow(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	window, err := resolveActivityWindow("24h", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.from; !got.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("from = %s; want %s", got, now.Add(-24*time.Hour))
	}
	if !window.to.Equal(now) {
		t.Fatalf("to = %s; want %s", window.to, now)
	}

	window, err = resolveActivityWindow("", "2026-09-03", "2026-09-04", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.to.Sub(window.from); got != 24*time.Hour {
		t.Fatalf("date window = %s; want 24h", got)
	}
}

func TestResolveActivityWindowRejectsAmbiguousFlags(t *testing.T) {
	if _, err := resolveActivityWindow("24h", "2026-09-03", "", time.Now()); err == nil {
		t.Fatal("expected --since plus --from to fail")
	}
	if _, err := resolveActivityWindow("", "", "2026-09-04", time.Now()); err == nil {
		t.Fatal("expected --to without --from to fail")
	}
}

func TestFilterActivitiesAcrossFlavorTimestamps(t *testing.T) {
	window, err := resolveActivityWindow("", "2026-09-03T00:00:00Z", "2026-09-04T00:00:00Z", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	actor := &apiclient.User{Slug: "alice"}
	dcWhen := strconv.FormatInt(time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC).UnixMilli(), 10)
	items := []apiclient.Activity{
		{Kind: "approval", Actor: apiclient.User{Name: "ALICE"}, When: "2026-09-03T10:00:00Z"},
		{Kind: "comment", Actor: apiclient.User{Slug: "alice"}, When: dcWhen},
		{Kind: "decline", Actor: apiclient.User{Slug: "bob"}, When: "2026-09-03T11:00:00Z"},
		{Kind: "approval", Actor: apiclient.User{Slug: "alice"}, When: "2026-09-04T00:00:00Z"},
	}
	kinds, err := normalizeActivityKinds([]string{"approve,comment"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := filterActivities(items, window, actor, kinds)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("matched %d activities; want 2: %+v", len(got), got)
	}
}

func TestNormalizeActivityKindsRejectsUnknown(t *testing.T) {
	if _, err := normalizeActivityKinds([]string{"reviewed"}); err == nil {
		t.Fatal("expected an unknown activity kind to fail")
	}
}

func TestDeclineFilterMatchesCloudStateUpdate(t *testing.T) {
	kinds, err := normalizeActivityKinds([]string{"decline"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := filterActivities([]apiclient.Activity{
		{Kind: "update", State: "DECLINED"},
		{Kind: "update", State: "OPEN"},
	}, activityWindow{}, nil, kinds)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].State != "DECLINED" {
		t.Fatalf("decline filter returned %+v", got)
	}
}

func TestParseClosedSince(t *testing.T) {
	got, err := parseClosedSince("2d")
	if err != nil {
		t.Fatal(err)
	}
	if got != 172800 {
		t.Fatalf("seconds = %d; want 172800", got)
	}
	if _, err := parseClosedSince("0h"); err == nil {
		t.Fatal("expected a zero duration to fail")
	}
}

func TestActivityBatchRequiresTimeRange(t *testing.T) {
	err := runRoot(t, "pr", "activity", "PROJ/repo/1", "PROJ/repo/2")
	if err == nil {
		t.Fatal("expected an unbounded batch query to fail")
	}
	if got := cerrors.AsCLIError(err).Code; got != "ACTIVITY_TIME_REQUIRED" {
		t.Fatalf("error code = %q; want ACTIVITY_TIME_REQUIRED", got)
	}
}

func TestActivityStdinRequiresTimeRange(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--config", t.TempDir(), "pr", "activity", "-"})
	cmd.SetIn(strings.NewReader("PROJ/repo/1\n"))
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an unbounded stdin query to fail")
	}
	if got := cerrors.AsCLIError(err).Code; got != "ACTIVITY_TIME_REQUIRED" {
		t.Fatalf("error code = %q; want ACTIVITY_TIME_REQUIRED", got)
	}
}

func TestActivityStdinRejectsCursor(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--config", t.TempDir(), "pr", "activity", "-", "--since", "24h", "--cursor", "1"})
	cmd.SetIn(strings.NewReader("PROJ/repo/1\n"))
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a stdin cursor to fail")
	}
	if got := cerrors.AsCLIError(err).Code; got != "ACTIVITY_BATCH_CURSOR" {
		t.Fatalf("error code = %q; want ACTIVITY_BATCH_CURSOR", got)
	}
}
