package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

func TestParseReviewInput(t *testing.T) {
	t.Run("JSON object", func(t *testing.T) {
		doc, err := parseReviewInput([]byte(`{
			"verdict": "request-changes",
			"expected_source_commit": "abc123",
			"comments": [{"path":"a.go","line":2,"content":"fix this"}]
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if doc.Verdict != "request-changes" || doc.ExpectedSourceCommit != "abc123" || len(doc.Comments) != 1 {
			t.Fatalf("got %+v", doc)
		}
	})

	t.Run("JSON bare array", func(t *testing.T) {
		doc, err := parseReviewInput([]byte(`[{"content":"looks good"},{"reply_to":12,"content":"done"}]`))
		if err != nil {
			t.Fatal(err)
		}
		if len(doc.Comments) != 2 || doc.Comments[1].ReplyTo != 12 {
			t.Fatalf("got %+v", doc)
		}
	})

	t.Run("YAML object", func(t *testing.T) {
		raw := "verdict: approve\nexpected_source_commit: deadbeef\ncomments:\n  - path: main.go\n    line: 5\n    side: new\n    content: nice\n"
		doc, err := parseReviewInput([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if doc.Verdict != "approve" || doc.ExpectedSourceCommit != "deadbeef" || len(doc.Comments) != 1 || doc.Comments[0].Line != 5 {
			t.Fatalf("got %+v", doc)
		}
	})

	t.Run("YAML bare sequence", func(t *testing.T) {
		raw := "- content: general note\n- path: x.go\n  line: 10\n  content: inline note\n"
		doc, err := parseReviewInput([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if len(doc.Comments) != 2 || doc.Comments[1].Path != "x.go" {
			t.Fatalf("got %+v", doc)
		}
	})

	t.Run("unknown JSON key fails loudly", func(t *testing.T) {
		_, err := parseReviewInput([]byte(`{"comment":[{"content":"typo key"}]}`))
		if err == nil {
			t.Fatal("expected BAD_REVIEW_INPUT for unknown key")
		}
		if ce := cerrors.AsCLIError(err); ce.Code != "BAD_REVIEW_INPUT" {
			t.Fatalf("code = %s, want BAD_REVIEW_INPUT", ce.Code)
		}
	})

	t.Run("empty input fails loudly", func(t *testing.T) {
		_, err := parseReviewInput([]byte("   \n"))
		if err == nil || cerrors.AsCLIError(err).Code != "BAD_REVIEW_INPUT" {
			t.Fatalf("want BAD_REVIEW_INPUT, got %v", err)
		}
	})

	t.Run("concatenated JSON objects fail loudly", func(t *testing.T) {
		_, err := parseReviewInput([]byte(`{"verdict":"approve"}{"comments":[{"content":"dropped"}]}`))
		if err == nil || cerrors.AsCLIError(err).Code != "BAD_REVIEW_INPUT" {
			t.Fatalf("want BAD_REVIEW_INPUT for concatenated JSON objects, got %v", err)
		}
	})

	t.Run("JSON array with trailing content fails loudly", func(t *testing.T) {
		_, err := parseReviewInput([]byte(`[{"content":"ok"}] {"verdict":"approve"}`))
		if err == nil || cerrors.AsCLIError(err).Code != "BAD_REVIEW_INPUT" {
			t.Fatalf("want BAD_REVIEW_INPUT for trailing JSON content, got %v", err)
		}
	})

	t.Run("multi-document YAML fails loudly", func(t *testing.T) {
		raw := "verdict: approve\n---\ncomments:\n  - content: second doc\n"
		_, err := parseReviewInput([]byte(raw))
		if err == nil || cerrors.AsCLIError(err).Code != "BAD_REVIEW_INPUT" {
			t.Fatalf("want BAD_REVIEW_INPUT for multi-document YAML, got %v", err)
		}
	})

	t.Run("YAML flow sequence [{content: hello}] succeeds", func(t *testing.T) {
		doc, err := parseReviewInput([]byte(`[{content: hello}, {path: a.go, line: 3, content: fix}]`))
		if err != nil {
			t.Fatalf("flow YAML sequence failed: %v", err)
		}
		if len(doc.Comments) != 2 || doc.Comments[0].Content != "hello" || doc.Comments[1].Path != "a.go" || doc.Comments[1].Line != 3 {
			t.Fatalf("got %+v", doc)
		}
	})

	t.Run("YAML flow mapping succeeds", func(t *testing.T) {
		doc, err := parseReviewInput([]byte(`{verdict: approve, expected_source_commit: abc123, comments: [{content: ship it}]}`))
		if err != nil {
			t.Fatalf("flow YAML mapping failed: %v", err)
		}
		if doc.Verdict != "approve" || doc.ExpectedSourceCommit != "abc123" || len(doc.Comments) != 1 || doc.Comments[0].Content != "ship it" {
			t.Fatalf("got %+v", doc)
		}
	})

	t.Run("JSON UTF-16 surrogate pair escape (Python json.dumps default) succeeds", func(t *testing.T) {
		doc, err := parseReviewInput([]byte(`{"comments":[{"content":"\ud83d\udc4d looks good"}]}`))
		if err != nil {
			t.Fatalf("JSON UTF-16 surrogate escape failed: %v", err)
		}
		if len(doc.Comments) != 1 || doc.Comments[0].Content != "👍 looks good" {
			t.Fatalf("got %+v, want 👍 looks good", doc.Comments)
		}
	})

	t.Run("fractional reply_to or line is rejected in both JSON and YAML", func(t *testing.T) {
		for _, raw := range []string{
			`{"comments":[{"reply_to":42.9,"content":"bad"}]}`,
			`[{"line":10.5,"path":"a.go","content":"bad"}]`,
			`[{reply_to: 42.9, content: bad}]`,
			"comments:\n  - path: a.go\n    line: 10.5\n    content: bad\n",
		} {
			if _, err := parseReviewInput([]byte(raw)); err == nil || cerrors.AsCLIError(err).Code != "BAD_REVIEW_INPUT" {
				t.Errorf("input %q: want BAD_REVIEW_INPUT, got %v", raw, err)
			}
		}
	})

	t.Run("duplicate or case-variant keys in JSON or YAML are rejected", func(t *testing.T) {
		for _, raw := range []string{
			`{"verdict":"approve","comments":[{"content":"finding"}],"comments":[]}`,
			`{"verdict":"approve","comments":[{"content":"finding"}],"Comments":[]}`,
			`{"verdict":"approve","Verdict":"request-changes"}`,
			`[{"content":"first","content":"second"}]`,
			`[{"content":"first","Content":"second"}]`,
			"verdict: approve\ncomments:\n  - content: finding\ncomments: []\n",
			"verdict: approve\ncomments:\n  - content: finding\nComments: []\n",
		} {
			if _, err := parseReviewInput([]byte(raw)); err == nil || cerrors.AsCLIError(err).Code != "BAD_REVIEW_INPUT" {
				t.Errorf("input %q: want BAD_REVIEW_INPUT, got %v", raw, err)
			}
		}
	})

	t.Run("single YAML document with explicit end marker succeeds", func(t *testing.T) {
		raw := "---\nverdict: approve\n...\n"
		doc, err := parseReviewInput([]byte(raw))
		if err != nil {
			t.Fatalf("valid single YAML doc failed: %v", err)
		}
		if doc.Verdict != "approve" {
			t.Fatalf("got %+v", doc)
		}
	})
}

// runCommandCapture executes the full root CLI against a fake Bitbucket server,
// capturing os.Stdout (where output.Emit writes) so CLI wire contracts can be
// verified end-to-end without leaving the process.
func runCommandCapture(t *testing.T, readOnly bool, stdin string, baseURL string, extraArgs ...string) (string, error) {
	t.Helper()
	return runCommandCaptureWithFlavor(t, readOnly, stdin, baseURL, "datacenter", extraArgs...)
}

func runCommandCaptureWithFlavor(t *testing.T, readOnly bool, stdin string, baseURL string, flavor string, extraArgs ...string) (string, error) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("BITBUCKET_TOKEN", "test-pat")
	t.Setenv("BITBUCKET_PERSONAL_ACCESS_TOKEN", "")
	t.Setenv("BITBUCKET_USERNAME", "")
	t.Setenv("BITBUCKET_PASSWORD", "")
	t.Setenv("BITBUCKET_API_TOKEN", "")
	t.Setenv("BITBUCKET_SERVER", "")
	t.Setenv("BITBUCKET_CONTEXT", "")
	t.Setenv("BITBUCKET_CLI_NO_SKILL_HINT", "1")
	t.Setenv("BITBUCKET_CLI_NO_CONTEXT_HINT", "1")
	if readOnly {
		t.Setenv("BITBUCKET_CLI_READ_ONLY", "1")
	} else {
		t.Setenv("BITBUCKET_CLI_READ_ONLY", "")
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	cmd := NewRootCmd()
	args := []string{
		"--config", t.TempDir(),
		"--auth-scheme", "pat",
		"--format", "json",
	}
	if baseURL != "" {
		args = append(args, "--base-url", baseURL)
	}
	if flavor != "" {
		args = append(args, "--flavor", flavor)
	}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	cmd.SetErr(new(bytes.Buffer))

	runErr := cmd.Execute()
	_ = w.Close()
	outBytes, _ := io.ReadAll(r)
	_ = r.Close()
	return string(outBytes), runErr
}

func TestPRReviewCLIContract(t *testing.T) {
	var writes int32
	var failSecondComment atomic.Bool
	var forbidVote atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/1.0/application-properties":
			w.Header().Set("X-AUSERNAME", "alice")
			_, _ = w.Write([]byte(`{"version":"8.19.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/1.0/users/alice":
			_, _ = w.Write([]byte(`{"name":"alice","slug":"alice","displayName":"Alice","type":"NORMAL"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/1":
			_, _ = w.Write([]byte(`{
				"id": 1, "title": "Fix bug", "state": "OPEN", "version": 2,
				"fromRef": {"displayId": "feat", "latestCommit": "aaaa1111"},
				"toRef": {"displayId": "main", "latestCommit": "00001111"}
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/1/diff":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -1,2 +1,3 @@\n package app\n+func New() {}\n func Old() {}\n"))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/1/comments":
			n := atomic.AddInt32(&writes, 1)
			if failSecondComment.Load() && n == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"id": 100, "text": "posted", "author": {"name": "alice", "type": "NORMAL"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/1/participants/alice":
			atomic.AddInt32(&writes, 1)
			if forbidVote.Load() {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"errors":[{"message":"cannot approve own PR"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"approved": true, "status": "APPROVED"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Run("dry-run works in read-only mode with stdin YAML", func(t *testing.T) {
		atomic.StoreInt32(&writes, 0)
		stdin := "expected_source_commit: aaaa1111\ncomments:\n  - path: app.go\n    line: 2\n    content: Looks good\n"
		out, err := runCommandCapture(t, true, stdin, srv.URL, "pr", "review", "PROJ/demo/1",
			"--verdict", "approve", "--input", "-", "--dry-run")
		if err != nil {
			t.Fatalf("dry-run error: %v", err)
		}
		if atomic.LoadInt32(&writes) != 0 {
			t.Fatalf("dry-run sent %d writes", writes)
		}
		var plan apiclient.PRReviewPlan
		if err := json.Unmarshal([]byte(out), &plan); err != nil {
			t.Fatalf("decode plan: %v\nraw: %s", err, out)
		}
		if !plan.Validated || plan.Verdict != apiclient.ReviewVerdictApprove || len(plan.Comments) != 1 || plan.Vote == nil {
			t.Fatalf("unexpected plan: %+v", plan)
		}
		if plan.Vote.Method != "PUT" || !strings.HasSuffix(plan.Vote.URL, "/participants/alice") {
			t.Fatalf("DC approve plan should use PUT /participants/alice with lastReviewedCommit, got %+v", plan.Vote)
		}
	})

	t.Run("read-only blocks live submission with zero writes", func(t *testing.T) {
		atomic.StoreInt32(&writes, 0)
		_, err := runCommandCapture(t, true, "", srv.URL, "pr", "review", "PROJ/demo/1", "--verdict", "approve")
		if err == nil {
			t.Fatal("expected READONLY_BLOCKED")
		}
		if ce := cerrors.AsCLIError(err); ce.Code != "READONLY_BLOCKED" {
			t.Fatalf("code = %s, want READONLY_BLOCKED", ce.Code)
		}
		if atomic.LoadInt32(&writes) != 0 {
			t.Fatalf("read-only sent %d writes", writes)
		}
	})

	t.Run("CLI flags override input document and mismatch aborts cleanly", func(t *testing.T) {
		atomic.StoreInt32(&writes, 0)
		stdin := `{"verdict":"comment-only","expected_source_commit":"aaaa1111","comments":[{"content":"hi"}]}`
		_, err := runCommandCapture(t, false, stdin, srv.URL, "pr", "review", "PROJ/demo/1",
			"--expected-source-commit", "deadbeef", "--input", "-")
		if err == nil {
			t.Fatal("expected PR_HEAD_MOVED")
		}
		if ce := cerrors.AsCLIError(err); ce.Code != "PR_HEAD_MOVED" {
			t.Fatalf("code = %s, want PR_HEAD_MOVED", ce.Code)
		}
		if atomic.LoadInt32(&writes) != 0 {
			t.Fatalf("head mismatch sent %d writes", writes)
		}
	})

	t.Run("mid-flight failure emits partial stdout result and returns REVIEW_PARTIAL_FAILURE", func(t *testing.T) {
		atomic.StoreInt32(&writes, 0)
		failSecondComment.Store(true)
		defer failSecondComment.Store(false)

		stdin := `[{"content":"first"},{"content":"second"}]`
		out, err := runCommandCapture(t, false, stdin, srv.URL, "pr", "review", "PROJ/demo/1",
			"--verdict", "approve", "--expected-source-commit", "aaaa1111", "--input", "-")
		if err == nil {
			t.Fatal("expected REVIEW_PARTIAL_FAILURE")
		}
		if ce := cerrors.AsCLIError(err); ce.Code != "REVIEW_PARTIAL_FAILURE" || ce.Retryable {
			t.Fatalf("err = %+v, want non-retryable REVIEW_PARTIAL_FAILURE", ce)
		}
		var res apiclient.PRReviewResult
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("decode partial result: %v\nraw: %s", err, out)
		}
		if len(res.PostedComments) != 1 || res.VoteApplied || res.FailedItem == nil || res.FailedItem.Index != 1 {
			t.Fatalf("unexpected partial result: %+v", res)
		}
	})

	t.Run("vote-only 403 failure still emits failed_item and vote_applied on stdout", func(t *testing.T) {
		atomic.StoreInt32(&writes, 0)
		forbidVote.Store(true)
		defer forbidVote.Store(false)

		out, err := runCommandCapture(t, false, "", srv.URL, "pr", "review", "PROJ/demo/1",
			"--verdict", "approve", "--expected-source-commit", "aaaa1111")
		if err == nil {
			t.Fatal("expected REVIEW_PARTIAL_FAILURE")
		}
		ce := cerrors.AsCLIError(err)
		if ce.Code != "REVIEW_PARTIAL_FAILURE" || ce.Category != cerrors.CategoryPermission || ce.HTTPStatus != http.StatusForbidden {
			t.Fatalf("err = %+v, want permission/REVIEW_PARTIAL_FAILURE/403", ce)
		}
		var res apiclient.PRReviewResult
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("decode vote-only failure stdout: %v\nraw: %s", err, out)
		}
		if len(res.PostedComments) != 0 || res.VoteApplied || res.FailedItem == nil ||
			res.FailedItem.Kind != "vote" || res.FailedItem.Error == nil || res.FailedItem.Error.Code != "HTTP_Forbidden" {
			t.Fatalf("unexpected vote-only failure result: %+v", res)
		}
	})

	t.Run("duplicate or case-divergent comments keys abort before any server write", func(t *testing.T) {
		for _, stdin := range []string{
			`{"verdict":"approve","comments":[{"content":"blocker"}],"comments":[]}`,
			`{"verdict":"approve","comments":[{"content":"blocker"}],"Comments":[]}`,
		} {
			atomic.StoreInt32(&writes, 0)
			_, err := runCommandCapture(t, false, stdin, srv.URL, "pr", "review", "PROJ/demo/1", "--input", "-")
			if err == nil {
				t.Fatalf("stdin=%s: expected BAD_REVIEW_INPUT", stdin)
			}
			if ce := cerrors.AsCLIError(err); ce.Code != "BAD_REVIEW_INPUT" {
				t.Fatalf("stdin=%s: code = %s, want BAD_REVIEW_INPUT", stdin, ce.Code)
			}
			if atomic.LoadInt32(&writes) != 0 {
				t.Fatalf("stdin=%s: sent %d writes, want 0", stdin, writes)
			}
		}
	})
}

func TestBatchReadAndCommentMetadataCLI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/1.0/application-properties":
			w.Header().Set("X-AUSERNAME", "alice")
			_, _ = w.Write([]byte(`{"version":"8.19.0"}`))
		case "/rest/api/1.0/users/alice":
			_, _ = w.Write([]byte(`{"name":"alice","slug":"alice","displayName":"Alice","type":"NORMAL"}`))
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/1":
			_, _ = w.Write([]byte(`{"id":1,"title":"PR One","state":"OPEN","fromRef":{"displayId":"a","latestCommit":"1111"},"toRef":{"displayId":"main","latestCommit":"0000"}}`))
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/2":
			_, _ = w.Write([]byte(`{"id":2,"title":"PR Two","state":"OPEN","fromRef":{"displayId":"b","latestCommit":"2222"},"toRef":{"displayId":"main","latestCommit":"0000"}}`))
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/1/commits":
			_, _ = w.Write([]byte(`{"values":[{"id":"c1","message":"first"}],"isLastPage":false,"nextPageStart":1}`))
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/2/commits":
			_, _ = w.Write([]byte(`{"values":[{"id":"c2","message":"only"}],"isLastPage":true}`))
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/1/activities":
			_, _ = w.Write([]byte(`{
				"values": [
					{"id":10,"action":"COMMENTED","comment":{"id":51,"text":"[[AI]](https://example.com) Looks good","author":{"name":"alice","slug":"alice","type":"NORMAL"}}},
					{"id":11,"action":"COMMENTED","comment":{"id":52,"text":"Build green","author":{"name":"ci-bot","slug":"ci-bot","type":"SERVICE"}}}
				],
				"isLastPage": false,
				"nextPageStart": 2
			}`))
		case "/rest/api/1.0/projects/PROJ/repos/demo/pull-requests/2/activities":
			_, _ = w.Write([]byte(`{"values":[],"isLastPage":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"message":"not found"}]}`))
		}
	}))
	defer srv.Close()

	t.Run("pr get batch via stdin handles partial failure", func(t *testing.T) {
		out, err := runCommandCapture(t, false, "PROJ/demo/1\nPROJ/demo/999\nPROJ/demo/2\n", srv.URL, "pr", "get", "-")
		if err == nil {
			t.Fatal("expected BATCH_PARTIAL_FAILURE")
		}
		if ce := cerrors.AsCLIError(err); ce.Code != "BATCH_PARTIAL_FAILURE" {
			t.Fatalf("code = %s, want BATCH_PARTIAL_FAILURE", ce.Code)
		}
		var env struct {
			Items []batchItem `json:"items"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Fatalf("decode: %v\nraw: %s", err, out)
		}
		if len(env.Items) != 3 || !env.Items[0].OK || env.Items[1].OK || !env.Items[2].OK {
			t.Fatalf("unexpected batch items: %+v", env.Items)
		}
	})

	t.Run("pr get batch --scope commits and activity preserve per-PR pagination", func(t *testing.T) {
		for _, tc := range []struct {
			scope    string
			wantNext string
		}{
			{scope: "commits", wantNext: "1"},
			{scope: "activity", wantNext: "2"},
		} {
			out, err := runCommandCapture(t, false, "", srv.URL, "pr", "get", "--scope", tc.scope, "PROJ/demo/1", "PROJ/demo/2")
			if err != nil {
				t.Fatalf("scope=%s error: %v", tc.scope, err)
			}
			var env struct {
				Items []struct {
					Ref    string `json:"ref"`
					OK     bool   `json:"ok"`
					Result struct {
						Items   []json.RawMessage `json:"items"`
						Next    string            `json:"next"`
						HasMore bool              `json:"has_more"`
					} `json:"result"`
				} `json:"items"`
			}
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Fatalf("scope=%s decode: %v\nraw: %s", tc.scope, err, out)
			}
			if len(env.Items) != 2 {
				t.Fatalf("scope=%s len=%d", tc.scope, len(env.Items))
			}
			if !env.Items[0].Result.HasMore || env.Items[0].Result.Next != tc.wantNext {
				t.Errorf("scope=%s PR 1 pagination = %+v, want has_more=true next=%q", tc.scope, env.Items[0].Result, tc.wantNext)
			}
			if env.Items[1].Result.HasMore || env.Items[1].Result.Next != "" {
				t.Errorf("scope=%s PR 2 pagination = %+v, want has_more=false next=\"\"", tc.scope, env.Items[1].Result)
			}
		}
	})

	t.Run("comment list projects is_self, is_ai_attributed, author_kind", func(t *testing.T) {
		out, err := runCommandCapture(t, false, "", srv.URL, "comment", "list", "--pr", "PROJ/demo/1",
			"--fields", "id,is_self,is_ai_attributed,author_kind")
		if err != nil {
			t.Fatalf("comment list error: %v", err)
		}
		var env struct {
			Items []struct {
				ID             int    `json:"id"`
				IsSelf         *bool  `json:"is_self"`
				IsAIAttributed bool   `json:"is_ai_attributed"`
				AuthorKind     string `json:"author_kind"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Fatalf("decode: %v\nraw: %s", err, out)
		}
		if len(env.Items) != 2 {
			t.Fatalf("got %d comments, want 2: %s", len(env.Items), out)
		}
		// Comments are reversed into chronological order or returned as listed; match by ID.
		byID := map[int]struct {
			isSelf *bool
			isAI   bool
			kind   string
		}{}
		for _, c := range env.Items {
			byID[c.ID] = struct {
				isSelf *bool
				isAI   bool
				kind   string
			}{c.IsSelf, c.IsAIAttributed, c.AuthorKind}
		}
		c51 := byID[51]
		if c51.isSelf == nil || !*c51.isSelf || !c51.isAI || c51.kind != "human" {
			t.Fatalf("comment 51 = %+v, want is_self=true is_ai=true kind=human", c51)
		}
		c52 := byID[52]
		if c52.isSelf == nil || *c52.isSelf || c52.isAI || c52.kind != "bot" {
			t.Fatalf("comment 52 = %+v, want is_self=false is_ai=false kind=bot", c52)
		}
	})
}

func TestPRFetchOfflineBaseCloudAutoFlavorAndRedaction(t *testing.T) {
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("remote", "add", "origin", "https://x-token-auth:secret-pass@bitbucket.org/ws/demo.git?tok=1")

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// Default flavor=auto with Cloud base URL https://api.bitbucket.org and --base main (offline).
	out, err := runCommandCaptureWithFlavor(t, false, "", "https://api.bitbucket.org", "auto",
		"pr", "fetch", "ws/demo/1", "--base", "main")
	if err != nil {
		t.Fatalf("offline Cloud pr fetch failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode: %v\nraw: %s", err, out)
	}
	if payload["remote"] != "origin" {
		t.Errorf("remote = %v, want origin", payload["remote"])
	}
	if payload["remote_url"] != "https://bitbucket.org/ws/demo.git" {
		t.Errorf("remote_url = %v, want redacted https://bitbucket.org/ws/demo.git", payload["remote_url"])
	}
	if strings.Contains(out, "secret-pass") || strings.Contains(out, "x-token-auth") {
		t.Errorf("stdout leaked remote credentials: %s", out)
	}
}

func TestPRFetchOfflineFullPRURLVerifiesHostWithoutServerConfig(t *testing.T) {
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("remote", "add", "origin", "git@other-host.example:ws/demo.git")

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// No --base-url configured, full bitbucket.org PR URL passed with --base main:
	// must reject the same-named repo on other-host.example!
	_, err = runCommandCaptureWithFlavor(t, false, "", "", "",
		"pr", "fetch", "https://bitbucket.org/ws/demo/pull-requests/1", "--base", "main")
	if err == nil {
		t.Fatal("expected GIT_REMOTE_MISMATCH when PR URL host differs from local remote host")
	}
	if ce := cerrors.AsCLIError(err); ce.Code != "GIT_REMOTE_MISMATCH" {
		t.Fatalf("got %s (%v), want GIT_REMOTE_MISMATCH", ce.Code, ce)
	}

	// Once a matching bitbucket.org remote exists, it is selected even without server config.
	runGit("remote", "add", "upstream", "git@bitbucket.org:ws/demo.git")
	out, err := runCommandCaptureWithFlavor(t, false, "", "", "",
		"pr", "fetch", "https://bitbucket.org/ws/demo/pull-requests/1", "--base", "main")
	if err != nil {
		t.Fatalf("expected matching bitbucket.org upstream to succeed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode: %v\nraw: %s", err, out)
	}
	if payload["remote"] != "upstream" {
		t.Fatalf("remote = %v, want upstream", payload["remote"])
	}
}

func TestPRFetchAndCheckoutOnlinePRURLRejectsMismatchedServer(t *testing.T) {
	var apiHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 1, "state": "OPEN",
			"fromRef": {"displayId": "feat", "latestCommit": "1111", "repository": {"slug": "demo", "project": {"key": "ws"}}},
			"toRef":   {"displayId": "main", "latestCommit": "0000", "repository": {"slug": "demo", "project": {"key": "ws"}}}
		}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	// Local checkout matches the configured DC test server, NOT bitbucket.org.
	runGit("remote", "add", "origin", srv.URL+"/scm/ws/demo.git")

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	for _, subcmd := range []string{"fetch", "checkout"} {
		atomic.StoreInt32(&apiHits, 0)
		_, err := runCommandCapture(t, false, "", srv.URL,
			"pr", subcmd, "https://bitbucket.org/ws/demo/pull-requests/1")
		if err == nil {
			t.Fatalf("pr %s: expected PR_SERVER_MISMATCH for bitbucket.org URL against %s", subcmd, srv.URL)
		}
		if ce := cerrors.AsCLIError(err); ce.Code != "PR_SERVER_MISMATCH" || ce.Category != cerrors.CategoryUsage {
			t.Fatalf("pr %s: got %s/%s (%v), want usage/PR_SERVER_MISMATCH", subcmd, ce.Category, ce.Code, ce)
		}
		if atomic.LoadInt32(&apiHits) != 0 {
			t.Fatalf("pr %s: must abort before querying mismatched server, got %d API hits", subcmd, apiHits)
		}
	}
}
