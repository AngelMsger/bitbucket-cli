package apiclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

func TestHTTPConflictPreservesReason(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		for _, tc := range []struct {
			name    string
			message string
		}{
			{"closed PR", "The pull request is not open."},
			{"merge conflict", "The pull request has conflicts."},
			{"existing branch", "The branch already exists."},
			{"version conflict", "The specified version is out of date."},
			{"no reason", ""},
		} {
			t.Run(string(flavor)+"/"+tc.name, func(t *testing.T) {
				var payload any = map[string]any{"error": map[string]string{"message": tc.message}}
				if flavor == FlavorDataCenter {
					payload = map[string]any{"errors": []map[string]string{{"message": tc.message}}}
				}
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				c := &apiClient{flavor: flavor}
				err = c.httpError(&http.Response{
					StatusCode: http.StatusConflict,
					Body:       io.NopCloser(strings.NewReader(string(body))),
				})
				var ce *cerrors.CLIError
				if !errors.As(err, &ce) || ce.Code != "HTTP_Conflict" || ce.Category != cerrors.CategoryConflict || ce.HTTPStatus != 409 || ce.Retryable || cerrors.ExitCode(err) != 11 {
					t.Fatalf("expected a non-retryable HTTP conflict, got %#v", err)
				}
				wantMessage := "Bitbucket returned HTTP 409"
				if tc.message != "" {
					wantMessage += ": " + tc.message
				}
				if ce.Message != wantMessage {
					t.Errorf("message = %q, want %q", ce.Message, wantMessage)
				}
				guidance := ce.Hint + " " + strings.Join(ce.NextSteps, " ")
				if strings.Contains(strings.ToLower(guidance), "version") {
					t.Errorf("generic guidance assumes a version conflict: %s", guidance)
				}
				if ce.Hint == "" || len(ce.NextSteps) == 0 {
					t.Errorf("missing conflict recovery guidance: %#v", ce)
				}
			})
		}
	}
}
