package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// cloudEmptyPRList answers any PR-collection GET with an empty Cloud page.
func cloudEmptyPRList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}})
}

// TestListPRsCloudStateAllEnumeratesStates guards the `--state ALL` fix on
// Cloud: the API defaults to OPEN when no `state` param is sent, so ALL must
// be expanded into one repeated `state` param per PR state.
func TestListPRsCloudStateAllEnumeratesStates(t *testing.T) {
	var gotStates []string
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStates = r.URL.Query()["state"]
		cloudEmptyPRList(w)
	}))

	if _, err := c.ListPRs(context.Background(), PRListOpts{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, State: "ALL",
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"DECLINED", "MERGED", "OPEN", "SUPERSEDED"}
	sort.Strings(gotStates)
	if strings.Join(gotStates, ",") != strings.Join(want, ",") {
		t.Errorf("state params = %v; want %v", gotStates, want)
	}
}

// TestListPRsDataCenterStateAllPassedThrough guards the DC side: the REST API
// natively accepts state=ALL, and omitting the param would default to OPEN.
func TestListPRsDataCenterStateAllPassedThrough(t *testing.T) {
	var gotQuery url.Values
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}, "isLastPage": true})
	}))

	if _, err := c.ListPRs(context.Background(), PRListOpts{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, State: "all",
	}); err != nil {
		t.Fatal(err)
	}
	if got := gotQuery.Get("state"); got != "ALL" {
		t.Errorf("state param = %q; want ALL", got)
	}
}

// TestListMyPRsCloudAuthorStateAllEnumeratesStates covers the same Cloud
// default-to-OPEN trap on the inbox author path (/2.0/pullrequests/{user}).
func TestListMyPRsCloudAuthorStateAllEnumeratesStates(t *testing.T) {
	var gotStates []string
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/user") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"uuid": "{u-1}", "username": "me"})
			return
		}
		gotStates = r.URL.Query()["state"]
		cloudEmptyPRList(w)
	}))

	if _, err := c.ListMyPRs(context.Background(), MyPRListOpts{
		Role: "AUTHOR", State: "ALL",
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"DECLINED", "MERGED", "OPEN", "SUPERSEDED"}
	sort.Strings(gotStates)
	if strings.Join(gotStates, ",") != strings.Join(want, ",") {
		t.Errorf("state params = %v; want %v", gotStates, want)
	}
}

func TestListMyPRsDataCenterClosedSinceAndAnyRole(t *testing.T) {
	var gotQuery url.Values
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{}, "isLastPage": true})
	}))

	if _, err := c.ListMyPRs(context.Background(), MyPRListOpts{
		Role: "ANY", State: "ALL", ClosedSinceSeconds: 172800,
	}); err != nil {
		t.Fatal(err)
	}
	if got := gotQuery.Get("closedSince"); got != "172800" {
		t.Errorf("closedSince = %q; want 172800", got)
	}
	if got := gotQuery.Get("order"); got != "CLOSED_DATE" {
		t.Errorf("order = %q; want CLOSED_DATE", got)
	}
	if got := gotQuery.Get("role"); got != "" {
		t.Errorf("role = %q; want omitted for ANY", got)
	}
	if got := gotQuery.Get("state"); got != "" {
		t.Errorf("state = %q; want omitted for ALL", got)
	}
}

func TestListMyPRsCloudRejectsClosedSince(t *testing.T) {
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("Cloud --closed-since should fail before making an HTTP request")
	}))

	_, err := c.ListMyPRs(context.Background(), MyPRListOpts{ClosedSinceSeconds: 3600})
	if err == nil {
		t.Fatal("expected --closed-since to be rejected on Cloud")
	}
	if got := cerrors.AsCLIError(err).Code; got != "INBOX_CLOSED_SINCE_UNSUPPORTED" {
		t.Fatalf("error code = %q; want INBOX_CLOSED_SINCE_UNSUPPORTED", got)
	}
}

func TestListMyPRsCloudAnyRoleUsesOneCombinedFilter(t *testing.T) {
	var gotFilter string
	c := newWriteTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/2.0/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"uuid": "{u-1}", "nickname": "me"})
		case "/2.0/repositories/ws":
			_ = json.NewEncoder(w).Encode(map[string]any{"values": []any{
				map[string]any{"slug": "repo", "workspace": map[string]any{"slug": "ws"}},
			}})
		case "/2.0/repositories/ws/repo/pullrequests":
			gotFilter = r.URL.Query().Get("q")
			cloudEmptyPRList(w)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))

	if _, err := c.ListMyPRs(context.Background(), MyPRListOpts{
		Role: "ANY", State: "OPEN", Workspace: "ws",
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"author.uuid", "reviewers.uuid", "participants.uuid"} {
		if !strings.Contains(gotFilter, want) {
			t.Errorf("combined filter %q does not contain %q", gotFilter, want)
		}
	}
}
