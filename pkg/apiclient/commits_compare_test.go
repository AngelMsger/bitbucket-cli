package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

func TestCompareCommitsDataCenterFollowsServerCursor(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("since") != "main" || q.Get("until") != "dev" || q.Get("limit") != "25" {
			t.Errorf("query = %v; want since=main until=dev limit=25", q)
		}
		w.Header().Set("Content-Type", "application/json")
		switch q.Get("start") {
		case "0":
			_, _ = w.Write([]byte(`{
				"values":[{"id":"aaaa111","message":"page one"}],
				"size":1,"limit":25,"start":0,
				"isLastPage":false,"nextPageStart":137
			}`))
		case "137":
			_, _ = w.Write([]byte(`{
				"values":[{"id":"bbbb222","message":"page two"}],
				"size":1,"limit":25,"start":137,"isLastPage":true
			}`))
		default:
			t.Fatalf("unexpected start cursor %q", q.Get("start"))
		}
	}))
	t.Cleanup(srv.Close)
	c := New(Config{
		Flavor: FlavorDataCenter, BaseURL: srv.URL,
		Transport: transport.New(transport.Options{}),
	})
	req := CompareCommitsReq{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, From: "main", To: "dev",
	}
	first, err := c.CompareCommits(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Next != "137" || len(first.Items) != 1 || first.Items[0].Hash != "aaaa111" {
		t.Fatalf("first page = %+v; want hash aaaa111 and next 137", first)
	}
	req.Cursor = first.Next
	second, err := c.CompareCommits(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Next != "" || len(second.Items) != 1 || second.Items[0].Hash != "bbbb222" {
		t.Fatalf("second page = %+v; want final hash bbbb222", second)
	}
	if calls != 2 {
		t.Fatalf("calls = %d; want 2", calls)
	}
}

func TestCompareCommitsCloudFollowsNextURL(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/page-2" {
			if r.URL.RawQuery != "" {
				t.Errorf("follow-up query = %q; want empty", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"values":[{"hash":"bbbb222","message":"page two"}]}`))
			return
		}
		q := r.URL.Query()
		if q.Get("include") != "dev" || q.Get("exclude") != "main" || q.Get("pagelen") != "25" {
			t.Errorf("query = %v; want include=dev exclude=main pagelen=25", q)
		}
		_, _ = w.Write([]byte(`{
			"values":[{"hash":"aaaa111","message":"page one"}],
			"next":"http://` + r.Host + `/page-2"
		}`))
	}))
	t.Cleanup(srv.Close)
	c := New(Config{
		Flavor: FlavorCloud, BaseURL: srv.URL,
		Transport: transport.New(transport.Options{}),
	})
	req := CompareCommitsReq{
		Repo: RepoRef{Workspace: "ws", Slug: "repo"}, From: "main", To: "dev",
	}
	first, err := c.CompareCommits(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Next == "" || len(first.Items) != 1 || first.Items[0].Hash != "aaaa111" {
		t.Fatalf("first page = %+v; want hash aaaa111 and next URL", first)
	}
	req.Cursor = first.Next
	second, err := c.CompareCommits(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Next != "" || len(second.Items) != 1 || second.Items[0].Hash != "bbbb222" {
		t.Fatalf("second page = %+v; want final hash bbbb222", second)
	}
	if calls != 2 {
		t.Fatalf("calls = %d; want 2", calls)
	}
}
