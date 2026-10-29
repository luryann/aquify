package dbutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRows is a minimal pgx.Rows over a slice of ints.
type fakeRows struct {
	pgx.Rows
	data   []int
	pos    int
	err    error
	closed bool
}

func (f *fakeRows) Next() bool {
	if f.pos >= len(f.data) {
		return false
	}
	f.pos++
	return true
}
func (f *fakeRows) Close()                        { f.closed = true }
func (f *fakeRows) Err() error                    { return f.err }
func (f *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func TestCollectRows(t *testing.T) {
	rows := &fakeRows{data: []int{1, 2, 3}}
	got, err := CollectRows(rows, "scan", func(r pgx.Rows) (int, error) { return r.(*fakeRows).data[r.(*fakeRows).pos-1] * 10, nil })
	if err != nil || len(got) != 3 || got[0] != 10 || got[2] != 30 || !rows.closed {
		t.Fatalf("got %v err %v closed %v", got, err, rows.closed)
	}
}

func TestCollectRowsEmptyIsNil(t *testing.T) {
	got, err := CollectRows(&fakeRows{}, "scan", func(pgx.Rows) (int, error) { return 0, nil })
	if err != nil || got != nil {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestCollectRowsScanErrorWrapsContext(t *testing.T) {
	rows := &fakeRows{data: []int{1}}
	boom := errors.New("boom")
	_, err := CollectRows(rows, "comms: scan thing", func(pgx.Rows) (int, error) { return 0, boom })
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "comms: scan thing: ") || !rows.closed {
		t.Fatalf("err %v closed %v", err, rows.closed)
	}
}

func TestCollectRowsReturnsIterationError(t *testing.T) {
	iter := errors.New("iteration")
	_, err := CollectRows(&fakeRows{data: []int{1}, err: iter}, "scan", func(pgx.Rows) (int, error) { return 1, nil })
	if !errors.Is(err, iter) {
		t.Fatalf("err %v", err)
	}
}

func TestCollectRowsNonNil(t *testing.T) {
	got, err := CollectRowsNonNil(&fakeRows{}, "scan", func(pgx.Rows) (int, error) { return 0, nil })
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v err %v", got, err)
	}
	if _, err := CollectRowsNonNil(&fakeRows{data: []int{1}}, "scan", func(pgx.Rows) (int, error) { return 0, errors.New("x") }); err == nil {
		t.Fatal("expected error")
	}
}
