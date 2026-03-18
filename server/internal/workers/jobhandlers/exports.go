package jobhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dareaquatics/aquify/server/internal/workers"
)

// GenerateExportPayload is the JSON payload for the TypeGenerateExport task.
type GenerateExportPayload struct {
	ExportID   string `json:"export_id"`
	OrgID      string `json:"org_id"`
	MeetID     string `json:"meet_id"`
	ExportType string `json:"export_type"`
}

// GenerateExportHandler returns a Handler that processes an SD3 export job.
func GenerateExportHandler(logger *slog.Logger) workers.Handler {
	return func(ctx context.Context, task *workers.Task) error {
		var p GenerateExportPayload
		if err := json.Unmarshal(task.Payload, &p); err != nil {
			return fmt.Errorf("generate export: decode payload: %w", err)
		}
		if p.ExportID == "" || p.MeetID == "" {
			return fmt.Errorf("generate export: missing required fields export_id or meet_id")
		}

		logger.Info("generate export: starting",
			"export_id", p.ExportID,
			"meet_id", p.MeetID,
			"export_type", p.ExportType,
		)

		// Scaffold: simulate generation.
		// Production: fetch meet entries → call sdif.Exporter → write to object
		// storage → update exports.Service status to "ready" with download URL.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}

		logger.Info("generate export: completed", "export_id", p.ExportID)
		return nil
	}
}
