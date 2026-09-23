package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestListPaginationNotice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		items    []map[string]any
		hasMore  bool
		nextFlag string
	}{
		{"projected page", []map[string]any{{"name": "record", "extra": true}}, true, ""},
		{"empty filtered page", nil, true, ""},
		{"offset continuation", nil, true, "--offset"},
		{"complete page", []map[string]any{{"name": "record"}}, false, ""},
		{"complete empty page", nil, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data, notices bytes.Buffer
			if err := EmitList(tc.items, "opaque<cursor>", tc.hasMore, Options{
				Format: FormatNDJSON, Writer: &data, NoticeWriter: &notices,
				Fields: []string{"name"}, NextFlag: tc.nextFlag, Pretty: true,
			}); err != nil {
				t.Fatal(err)
			}
			wantRows := strings.Repeat("{\"name\":\"record\"}\n", len(tc.items))
			if data.String() != wantRows {
				t.Fatalf("stdout = %q, want %q", data.String(), wantRows)
			}
			if !tc.hasMore {
				if notices.Len() != 0 {
					t.Fatalf("complete page emitted notice: %s", &notices)
				}
				return
			}
			var notice struct {
				Notice struct {
					Pagination struct {
						Next    string `json:"next"`
						HasMore bool   `json:"has_more"`
					} `json:"pagination"`
					NextSteps []string `json:"next_steps"`
				} `json:"_notice"`
			}
			if err := json.Unmarshal(notices.Bytes(), &notice); err != nil {
				t.Fatal(err)
			}
			if notice.Notice.Pagination.Next != "opaque<cursor>" || !notice.Notice.Pagination.HasMore {
				t.Fatalf("continuation lost: %s", &notices)
			}
			nextFlag := tc.nextFlag
			if nextFlag == "" {
				nextFlag = "--cursor"
			}
			if len(notice.Notice.NextSteps) != 1 || notice.Notice.NextSteps[0] != "Pass next as "+nextFlag+" to retrieve the next page." {
				t.Fatalf("wrong recovery guidance: %s", &notices)
			}
			if bytes.Count(notices.Bytes(), []byte("\n")) != 1 || bytes.Contains(notices.Bytes(), []byte("\\u003c")) {
				t.Fatalf("notice is not a compact unescaped line: %q", notices.String())
			}
		})
	}
}

type paginationFailWriter struct{ err error }

func (w paginationFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestListPaginationNoticeWriteFailures(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("write failed")
	var notices bytes.Buffer
	err := EmitList([]string{"row"}, "next", true, Options{
		Format: FormatNDJSON, Writer: paginationFailWriter{wantErr}, NoticeWriter: &notices,
	})
	if !errors.Is(err, wantErr) || notices.Len() != 0 {
		t.Fatalf("failed stdout must suppress continuation: err=%v notice=%q", err, notices.String())
	}
	var data bytes.Buffer
	if err := EmitList([]string{"row"}, "next", true, Options{
		Format: FormatNDJSON, Writer: &data, NoticeWriter: paginationFailWriter{wantErr},
	}); err != nil {
		t.Fatalf("notice failure changed successful data result: %v", err)
	}
}

func TestListPaginationOtherFormats(t *testing.T) {
	t.Parallel()
	for _, format := range []string{FormatJSON, FormatTable} {
		t.Run(format, func(t *testing.T) {
			var data, notices bytes.Buffer
			if err := EmitList([]map[string]any{{"name": "record"}}, "42", true, Options{
				Format: format, Writer: &data, NoticeWriter: &notices, NextFlag: "--offset",
			}); err != nil {
				t.Fatal(err)
			}
			if notices.Len() != 0 {
				t.Fatalf("unexpected NDJSON notice: %s", &notices)
			}
			if format == FormatTable {
				if !strings.Contains(data.String(), "--offset 42") || strings.Contains(data.String(), "--cursor") {
					t.Fatalf("incorrect table continuation: %s", &data)
				}
				return
			}
			var envelope map[string]any
			if err := json.Unmarshal(data.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope) != 3 || envelope["next"] != "42" || envelope["has_more"] != true {
				t.Fatalf("JSON envelope changed: %s", &data)
			}
		})
	}
}
