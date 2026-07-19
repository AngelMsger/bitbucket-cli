package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
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
