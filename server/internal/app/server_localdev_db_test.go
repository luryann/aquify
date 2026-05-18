package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/authn"
	"github.com/dareaquatics/aquify/server/internal/app/imports"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func cleanupLocalDevFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	cleanupImportsFixtures(t, pool, authn.LocalDevOrgID)
	cleanupMeetsFixtures(t, pool, authn.LocalDevOrgID)
	cleanupMembersFixtures(t, pool, authn.LocalDevOrgID)

	ctx := context.Background()
	for _, q := range []string{
		"DELETE FROM mfa_factors WHERE org_id = $1::uuid",
		"DELETE FROM users WHERE org_id = $1::uuid",
	} {
		if _, err := pool.Exec(ctx, q, authn.LocalDevOrgID); err != nil {
			t.Logf("cleanup warning (%s): %v", q, err)
		}
	}
}

func countUsersForOrg(t *testing.T, pool *pgxpool.Pool, orgID string) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM users WHERE org_id = $1::uuid`, orgID).Scan(&count); err != nil {
		t.Fatalf("count users for org %s: %v", orgID, err)
	}
	return count
}

func TestDBLocalDevBootstrap_SeedsKnownUsers(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupLocalDevFixtures(t, pool)

	authService := authn.NewService(authn.Config{
		EnableDevStubTokens: true,
		SeedLocalUsers:      true,
	})
	authService.SetPool(pool)
	authService.SetPool(pool)

	if got, want := countUsersForOrg(t, pool, authn.LocalDevOrgID), len(authn.LocalDevUsers()); got != want {
		t.Fatalf("expected %d seeded local users, got %d", want, got)
	}
}

func TestDBLocalDevSessionIssue_SyncsUserRecord(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupLocalDevFixtures(t, pool)

	authService := authn.NewService(authn.Config{})
	authService.SetPool(pool)

	const (
		userID = "00000000-0000-0000-0000-00000000b201"
		email  = "session-sync@example.com"
		name   = "Session Sync"
	)

	if _, err := authService.IssueSession(authn.SessionInput{
		UserID: userID,
		Email:  email,
		Name:   name,
		OrgID:  authn.LocalDevOrgID,
		Roles:  []string{"admin"},
	}); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	var gotEmail, gotSubject, gotName string
	if err := pool.QueryRow(context.Background(), `
		SELECT email, oidc_subject, COALESCE(display_name, '')
		FROM users
		WHERE id = $1::uuid`,
		userID,
	).Scan(&gotEmail, &gotSubject, &gotName); err != nil {
		t.Fatalf("load synced user: %v", err)
	}
	if gotEmail != email {
		t.Fatalf("expected synced email %q, got %q", email, gotEmail)
	}
	if gotSubject != userID {
		t.Fatalf("expected oidc_subject %q, got %q", userID, gotSubject)
	}
	if gotName != name {
		t.Fatalf("expected display_name %q, got %q", name, gotName)
	}
}

func TestDBLocalDevSessionIssue_DuplicateEmailFallsBackToSurrogate(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupLocalDevFixtures(t, pool)

	authService := authn.NewService(authn.Config{})
	authService.SetPool(pool)

	const (
		existingUserID = "00000000-0000-0000-0000-00000000b211"
		userID         = "00000000-0000-0000-0000-00000000b212"
		email          = "duplicate-login@example.com"
		name           = "Duplicate Login"
	)

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, org_id, email, oidc_subject, display_name, status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'active', now(), now())`,
		existingUserID, authn.LocalDevOrgID, email, existingUserID, "Existing User",
	); err != nil {
		t.Fatalf("seed conflicting user: %v", err)
	}

	if _, err := authService.IssueSession(authn.SessionInput{
		UserID: userID,
		Email:  email,
		Name:   name,
		OrgID:  authn.LocalDevOrgID,
		Roles:  []string{"admin"},
	}); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	var gotEmail, gotSubject, gotName string
	if err := pool.QueryRow(context.Background(), `
		SELECT email, oidc_subject, COALESCE(display_name, '')
		FROM users
		WHERE id = $1::uuid`,
		userID,
	).Scan(&gotEmail, &gotSubject, &gotName); err != nil {
		t.Fatalf("load synced user: %v", err)
	}

	wantEmail := fmt.Sprintf("%s@users.local.invalid", userID)
	if gotEmail != wantEmail {
		t.Fatalf("expected surrogate email %q, got %q", wantEmail, gotEmail)
	}
	if gotSubject != userID {
		t.Fatalf("expected oidc_subject %q, got %q", userID, gotSubject)
	}
	if gotName != name {
		t.Fatalf("expected display_name %q, got %q", name, gotName)
	}

	var preservedID string
	if err := pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE email = $1`, email).Scan(&preservedID); err != nil {
		t.Fatalf("load conflicting user by email: %v", err)
	}
	if preservedID != existingUserID {
		t.Fatalf("expected original email row to stay on %q, got %q", existingUserID, preservedID)
	}
}

func TestDBLocalDevHouseholdAndMeetWrites_SucceedWithSeededLocalAdmin(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupLocalDevFixtures(t, pool)

	authService := authn.NewService(authn.Config{
		EnableDevStubTokens: true,
		SeedLocalUsers:      true,
	})
	authService.SetPool(pool)

	principal, ok := authService.ResolvePrincipal("stub-admin")
	if !ok || principal == nil {
		t.Fatal("expected stub-admin principal")
	}

	memberSvc := members.NewServiceWithPool(pool)
	household, err := memberSvc.CreateHousehold(principal, members.CreateHouseholdInput{
		OrgID:         authn.LocalDevOrgID,
		HouseholdName: "Local Dev Household",
		PrimaryEmail:  "local-dev-household@example.com",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	if household.OrgID != authn.LocalDevOrgID {
		t.Fatalf("expected household org %q, got %q", authn.LocalDevOrgID, household.OrgID)
	}

	meetsSvc := meets.NewServiceWithPool(pool)
	meet, _, err := meetsSvc.CreateMeet(principal, authn.LocalDevOrgID, meets.CreateMeetInput{
		Name:     "Local Dev Meet",
		MeetType: "invitational",
		Course:   "SCY",
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if meet.CreatedBy != authn.LocalDevAdminUserID {
		t.Fatalf("expected meet created_by %q, got %q", authn.LocalDevAdminUserID, meet.CreatedBy)
	}
}

func TestDBRepoImports_AcceptsHYVAndEV3(t *testing.T) {
	pool := openTestPool(t)
	_ = testutil.RequireDBHarness(t)

	cleanupImportsFixtures(t, pool, testOrgID)

	svc := imports.NewServiceWithPool(pool, storage.NewStubStorage(""))
	principal := &appmiddleware.Principal{
		UserID: authn.LocalDevAdminUserID,
		OrgID:  testOrgID,
		Roles:  []string{"admin"},
	}

	for _, tc := range []struct {
		filename string
		fileType string
	}{
		{filename: "meet-events.hyv", fileType: "hyv"},
		{filename: "meet-events.ev3", fileType: "ev3"},
	} {
		t.Run(tc.fileType, func(t *testing.T) {
			imp, _, err := svc.RequestUploadURL(principal, testOrgID, imports.RequestUploadURLInput{
				Filename:  tc.filename,
				FileType:  tc.fileType,
				SizeBytes: 1024,
				MIMEType:  "application/octet-stream",
			})
			if err != nil {
				t.Fatalf("RequestUploadURL(%s): %v", tc.fileType, err)
			}
			if imp.FileType != tc.fileType {
				t.Fatalf("expected file_type %q, got %q", tc.fileType, imp.FileType)
			}
		})
	}
}
