package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/volunteer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func cleanupVolunteerFixtures(t *testing.T, pool *pgxpool.Pool, orgID, seasonID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM volunteer_buyouts WHERE org_id = $1`,
		`DELETE FROM volunteer_credits WHERE org_id = $1`,
		`DELETE FROM volunteer_requirements WHERE org_id = $1`,
		`DELETE FROM volunteer_signups WHERE org_id = $1`,
		`DELETE FROM volunteer_jobs WHERE org_id = $1`,
		`DELETE FROM volunteer_settings WHERE org_id = $1`,
	} {
		if _, err := pool.Exec(ctx, q, orgID); err != nil {
			t.Fatalf("cleanup volunteer: %v", err)
		}
	}
	_, _ = pool.Exec(ctx, `DELETE FROM seasons WHERE id = $1::uuid`, seasonID)
}

// TestDBRepoVolunteer_CapacityAndRoundTrip proves the Postgres store enforces
// capacity under concurrent signups from two service instances (simulating
// two API replicas), and that ledger state is visible across instances.
func TestDBRepoVolunteer_CapacityAndRoundTrip(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	orgID := "vol-db-" + uuid.NewString()[:8]
	seasonID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO seasons (id, org_id, name, start_date, end_date, status)
		VALUES ($1::uuid, $2, 'DB Season', CURRENT_DATE - 30, CURRENT_DATE + 180, 'active')`, seasonID, orgID); err != nil {
		t.Fatalf("seed season: %v", err)
	}
	t.Cleanup(func() { cleanupVolunteerFixtures(t, pool, orgID, seasonID) })

	seasons := func(string) ([]volunteer.SeasonRef, error) {
		return []volunteer.SeasonRef{{ID: seasonID, Name: "DB Season", Status: "active",
			StartDate: time.Now().AddDate(0, 0, -30).Format("2006-01-02"), EndDate: time.Now().AddDate(0, 0, 180).Format("2006-01-02")}}, nil
	}
	households := func(string) ([]volunteer.HouseholdRef, error) {
		out := []volunteer.HouseholdRef{}
		for i := 0; i < 20; i++ {
			out = append(out, volunteer.HouseholdRef{ID: fmt.Sprintf("hh-%d", i), Name: fmt.Sprintf("House %d", i), Status: "active"})
		}
		return out, nil
	}
	newSvc := func() *volunteer.Service {
		s := volunteer.NewServiceWithPool(pool)
		s.SetSeasonLookup(seasons)
		s.SetHouseholdDirectory(households)
		return s
	}
	a, b := newSvc(), newSvc()
	admin := &appmiddleware.Principal{UserID: "db-admin", OrgID: orgID, Roles: []string{"admin"}}

	start := time.Now().UTC().Add(96 * time.Hour).Truncate(time.Second)
	job, err := a.CreateJob(ctx, admin, orgID, volunteer.JobInput{
		EventName: "Swim-a-thon", Role: "timer", Title: "Timer", StartsAt: start, EndsAt: start.Add(2 * time.Hour), Capacity: 4,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	var wg sync.WaitGroup
	var ok, full int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := a
			if i%2 == 1 {
				svc = b
			}
			hh := fmt.Sprintf("hh-%d", i)
			parent := &appmiddleware.Principal{UserID: "p-" + hh, OrgID: orgID, HouseholdID: hh, Roles: []string{"parent"}}
			_, err := svc.SignUp(ctx, parent, orgID, job.ID, volunteer.SignupInput{VolunteerName: "Parent " + hh})
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, volunteer.ErrJobFull):
				atomic.AddInt32(&full, 1)
			default:
				t.Errorf("signup %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if ok != 4 || full != 16 {
		t.Fatalf("want 4 ok / 16 full, got %d / %d", ok, full)
	}
	var live int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM volunteer_signups WHERE job_id = $1::uuid AND status <> 'cancelled'`, job.ID).Scan(&live); err != nil || live != 4 {
		t.Fatalf("db live signups: %d %v", live, err)
	}

	// Check in one volunteer on instance A, read the ledger on instance B.
	signups, err := a.ListSignups(ctx, admin, orgID, volunteer.SignupFilter{JobID: job.ID})
	if err != nil || len(signups) != 4 {
		t.Fatalf("list signups: %v %d", err, len(signups))
	}
	hours := 2.5
	if _, _, err := a.CheckIn(ctx, admin, orgID, signups[0].ID, volunteer.StatusWorked, &hours); err != nil {
		t.Fatalf("check in: %v", err)
	}
	if _, err := a.UpsertRequirement(ctx, admin, orgID, volunteer.RequirementInput{SeasonID: seasonID, RequiredHours: 4, BuyoutCentsPerHour: 1500}); err != nil {
		t.Fatalf("requirement: %v", err)
	}
	if _, err := a.UpsertRequirement(ctx, admin, orgID, volunteer.RequirementInput{SeasonID: seasonID, RequiredHours: 6, BuyoutCentsPerHour: 1500}); err != nil {
		t.Fatalf("requirement upsert: %v", err)
	}
	if _, err := a.AddCredit(ctx, admin, orgID, volunteer.CreditInput{SeasonID: seasonID, HouseholdID: signups[0].HouseholdID, Hours: 0.5, Reason: "setup"}); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if _, err := b.UpdateSettings(ctx, admin, orgID, volunteer.SettingsUpdate{Timezone: volunteerPtr("America/Chicago")}); err != nil {
		t.Fatalf("settings: %v", err)
	}
	st, err := a.GetSettings(ctx, admin, orgID)
	if err != nil || st.Timezone != "America/Chicago" || st.CancellationCutoffHours != 48 {
		t.Fatalf("settings round trip: %v %+v", err, st)
	}
	rows, err := b.HouseholdLedger(ctx, admin, orgID, signups[0].HouseholdID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ledger: %v %+v", err, rows)
	}
	if r := rows[0]; r.RequiredHours != 6 || r.WorkedHours != 2.5 || r.CreditedHours != 0.5 || r.RemainingHours != 3 || r.BuyoutDueCents != 4500 {
		t.Fatalf("ledger row: %+v", r)
	}

	// Parent cancel then the freed spot is reusable; duplicate person blocked by DB.
	parent := &appmiddleware.Principal{UserID: "p", OrgID: orgID, HouseholdID: signups[1].HouseholdID, Roles: []string{"parent"}}
	if _, _, err := b.Cancel(ctx, parent, orgID, signups[1].ID, "sick"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := a.SignUp(ctx, parent, orgID, job.ID, volunteer.SignupInput{VolunteerName: signups[2].VolunteerName}); err != nil {
		t.Fatalf("re-signup into freed spot: %v", err)
	}
	if _, err := a.SignUp(ctx, parent, orgID, job.ID, volunteer.SignupInput{VolunteerName: signups[2].VolunteerName}); !errors.Is(err, volunteer.ErrJobFull) && !errors.Is(err, volunteer.ErrDuplicateSignup) {
		t.Fatalf("over capacity: %v", err)
	}
	if _, err := a.GetJob(ctx, admin, orgID, "not-a-uuid"); !errors.Is(err, volunteer.ErrJobNotFound) {
		t.Fatalf("non-uuid job id: %v", err)
	}
}

func volunteerPtr[T any](v T) *T { return &v }
