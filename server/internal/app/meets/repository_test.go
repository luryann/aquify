package meets

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingExecer struct {
	query string
	args  []any
}

func (r *recordingExecer) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	r.query = sql
	r.args = arguments
	return pgconn.CommandTag{}, nil
}

func TestUpsertEntryPersistsEmptyOverrideReasonAsEmptyNotes(t *testing.T) {
	t.Parallel()

	db := &recordingExecer{}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	entry := Entry{
		ID:          "00000000-0000-0000-0000-000000000001",
		OrgID:       "00000000-0000-0000-0000-000000000002",
		MeetID:      "00000000-0000-0000-0000-000000000003",
		SwimmerID:   "00000000-0000-0000-0000-000000000004",
		MeetEventID: "00000000-0000-0000-0000-000000000005",
		Status:      "draft",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := upsertEntry(context.Background(), db, entry); err != nil {
		t.Fatalf("upsertEntry returned error: %v", err)
	}

	if !strings.Contains(db.query, "INSERT INTO entries") {
		t.Fatalf("expected entries insert, got query %q", db.query)
	}
	if len(db.args) != 11 {
		t.Fatalf("expected 11 arguments, got %d", len(db.args))
	}
	if db.args[8] != "" {
		t.Fatalf("expected empty string notes argument, got %#v", db.args[8])
	}
}
