package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestQueryIntAndBoolRejectMalformedValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		query  string
		want   int
		wantOK bool
	}{
		{"", 0, true},
		{"distance=100", 100, true},
		{"distance=abc", 0, false},
		{"distance=-5", 0, false},
		{"distance=1.5", 0, false},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/x?"+tc.query, nil)
		got, ok := queryInt(rec, req, "distance")
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("%q: got (%d,%v) want (%d,%v)", tc.query, got, ok, tc.want, tc.wantOK)
		}
		if !tc.wantOK && rec.Code != 400 {
			t.Errorf("%q: malformed value must answer 400, got %d", tc.query, rec.Code)
		}
	}

	for _, tc := range []struct {
		query  string
		want   bool
		wantOK bool
	}{
		{"", false, true}, {"active_only=true", true, true}, {"active_only=1", true, true},
		{"active_only=false", false, true}, {"active_only=maybe", false, false},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/x?"+tc.query, nil)
		got, ok := queryBool(rec, req, "active_only")
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("%q: got (%v,%v) want (%v,%v)", tc.query, got, ok, tc.want, tc.wantOK)
		}
		if !tc.wantOK && rec.Code != 400 {
			t.Errorf("%q: malformed bool must answer 400, got %d", tc.query, rec.Code)
		}
	}
}
