package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyRejectsUnknownTrailingAndOversizedBodies(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name    string
		body    string
		opts    decodeJSONOptions
		wantErr string
	}{
		{
			name:    "unknown field",
			body:    `{"name":"ok","extra":true}`,
			wantErr: `json: unknown field "extra"`,
		},
		{
			name:    "trailing object",
			body:    `{"name":"ok"}{"name":"again"}`,
			wantErr: errSingleJSONObject.Error(),
		},
		{
			name:    "oversized",
			body:    `{"name":"too long"}`,
			opts:    decodeJSONOptions{MaxBytes: 4},
			wantErr: "request body must not exceed 4 bytes",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("POST", "/test", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			var got payload
			err := decodeJSONBodyWithOptions(rec, req, &got, tc.opts)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("decodeJSONBodyWithOptions() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseListPaginationClampsAndSlices(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "/items?limit=999&offset=2", nil)
	pagination := parseListPagination(req, listPaginationOptions{DefaultLimit: 3, MaxLimit: 4})
	if pagination.Limit != 4 || pagination.Offset != 2 {
		t.Fatalf("parseListPagination() = %+v, want limit 4 offset 2", pagination)
	}

	page, total := paginateSlice([]int{1, 2, 3, 4, 5}, pagination)
	if total != 5 {
		t.Fatalf("paginateSlice() total = %d, want 5", total)
	}
	if len(page) != 3 || page[0] != 3 || page[2] != 5 {
		t.Fatalf("paginateSlice() page = %#v, want [3 4 5]", page)
	}
}
