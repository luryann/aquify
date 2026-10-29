package app

import (
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
	chimiddleware "github.com/go-chi/chi/v5/middleware"
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
	return persistRuntimeSnapshotDomains(logger, repo, participants, nil)
}

// persistRuntimeSnapshotDomains exports and saves only the named domains
// (nil = every participant), so one mutation does not re-serialise all state.
func persistRuntimeSnapshotDomains(logger *slog.Logger, repo *runtimepersist.Repository, participants []runtimeParticipant, domains map[string]struct{}) error {
	if repo == nil || !repo.Available() {
		return nil
	}

	snapshots := make(map[string]json.RawMessage, len(participants))
	for _, participant := range participants {
		if participant.service == nil {
			continue
		}
		if domains != nil {
			if _, ok := domains[participant.domain]; !ok {
				continue
			}
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

// snapshotDomainsByPrefix maps the first path segment under /api/v1 to the
// snapshot domains a mutation there can change, including cross-domain hooks.
// Unlisted segments conservatively snapshot every domain.
var snapshotDomainsByPrefix = func() map[string][]string {
	people := []string{"members", "groups", "meets", "billing", "performance"}
	meetDomains := []string{"meets", "exports", "performance", "groups", "billing"}
	perf := []string{"performance", "meets"}
	bill := []string{"billing", "comms"}
	comm := []string{"comms"}
	m := map[string][]string{}
	set := func(domains []string, prefixes ...string) {
		for _, p := range prefixes {
			m[p] = domains
		}
	}
	set(people, "households", "swimmer-groups", "swimmers", "groups", "registrations", "swimmer-link-requests")
	set(meetDomains, "meets", "exports")
	set(perf, "performance", "results", "time-standards")
	set(bill, "billing", "invoices", "payments", "ledger-events", "autopay", "stripe", "webhooks")
	set(comm, "announcements", "communication-preferences", "email-campaigns", "message-threads", "messages", "suppression")
	set([]string{"compliance", "members", "groups", "comms", "billing"}, "compliance")
	set([]string{"reports"}, "reports", "invoice-aging", "invoice-detail", "meet-fees", "membership-status", "payment-reconciliation", "registration-status")
	// Domains that are not part of the snapshot bridge: Postgres is their only
	// store, so a mutation there needs no snapshot work at all.
	set([]string{}, "club-events", "calendar", "org-settings", "wellness", "volunteer", "training", "practices",
		"workouts", "training-phases", "seasons", "jobs", "audit-events", "document-types", "documents", "users", "public")
	return m
}()

// runtimeSnapshotDomainsFor returns the domains to snapshot after a mutation of
// path. A nil set means "all domains"; an empty non-nil set means "skip".
func runtimeSnapshotDomainsFor(path string) map[string]struct{} {
	rest, ok := strings.CutPrefix(path, "/api/v1/")
	if !ok {
		return nil
	}
	segment, _, _ := strings.Cut(rest, "/")
	domains, known := snapshotDomainsByPrefix[segment]
	if !known {
		return nil
	}
	out := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		out[d] = struct{}{}
	}
	return out
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
			domains := runtimeSnapshotDomainsFor(r.URL.Path)
			if domains != nil && len(domains) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Pass the response straight through (no buffering) so streaming,
			// flushing and large downloads behave normally; the snapshot is taken
			// once the handler returns.
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			if status >= http.StatusBadRequest {
				return
			}

			// The runtime-snapshot bridge is a backward-compat, best-effort mirror
			// of in-memory state; the DB pool is the canonical store. A snapshot
			// save failure must never turn an already-committed mutation into a
			// 500 for the client; log it.
			if err := persistRuntimeSnapshotDomains(logger, repo, participants, domains); err != nil {
				logger.Error("runtime snapshot persistence failed after successful mutation; response already sent (DB write is canonical)",
					"error", err, "method", r.Method, "path", r.URL.Path)
			}
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

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
