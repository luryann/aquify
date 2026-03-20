// Package jobhandlers contains the concrete handler functions for each
// background task type. Each handler satisfies workers.Handler.
package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dareaquatics/aquify/server/internal/workers"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const tracerNameImports = "github.com/dareaquatics/aquify/server/internal/workers/jobhandlers/imports"

// ParseImportPayload is the JSON payload for the TypeParseImport task.
type ParseImportPayload struct {
	ImportID   string `json:"import_id"`
	OrgID      string `json:"org_id"`
	StorageKey string `json:"storage_key"`
	FileType   string `json:"file_type"`
	MeetID     string `json:"meet_id,omitempty"`
}

// ParseImportHandler returns a Handler that processes an import parse job.
// In the scaffold, parsing is simulated; in production this calls the
// internal/meetfiles parser and writes results back to the imports service.
func ParseImportHandler(logger *slog.Logger) workers.Handler {
	tracer := otel.Tracer(tracerNameImports)

	return func(ctx context.Context, task *workers.Task) error {
		ctx, span := tracer.Start(ctx, "job.parse_import")
		defer span.End()

		var p ParseImportPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "decode payload failed")
			return fmt.Errorf("parse import: decode payload: %w", err)
		}
		if p.ImportID == "" || p.OrgID == "" {
			err := fmt.Errorf("parse import: missing required fields import_id or org_id")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		span.SetAttributes(
			attribute.String("import.id", p.ImportID),
			attribute.String("org.id", p.OrgID),
			attribute.String("import.file_type", p.FileType),
			attribute.String("import.storage_key", p.StorageKey),
		)

		logger.Info("parse import: starting",
			"import_id", p.ImportID,
			"org_id", p.OrgID,
			"file_type", p.FileType,
			"storage_key", p.StorageKey,
		)

		// Scaffold: simulate parse work.
		// Production: fetch file from object storage → run meetfiles parser →
		// call imports.Service.ApplyResults(ctx, importID, records).
		select {
		case <-ctx.Done():
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "context cancelled")
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		span.SetStatus(codes.Ok, "")
		logger.Info("parse import: completed", "import_id", p.ImportID)
		return nil
	}
}
