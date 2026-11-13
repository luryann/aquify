package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

var cov1xT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

type cov1xDB struct {
	pool *pgxpool.Pool
	svc  *Service
	repo *Repository
	org  string
}

// cov1xShared is the one isolated schema every DB case runs against (creating
// ~hundreds of tables per schema is expensive). Each case uses a fresh org and
// reverts any fault it injected, so cases do not depend on each other.
var cov1xShared *pgxpool.Pool

func cov1xNewDB(t *testing.T) *cov1xDB {
	t.Helper()
	pool := cov1xShared
	if pool == nil {
		t.Fatal("cov1x DB cases must run through TestCov1xDBSuite")
	}
	svc := NewServiceWithPool(pool)
	svc.SetClock(func() time.Time { return cov1xT0 })
	return &cov1xDB{pool: pool, svc: svc, repo: svc.repo, org: uuid.NewString()}
}

// cov1xDead returns a service/repository whose pool is already closed, so every
// statement fails with a connection error.
func cov1xDead(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()
	dead, err := pgxpool.NewWithConfig(context.Background(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	dead.Close()
	svc := NewServiceWithPool(dead)
	svc.SetClock(func() time.Time { return cov1xT0 })
	return svc
}

func (d *cov1xDB) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := d.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// failOn installs a trigger on table that raises for the given events/WHEN
// condition. deferred triggers fire at COMMIT, so they exercise commit errors.
var cov1xTriggerSeq atomic.Int64

func (d *cov1xDB) failOn(t *testing.T, table, events, when string, deferred bool) {
	t.Helper()
	n := cov1xTriggerSeq.Add(1)
	fn := fmt.Sprintf("cov1x_fail_%d", n)
	d.exec(t, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cov1x injected failure'; END $$`, fn))
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, fn, table))
		_, _ = d.pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, fn))
	})
	if deferred {
		d.exec(t, fmt.Sprintf(`CREATE CONSTRAINT TRIGGER %s AFTER %s ON %s DEFERRABLE INITIALLY DEFERRED FOR EACH ROW %s EXECUTE FUNCTION %s()`, fn, events, table, when, fn))
		return
	}
	d.exec(t, fmt.Sprintf(`CREATE TRIGGER %s BEFORE %s ON %s FOR EACH ROW %s EXECUTE FUNCTION %s()`, fn, events, table, when, fn))
}

// rename renames a column to inject a query failure and restores it afterwards.
func (d *cov1xDB) rename(t *testing.T, table, from, to string) {
	t.Helper()
	d.exec(t, fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, table, from, to))
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, table, to, from))
	})
}

// guestCountAsText makes club_event_rsvps.guest_count unscannable into an int.
func (d *cov1xDB) guestCountAsText(t *testing.T) {
	t.Helper()
	d.exec(t, `ALTER TABLE club_event_rsvps DROP CONSTRAINT club_event_rsvps_guest_count_check`)
	d.exec(t, `ALTER TABLE club_event_rsvps ALTER COLUMN guest_count TYPE text USING guest_count::text`)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = d.pool.Exec(ctx, `ALTER TABLE club_event_rsvps ALTER COLUMN guest_count TYPE integer USING guest_count::integer`)
		_, _ = d.pool.Exec(ctx, `ALTER TABLE club_event_rsvps ADD CONSTRAINT club_event_rsvps_guest_count_check CHECK (guest_count >= 0 AND guest_count <= 50)`)
	})
}

func (d *cov1xDB) newEvent(t *testing.T, mutate func(*EventInput)) *Event {
	t.Helper()
	in := validInput()
	in.StartsAt = cov1xT0.Add(10 * 24 * time.Hour)
	e := in.StartsAt.Add(2 * time.Hour)
	in.EndsAt = &e
	if mutate != nil {
		mutate(&in)
	}
	ev, err := d.svc.Create(d.org, "admin", in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return ev
}

func cov1xWantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got %v", substr, err)
	}
}

func cov1xCaseRepoEventRoundTrip(t *testing.T) {
	d := cov1xNewDB(t)
	dl := cov1xT0.Add(24 * time.Hour)
	ev := d.newEvent(t, func(in *EventInput) {
		in.Capacity = intPtr(12)
		in.RSVPDeadline = &dl
		in.FeeCents = 750
		in.Public = true
		in.Audience = "groups"
		in.GroupCodes = []string{"gold", "silver"}
		in.Description = "desc"
		in.Location = "pool"
	})
	got, err := d.svc.Get(d.org, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Capacity == nil || *got.Capacity != 12 || got.RSVPDeadline == nil || !got.RSVPDeadline.Equal(dl) ||
		got.EndsAt == nil || got.FeeCents != 750 || !got.Public || strings.Join(got.GroupCodes, ",") != "GOLD,SILVER" ||
		got.StartsAt.Location() != time.UTC || got.Location != "pool" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	plain := d.newEvent(t, func(in *EventInput) { in.RSVPEnabled = false; in.EndsAt = nil })
	got, _ = d.svc.Get(d.org, plain.ID)
	if got.Capacity != nil || got.EndsAt != nil || got.RSVPDeadline != nil || got.GroupCodes == nil || len(got.GroupCodes) != 0 {
		t.Fatalf("expected nil optionals and empty non-nil codes: %+v", got)
	}

	// Lookups: malformed id, unknown id, other org.
	for _, id := range []string{"not-a-uuid", uuid.NewString()} {
		if _, err := d.svc.Get(d.org, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get %q: %v", id, err)
		}
	}
	if _, err := d.svc.Get(uuid.NewString(), ev.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}

	list, err := d.svc.List(d.org, ListFilter{})
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %v", list, err)
	}
	if other, _ := d.svc.List(uuid.NewString(), ListFilter{}); len(other) != 0 {
		t.Fatal("list leaked across orgs")
	}
}

func cov1xCaseUpsertNilGroupCodes(t *testing.T) {
	d := cov1xNewDB(t)
	now := cov1xT0
	e := &Event{ID: uuid.NewString(), OrgID: d.org, Title: "Nil codes", Category: "social", StartsAt: now,
		Audience: "all", RSVPMode: "household", Status: "scheduled", CreatedAt: now, UpdatedAt: now}
	if err := d.repo.UpsertEvent(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	got, err := d.svc.Get(d.org, e.ID)
	if err != nil || got.GroupCodes == nil || len(got.GroupCodes) != 0 {
		t.Fatalf("nil codes must persist as an empty array: %+v %v", got, err)
	}
	// Upserting again by id updates in place and cannot cross orgs.
	e.Title = "Renamed"
	if err := d.repo.UpsertEvent(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.svc.Get(d.org, e.ID); got.Title != "Renamed" {
		t.Fatalf("upsert must update: %+v", got)
	}
}

func cov1xCaseRepoDeleteEvent(t *testing.T) {
	d := cov1xNewDB(t)
	ctx := context.Background()
	ev := d.newEvent(t, nil)
	if err := d.repo.DeleteEvent(ctx, d.org, "bad"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	if err := d.repo.DeleteEvent(ctx, uuid.NewString(), ev.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross org: %v", err)
	}
	if _, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true); err != nil {
		t.Fatal(err)
	}
	if err := d.repo.DeleteEvent(ctx, d.org, ev.ID); err != nil {
		t.Fatal(err)
	}
	if rs, _ := d.repo.ListRSVPs(ctx, d.org, ev.ID); len(rs) != 0 {
		t.Fatal("rsvps must cascade")
	}
	if err := d.repo.DeleteEvent(ctx, d.org, ev.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	d.rename(t, "club_events", "org_id", "org_id_x")
	cov1xWantErr(t, d.repo.DeleteEvent(ctx, d.org, uuid.NewString()), "club events: delete event")
}

func cov1xCaseServiceDBEventLifecycleAndBilling(t *testing.T) {
	d := cov1xNewDB(t)
	biller := &cov1xBiller{}
	d.svc.SetBiller(biller)
	ev := d.newEvent(t, func(in *EventInput) { in.FeeCents = 400; in.Capacity = intPtr(3) })

	res, err := d.svc.Respond(d.org, ev.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes", GuestCount: 1}, true)
	if err != nil || res.Billing != BillingCharged || res.RSVP.InvoiceID != "cov1x-inv" || res.RSVP.BilledQuantity != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	stored, _ := d.svc.ListRSVPs(d.org, ev.ID)
	if len(stored) != 1 || stored[0].InvoiceID != "cov1x-inv" || stored[0].BilledQuantity != 2 {
		t.Fatalf("billing must persist: %+v", stored)
	}
	// Capacity enforced under lock.
	if _, err := d.svc.Respond(d.org, ev.ID, "p2", RSVPInput{HouseholdID: "h2", Response: "yes", GuestCount: 1}, true); !errors.Is(err, ErrEventFull) {
		t.Fatalf("expected full: %v", err)
	}
	// Existing response is updated in place (same row).
	res2, err := d.svc.Respond(d.org, ev.ID, "p1", RSVPInput{HouseholdID: "h1", Response: "yes", GuestCount: 2, Note: "n"}, true)
	if err != nil || res2.RSVP.ID != res.RSVP.ID || res2.RSVP.Note != "n" {
		t.Fatalf("%+v %v", res2, err)
	}
	if _, err := d.svc.Respond(d.org, ev.ID, "p9", RSVPInput{HouseholdID: "h9", Response: "no"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := d.svc.Respond(d.org, "bad-id", "p", RSVPInput{HouseholdID: "h", Response: "no"}, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}

	byEvent, err := d.svc.ListRSVPsForEvents(d.org, []string{ev.ID})
	if err != nil || len(byEvent[ev.ID]) != 2 {
		t.Fatalf("grouped: %v %v", byEvent, err)
	}
	if rs, err := d.repo.ListRSVPs(context.Background(), d.org, "bad"); err != nil || len(rs) != 0 {
		t.Fatalf("bad event id list: %v %v", rs, err)
	}

	// Price change re-bills confirmed RSVPs via the DB path.
	in := validInput()
	in.StartsAt = ev.StartsAt
	in.EndsAt = ev.EndsAt
	in.Capacity = intPtr(10)
	in.FeeCents = 900
	updated, err := d.svc.Update(d.org, ev.ID, "admin2", in)
	if err != nil || updated.FeeCents != 900 || updated.UpdatedBy != "admin2" {
		t.Fatalf("%+v %v", updated, err)
	}
	if last := biller.charges[len(biller.charges)-1]; last.UnitAmountCents != 900 || last.Quantity != 3 {
		t.Fatalf("rebill: %+v", last)
	}
	// Mode locked, capacity floor and unknown event errors.
	in.RSVPMode = "swimmer"
	if _, err := d.svc.Update(d.org, ev.ID, "a", in); !errors.Is(err, ErrRSVPModeLocked) {
		t.Fatalf("mode: %v", err)
	}
	in.RSVPMode = "household"
	in.Capacity = intPtr(2)
	if _, err := d.svc.Update(d.org, ev.ID, "a", in); !errors.Is(err, ErrCapacityBelowYes) {
		t.Fatalf("cap: %v", err)
	}
	if _, err := d.svc.Update(d.org, uuid.NewString(), "a", in); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := d.svc.Update(d.org, "junk", "a", in); !errors.Is(err, ErrNotFound) {
		t.Fatalf("junk: %v", err)
	}

	// Cancelling releases the unpaid invoice and clears the link in the DB.
	in.Capacity = nil
	in.Status = "cancelled"
	if _, err := d.svc.Update(d.org, ev.ID, "admin", in); err != nil {
		t.Fatal(err)
	}
	for _, r := range func() []*RSVP { rs, _ := d.svc.ListRSVPs(d.org, ev.ID); return rs }() {
		if r.InvoiceID != "" || r.BilledQuantity != 0 {
			t.Fatalf("invoice link should be cleared: %+v", r)
		}
	}

	// Delete: not found, then success removes RSVPs.
	if err := d.svc.Delete(d.org, uuid.NewString(), "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if err := d.svc.Delete(d.org, ev.ID, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.svc.Get(d.org, ev.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("event must be gone")
	}
}

func cov1xCaseRespondRevalidatesUnderLock(t *testing.T) {
	cases := []struct {
		name   string
		sql    string
		mutate func(*EventInput)
		want   error
	}{
		{"cancelled", `UPDATE club_events SET status = 'cancelled' WHERE id = $1::uuid`, nil, ErrEventCancelled},
		{"rsvp disabled", `UPDATE club_events SET rsvp_enabled = FALSE WHERE id = $1::uuid`, nil, ErrRSVPDisabled},
		{"deadline passed", `UPDATE club_events SET rsvp_deadline = '2026-09-01T00:00:00Z' WHERE id = $1::uuid`, nil, ErrRSVPClosed},
		{"mode changed", `UPDATE club_events SET rsvp_mode = 'swimmer' WHERE id = $1::uuid`, nil, ErrInvalidRSVP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := cov1xNewDB(t)
			ev := d.newEvent(t, tc.mutate)
			var calls atomic.Int32
			d.svc.SetClock(func() time.Time {
				// The first clock read is the pre-lock window check; edit the
				// row right after it so only the locked re-validation can see it.
				if calls.Add(1) == 1 {
					d.exec(t, tc.sql, ev.ID)
				}
				return cov1xT0
			})
			_, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if rs, _ := d.svc.ListRSVPs(d.org, ev.ID); len(rs) != 0 {
				t.Fatal("rejected response must not be stored")
			}
		})
	}
}

func cov1xCaseRespondTxDirect(t *testing.T) {
	d := cov1xNewDB(t)
	ctx := context.Background()
	ev := d.newEvent(t, nil)
	okBuild := func(e *Event, existing *RSVP, others int) (*RSVP, error) {
		return &RSVP{ID: uuid.NewString(), OrgID: d.org, EventID: e.ID, HouseholdID: "h", Response: "yes",
			GuestCount: 1, CreatedAt: cov1xT0, UpdatedAt: cov1xT0}, nil
	}
	if _, err := d.repo.RespondTx(ctx, d.org, "nope", "h", "", okBuild); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := d.repo.RespondTx(ctx, d.org, uuid.NewString(), "h", "", okBuild); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	boom := errors.New("build refused")
	if _, err := d.repo.RespondTx(ctx, d.org, ev.ID, "h", "", func(*Event, *RSVP, int) (*RSVP, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("build err: %v", err)
	}
	// Existing and othersYes are passed to build.
	if _, err := d.repo.RespondTx(ctx, d.org, ev.ID, "h", "", okBuild); err != nil {
		t.Fatal(err)
	}
	var seenExisting *RSVP
	var seenOthers int
	if _, err := d.repo.RespondTx(ctx, d.org, ev.ID, "h2", "", func(e *Event, ex *RSVP, others int) (*RSVP, error) {
		seenExisting, seenOthers = ex, others
		r, _ := okBuild(e, ex, others)
		r.HouseholdID = "h2"
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	if seenExisting != nil || seenOthers != 2 {
		t.Fatalf("existing=%v others=%d", seenExisting, seenOthers)
	}
	// Re-responding as h sees the stored row and the other party's headcount.
	if _, err := d.repo.RespondTx(ctx, d.org, ev.ID, "h", "", func(e *Event, ex *RSVP, others int) (*RSVP, error) {
		seenExisting, seenOthers = ex, others
		r, _ := okBuild(e, ex, others)
		r.ID = ex.ID
		return r, nil
	}); err != nil || seenExisting == nil || seenExisting.GuestCount != 1 || seenOthers != 2 {
		t.Fatalf("existing=%v others=%d err=%v", seenExisting, seenOthers, err)
	}
	// An invalid row fails the upsert and nothing is stored.
	_, err := d.repo.RespondTx(ctx, d.org, ev.ID, "hz", "", func(e *Event, ex *RSVP, others int) (*RSVP, error) {
		r, _ := okBuild(e, ex, others)
		r.HouseholdID, r.Response = "hz", "bogus"
		return r, nil
	})
	cov1xWantErr(t, err, "club events: upsert rsvp")
}

func cov1xCaseRespondTxCommitSumAndLoadFailures(t *testing.T) {
	t.Run("commit failure", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.failOn(t, "club_event_rsvps", "INSERT", "", true)
		_, err := d.repo.RespondTx(context.Background(), d.org, ev.ID, "h", "", func(e *Event, ex *RSVP, o int) (*RSVP, error) {
			return &RSVP{ID: uuid.NewString(), OrgID: d.org, EventID: e.ID, HouseholdID: "h", Response: "yes", CreatedAt: cov1xT0, UpdatedAt: cov1xT0}, nil
		})
		cov1xWantErr(t, err, "club events: commit rsvp")
	})
	t.Run("sum overflow", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.exec(t, `ALTER TABLE club_event_rsvps DROP CONSTRAINT club_event_rsvps_guest_count_check`)
		t.Cleanup(func() {
			ctx := context.Background()
			_, _ = d.pool.Exec(ctx, `DELETE FROM club_event_rsvps WHERE guest_count > 50`)
			_, _ = d.pool.Exec(ctx, `ALTER TABLE club_event_rsvps ADD CONSTRAINT club_event_rsvps_guest_count_check CHECK (guest_count >= 0 AND guest_count <= 50)`)
		})
		for _, hh := range []string{"a", "b"} {
			d.exec(t, `INSERT INTO club_event_rsvps (org_id, event_id, household_id, response, guest_count) VALUES ($1, $2::uuid, $3, 'yes', 2000000000)`, d.org, ev.ID, hh)
		}
		_, err := d.repo.RespondTx(context.Background(), d.org, ev.ID, "h", "", func(*Event, *RSVP, int) (*RSVP, error) {
			t.Fatal("build must not run when the headcount query fails")
			return nil, nil
		})
		cov1xWantErr(t, err, "club events: sum headcount")
	})
	t.Run("load rsvp failure", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.rename(t, "club_event_rsvps", "note", "note_x")
		_, err := d.repo.RespondTx(context.Background(), d.org, ev.ID, "h", "", func(*Event, *RSVP, int) (*RSVP, error) { return nil, nil })
		cov1xWantErr(t, err, "club events: load rsvp")
	})
	t.Run("lock event failure", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.rename(t, "club_events", "title", "title_x")
		_, err := d.repo.RespondTx(context.Background(), d.org, ev.ID, "h", "", func(*Event, *RSVP, int) (*RSVP, error) { return nil, nil })
		cov1xWantErr(t, err, "club events: lock event")
	})
}

func cov1xCaseUpdateAndDeleteEventTxFailures(t *testing.T) {
	ctx := context.Background()
	same := func(e *Event, _ []*RSVP) (*Event, error) { n := copyEvent(e); n.Title = "renamed"; return n, nil }

	t.Run("update bad id and missing", func(t *testing.T) {
		d := cov1xNewDB(t)
		if _, _, _, err := d.repo.UpdateEventTx(ctx, d.org, "bad", same); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%v", err)
		}
		if _, _, _, err := d.repo.UpdateEventTx(ctx, d.org, uuid.NewString(), same); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%v", err)
		}
		if _, err := d.repo.DeleteEventTx(ctx, d.org, "bad"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("update success returns prior state", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		if _, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true); err != nil {
			t.Fatal(err)
		}
		prior, rsvps, next, err := d.repo.UpdateEventTx(ctx, d.org, ev.ID, same)
		if err != nil || prior.Title != "Team Banquet" || len(rsvps) != 1 || next.Title != "renamed" {
			t.Fatalf("%+v %v %+v %v", prior, rsvps, next, err)
		}
		if got, _ := d.svc.Get(d.org, ev.ID); got.Title != "renamed" {
			t.Fatal("update must persist")
		}
	})
	t.Run("build error rolls back", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		boom := errors.New("refuse")
		if _, _, _, err := d.repo.UpdateEventTx(ctx, d.org, ev.ID, func(*Event, []*RSVP) (*Event, error) { return nil, boom }); !errors.Is(err, boom) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("invalid row fails upsert", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		_, _, _, err := d.repo.UpdateEventTx(ctx, d.org, ev.ID, func(e *Event, _ []*RSVP) (*Event, error) {
			n := copyEvent(e)
			n.Category = "bogus"
			return n, nil
		})
		cov1xWantErr(t, err, "club events: upsert event")
		if got, _ := d.svc.Get(d.org, ev.ID); got.Category != "banquet" {
			t.Fatal("failed update must roll back")
		}
	})
	t.Run("update commit failure", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.failOn(t, "club_events", "UPDATE", "", true)
		_, _, _, err := d.repo.UpdateEventTx(ctx, d.org, ev.ID, same)
		cov1xWantErr(t, err, "club events: commit update")
	})
	t.Run("lock failures", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.rename(t, "club_event_rsvps", "note", "note_x")
		_, _, _, err := d.repo.UpdateEventTx(ctx, d.org, ev.ID, same)
		cov1xWantErr(t, err, `column "note" does not exist`)
		d.rename(t, "club_events", "title", "title_x")
		_, _, _, err = d.repo.UpdateEventTx(ctx, d.org, ev.ID, same)
		cov1xWantErr(t, err, "club events: lock event")
	})
	t.Run("rsvp scan failure under lock", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		if _, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true); err != nil {
			t.Fatal(err)
		}
		d.guestCountAsText(t)
		_, _, _, err := d.repo.UpdateEventTx(ctx, d.org, ev.ID, same)
		cov1xWantErr(t, err, "club events: scan rsvp")
	})
	t.Run("delete statement failure", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.failOn(t, "club_events", "DELETE", "", false)
		_, err := d.repo.DeleteEventTx(ctx, d.org, ev.ID)
		cov1xWantErr(t, err, "club events: delete event")
	})
	t.Run("delete commit failure", func(t *testing.T) {
		d := cov1xNewDB(t)
		ev := d.newEvent(t, nil)
		d.failOn(t, "club_events", "DELETE", "", true)
		_, err := d.repo.DeleteEventTx(ctx, d.org, ev.ID)
		cov1xWantErr(t, err, "club events: commit delete")
		if _, err := d.svc.Get(d.org, ev.ID); err != nil {
			t.Fatalf("failed delete must keep the event: %v", err)
		}
		if _, err := d.repo.DeleteEventTx(ctx, d.org, uuid.NewString()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%v", err)
		}
	})
}

func cov1xCaseDBConnectionFailuresSurface(t *testing.T) {
	d := cov1xNewDB(t)
	ev := d.newEvent(t, nil)
	dead := cov1xDead(t, d.pool)
	ctx := context.Background()
	in := validInput()

	if _, err := dead.Create(d.org, "a", in); err == nil || !strings.Contains(err.Error(), "club events: upsert event") {
		t.Fatalf("create: %v", err)
	}
	if _, err := dead.Get(d.org, ev.ID); err == nil || !strings.Contains(err.Error(), "club events: get event") {
		t.Fatalf("get: %v", err)
	}
	if _, err := dead.List(d.org, ListFilter{}); err == nil || !strings.Contains(err.Error(), "club events: list events") {
		t.Fatalf("list: %v", err)
	}
	if _, err := dead.Update(d.org, ev.ID, "a", in); err == nil || !strings.Contains(err.Error(), "begin update tx") {
		t.Fatalf("update: %v", err)
	}
	if err := dead.Delete(d.org, ev.ID, "a"); err == nil || !strings.Contains(err.Error(), "begin delete tx") {
		t.Fatalf("delete: %v", err)
	}
	if _, err := dead.ListRSVPs(d.org, ev.ID); err == nil || !strings.Contains(err.Error(), "list rsvps") {
		t.Fatalf("list rsvps: %v", err)
	}
	if _, err := dead.ListRSVPsForEvents(d.org, []string{ev.ID}); err == nil || !strings.Contains(err.Error(), "list rsvps") {
		t.Fatalf("list rsvps for events: %v", err)
	}
	if _, err := dead.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, false); err == nil {
		t.Fatal("respond must fail")
	} else if errors.Is(err, ErrNotFound) {
		t.Fatalf("connection failure must not masquerade as not found: %v", err)
	}
	if _, err := dead.repo.RespondTx(ctx, d.org, ev.ID, "h", "", nil); err == nil || !strings.Contains(err.Error(), "begin rsvp tx") {
		t.Fatalf("respond tx: %v", err)
	}
	cov1xWantErr(t, dead.repo.UpdateRSVPBilling(ctx, d.org, uuid.NewString(), "i", 1), "club events: update rsvp billing")
	if _, _, err := dead.RotateFeedToken(d.org, "h", "a"); err == nil || !strings.Contains(err.Error(), "begin token tx") {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := dead.ActiveFeedToken(d.org, "h"); err == nil || !strings.Contains(err.Error(), "active token") {
		t.Fatalf("active: %v", err)
	}
	if err := dead.RevokeFeedToken(d.org, "h"); err == nil || !strings.Contains(err.Error(), "revoke token") {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := dead.ResolveFeedToken(strings.Repeat("z", 43)); err == nil || !strings.Contains(err.Error(), "resolve token") {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := dead.EraseHouseholdData(ctx, d.org, "h"); err == nil || !strings.Contains(err.Error(), "erase rsvps") {
		t.Fatalf("erase: %v", err)
	}
}

func cov1xCaseFeedTokensDB(t *testing.T) {
	d := cov1xNewDB(t)
	if tok, err := d.svc.ActiveFeedToken(d.org, "h1"); tok != nil || err != nil {
		t.Fatalf("none yet: %v %v", tok, err)
	}
	s1, t1, err := d.svc.RotateFeedToken(d.org, "h1", "parent")
	if err != nil || t1.TokenHash == s1 {
		t.Fatalf("%v", err)
	}
	active, err := d.svc.ActiveFeedToken(d.org, "h1")
	if err != nil || active.ID != t1.ID || active.CreatedBy != "parent" || active.RevokedAt != nil {
		t.Fatalf("%+v %v", active, err)
	}
	// First resolve stamps last_used_at; a resolve within a minute does not move it.
	got, err := d.svc.ResolveFeedToken(s1)
	if err != nil || got.HouseholdID != "h1" || got.LastUsedAt == nil || !got.LastUsedAt.Equal(cov1xT0) {
		t.Fatalf("%+v %v", got, err)
	}
	d.svc.SetClock(func() time.Time { return cov1xT0.Add(30 * time.Second) })
	got, _ = d.svc.ResolveFeedToken(s1)
	if !got.LastUsedAt.Equal(cov1xT0) {
		t.Fatalf("last_used_at must be debounced, got %v", got.LastUsedAt)
	}
	d.svc.SetClock(func() time.Time { return cov1xT0.Add(2 * time.Minute) })
	got, _ = d.svc.ResolveFeedToken(s1)
	if !got.LastUsedAt.Equal(cov1xT0.Add(2 * time.Minute)) {
		t.Fatalf("last_used_at must advance, got %v", got.LastUsedAt)
	}

	// Rotation revokes the old secret.
	s2, _, err := d.svc.RotateFeedToken(d.org, "h1", "parent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.svc.ResolveFeedToken(s1); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatalf("old token: %v", err)
	}
	if _, err := d.svc.ResolveFeedToken(s2); err != nil {
		t.Fatal(err)
	}
	if err := d.svc.RevokeFeedToken(d.org, "h1"); err != nil {
		t.Fatal(err)
	}
	if err := d.svc.RevokeFeedToken(d.org, "h1"); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatalf("double revoke: %v", err)
	}
	if _, err := d.svc.ResolveFeedToken(s2); !errors.Is(err, ErrFeedTokenMissing) {
		t.Fatalf("revoked: %v", err)
	}
}

func cov1xCaseFeedRotationFailures(t *testing.T) {
	ctx := context.Background()
	tok := func(d *cov1xDB, hh string) *FeedToken {
		return &FeedToken{ID: uuid.NewString(), OrgID: d.org, HouseholdID: hh, TokenHash: HashFeedToken(uuid.NewString()), CreatedAt: cov1xT0, CreatedBy: "p"}
	}
	t.Run("revoke statement fails", func(t *testing.T) {
		d := cov1xNewDB(t)
		d.rename(t, "calendar_feed_tokens", "revoked_at", "revoked_at_x")
		cov1xWantErr(t, d.repo.RotateFeedToken(ctx, tok(d, "h"), cov1xT0), "club events: revoke token")
	})
	t.Run("insert fails on duplicate hash", func(t *testing.T) {
		d := cov1xNewDB(t)
		first := tok(d, "h")
		if err := d.repo.RotateFeedToken(ctx, first, cov1xT0); err != nil {
			t.Fatal(err)
		}
		dup := tok(d, "h2")
		dup.TokenHash = first.TokenHash
		cov1xWantErr(t, d.repo.RotateFeedToken(ctx, dup, cov1xT0), "club events: insert token")
	})
	t.Run("commit fails", func(t *testing.T) {
		d := cov1xNewDB(t)
		d.failOn(t, "calendar_feed_tokens", "INSERT", "", true)
		cov1xWantErr(t, d.repo.RotateFeedToken(ctx, tok(d, "h"), cov1xT0), "injected failure")
		if a, _ := d.repo.ActiveFeedToken(ctx, d.org, "h"); a != nil {
			t.Fatal("failed rotation must not leave a token")
		}
	})
	t.Run("advisory lock wait is cancelled", func(t *testing.T) {
		d := cov1xNewDB(t)
		holder, err := d.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = holder.Rollback(ctx) })
		if _, err := holder.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2::text, 0))`, d.org, "h"); err != nil {
			t.Fatal(err)
		}
		cctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
		defer cancel()
		cov1xWantErr(t, d.repo.RotateFeedToken(cctx, tok(d, "h"), cov1xT0), "club events: lock token rotation")
	})
}

func cov1xCaseEraseHouseholdDB(t *testing.T) {
	d := cov1xNewDB(t)
	ctx := context.Background()
	ev := d.newEvent(t, func(in *EventInput) { in.RSVPMode = "swimmer" })
	for _, hh := range []string{"h1", "h2"} {
		if _, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: hh, SwimmerID: "s-" + hh, Response: "yes"}, true); err != nil {
			t.Fatal(err)
		}
	}
	_, _, _ = d.svc.RotateFeedToken(d.org, "h1", "p")
	s2, _, _ := d.svc.RotateFeedToken(d.org, "h2", "p")
	n, err := d.svc.EraseHouseholdData(ctx, d.org, "h1")
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if a, _ := d.svc.ActiveFeedToken(d.org, "h1"); a != nil {
		t.Fatal("tokens must be erased")
	}
	if _, err := d.svc.ResolveFeedToken(s2); err != nil {
		t.Fatalf("other household token must survive: %v", err)
	}
	if rs, _ := d.svc.ListRSVPs(d.org, ev.ID); len(rs) != 1 || rs[0].HouseholdID != "h2" {
		t.Fatalf("unexpected remaining rsvps: %+v", rs)
	}
	// Token delete failure after rsvps were removed.
	d.rename(t, "calendar_feed_tokens", "household_id", "household_id_x")
	_, err = d.svc.EraseHouseholdData(ctx, d.org, "h2")
	cov1xWantErr(t, err, "club events: erase feed tokens")
}

func cov1xCaseSaveBillingFailureReportedDB(t *testing.T) {
	d := cov1xNewDB(t)
	b := &cov1xBiller{}
	d.svc.SetBiller(b)
	ev := d.newEvent(t, func(in *EventInput) { in.FeeCents = 300 })
	d.failOn(t, "club_event_rsvps", "UPDATE", "WHEN (NEW.invoice_id IS DISTINCT FROM OLD.invoice_id)", false)

	res, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true)
	if err != nil || res.Billing != BillingFailed {
		t.Fatalf("charge ok but persisting the link failed: %+v %v", res, err)
	}
	if len(b.charges) != 1 {
		t.Fatalf("expected the charge to have happened: %d", len(b.charges))
	}
	// Charge failure also tries to persist and still reports failure.
	b.chargeErr = errors.New("down")
	res, _ = d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes", GuestCount: 1}, true)
	if res.Billing != BillingFailed {
		t.Fatalf("%+v", res)
	}
}

func cov1xCaseSaveBillingFailureOnReleaseDB(t *testing.T) {
	d := cov1xNewDB(t)
	b := &cov1xBiller{}
	d.svc.SetBiller(b)
	ev := d.newEvent(t, func(in *EventInput) { in.FeeCents = 300 })
	if _, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "yes"}, true); err != nil {
		t.Fatal(err)
	}
	rs, _ := d.svc.ListRSVPs(d.org, ev.ID)
	if rs[0].InvoiceID != "cov1x-inv" {
		t.Fatalf("setup: %+v", rs[0])
	}
	d.failOn(t, "club_event_rsvps", "UPDATE", "WHEN (NEW.invoice_id IS DISTINCT FROM OLD.invoice_id)", false)
	// Switching to "no" releases the invoice; persisting the cleared link fails.
	res, err := d.svc.Respond(d.org, ev.ID, "p", RSVPInput{HouseholdID: "h", Response: "no"}, true)
	if err != nil || res.Billing != BillingFailed {
		t.Fatalf("%+v %v", res, err)
	}
	// releaseAll tolerates the same failure and keeps the DB link intact.
	d.svc.releaseAll(d.org, "a", rs)
	after, _ := d.svc.ListRSVPs(d.org, ev.ID)
	if after[0].InvoiceID != "cov1x-inv" {
		t.Fatalf("link must remain after failed persist: %+v", after[0])
	}
}

// TestCov1xDBSuite runs every Postgres-backed case against one isolated schema.
func TestCov1xDBSuite(t *testing.T) {
	cov1xShared = testutil.IsolatedSchemaPool(t)
	t.Cleanup(func() { cov1xShared = nil })
	t.Run("RepoEventRoundTrip", cov1xCaseRepoEventRoundTrip)
	t.Run("UpsertNilGroupCodes", cov1xCaseUpsertNilGroupCodes)
	t.Run("RepoDeleteEvent", cov1xCaseRepoDeleteEvent)
	t.Run("ServiceDBEventLifecycleAndBilling", cov1xCaseServiceDBEventLifecycleAndBilling)
	t.Run("RespondRevalidatesUnderLock", cov1xCaseRespondRevalidatesUnderLock)
	t.Run("RespondTxDirect", cov1xCaseRespondTxDirect)
	t.Run("RespondTxCommitSumAndLoadFailures", cov1xCaseRespondTxCommitSumAndLoadFailures)
	t.Run("UpdateAndDeleteEventTxFailures", cov1xCaseUpdateAndDeleteEventTxFailures)
	t.Run("DBConnectionFailuresSurface", cov1xCaseDBConnectionFailuresSurface)
	t.Run("FeedTokensDB", cov1xCaseFeedTokensDB)
	t.Run("FeedRotationFailures", cov1xCaseFeedRotationFailures)
	t.Run("EraseHouseholdDB", cov1xCaseEraseHouseholdDB)
	t.Run("SaveBillingFailureReportedDB", cov1xCaseSaveBillingFailureReportedDB)
	t.Run("SaveBillingFailureOnReleaseDB", cov1xCaseSaveBillingFailureOnReleaseDB)
}
