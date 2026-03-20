package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/dareaquatics/aquify/server/internal/app/exports"
	"github.com/dareaquatics/aquify/server/internal/workers"
)

// GenerateExportHandler returns a Handler that processes an SD3 export job.
func GenerateExportHandler(logger *slog.Logger, service *exports.Service) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p exports.GenerateExportTaskPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("generate export: decode payload: %w", err)
		}
		if p.ExportID == "" {
			return fmt.Errorf("generate export: missing export_id")
		}
		if service == nil {
			return fmt.Errorf("generate export: exports service is not configured")
		}

		logger.Info("generate export: starting", "export_id", p.ExportID)
		if err := service.ProcessExport(ctx, p.ExportID); err != nil {
			return err
		}
		logger.Info("generate export: completed", "export_id", p.ExportID)
		return nil
	}
}
