package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

// newUserTestClient serves a Data Center instance that stamps X-AUSERNAME on
// authenticated responses, as a real one does, and resolves user records.
func newUserTestClient(t *testing.T, username string, serveRecord bool) (Client, *[]string) {
	t.Helper()
	var paths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if username != "" {
			w.Header().Set("X-AUSERNAME", username)
		}
		w.Header().Set("Content-Type", "application/json")

		if serveRecord && r.URL.Path == "/rest/api/1.0/users/"+username {
			_, _ = io.WriteString(w,
				`{"name":"`+username+`","slug":"`+username+`","displayName":"Alice Example",`+
					`"emailAddress":"alice@example.com"}`)
			return
		}
		_, _ = io.WriteString(w, `{"size":1,"values":[]}`)
	}))
	t.Cleanup(srv.Close)

	c := New(Config{
		Flavor:    FlavorDataCenter,
		BaseURL:   srv.URL,
		Transport: transport.New(transport.Options{}),
	})
	return c, &paths
}

// Data Center has no current-user endpoint; the username arrives only in a
// response header. Recovering it is what lets a caller address its own
// personal project, which is where forks live.
func TestCurrentUserDataCenterReadsTheUsernameHeader(t *testing.T) {
	t.Parallel()
	c, paths := newUserTestClient(t, "alice", true)

	user, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if user.Slug != "alice" {
		t.Errorf("Slug = %q, want alice", user.Slug)
	}
	if user.DisplayName != "Alice Example" {
		t.Errorf("DisplayName = %q, want the resolved record's name", user.DisplayName)
	}
	if len(*paths) < 2 {
		t.Errorf("expected a header probe followed by a record lookup, got %v", *paths)
	}
	if got := (*paths)[0]; got != "/rest/api/1.0/application-properties" {
		t.Errorf("header probe path = %q, want application-properties", got)
	}
}

// The username is the part callers act on. When the record cannot be fetched,
// returning the username alone beats failing outright.
func TestCurrentUserDataCenterFallsBackToTheHeaderAlone(t *testing.T) {
	t.Parallel()
	c, _ := newUserTestClient(t, "alice", false)

	user, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if user.Slug != "alice" {
		t.Errorf("Slug = %q, want alice from the header", user.Slug)
	}
	if user.Type != "dc" {
		t.Errorf("Type = %q, want dc", user.Type)
	}
}

// A deployment that strips the header, or an anonymous session, cannot satisfy
// whoami and must not be reported as a successful identity lookup.
func TestCurrentUserDataCenterWithoutTheHeader(t *testing.T) {
	t.Parallel()
	c, _ := newUserTestClient(t, "", false)

	user, err := c.CurrentUser(context.Background())
	if err == nil {
		t.Fatalf("CurrentUser returned user %v without an identity header; want an error", user)
	}
	ce := cerrors.AsCLIError(err)
	if ce.Category != cerrors.CategoryAuth {
		t.Errorf("category = %q, want auth", ce.Category)
	}
	if ce.Code != "AUTH_IDENTITY_UNAVAILABLE" {
		t.Errorf("code = %q, want AUTH_IDENTITY_UNAVAILABLE", ce.Code)
	}
}

// Cloud is unchanged: it has a real current-user endpoint.
func TestCurrentUserCloudUsesItsEndpoint(t *testing.T) {
	t.Parallel()
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"uuid":"{u}","nickname":"alice","display_name":"Alice"}`)
	}))
	t.Cleanup(srv.Close)

	c := New(Config{
		Flavor:    FlavorCloud,
		BaseURL:   srv.URL,
		Transport: transport.New(transport.Options{}),
	})
	if _, err := c.CurrentUser(context.Background()); err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if want := "/2.0/user"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}
