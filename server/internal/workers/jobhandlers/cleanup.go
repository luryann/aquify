package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dareaquatics/aquify/server/internal/workers"
)

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
// (export objects in object storage and expired import upload URLs).
func PurgeExpiredFilesHandler(logger *slog.Logger) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p PurgeExpiredFilesPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("purge expired files: decode payload: %w", err)
		}

		logger.Info("purge expired files: starting", "org_id", p.OrgID)

		// Scaffold: simulate purge work.
		// Production:
		//   - List exports.Service.ListExpiredExports(cutoff) → delete from storage
		//   - List imports.Service.ListExpiredUploadURLs(cutoff) → mark as expired
		//   - List compliance exports past TTL → transition to "expired"
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		logger.Info("purge expired files: completed", "org_id", p.OrgID)
		return nil
	}
}

// ProcessDataExportHandler returns a Handler that generates a GDPR/compliance
// data export archive for a household.
func ProcessDataExportHandler(logger *slog.Logger) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p ProcessDataExportPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("process data export: decode payload: %w", err)
		}
		if p.RequestID == "" || p.HouseholdID == "" {
			return fmt.Errorf("process data export: missing required fields")
		}

		logger.Info("process data export: starting",
			"request_id", p.RequestID,
			"household_id", p.HouseholdID,
		)

		// Scaffold: simulate export generation.
		// Production: collect household + swimmer + invoice + audit data →
		// marshal to JSON/CSV zip archive → upload to storage →
		// call compliance.Service.CompleteExport(requestID, downloadURL, expiresAt).
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		logger.Info("process data export: completed", "request_id", p.RequestID)
		return nil
	}
}

// ProcessDataDeletionHandler returns a Handler that anonymises/deletes household
// data per a GDPR right-to-erasure request.
func ProcessDataDeletionHandler(logger *slog.Logger) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p ProcessDataDeletionPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("process data deletion: decode payload: %w", err)
		}
		if p.RequestID == "" || p.HouseholdID == "" {
			return fmt.Errorf("process data deletion: missing required fields")
		}

		logger.Info("process data deletion: starting",
			"request_id", p.RequestID,
			"household_id", p.HouseholdID,
		)

		// Scaffold: simulate deletion.
		// Production: anonymise PII in swimmers, contacts, users tables →
		// soft-delete or redact financial records within legal retention limits →
		// update compliance.Service.CompleteDeletion(requestID).
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		logger.Info("process data deletion: completed", "request_id", p.RequestID)
		return nil
	}
}
