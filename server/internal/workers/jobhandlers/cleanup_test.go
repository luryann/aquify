package jobhandlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

func TestPurgeExpiredFilesHandler_ExpiresExportsAndReports(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := storage.NewStubStorage("")
	exportsService := exports.NewServiceWithStorage(store)
	reportsService := reports.NewService(nil, nil, nil, nil)
	reportsService.SetStorage(store)

	principal := &appmiddleware.Principal{UserID: "admin-user", OrgID: "org-cleanup", Roles: []string{"admin"}}

	exportsService.ConfigureArtifactTTL(time.Millisecond)
	exportRecord, _, err := exportsService.GenerateExport(principal, "org-cleanup", "meet-cleanup", exports.ReportInput{
		ReportType: exports.ReportTypeEvent,
		Meet: exports.ReportMeet{
			Name:      "Cleanup Meet",
			StartDate: "2026-03-15",
			EndDate:   "2026-03-16",
			Course:    "SCY",
		},
	})
	if err != nil {
		t.Fatalf("GenerateExport: %v", err)
	}

	reportsService.ConfigureArtifactTTL(time.Millisecond)
	reportArtifact, err := reportsService.StoreCSVArtifact("org-cleanup", "invoice_aging", time.Now().UTC(), &reports.CSVDocument{
		Filename: "invoice-aging.csv",
		Data:     []byte("invoice_id\ninv-1\n"),
	})
	if err != nil {
		t.Fatalf("StoreCSVArtifact: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	cutoff := time.Now().UTC()
	task, err := workers.NewTask(workers.TypePurgeExpiredFiles, PurgeExpiredFilesPayload{
		OrgID:      "org-cleanup",
		CutoffTime: cutoff.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := PurgeExpiredFilesHandler(logger, exportsService, reportsService)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	exportAfter, err := exportsService.GetExport(principal, "org-cleanup", exportRecord.ID)
	if err != nil {
		t.Fatalf("GetExport: %v", err)
	}
	if exportAfter.Status != "expired" {
		t.Fatalf("export status = %q, want expired", exportAfter.Status)
	}

	reportSnapshot, err := reportsService.ExportRuntimeSnapshot()
	if err != nil {
		t.Fatalf("ExportRuntimeSnapshot: %v", err)
	}
	var snapshot struct {
		Files map[string]reports.StoredArtifact `json:"files"`
	}
	if err := json.Unmarshal(reportSnapshot, &snapshot); err != nil {
		t.Fatalf("decode report snapshot: %v", err)
	}
	if len(snapshot.Files) != 0 {
		t.Fatalf("expected report artifacts to be purged, got %d", len(snapshot.Files))
	}

	for _, key := range []string{exportRecord.StorageKey, reportArtifact.StorageKey} {
		exists, err := store.Exists(context.Background(), key)
		if err != nil {
			t.Fatalf("Exists(%q): %v", key, err)
		}
		if exists {
			t.Fatalf("expected storage key %q to be deleted", key)
		}
	}
}

func TestProcessDataExportHandler_CompletesRequestAndStoresArtifact(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	complianceService := compliance.NewService()
	store := storage.NewStubStorage("http://storage.test/aquify")
	req, err := complianceService.CreateDataExportRequest("org-export", "hh-export", "admin-user")
	if err != nil {
		t.Fatalf("CreateDataExportRequest: %v", err)
	}
	task, err := workers.NewTask(workers.TypeProcessDataExport, ProcessDataExportPayload{
		RequestID:   req.ID,
		OrgID:       "org-export",
		HouseholdID: "hh-export",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := ProcessDataExportHandler(logger, complianceService, store)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	after, err := complianceService.GetDataExportRequest("org-export", req.ID)
	if err != nil {
		t.Fatalf("GetDataExportRequest: %v", err)
	}
	if after.Status != "ready" {
		t.Fatalf("status = %q, want ready", after.Status)
	}
	if after.DownloadURL == "" {
		t.Fatal("DownloadURL is empty")
	}
	if after.CompletedAt == nil {
		t.Fatal("CompletedAt is nil")
	}

	key := complianceExportStorageKey("org-export", req.ID)
	data, contentType, err := store.GetObject(context.Background(), key)
	if err != nil {
		t.Fatalf("GetObject(%q): %v", key, err)
	}
	if contentType != "application/json; charset=utf-8" {
		t.Fatalf("contentType = %q", contentType)
	}
	var artifact map[string]any
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	if artifact["request_id"] != req.ID || artifact["household_id"] != "hh-export" {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}
}

func TestProcessDataDeletionHandler_CompletesRequest(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	complianceService := compliance.NewService()
	membersService := members.NewService()
	commsService := comms.NewService()
	documentsService := documents.NewService()
	objectStore := storage.NewStubStorage("")
	kcClient := keycloak.NewStubClient()

	principal := &appmiddleware.Principal{UserID: "admin-user", OrgID: "org-delete", Roles: []string{"admin"}}
	household, err := membersService.CreateHousehold(principal, members.CreateHouseholdInput{
		OrgID:         "org-delete",
		HouseholdName: "The Delete Household",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}

	req, err := complianceService.CreateDataDeletionRequest("org-delete", household.ID, "admin-user", "requested")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest: %v", err)
	}
	task, err := workers.NewTask(workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{
		RequestID:   req.ID,
		OrgID:       "org-delete",
		HouseholdID: household.ID,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := ProcessDataDeletionHandler(logger, complianceService, membersService, commsService, documentsService, objectStore, kcClient)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	anonymized, err := membersService.GetHousehold(principal, "org-delete", household.ID)
	if err != nil {
		t.Fatalf("GetHousehold: %v", err)
	}
	if anonymized.HouseholdName != "Deleted Household" {
		t.Fatalf("household not anonymized: %+v", anonymized)
	}

	after, err := complianceService.GetDataDeletionRequest("org-delete", req.ID)
	if err != nil {
		t.Fatalf("GetDataDeletionRequest: %v", err)
	}
	if after.Status != "completed" {
		t.Fatalf("status = %q, want completed", after.Status)
	}
	if after.CompletedAt == nil {
		t.Fatal("CompletedAt is nil")
	}
	if after.ReviewNotes == "" {
		t.Fatal("ReviewNotes is empty")
	}
}

// TestProcessDataDeletionHandler_ErasesCrossDomainRecords proves the SEC-012
// follow-up actually erases comms, documents, storage, and the Keycloak
// account for a household — not just anonymizing the members-domain record.
func TestProcessDataDeletionHandler_ErasesCrossDomainRecords(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	complianceService := compliance.NewService()
	membersService := members.NewService()
	commsService := comms.NewService()
	documentsService := documents.NewService()
	objectStore := storage.NewStubStorage("")
	kcClient := keycloak.NewStubClient()

	principal := &appmiddleware.Principal{UserID: "admin-user", OrgID: "org-erase", Roles: []string{"admin"}}
	household, err := membersService.CreateHousehold(principal, members.CreateHouseholdInput{
		OrgID:         "org-erase",
		HouseholdName: "The Erasure Household",
		PrimaryEmail:  "parent@example.test",
	})
	if err != nil {
		t.Fatalf("CreateHousehold: %v", err)
	}
	swimmer, _, err := membersService.CreateSwimmer(principal, "org-erase", members.CreateSwimmerInput{
		HouseholdID: household.ID,
		FirstName:   "Test",
		LastName:    "Swimmer",
		DateOfBirth: "2012-01-01",
		Gender:      "female",
	})
	if err != nil {
		t.Fatalf("CreateSwimmer: %v", err)
	}

	// Link a Keycloak account to the household (parent login).
	kcUserID, err := kcClient.CreateUser(context.Background(), "parent@example.test", "Parent", "Test", "org-erase")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := membersService.LinkHouseholdToUser("org-erase", household.ID, kcUserID); err != nil {
		t.Fatalf("LinkHouseholdToUser: %v", err)
	}

	// Seed a message with an attachment, so both the message and its object
	// key must be erased.
	const attachmentKey = "messages/org-erase/attachment.png"
	if err := objectStore.PutObject(context.Background(), attachmentKey, "image/png", []byte("fake-image")); err != nil {
		t.Fatalf("PutObject(attachment): %v", err)
	}
	commsService.SetStorage(objectStore)
	if _, err := commsService.SendMessage(comms.CreateMessageInput{
		OrgID:        "org-erase",
		HouseholdID:  household.ID,
		SwimmerID:    swimmer.ID,
		Body:         "hello coach",
		SenderUserID: kcUserID,
		SenderRole:   "parent",
		Attachments: []comms.AttachmentInput{
			{ObjectKey: attachmentKey, Filename: "attachment.png", MimeType: "image/png", SizeBytes: 10},
		},
	}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Seed a document submission with an uploaded file.
	const docKey = "documents/org-erase/waiver.pdf"
	if err := objectStore.PutObject(context.Background(), docKey, "application/pdf", []byte("fake-pdf")); err != nil {
		t.Fatalf("PutObject(document): %v", err)
	}
	docType, err := documentsService.CreateDocumentType("org-erase", "Waiver", "waiver", "", "swimmer", true, true, nil, "admin-user")
	if err != nil {
		t.Fatalf("CreateDocumentType: %v", err)
	}
	submission, err := documentsService.CreateSubmission("org-erase", docType.ID, household.ID, swimmer.ID, "admin-user")
	if err != nil {
		t.Fatalf("CreateSubmission: %v", err)
	}
	if _, err := documentsService.AttachFile("org-erase", submission.ID, docKey, "waiver.pdf", "application/pdf", 8, "admin-user"); err != nil {
		t.Fatalf("AttachFile: %v", err)
	}

	req, err := complianceService.CreateDataDeletionRequest("org-erase", household.ID, "admin-user", "requested")
	if err != nil {
		t.Fatalf("CreateDataDeletionRequest: %v", err)
	}
	task, err := workers.NewTask(workers.TypeProcessDataDeletion, ProcessDataDeletionPayload{
		RequestID:   req.ID,
		OrgID:       "org-erase",
		HouseholdID: household.ID,
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	handler := ProcessDataDeletionHandler(logger, complianceService, membersService, commsService, documentsService, objectStore, kcClient)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}

	after, err := complianceService.GetDataDeletionRequest("org-erase", req.ID)
	if err != nil {
		t.Fatalf("GetDataDeletionRequest: %v", err)
	}
	if after.Status != "completed" {
		t.Fatalf("status = %q, want completed", after.Status)
	}

	if threads := commsService.ListMessages("org-erase", household.ID, swimmer.ID, kcUserID); len(threads) != 0 {
		t.Fatalf("expected messages erased, got %d", len(threads))
	}
	if exists, err := objectStore.Exists(context.Background(), attachmentKey); err != nil {
		t.Fatalf("Exists(attachment): %v", err)
	} else if exists {
		t.Fatal("expected message attachment object to be purged from storage")
	}

	if subs, err := documentsService.ListSubmissions("org-erase", household.ID); err != nil {
		t.Fatalf("ListSubmissions: %v", err)
	} else if len(subs) != 0 {
		t.Fatalf("expected document submissions erased, got %d", len(subs))
	}
	if exists, err := objectStore.Exists(context.Background(), docKey); err != nil {
		t.Fatalf("Exists(document): %v", err)
	} else if exists {
		t.Fatal("expected document object to be purged from storage")
	}

	if _, err := kcClient.GetUser(context.Background(), kcUserID); err == nil {
		t.Fatal("expected Keycloak user account to be deleted")
	}
}

type flakyDeleteStore struct {
	*storage.StubStorage
	failures int
}

func (f *flakyDeleteStore) Delete(key string) error {
	if f.failures > 0 {
		f.failures--
		return io.ErrUnexpectedEOF
	}
	return f.StubStorage.Delete(key)
}

// A failed object delete must leave the artifact pending (not marked expired)
// so the next sweep retries it, instead of dropping the object.
func TestPurgeExpiredFilesHandler_RetriesFailedStorageDelete(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &flakyDeleteStore{StubStorage: storage.NewStubStorage(""), failures: 1}
	exportsService := exports.NewServiceWithStorage(store)
	principal := &appmiddleware.Principal{UserID: "admin-user", OrgID: "org-retry", Roles: []string{"admin"}}
	exportsService.ConfigureArtifactTTL(time.Millisecond)
	rec, _, err := exportsService.GenerateExport(principal, "org-retry", "meet-retry", exports.ReportInput{
		ReportType: exports.ReportTypeEvent,
		Meet:       exports.ReportMeet{Name: "Retry Meet", StartDate: "2026-03-15", EndDate: "2026-03-16", Course: "SCY"},
	})
	if err != nil {
		t.Fatalf("GenerateExport: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	task, _ := workers.NewTask(workers.TypePurgeExpiredFiles, PurgeExpiredFilesPayload{OrgID: "org-retry"})
	handler := PurgeExpiredFilesHandler(logger, exportsService, nil)

	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if exists, _ := store.Exists(context.Background(), rec.StorageKey); !exists {
		t.Fatal("object should still exist after failed delete")
	}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("retry run: %v", err)
	}
	if exists, _ := store.Exists(context.Background(), rec.StorageKey); exists {
		t.Fatal("object should be deleted by the retry")
	}
}
