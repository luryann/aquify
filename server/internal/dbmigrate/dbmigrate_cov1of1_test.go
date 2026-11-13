package dbmigrate

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func cov1xOKFS() fstest.MapFS {
	return fstest.MapFS{"00001_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 1;\n")}}
}

func TestCov1xEmptyDSNRejected(t *testing.T) {
	for _, dsn := range []string{"", "   \t"} {
		if _, err := Up(context.Background(), dsn, cov1xOKFS()); err == nil || !strings.Contains(err.Error(), "database URL is empty") {
			t.Fatalf("Up(%q) err = %v", dsn, err)
		}
		if _, err := Status(context.Background(), dsn, cov1xOKFS()); err == nil || !strings.Contains(err.Error(), "database URL is empty") {
			t.Fatalf("Status(%q) err = %v", dsn, err)
		}
	}
}

func TestCov1xInvalidDSNReturnsGenericParseErrorWithoutEchoing(t *testing.T) {
	const bad = "postgres://user:topsecret@host:notaport/db"
	_, err := Up(context.Background(), bad, cov1xOKFS())
	if !errors.Is(err, errParse) {
		t.Fatalf("err = %v, want errParse", err)
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("parse error leaks password: %v", err)
	}
}

func TestCov1xDuplicateVersionsFailProviderCreation(t *testing.T) {
	dsn := freshSchemaDSN(t)
	fsys := fstest.MapFS{
		"00001_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")},
		"00001_b.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")},
	}
	_, err := Up(context.Background(), dsn, fsys)
	if err == nil || !strings.Contains(err.Error(), "dbmigrate:") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v, want duplicate-version provider error", err)
	}
}

func TestCov1xStatusReportsUnappliedThenErrorsWhenBookkeepingCannotBeRead(t *testing.T) {
	dsn := freshSchemaDSN(t)
	st, err := Status(context.Background(), dsn, cov1xOKFS())
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 1 || st[0].Version != 1 || st[0].Applied {
		t.Fatalf("status = %+v, want one unapplied version 1", st)
	}

	// A search_path naming no existing schema leaves goose nowhere to create
	// its bookkeeping table, so Status must surface a wrapped error.
	base := os.Getenv("DATABASE_URL")
	u, perr := url.Parse(base)
	if perr != nil {
		t.Fatal(perr)
	}
	q := u.Query()
	q.Set("search_path", "cov1x_missing_schema_does_not_exist")
	u.RawQuery = q.Encode()
	_, err = Status(context.Background(), u.String(), cov1xOKFS())
	if err == nil || !strings.HasPrefix(err.Error(), "dbmigrate: status:") {
		t.Fatalf("err = %v, want dbmigrate: status: prefix", err)
	}
}

func TestCov1xScrub(t *testing.T) {
	if got := scrub(nil, "postgres://u:p@h/db"); got != nil {
		t.Fatalf("scrub(nil) = %v", got)
	}
	orig := errors.New("boom")
	if got := scrub(orig, ""); got != orig {
		t.Fatalf("empty dsn should return the error unchanged, got %v", got)
	}
	if got := scrub(orig, "postgres://u:p@h/db"); got != orig {
		t.Fatalf("unrelated message should return the same error, got %v", got)
	}
	dsn := "postgres://u:pw123@h/db"
	got := scrub(errors.New("dial "+dsn+" failed; pw123 rejected"), dsn)
	if got.Error() != "dial [redacted-dsn] failed; [redacted] rejected" {
		t.Fatalf("scrubbed = %q", got.Error())
	}
}

func TestCov1xDSNPassword(t *testing.T) {
	cases := map[string]string{
		"postgres://u:pw@h/db": "pw",
		"postgres://u@h/db":    "",
		"postgres://h/db":      "",
		"::not a url":          "",
	}
	for in, want := range cases {
		if got := dsnPassword(in); got != want {
			t.Errorf("dsnPassword(%q) = %q, want %q", in, got, want)
		}
	}
}
