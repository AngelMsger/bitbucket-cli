package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/angelmsger/bitbucket-cli/pkg/transport"
)

// A server denial must remain distinguishable from READONLY_BLOCKED, retain
// its explanation, and never trigger an automatic write retry or fallback.
func TestPRReviewPermissionRejection(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		for _, action := range []string{"approve", "decline"} {
			t.Run(string(flavor)+"/"+action, func(t *testing.T) {
				reads, writes := 0, 0
				prPath := "/2.0/repositories/PROJ/repo/pullrequests/7"
				if flavor == FlavorDataCenter {
					prPath = "/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/7"
				}
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet && flavor == FlavorDataCenter && action == "decline" && r.URL.Path == prPath {
						reads++
						_, _ = fmt.Fprint(w, `{"id":7,"version":9}`)
						return
					}
					writes++
					if r.Method != http.MethodPost || r.URL.Path != prPath+"/"+action {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
					w.WriteHeader(http.StatusForbidden)
					if flavor == FlavorCloud {
						_, _ = fmt.Fprint(w, `{"error":{"message":"Token cannot perform this PR action"}}`)
					} else {
						_, _ = fmt.Fprint(w, `{"errors":[{"message":"Token cannot perform this PR action"}]}`)
					}
				}))
				defer srv.Close()
				c := New(Config{Flavor: flavor, BaseURL: srv.URL, Transport: transport.New(transport.Options{MaxRetries: 3})})
				repo := RepoRef{Workspace: "PROJ", Slug: "repo"}
				var err error
				if action == "approve" {
					err = c.ApprovePR(context.Background(), ApprovePRReq{Repo: repo, ID: 7, Approve: true})
				} else {
					_, err = c.DeclinePR(context.Background(), DeclinePRReq{Repo: repo, ID: 7})
				}
				var ce *cerrors.CLIError
				if !errors.As(err, &ce) || ce.Category != cerrors.CategoryPermission || ce.Code != "HTTP_Forbidden" || ce.HTTPStatus != 403 || ce.Retryable {
					t.Fatalf("expected a non-retryable server permission error, got %#v", err)
				}
				if !strings.Contains(ce.Message, "Token cannot perform this PR action") {
					t.Errorf("server explanation lost: %s", ce.Message)
				}
				wantReads := 0
				if flavor == FlavorDataCenter && action == "decline" {
					wantReads = 1
				}
				if writes != 1 || reads != wantReads {
					t.Errorf("requests = %d reads, %d writes; want %d reads and one rejected write", reads, writes, wantReads)
				}
			})
		}
	}
}

func TestDeclinePRDataCenterVersionReadFailure(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry-run=%t", dryRun), func(t *testing.T) {
			requests := 0
			c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodGet {
					t.Errorf("write after failed version lookup: %s", r.Method)
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"errors":[{"message":"Repository access denied"}]}`)
			}))
			req := DeclinePRReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7}
			var err error
			if dryRun {
				_, err = c.DescribeWrite(context.Background(), req)
			} else {
				_, err = c.DeclinePR(context.Background(), req)
			}
			var ce *cerrors.CLIError
			if !errors.As(err, &ce) || ce.Code != "HTTP_Forbidden" || requests != 1 {
				t.Fatalf("version lookup failure = %v; requests = %d", err, requests)
			}
		})
	}
}
