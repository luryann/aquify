package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/compliance"
	"github.com/dareaquatics/aquify/server/internal/app/documents"
	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	"github.com/dareaquatics/aquify/server/internal/app/reports"
	"github.com/dareaquatics/aquify/server/internal/keycloak"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

const complianceExportTTL = 30 * 24 * time.Hour

// PurgeExpiredFilesPayload is the JSON payload for the TypePurgeExpiredFiles task.
type PurgeExpiredFilesPayload struct {
	OrgID string `json:"org_id"`
	// CutoffTime is the ISO-8601 timestamp; files older than this are purged.
	// If empty, defaults to now minus the configured export TTL.
	CutoffTime string `json:"cutoff_time,omitempty"`
}

// ProcessDataExportPayload is the JSON payload for the TypeProcessDataExport task.
type ProcessDataExportPayload struct {
	RequestID   string `json:"request_id"`
	OrgID       string `json:"org_id"`
	HouseholdID string `json:"household_id"`
}

// ProcessDataDeletionPayload is the JSON payload for the TypeProcessDataDeletion task.
type ProcessDataDeletionPayload struct {
	RequestID   string `json:"request_id"`
	OrgID       string `json:"org_id"`
	HouseholdID string `json:"household_id"`
}

// PurgeExpiredFilesHandler returns a Handler that removes expired temp files
// (export objects in object storage).
func PurgeExpiredFilesHandler(logger *slog.Logger, exportsService *exports.Service, reportsService *reports.Service) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p PurgeExpiredFilesPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("purge expired files: decode payload: %w", err)
		}

		logger.Info("purge expired files: starting", "org_id", p.OrgID)

		cutoff := parseCutoffTime(p.CutoffTime)

		if exportsService != nil {
			for _, artifact := range exportsService.ExpireArtifacts(cutoff) {
				if artifact.StorageKey != "" {
					_ = exportsService.DeleteArtifact(artifact.StorageKey)
				}
			}
		}
		if reportsService != nil {
			for _, artifact := range reportsService.ExpireArtifacts(cutoff) {
				if artifact.StorageKey != "" {
					_ = reportsService.DeleteArtifact(artifact.StorageKey)
				}
			}
		}

		logger.Info("purge expired files: completed", "org_id", p.OrgID)
		return nil
	}
}

func parseCutoffTime(raw string) (cutoff time.Time) {
	cutoff = time.Now().UTC()
	if raw == "" {
		return cutoff
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return cutoff
	}
	return parsed.UTC()
}

// ProcessDataExportHandler returns a Handler that generates a GDPR/compliance
// data export record for a household and stores the export artifact.
func ProcessDataExportHandler(logger *slog.Logger, complianceService *compliance.Service, store storage.ObjectStorage) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p ProcessDataExportPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("process data export: decode payload: %w", err)
		}
		if p.RequestID == "" || p.OrgID == "" || p.HouseholdID == "" {
			return fmt.Errorf("process data export: missing required fields")
		}
		if complianceService == nil {
			return fmt.Errorf("process data export: compliance service is not configured")
		}
		if store == nil {
			return fmt.Errorf("process data export: object storage is not configured")
		}

		logger.Info("process data export: starting",
			"request_id", p.RequestID,
			"org_id", p.OrgID,
			"household_id", p.HouseholdID,
		)

		req, err := complianceService.GetDataExportRequest(p.OrgID, p.RequestID)
		if err != nil {
			return fmt.Errorf("process data export: load request %s: %w", p.RequestID, err)
		}
		if req.HouseholdID != p.HouseholdID {
			return fmt.Errorf("process data export: household mismatch for request %s", p.RequestID)
		}
		if _, err := complianceService.MarkDataExportProcessing(p.OrgID, p.RequestID); err != nil {
			return fmt.Errorf("process data export: mark processing: %w", err)
		}

		generatedAt := time.Now().UTC()
		artifact := map[string]any{
			"request_id":   req.ID,
			"org_id":       req.OrgID,
			"household_id": req.HouseholdID,
			"requested_by": req.RequestedBy,
			"requested_at": req.RequestedAt,
			"generated_at": generatedAt,
			"source":       "compliance_worker",
		}
		data, err := json.MarshalIndent(artifact, "", "  ")
		if err != nil {
			_, _ = complianceService.FailDataExport(p.OrgID, p.RequestID)
			return fmt.Errorf("process data export: marshal artifact: %w", err)
		}

		key := complianceExportStorageKey(p.OrgID, p.RequestID)
		if err := store.PutObject(ctx, key, "application/json; charset=utf-8", data); err != nil {
			_, _ = complianceService.FailDataExport(p.OrgID, p.RequestID)
			return fmt.Errorf("process data export: store artifact %s: %w", key, err)
		}
		downloadURL, err := store.PresignedGetURL(key, complianceExportTTL)
		if err != nil {
			_, _ = complianceService.FailDataExport(p.OrgID, p.RequestID)
			return fmt.Errorf("process data export: presign artifact %s: %w", key, err)
		}

		expiresAt := generatedAt.Add(complianceExportTTL)
		if _, err := complianceService.CompleteDataExport(p.OrgID, p.RequestID, downloadURL, expiresAt); err != nil {
			_, _ = complianceService.FailDataExport(p.OrgID, p.RequestID)
			return fmt.Errorf("process data export: complete request %s: %w", p.RequestID, err)
		}

		logger.Info("process data export: completed", "request_id", p.RequestID, "storage_key", key)
		return nil
	}
}

// ProcessDataDeletionHandler returns a Handler that performs an approved
// compliance right-to-erasure workflow. Completion is gated on actually
// performing every erasure step — a request must never be marked complete
// without the real erasure steps having succeeded first.
//
// Scope (per owner decision recorded in AUDIT_FINDINGS_2026-07-05.md,
// SEC-012): billing/financial ledger records (invoices, payments, refunds,
// Stripe references) are RETAINED for tax/audit purposes — only the
// household's link to them is anonymized (handled by
// membersService.AnonymizeHousehold, which scrubs the household record
// itself). Comms, documents, the underlying object-storage files, and the
// household's Keycloak login account(s) are FULLY ERASED — no retention
// exception for those domains.
func ProcessDataDeletionHandler(
	logger *slog.Logger,
	complianceService *compliance.Service,
	membersService *members.Service,
	commsService *comms.Service,
	documentsService *documents.Service,
	objectStore storage.ObjectStorage,
	kcClient keycloak.Client,
) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p ProcessDataDeletionPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("process data deletion: decode payload: %w", err)
		}
		if p.RequestID == "" || p.OrgID == "" || p.HouseholdID == "" {
			return fmt.Errorf("process data deletion: missing required fields")
		}
		if complianceService == nil {
			return fmt.Errorf("process data deletion: compliance service is not configured")
		}
		if membersService == nil {
			return fmt.Errorf("process data deletion: members service is not configured; cannot perform erasure")
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		logger.Info("process data deletion: starting",
			"request_id", p.RequestID,
			"org_id", p.OrgID,
			"household_id", p.HouseholdID,
		)

		req, err := complianceService.GetDataDeletionRequest(p.OrgID, p.RequestID)
		if err != nil {
			return fmt.Errorf("process data deletion: load request %s: %w", p.RequestID, err)
		}
		if req.HouseholdID != p.HouseholdID {
			return fmt.Errorf("process data deletion: household mismatch for request %s", p.RequestID)
		}
		if _, err := complianceService.MarkDataDeletionProcessing(p.OrgID, p.RequestID); err != nil {
			return fmt.Errorf("process data deletion: mark processing: %w", err)
		}

		fail := func(step string, cause error) error {
			_, _ = complianceService.FailDataDeletion(p.OrgID, p.RequestID, fmt.Sprintf("Erasure failed at %s: %v", step, cause))
			return fmt.Errorf("process data deletion: %s: %w", step, cause)
		}

		// Capture cross-domain identifiers BEFORE anonymizing the household —
		// AnonymizeHousehold clears household.KeycloakUserID and every
		// contact's Email, which are needed to erase comms/auth records.
		erasureCtx, err := membersService.GetHouseholdErasureContext(p.OrgID, p.HouseholdID)
		if err != nil {
			return fail("capture erasure context", err)
		}

		if err := membersService.AnonymizeHousehold(p.OrgID, p.HouseholdID); err != nil {
			return fail(fmt.Sprintf("anonymize household %s", p.HouseholdID), err)
		}

		var purgedObjectKeys []string

		if commsService != nil {
			keys, err := commsService.EraseHouseholdData(ctx, p.OrgID, p.HouseholdID, erasureCtx.ContactEmails)
			if err != nil {
				return fail("erase comms records", err)
			}
			purgedObjectKeys = append(purgedObjectKeys, keys...)
		}

		if documentsService != nil {
			keys, err := documentsService.EraseHouseholdData(ctx, p.OrgID, p.HouseholdID)
			if err != nil {
				return fail("erase document submissions", err)
			}
			purgedObjectKeys = append(purgedObjectKeys, keys...)
		}

		if objectStore != nil {
			for _, key := range purgedObjectKeys {
				if key == "" {
					continue
				}
				if err := objectStore.Delete(key); err != nil {
					return fail(fmt.Sprintf("purge object storage key %s", key), err)
				}
			}
		}

		if kcClient != nil {
			for _, userID := range erasureCtx.KeycloakUserIDs {
				if userID == "" {
					continue
				}
				if err := kcClient.DeleteUser(ctx, userID); err != nil {
					return fail(fmt.Sprintf("delete keycloak user %s", userID), err)
				}
			}
		}

		if _, err := complianceService.CompleteDataDeletion(p.OrgID, p.RequestID, "Household PII anonymized (members domain); comms, document submissions, object-storage files, and Keycloak login account(s) fully erased; billing/financial ledger records retained per tax/audit requirements (household link anonymized only)."); err != nil {
			return fail(fmt.Sprintf("complete request %s", p.RequestID), err)
		}

		logger.Info("process data deletion: completed",
			"request_id", p.RequestID,
			"purged_object_keys", len(purgedObjectKeys),
			"deleted_keycloak_users", len(erasureCtx.KeycloakUserIDs),
		)
		return nil
	}
}

func complianceExportStorageKey(orgID, requestID string) string {
	orgID = strings.Trim(strings.TrimSpace(orgID), "/")
	requestID = strings.Trim(strings.TrimSpace(requestID), "/")
	return fmt.Sprintf("compliance/%s/exports/%s.json", orgID, requestID)
}
