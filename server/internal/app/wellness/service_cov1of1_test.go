package wellness

import (
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov1xValidationMessages(t *testing.T) {
	s := fixedService("2026-10-20")
	m := func(mut func(*Metrics)) Metrics {
		v := Metrics{SleepHours: 7, SleepQuality: 3, Soreness: 3, Energy: 3, Stress: 3, Mood: 3}
		mut(&v)
		return v
	}
	cases := []struct {
		name  string
		in    Input
		field string
		msg   string
	}{
		{"no swimmer", Input{Metrics: good()}, "swimmer_id", "is required"},
		{"bad date", Input{SwimmerID: "s", Date: "nope", Metrics: good()}, "date", "must be YYYY-MM-DD"},
		{"future", Input{SwimmerID: "s", Date: "2026-10-22", Metrics: good()}, "date", "cannot be in the future"},
		{"old", Input{SwimmerID: "s", Date: "2026-10-01", Metrics: good()}, "date", "cannot be more than 7 days ago"},
		{"nan sleep", Input{SwimmerID: "s", Metrics: m(func(v *Metrics) { v.SleepHours = math.NaN() })}, "sleep_hours", "must be between 0 and 24"},
		{"negative sleep", Input{SwimmerID: "s", Metrics: m(func(v *Metrics) { v.SleepHours = -1 })}, "sleep_hours", "must be between 0 and 24"},
		{"sleep quality", Input{SwimmerID: "s", Metrics: m(func(v *Metrics) { v.SleepQuality = 0 })}, "sleep_quality", "must be 1–5"},
		{"soreness", Input{SwimmerID: "s", Metrics: m(func(v *Metrics) { v.Soreness = 6 })}, "soreness", "must be 1–5"},
		{"energy", Input{SwimmerID: "s", Metrics: m(func(v *Metrics) { v.Energy = 0 })}, "energy", "must be 1–5"},
		{"stress", Input{SwimmerID: "s", Metrics: m(func(v *Metrics) { v.Stress = 9 })}, "stress", "must be 1–5"},
		{"mood", Input{SwimmerID: "s", Metrics: m(func(v *Metrics) { v.Mood = -2 })}, "mood", "must be 1–5"},
		{"note", Input{SwimmerID: "s", Metrics: good(), Note: strings.Repeat("é", MaxNoteLen+1)}, "note", "must be at most 500 characters"},
	}
	for _, tc := range cases {
		_, _, err := s.Upsert("org", tc.in)
		var ve *ValidationError
		if !errors.As(err, &ve) || !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: expected ValidationError, got %v", tc.name, err)
		}
		if ve.Field != tc.field || ve.Message != tc.msg {
			t.Errorf("%s: got %q/%q want %q/%q", tc.name, ve.Field, ve.Message, tc.field, tc.msg)
		}
		if err.Error() != tc.field+": "+tc.msg {
			t.Errorf("%s: Error() = %q", tc.name, err.Error())
		}
	}
	// Note exactly at the limit is accepted.
	if _, _, err := s.Upsert("org", Input{SwimmerID: "s", Metrics: good(), Note: strings.Repeat("é", MaxNoteLen)}); err != nil {
		t.Fatalf("note at limit: %v", err)
	}
	if got := s.Today(); got != "2026-10-20" {
		t.Fatalf("Today = %q", got)
	}
}

func TestCov1xDateRangeEdges(t *testing.T) {
	from, to, dates := DateRange("2026-10-20", 0)
	if from != "2026-10-20" || to != "2026-10-20" || len(dates) != 1 {
		t.Fatalf("days<1 should clamp to 1: %s %s %v", from, to, dates)
	}
	before := time.Now().UTC().Format(dateLayout)
	from, to, dates = DateRange("garbage", 3)
	after := time.Now().UTC().Format(dateLayout)
	if len(dates) != 3 || to != dates[2] || from != dates[0] {
		t.Fatalf("fallback range wrong: %v", dates)
	}
	// Tolerate a UTC midnight rollover between the clock reads.
	if to != before && to != after {
		t.Fatalf("fallback end %q not today (%q..%q)", to, before, after)
	}
	f, _ := time.Parse(dateLayout, from)
	e, _ := time.Parse(dateLayout, to)
	if e.Sub(f) != 48*time.Hour {
		t.Fatalf("fallback span %s..%s is not 3 consecutive days", from, to)
	}
}

func TestCov1xDBUpsertListAndErrors(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	ctx := t.Context()
	org, other := uuid.NewString(), uuid.NewString()
	hh, hhO := uuid.NewString(), uuid.NewString()
	sw1, sw2, swO := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, h := range [][2]string{{hh, org}, {hhO, other}} {
		if _, err := pool.Exec(ctx, `INSERT INTO households (id, org_id, household_name, status) VALUES ($1::uuid, $2::uuid, 'H', 'active')`, h[0], h[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range [][3]string{{sw1, org, hh}, {sw2, org, hh}, {swO, other, hhO}} {
		if _, err := pool.Exec(ctx, `INSERT INTO swimmers (id, org_id, household_id, first_name, last_name, date_of_birth, gender, registration_status) VALUES ($1::uuid, $2::uuid, $3::uuid, 'W', 'S', '2011-01-01', 'female', 'active')`, r[0], r[1], r[2]); err != nil {
			t.Fatal(err)
		}
	}

	s := NewServiceWithPool(pool)
	day, _ := time.Parse(dateLayout, "2026-10-20")
	s.SetClock(func() time.Time { return day.Add(15 * time.Hour) })

	c1, created, err := s.Upsert(org, Input{SwimmerID: sw1, HouseholdID: hh, Metrics: good(), Note: "n1", SubmittedBy: "u1", SubmittedRole: "parent"})
	if err != nil || !created {
		t.Fatalf("insert: %v created=%v", err, created)
	}
	if c1.Readiness != 4 || c1.SleepHours != 8.3 || c1.Date != "2026-10-20" || c1.SubmittedRole != "parent" {
		t.Fatalf("unexpected saved check-in %+v", c1)
	}
	c2, created, err := s.Upsert(org, Input{SwimmerID: sw1, HouseholdID: hh, Metrics: poor(), SubmittedBy: "u2", SubmittedRole: "swimmer"})
	if err != nil || created {
		t.Fatalf("replace: %v created=%v", err, created)
	}
	if c2.ID != c1.ID || c2.Mood != 2 || c2.SubmittedBy != "u2" || c2.Readiness != Readiness(poor()) {
		t.Fatalf("replace result %+v", c2)
	}
	if _, _, err := s.Upsert(org, Input{SwimmerID: sw2, HouseholdID: hh, Date: "2026-10-19", Metrics: good()}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Upsert(other, Input{SwimmerID: swO, HouseholdID: hhO, Metrics: good()}); err != nil {
		t.Fatal(err)
	}

	all, err := s.List(org, nil, "2026-10-01", "2026-10-31")
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: %v %+v", err, all)
	}
	if all[0].SwimmerID > all[1].SwimmerID {
		t.Fatal("not sorted by swimmer")
	}
	for _, c := range all {
		if c.Readiness != Readiness(c.Metrics) {
			t.Fatalf("readiness not derived: %+v", c)
		}
	}
	one, err := s.List(org, []string{sw2}, "2026-10-01", "2026-10-31")
	if err != nil || len(one) != 1 || one[0].SwimmerID != sw2 {
		t.Fatalf("filtered list: %v %+v", err, one)
	}
	none, err := s.List(org, []string{}, "2026-10-01", "2026-10-31")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("empty filter: %v %#v", err, none)
	}
	narrow, _ := s.List(org, nil, "2026-10-20", "2026-10-20")
	if len(narrow) != 1 || narrow[0].SwimmerID != sw1 {
		t.Fatalf("date window: %+v", narrow)
	}

	// A check-in for a swimmer already holding a row in another org is refused.
	if _, _, err := s.Upsert(other, Input{SwimmerID: sw1, Metrics: good()}); err == nil || !strings.Contains(err.Error(), "wellness: upsert check-in") {
		t.Fatalf("cross-org upsert should fail, got %v", err)
	}
	// Unknown swimmer violates the FK.
	if _, _, err := s.Upsert(org, Input{SwimmerID: uuid.NewString(), Metrics: good()}); err == nil {
		t.Fatal("expected FK failure")
	}
	// Non-UUID swimmer id surfaces a database error rather than panicking.
	if _, _, err := s.Upsert(org, Input{SwimmerID: "not-a-uuid", Metrics: good()}); err == nil {
		t.Fatal("expected invalid uuid error")
	}
	// Malformed date window is a query error.
	if _, err := s.List(org, nil, "bad", "worse"); err == nil || !strings.Contains(err.Error(), "wellness: list check-ins") {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestCov1xDBClosedPoolErrors(t *testing.T) {
	// No schema needed: a closed pool fails every statement before touching data.
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServiceWithPool(pool)
	pool.Close()
	if _, err := s.List(uuid.NewString(), nil, "2026-10-01", "2026-10-31"); err == nil {
		t.Fatal("expected list error on closed pool")
	}
	if _, _, err := s.Upsert(uuid.NewString(), Input{SwimmerID: uuid.NewString(), Metrics: good()}); err == nil {
		t.Fatal("expected upsert error on closed pool")
	}
}

func TestCov1xNilPoolStaysInMemory(t *testing.T) {
	if s := NewServiceWithPool(nil); s.repo != nil {
		t.Fatal("nil pool must keep the service in memory")
	}
}
