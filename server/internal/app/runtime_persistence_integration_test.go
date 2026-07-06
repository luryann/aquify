package app

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/dareaquatics/aquify/server/internal/app/billing"
	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/groups"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
	"github.com/dareaquatics/aquify/server/internal/runtimepersist"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/testutil"
)

func TestRuntimeSnapshotsRoundTripAcrossConvertedDomains(t *testing.T) {
	t.Parallel()

	harness := testutil.RequireDBHarness(t)
	repo := runtimepersist.NewRepository(os.Getenv("DATABASE_URL"))
	if !repo.Available() {
		t.Skip("psql-backed runtime snapshot repository is unavailable")
	}
	if err := repo.EnsureSchema(); err != nil {
		t.Fatalf("ensure runtime snapshot schema: %v", err)
	}
	harness.Exec(t, "TRUNCATE runtime_snapshots;")

	admin := testutil.AdminPrincipal()
	parent := testutil.ParentPrincipal("parent-runtime", "")

	memberSvc := members.NewService()
	groupSvc := groups.NewService()
	meetSvc := meets.NewService()
	store := storage.NewStubStorage("")
	exportSvc := exports.NewServiceWithStorage(store)
	perfSvc := performance.NewService()
	billingSvc := billing.NewService()
	commsSvc := comms.NewService()
	complianceSvc := compliance.NewService()
	reportsSvc := reports.NewService(memberSvc, groupSvc, meetSvc, billingSvc)
	reportsSvc.SetStorage(store)

	household, err := memberSvc.CreateHousehold(parent, members.CreateHouseholdInput{
		OrgID:         testutil.DefaultOrgID,
		HouseholdName: "Persistence Family",
		PrimaryEmail:  "persist@example.com",
	})
	if err != nil {
		t.Fatalf("create household: %v", err)
	}
	swimmer, _, err := memberSvc.CreateSwimmer(parent, testutil.DefaultOrgID, members.CreateSwimmerInput{
		HouseholdID:        household.ID,
		FirstName:          "Taylor",
		LastName:           "Persist",
		DateOfBirth:        "2012-08-14",
		Gender:             "female",
		RegistrationStatus: "active",
	})
	if err != nil {
		t.Fatalf("create swimmer: %v", err)
	}

	groupSvc.SeedSwimmer(testutil.DefaultOrgID, swimmer.ID, household.ID, swimmer.DateOfBirth, swimmer.Gender, swimmer.RegistrationStatus)
	groupSvc.SeedParentHousehold(parent.UserID, household.ID)
	group, _, err := groupSvc.CreateGroup(admin, testutil.DefaultOrgID, groups.CreateGroupInput{
		Code: "gold", Name: "Gold",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	registration, err := groupSvc.CreateRegistration(admin, testutil.DefaultOrgID, groups.CreateRegistrationInput{
		HouseholdID: household.ID,
		SwimmerID:   swimmer.ID,
		GroupID:     group.ID,
		Season:      "2026",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := groupSvc.AcceptWaiver(admin, testutil.DefaultOrgID, registration.ID, groups.AcceptWaiverInput{WaiverType: "liability"}); err != nil {
		t.Fatalf("accept waiver: %v", err)
	}

	meetSvc.SeedSwimmer(testutil.DefaultOrgID, swimmer.ID, household.ID)
	meetSvc.SeedParentHousehold(parent.UserID, household.ID)
	meet, _, err := meetSvc.CreateMeet(admin, testutil.DefaultOrgID, meets.CreateMeetInput{
		Name:     "Spring Invite",
		MeetType: "invitational",
		Course:   "SCY",
	})
	if err != nil {
		t.Fatalf("create meet: %v", err)
	}

	exportJob, _, err := exportSvc.GenerateExport(admin, testutil.DefaultOrgID, meet.ID, exports.ReportInput{
		ReportType: exports.ReportTypeEvent,
		Meet: exports.ReportMeet{
			Name:      "Spring Invite",
			StartDate: "2026-03-18",
			EndDate:   "2026-03-19",
			Course:    "SCY",
		},
	})
	if err != nil {
		t.Fatalf("generate export: %v", err)
	}

	finTime := 34000
	if _, err := perfSvc.UpsertResult(testutil.DefaultOrgID, admin.UserID, admin, performance.UpsertResultInput{
		SwimmerID:  swimmer.ID,
		MeetID:     meet.ID,
		Distance:   50,
		Stroke:     "free",
		Course:     "SCY",
		Gender:     "F",
		FinTimeMs:  &finTime,
		IsOfficial: true,
	}); err != nil {
		t.Fatalf("upsert result: %v", err)
	}

	invoice, err := billingSvc.CreateInvoice(testutil.DefaultOrgID, admin.UserID, admin, billing.CreateInvoiceInput{
		HouseholdID:   household.ID,
		InvoiceNumber: "INV-RUNTIME-1",
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := billingSvc.AddInvoiceItem(testutil.DefaultOrgID, admin.UserID, admin, invoice.ID, billing.AddInvoiceItemInput{
		Description:     "Monthly dues",
		Quantity:        1,
		UnitAmountCents: 12000,
		Category:        "program_fee",
	}); err != nil {
		t.Fatalf("add invoice item: %v", err)
	}

	if _, err := commsSvc.CreateAnnouncement(comms.CreateAnnouncementInput{
		OrgID:     testutil.DefaultOrgID,
		Title:     "Welcome",
		Body:      "Runtime persistence check",
		Audience:  "all",
		CreatedBy: admin.UserID,
	}); err != nil {
		t.Fatalf("create announcement: %v", err)
	}

	if _, err := complianceSvc.CreateDataExportRequest(testutil.DefaultOrgID, household.ID, admin.UserID); err != nil {
		t.Fatalf("create data export request: %v", err)
	}

	participants := buildRuntimeParticipants(
		memberSvc,
		groupSvc,
		meetSvc,
		exportSvc,
		perfSvc,
		billingSvc,
		commsSvc,
		complianceSvc,
		reportsSvc,
	)
	if err := persistRuntimeSnapshots(discardLogger(), repo, participants); err != nil {
		t.Fatalf("persist runtime snapshots: %v", err)
	}

	reloadedMembers := members.NewService()
	reloadedGroups := groups.NewService()
	reloadedMeets := meets.NewService()
	reloadedExports := exports.NewServiceWithStorage(store)
	reloadedPerformance := performance.NewService()
	reloadedBilling := billing.NewService()
	reloadedComms := comms.NewService()
	reloadedCompliance := compliance.NewService()
	reloadedReports := reports.NewService(reloadedMembers, reloadedGroups, reloadedMeets, reloadedBilling)
	reloadedReports.SetStorage(store)

	loadRuntimeSnapshots(discardLogger(), repo, buildRuntimeParticipants(
		reloadedMembers,
		reloadedGroups,
		reloadedMeets,
		reloadedExports,
		reloadedPerformance,
		reloadedBilling,
		reloadedComms,
		reloadedCompliance,
		reloadedReports,
	))

	if got := len(reloadedMembers.SnapshotSwimmers(testutil.DefaultOrgID)); got != 1 {
		t.Fatalf("expected 1 persisted swimmer, got %d", got)
	}
	if got := len(reloadedGroups.SnapshotGroups(testutil.DefaultOrgID)); got != 1 {
		t.Fatalf("expected 1 persisted group, got %d", got)
	}
	if got := len(reloadedGroups.SnapshotRegistrations(testutil.DefaultOrgID)); got != 1 {
		t.Fatalf("expected 1 persisted registration, got %d", got)
	}
	if got := len(reloadedMeets.SnapshotMeets(testutil.DefaultOrgID)); got != 1 {
		t.Fatalf("expected 1 persisted meet, got %d", got)
	}
	if got, err := reloadedExports.ListExports(admin, testutil.DefaultOrgID, ""); err != nil || len(got) != 1 {
		t.Fatalf("expected 1 persisted export, got %d err=%v", len(got), err)
	}
	if _, content, err := reloadedExports.DownloadExport(admin, testutil.DefaultOrgID, exportJob.ID); err != nil || len(content) == 0 {
		t.Fatalf("expected persisted export content, got %d bytes err=%v", len(content), err)
	}
	if got, err := reloadedPerformance.ListBestTimes(testutil.DefaultOrgID, admin, swimmer.ID, performance.ListBestTimesFilters{}); err != nil || len(got) != 1 {
		t.Fatalf("expected 1 persisted best time, got %d err=%v", len(got), err)
	}
	if got := len(reloadedBilling.SnapshotInvoices(testutil.DefaultOrgID)); got != 1 {
		t.Fatalf("expected 1 persisted invoice, got %d", got)
	}
	if got, err := reloadedComms.ListAnnouncements(testutil.DefaultOrgID, ""); err != nil || len(got) != 1 {
		t.Fatalf("expected 1 persisted announcement, got %d err=%v", len(got), err)
	}
	if got, err := reloadedCompliance.ListDataExportRequests(testutil.DefaultOrgID, household.ID); err != nil || len(got) != 1 {
		t.Fatalf("expected 1 persisted compliance export request, got %d err=%v", len(got), err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
