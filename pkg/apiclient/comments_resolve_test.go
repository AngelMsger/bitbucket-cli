package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

func newResolveTestClient(t *testing.T, flavor Flavor, handler http.Handler) Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{Flavor: flavor, BaseURL: srv.URL, Transport: transport.New(transport.Options{})})
}

// TestResolvePRCommentCloud covers the Cloud path: a POST to the resolve
// endpoint followed by a re-read of the comment for a uniform result.
func TestResolvePRCommentCloud(t *testing.T) {
	var hitResolve, hitGet bool
	c := newResolveTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments/42/resolve"):
			hitResolve = true
			w.WriteHeader(http.StatusNoContent) // resolve returns no comment
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/comments/42"):
			hitGet = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         42,
				"content":    map[string]any{"raw": "please fix"},
				"resolution": map[string]any{"type": "resolution"},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))

	cm, err := c.ResolvePRComment(context.Background(), ResolvePRCommentReq{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, PRID: 7, ID: 42, Resolve: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hitResolve || !hitGet {
		t.Fatalf("expected resolve POST and re-read GET; got resolve=%v get=%v", hitResolve, hitGet)
	}
	if !cm.Resolved {
		t.Errorf("comment should be resolved: %+v", cm)
	}
}

// TestUnresolvePRCommentCloud covers the reopen path: a DELETE to the resolve
// endpoint, then a re-read.
func TestUnresolvePRCommentCloud(t *testing.T) {
	var method string
	c := newResolveTestClient(t, FlavorCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/resolve") {
			method = r.Method
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "content": map[string]any{"raw": "x"}})
	}))
	cm, err := c.ResolvePRComment(context.Background(), ResolvePRCommentReq{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, PRID: 7, ID: 42, Resolve: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete {
		t.Errorf("reopen should DELETE the resolve endpoint; got %q", method)
	}
	if cm.Resolved {
		t.Errorf("comment should be reopened: %+v", cm)
	}
}

// TestResolvePRCommentDataCenter covers the DC path: a version GET followed by
// a PUT that sets state = RESOLVED.
func TestResolvePRCommentDataCenter(t *testing.T) {
	var putBody map[string]any
	c := newResolveTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "version": 3, "text": "fix", "state": "OPEN"})
		case http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &putBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "version": 4, "text": "fix", "state": "RESOLVED"})
		default:
			t.Fatalf("unexpected %s", r.Method)
		}
	}))
	cm, err := c.ResolvePRComment(context.Background(), ResolvePRCommentReq{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, PRID: 7, ID: 42, Resolve: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if putBody["state"] != "RESOLVED" {
		t.Errorf("PUT state = %v; want RESOLVED", putBody["state"])
	}
	if v, _ := putBody["version"].(float64); int(v) != 3 {
		t.Errorf("PUT version = %v; want the current version 3", putBody["version"])
	}
	if !cm.Resolved {
		t.Errorf("comment should be resolved: %+v", cm)
	}
}

// TestDescribeResolveCloud confirms --dry-run reports the resolve call without
// sending it (Cloud needs no version pre-fetch).
func TestDescribeResolveCloud(t *testing.T) {
	c := newResolveTestClient(t, FlavorCloud, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("DescribeWrite should not send HTTP on Cloud: %s %s", r.Method, r.URL.Path)
	}))
	for _, tc := range []struct {
		resolve bool
		method  string
	}{{true, http.MethodPost}, {false, http.MethodDelete}} {
		plan, err := c.DescribeWrite(context.Background(), ResolvePRCommentReq{
			Repo: RepoRef{Workspace: "ws", Slug: "repo"}, PRID: 7, ID: 42, Resolve: tc.resolve,
		})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Method != tc.method || !strings.HasSuffix(plan.URL, "/comments/42/resolve") {
			t.Errorf("resolve=%v plan = %s %s", tc.resolve, plan.Method, plan.URL)
		}
	}
}
