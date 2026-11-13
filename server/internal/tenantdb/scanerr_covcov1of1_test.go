package tenantdb

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// cov1xFailedRows is a pgx.Rows that yields no rows and reports a stream error, which the
// real driver surfaces only for failures that arrive after the query was accepted.
type cov1xFailedRows struct {
	pgx.Rows
	err    error
	closed bool
}

func (r *cov1xFailedRows) Next() bool { return false }
func (r *cov1xFailedRows) Err() error { return r.err }
func (r *cov1xFailedRows) Close()     { r.closed = true }

func TestCov1xTenantdbScanReportsStreamErrorInsteadOfNoRows(t *testing.T) {
	boom := errors.New("stream broke")
	rows := &cov1xFailedRows{err: boom}
	var x int
	err := (&scopedRow{rows: rows}).Scan(&x)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the stream error", err)
	}
	if !rows.closed {
		t.Fatal("rows must be closed after Scan")
	}
}
