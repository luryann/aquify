package members

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/google/uuid"
)

func seedSwimmerForLoad(t *testing.T, repo *Repository, org string) (Swimmer, SwimmerGroupLink) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	hh := Household{ID: uuid.NewString(), OrgID: org, HouseholdName: "Load", Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertHousehold(ctx, hh); err != nil {
		t.Fatal(err)
	}
	sw := Swimmer{ID: uuid.NewString(), OrgID: org, HouseholdID: hh.ID, FirstName: "A", LastName: "B",
		DateOfBirth: "2012-01-02", Gender: "female", RegistrationStatus: "active", CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertSwimmer(ctx, sw); err != nil {
		t.Fatal(err)
	}
	link := SwimmerGroupLink{ID: uuid.NewString(), OrgID: org, SwimmerID: sw.ID, GroupCode: "silver", IsPrimary: true, AssignedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := repo.UpsertSwimmerGroupLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	return sw, link
}

func TestLoadAllNormalizesGroupCodesAndSurfacesCorruptMedicalFlags(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	repo := newRepository(pool)
	ctx := context.Background()
	sw, _ := seedSwimmerForLoad(t, repo, uuid.NewString())

	_, _, _, links, err := repo.LoadAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].GroupCode != "SILVER" {
		t.Fatalf("links = %+v, want canonical uppercase SILVER", links)
	}

	if _, err := pool.Exec(ctx, `UPDATE swimmers SET medical_flags = '{"asthma":true}'::jsonb WHERE id = $1::uuid`, sw.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := repo.LoadAll(ctx); err == nil || !strings.Contains(err.Error(), "medical_flags") {
		t.Fatalf("LoadAll err = %v, want medical_flags decode error", err)
	}
}
