// Package jobhandlers contains the concrete handler functions for each
// background task type. Each handler satisfies workers.Handler.
package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/dareaquatics/aquify/server/internal/app/imports"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const tracerNameImports = "github.com/dareaquatics/aquify/server/internal/workers/jobhandlers/imports"

// ParseImportHandler returns a Handler that processes an import parse job.
func ParseImportHandler(logger *slog.Logger, service *imports.Service) workers.Handler {
	tracer := otel.Tracer(tracerNameImports)

	return func(ctx context.Context, task *workers.Task) error {
		ctx, span := tracer.Start(ctx, "job.parse_import")
		defer span.End()

		var payload struct {
			ImportID string `json:"import_id"`
		}
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "decode payload failed")
			return fmt.Errorf("parse import: decode payload: %w", err)
		}
		if payload.ImportID == "" {
			err := fmt.Errorf("parse import: missing required field import_id")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		if service == nil {
			err := fmt.Errorf("parse import: imports service is not configured")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		span.SetAttributes(attribute.String("import.id", payload.ImportID))
		logger.Info("parse import: starting", "import_id", payload.ImportID)

		if err := service.ProcessImport(ctx, payload.ImportID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "process import failed")
			return err
		}

		span.SetStatus(codes.Ok, "")
		logger.Info("parse import: completed", "import_id", payload.ImportID)
		return nil
	}
}
