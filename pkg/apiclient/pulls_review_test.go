package apiclient

import (
	"context"
	"encoding/json"
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

// Identity failures must prevent both previews and writes.
func TestRequestChangesDataCenterIdentityFailure(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry-run=%t", dryRun), func(t *testing.T) {
			writes := 0
			c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					t.Errorf("write after failed identity lookup: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"errors":[{"message":"Authentication failed"}]}`)
			}))
			req := RequestChangesReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7, Request: true}
			var err error
			if dryRun {
				_, err = c.DescribeWrite(context.Background(), req)
			} else {
				err = c.RequestPRChanges(context.Background(), req)
			}
			var ce *cerrors.CLIError
			if !errors.As(err, &ce) || ce.HTTPStatus != 401 || writes != 0 {
				t.Fatalf("identity failure = %v; writes = %d", err, writes)
			}
		})
	}
}

func TestWithdrawChangesDataCenterPreservesOtherVotes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pr         string
		readStatus int
		wantError  string
	}{
		{"reviewer needs work", `{"reviewers":[{"user":{"slug":"alice"},"status":"NEEDS_WORK"}]}`, 200, ""},
		{"participant needs work", `{"participants":[{"user":{"slug":"alice"},"status":"NEEDS_WORK"}]}`, 200, ""},
		{"approved", `{"reviewers":[{"user":{"slug":"alice"},"approved":true,"status":"APPROVED"}]}`, 200, "PR_NO_CHANGE_REQUEST"},
		{"unapproved", `{"participants":[{"user":{"slug":"alice"},"status":"UNAPPROVED"}]}`, 200, "PR_NO_CHANGE_REQUEST"},
		{"another reviewer", `{"reviewers":[{"user":{"slug":"bob"},"status":"NEEDS_WORK"}]}`, 200, "PR_NO_CHANGE_REQUEST"},
		{"unknown status", `{"participants":[{"user":{"slug":"alice"}}]}`, 200, "PR_NO_CHANGE_REQUEST"},
		{"inconsistent approval", `{"reviewers":[{"user":{"slug":"alice"},"approved":true,"status":"NEEDS_WORK"}]}`, 200, "PR_NO_CHANGE_REQUEST"},
		{"conflicting records", `{"reviewers":[{"user":{"slug":"alice"},"status":"APPROVED"}],"participants":[{"user":{"slug":"alice"},"status":"NEEDS_WORK"}]}`, 200, "PR_NO_CHANGE_REQUEST"},
		{"read denied", `{"errors":[{"message":"Access denied"}]}`, 403, "HTTP_Forbidden"},
	} {
		for _, preview := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preview=%t", tc.name, preview), func(t *testing.T) {
				writes := 0
				c := dcWithdrawalTestClient(t, func() (string, int) { return tc.pr, tc.readStatus }, &writes)
				req := RequestChangesReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7}
				var err error
				if preview {
					var plan WriteRequestPlan
					plan, err = NewReadOnly(c).DescribeWrite(context.Background(), req)
					if err == nil && (plan.Method != "PUT" || plan.Payload.(map[string]any)["status"] != "UNAPPROVED") {
						t.Fatalf("unexpected plan: %+v", plan)
					}
				} else {
					err = c.RequestPRChanges(context.Background(), req)
				}
				wantWrites := 0
				if tc.wantError == "" {
					if err != nil {
						t.Fatal(err)
					}
					if !preview {
						wantWrites = 1
					}
				} else {
					var ce *cerrors.CLIError
					if !errors.As(err, &ce) || ce.Code != tc.wantError || ce.Retryable {
						t.Fatalf("error = %#v, want %s", err, tc.wantError)
					}
					if tc.wantError == "PR_NO_CHANGE_REQUEST" && (ce.Category != cerrors.CategoryConflict || len(ce.NextSteps) == 0 || !strings.Contains(ce.NextSteps[0], "pr get PROJ/repo/7")) {
						t.Fatalf("missing conflict recovery: %+v", ce)
					}
				}
				if writes != wantWrites {
					t.Fatalf("writes = %d, want %d", writes, wantWrites)
				}
			})
		}
	}
}

func TestWithdrawChangesRechecksAfterPreview(t *testing.T) {
	status, writes := "NEEDS_WORK", 0
	c := dcWithdrawalTestClient(t, func() (string, int) {
		return fmt.Sprintf(`{"participants":[{"user":{"slug":"alice"},"status":%q}]}`, status), 200
	}, &writes)
	req := RequestChangesReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7}
	if _, err := c.DescribeWrite(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	status = "APPROVED"
	var ce *cerrors.CLIError
	if err := c.RequestPRChanges(context.Background(), req); !errors.As(err, &ce) || ce.Code != "PR_NO_CHANGE_REQUEST" || writes != 0 {
		t.Fatalf("approval was not preserved: err=%v, writes=%d", err, writes)
	}
}

func dcWithdrawalTestClient(t *testing.T, readPR func() (string, int), writes *int) Client {
	t.Helper()
	prPath := "/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/7"
	return newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/1.0/application-properties":
			w.Header().Set("X-AUSERNAME", "alice")
			_, _ = fmt.Fprint(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/1.0/users/alice":
			_, _ = fmt.Fprint(w, `{"name":"alice","slug":"alice"}`)
		case r.Method == http.MethodGet && r.URL.Path == prPath:
			body, status := readPR()
			w.WriteHeader(status)
			_, _ = fmt.Fprint(w, body)
		case r.Method == http.MethodPut && r.URL.Path == prPath+"/participants/alice":
			*writes++
			var body struct{ Status string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status != "UNAPPROVED" {
				t.Errorf("unexpected withdrawal: %+v, %v", body, err)
			}
			_, _ = fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
