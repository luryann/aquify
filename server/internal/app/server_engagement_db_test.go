package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/wellness"
	"github.com/google/uuid"
)

// TestDBRepoEngagement_ClubEventsAndWellness round-trips club events, RSVPs,
// feed tokens and wellness check-ins through Postgres across two service
// instances (DB is canonical, so the second instance sees writes at once).
func TestDBRepoEngagement_ClubEventsAndWellness(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	orgID := "org-engage-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM club_events WHERE org_id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM calendar_feed_tokens WHERE org_id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM wellness_checkins WHERE org_id = $1`, orgID)
	})

	a := events.NewServiceWithPool(pool)
	b := events.NewServiceWithPool(pool)
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	capacity := 3
	ev, err := a.Create(orgID, "admin", events.EventInput{
		Title: "Banquet", Category: "banquet", StartsAt: start, RSVPEnabled: true,
		RSVPMode: "household", Capacity: &capacity, Audience: "groups", GroupCodes: []string{"senior"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := b.Get(orgID, ev.ID)
	if err != nil || got.Title != "Banquet" || got.Capacity == nil || *got.Capacity != 3 || len(got.GroupCodes) != 1 || !got.StartsAt.Equal(start) {
		t.Fatalf("second instance get: %+v %v", got, err)
	}
	if _, err := b.Get("other-org", ev.ID); !errors.Is(err, events.ErrNotFound) {
		t.Fatalf("cross-org get should be not found, got %v", err)
	}

	// Concurrent RSVPs of 2 people each against capacity 3: exactly one wins.
	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := a
			if i%2 == 1 {
				svc = b
			}
			_, results[i] = svc.Respond(orgID, ev.ID, "p", events.RSVPInput{HouseholdID: uuid.NewString(), Response: "yes", GuestCount: 1}, true)
		}(i)
	}
	wg.Wait()
	ok, full := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, events.ErrEventFull):
			full++
		default:
			t.Fatalf("unexpected rsvp error: %v", err)
		}
	}
	if ok != 1 || full != 3 {
		t.Fatalf("capacity race: ok=%d full=%d", ok, full)
	}
	rsvps, err := b.ListRSVPs(orgID, ev.ID)
	if err != nil || len(rsvps) != 1 {
		t.Fatalf("list rsvps: %d %v", len(rsvps), err)
	}
	byEvent, err := b.ListRSVPsForEvents(orgID, []string{ev.ID})
	if err != nil || len(byEvent[ev.ID]) != 1 {
		t.Fatalf("list rsvps for events: %v %v", byEvent, err)
	}

	secret, _, err := a.RotateFeedToken(orgID, "hh-1", "parent")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	tok, err := b.ResolveFeedToken(secret)
	if err != nil || tok.HouseholdID != "hh-1" {
		t.Fatalf("resolve: %+v %v", tok, err)
	}
	secret2, _, _ := b.RotateFeedToken(orgID, "hh-1", "parent")
	if _, err := a.ResolveFeedToken(secret); !errors.Is(err, events.ErrFeedTokenMissing) {
		t.Fatalf("old token should be revoked: %v", err)
	}
	if err := a.RevokeFeedToken(orgID, "hh-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ResolveFeedToken(secret2); !errors.Is(err, events.ErrFeedTokenMissing) {
		t.Fatalf("revoked token should not resolve: %v", err)
	}

	if err := b.Delete(orgID, ev.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if rs, _ := a.ListRSVPs(orgID, ev.ID); len(rs) != 0 {
		t.Fatalf("rsvps should cascade, got %d", len(rs))
	}

	// Wellness needs a real swimmer row (FK with ON DELETE CASCADE).
	householdID, swimmerID := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'Engage', 'active')`, householdID, testOrgID); err != nil {
		t.Fatalf("seed household: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, 'Well', 'Ness', '2011-01-01', 'female', 'active')`, swimmerID, testOrgID, householdID); err != nil {
		t.Fatalf("seed swimmer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM swimmers WHERE id = $1::uuid`, swimmerID)
		_, _ = pool.Exec(ctx, `DELETE FROM households WHERE id = $1::uuid`, householdID)
	})
	w1, w2 := wellness.NewServiceWithPool(pool), wellness.NewServiceWithPool(pool)
	m := wellness.Metrics{SleepHours: 7.5, SleepQuality: 4, Soreness: 2, Energy: 4, Stress: 2, Mood: 5}
	c1, created, err := w1.Upsert(orgID, wellness.Input{SwimmerID: swimmerID, HouseholdID: householdID, Metrics: m, Note: "fine"})
	if err != nil || !created {
		t.Fatalf("wellness insert: %v created=%v", err, created)
	}
	m.Mood = 2
	c2, created, err := w2.Upsert(orgID, wellness.Input{SwimmerID: swimmerID, HouseholdID: householdID, Metrics: m})
	if err != nil || created || c2.ID != c1.ID {
		t.Fatalf("wellness upsert: %v created=%v ids %s/%s", err, created, c1.ID, c2.ID)
	}
	today := w1.Today()
	list, err := w1.List(orgID, []string{swimmerID}, today, today)
	if err != nil || len(list) != 1 || list[0].Mood != 2 || list[0].SleepHours != 7.5 || list[0].Readiness != wellness.Readiness(m) {
		t.Fatalf("wellness list: %+v %v", list, err)
	}
	if other, _ := w2.List("other-org", nil, today, today); len(other) != 0 {
		t.Fatal("wellness org isolation broken")
	}
}
