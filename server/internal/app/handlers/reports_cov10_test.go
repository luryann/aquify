package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/billing"
	"github.com/luryann/aquify/server/internal/app/groups"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	"github.com/luryann/aquify/server/internal/app/reports"
	"github.com/luryann/aquify/server/internal/storage"
)

// cov10gFailingStore wraps the in-memory store and fails every write.
type cov10gFailingStore struct{ storage.ObjectStorage }

func (cov10gFailingStore) PutObject(context.Context, string, string, []byte) error {
	return errors.New("object store unavailable")
}

func TestCov10gReportsCSVArtifactPersistFailure(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(members.NewService(), groups.NewService(), meets.NewService(), billing.NewService())
	svc.SetStorage(cov10gFailingStore{storage.NewStubStorage("")})
	aud := audit.NewService()
	h := NewReportsHandlers(svc, aud)
	router := g2Router(t, g2Tokens, func(r chi.Router) { h.RegisterRoutes(r) })

	rr := g2Do(router, http.MethodGet, "/reports/membership-status?format=csv", "admin", "")
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "artifact_persist_failed") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "object store unavailable") {
		t.Fatalf("storage error detail leaked to client: %s", rr.Body.String())
	}
	// The run is still audited before the artifact is stored.
	events, err := aud.Query(audit.Filter{OrgID: engOrg})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != audit.ActionReportMembershipStatusRun || events[0].Metadata["format"] != "csv" {
		t.Fatalf("unexpected audit events: %+v", events)
	}

	// JSON format does not touch the store at all.
	rr = g2Do(router, http.MethodGet, "/reports/membership-status", "admin", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("json status %d: %s", rr.Code, rr.Body.String())
	}
	// Coaches may not run reports.
	if rr = g2Do(router, http.MethodGet, "/reports/meet-fees", "coach", ""); rr.Code != http.StatusForbidden {
		t.Fatalf("coach status %d, want 403", rr.Code)
	}
}
