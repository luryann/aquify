package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
)

const maxStandardsFileBytes = 5 << 20

// importTimeStandards handles POST /time-standards/import: a multipart CSV
// ("file" field) of cut times. With ?preview=true it only parses and plans
// (nothing is written, not audited); otherwise it creates/updates every
// standard in one atomic write, or writes nothing and returns 422 with the
// per-row errors. Admin only. CSV only (see performance/standards_import.go).
func (h *PerformanceHandlers) importTimeStandards(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
		return
	}
	preview, ok := queryBool(w, r, "preview")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxStandardsFileBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_file", "Multipart field 'file' (a .csv) is required.", nil)
		return
	}
	defer file.Close()
	lower := strings.ToLower(header.Filename)
	if strings.HasSuffix(lower, ".xlsx") || strings.HasSuffix(lower, ".xls") {
		httpx.WriteError(w, r, http.StatusUnsupportedMediaType, "unsupported_file_type", "Spreadsheets are not supported; export the sheet as CSV and upload that.", nil)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStandardsFileBytes+1))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_file", "Failed to read uploaded file.", nil)
		return
	}
	if len(data) > maxStandardsFileBytes {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "File exceeds the 5 MB upload cap.", nil)
		return
	}
	rows, err := performance.ParseTimeStandardsCSV(data)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_standards_file", err.Error(), nil)
		return
	}

	if preview {
		planned, err := h.service.PlanTimeStandardsImport(orgID, principal, rows)
		if err != nil {
			writeStandardsImportErr(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"preview": true,
			"rows":    planned,
			"counts":  performance.CountStandardsImport(planned),
		})
		return
	}

	planned, err := h.service.CommitTimeStandardsImport(orgID, principal.UserID, principal, rows)
	if err != nil {
		if errors.Is(err, performance.ErrImportHasErrors) {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "import_has_errors",
				"Fix the row errors and upload again; nothing was imported.",
				map[string]any{"rows": planned, "counts": performance.CountStandardsImport(planned)})
			return
		}
		writeStandardsImportErr(w, r, err)
		return
	}
	counts := performance.CountStandardsImport(planned)
	recordAudit(h.auditService, principal, orgID, "time_standards.import", "time_standard", orgID, nil, nil, map[string]string{
		"file":      header.Filename,
		"created":   fmt.Sprint(counts.Create),
		"updated":   fmt.Sprint(counts.Update),
		"unchanged": fmt.Sprint(counts.Unchanged),
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"preview":  false,
		"imported": true,
		"rows":     planned,
		"counts":   counts,
	})
}

func writeStandardsImportErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, performance.ErrForbidden) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage time standards.", nil)
		return
	}
	httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "An internal error occurred.", err)
}
