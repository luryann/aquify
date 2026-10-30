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
	// 10 core columns + commitment_id, is_relay, proposed_by, approved_by,
	// created_by, updated_by, scratch_reason.
	if len(db.args) != 17 {
		t.Fatalf("expected 17 arguments, got %d", len(db.args))
	}
	if db.args[7] != "" {
		t.Fatalf("expected empty string notes argument, got %#v", db.args[7])
	}
}

func TestUpsertEntryConflictUpdatesMeetEventID(t *testing.T) {
	t.Parallel()

	db := &recordingExecer{}
	if err := upsertEntry(context.Background(), db, Entry{ID: "e", Status: "submitted"}); err != nil {
		t.Fatalf("upsertEntry returned error: %v", err)
	}
	// A coach override moves an entry to a different event; without this the
	// change is lost on the next reload from Postgres.
	if !strings.Contains(db.query, "meet_event_id  = EXCLUDED.meet_event_id") {
		t.Fatalf("ON CONFLICT clause must update meet_event_id, got %q", db.query)
	}
}

func TestUpsertEntryPersistsLineageAndPreservesEntryTime(t *testing.T) {
	t.Parallel()

	db := &recordingExecer{}
	e := Entry{
		ID: "e", Status: "scratched", CommitmentID: "c1", IsRelay: true,
		ProposedBy: "u1", ApprovedBy: "u2", ScratchReason: "injury",
	}
	if err := upsertEntry(context.Background(), db, e); err != nil {
		t.Fatalf("upsertEntry returned error: %v", err)
	}
	for _, want := range []string{"commitment_id", "is_relay", "proposed_by", "approved_by", "scratch_reason"} {
		if !strings.Contains(db.query, want+" ") && !strings.Contains(db.query, want+"\n") && !strings.Contains(db.query, want+",") {
			t.Fatalf("query must persist %s: %q", want, db.query)
		}
	}
	if strings.Contains(db.query, "entry_time_ms") {
		t.Fatalf("upsert must not touch entry_time_ms (was reset to NULL on conflict): %q", db.query)
	}
	got := map[any]bool{}
	for _, a := range db.args {
		got[a] = true
	}
	for _, want := range []any{"c1", true, "u1", "u2", "injury"} {
		if !got[want] {
			t.Fatalf("args missing %v: %#v", want, db.args)
		}
	}
}
