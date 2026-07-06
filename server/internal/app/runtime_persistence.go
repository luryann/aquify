package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

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
)

type runtimeSnapshotter interface {
	ExportRuntimeSnapshot() ([]byte, error)
	LoadRuntimeSnapshot([]byte) error
}

type runtimeParticipant struct {
	domain  string
	service runtimeSnapshotter
}

func buildRuntimeParticipants(
	membershipService *members.Service,
	groupsService *groups.Service,
	meetsService *meets.Service,
	exportsService *exports.Service,
	performanceService *performance.Service,
	billingService *billing.Service,
	commsService *comms.Service,
	complianceService *compliance.Service,
	reportsService *reports.Service,
) []runtimeParticipant {
	return []runtimeParticipant{
		{domain: "members", service: membershipService},
		{domain: "groups", service: groupsService},
		{domain: "meets", service: meetsService},
		{domain: "exports", service: exportsService},
		{domain: "performance", service: performanceService},
		{domain: "billing", service: billingService},
		{domain: "comms", service: commsService},
		{domain: "compliance", service: complianceService},
		{domain: "reports", service: reportsService},
	}
}

func loadRuntimeSnapshots(logger *slog.Logger, repo *runtimepersist.Repository, participants []runtimeParticipant) {
	if repo == nil || !repo.Available() {
		return
	}

	snapshots, err := repo.LoadAll()
	if err != nil {
		logger.Error("runtime snapshot load failed", "error", err)
		return
	}

	for _, participant := range participants {
		if participant.service == nil {
			continue
		}
		payload := snapshots[participant.domain]
		if len(payload) == 0 || string(payload) == "null" {
			continue
		}
		if err := participant.service.LoadRuntimeSnapshot(payload); err != nil {
			logger.Error("runtime snapshot hydrate failed", "domain", participant.domain, "error", err)
		}
	}
}

func persistRuntimeSnapshots(logger *slog.Logger, repo *runtimepersist.Repository, participants []runtimeParticipant) error {
	if repo == nil || !repo.Available() {
		return nil
	}

	snapshots := make(map[string]json.RawMessage, len(participants))
	for _, participant := range participants {
		if participant.service == nil {
			continue
		}
		payload, err := participant.service.ExportRuntimeSnapshot()
		if err != nil {
			return err
		}
		snapshots[participant.domain] = json.RawMessage(payload)
	}

	if err := repo.SaveAll(snapshots); err != nil {
		logger.Error("runtime snapshot save failed", "error", err)
		return err
	}
	return nil
}

func runtimePersistenceMiddleware(logger *slog.Logger, repo *runtimepersist.Repository, participants []runtimeParticipant) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if repo == nil || !repo.Available() {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !shouldPersistMutation(r) {
				next.ServeHTTP(w, r)
				return
			}

			recorder := newBufferedResponse()
			next.ServeHTTP(recorder, r)

			if recorder.status >= http.StatusBadRequest {
				recorder.FlushTo(w)
				return
			}

			// The runtime-snapshot bridge is a backward-compat, best-effort mirror
			// of in-memory state; the DB pool is the canonical store. A snapshot
			// save failure must never turn an already-committed mutation into a
			// 500 for the client — log it and return the successful response.
			if err := persistRuntimeSnapshots(logger, repo, participants); err != nil {
				logger.Error("runtime snapshot persistence failed after successful mutation; returning success (DB write is canonical)",
					"error", err, "method", r.Method, "path", r.URL.Path)
			}

			recorder.FlushTo(w)
		})
	}
}

func shouldPersistMutation(r *http.Request) bool {
	if !isMutatingMethod(r.Method) {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
		return false
	}
	return true
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (r *bufferedResponse) Header() http.Header {
	return r.header
}

func (r *bufferedResponse) WriteHeader(status int) {
	r.status = status
}

func (r *bufferedResponse) Write(p []byte) (int, error) {
	return r.body.Write(p)
}

func (r *bufferedResponse) FlushTo(w http.ResponseWriter) {
	dst := w.Header()
	for key := range dst {
		dst.Del(key)
	}
	for key, values := range r.header {
		copied := append([]string{}, values...)
		dst[key] = copied
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body.Bytes())
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
