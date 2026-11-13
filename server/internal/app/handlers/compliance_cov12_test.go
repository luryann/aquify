package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/compliance"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/storage"
	"github.com/luryann/aquify/server/internal/testutil"
	"github.com/luryann/aquify/server/internal/workers"
)

// cov12gComplianceEnv wires ComplianceHandlers behind the real auth and org
// scope middleware for one org.
type cov12gComplianceEnv struct {
	h      *ComplianceHandlers
	router http.Handler
	org    string
	audit  *audit.Service
}

func cov12gNewCompliance(t *testing.T, org string, svc *compliance.Service) *cov12gComplianceEnv {
	t.Helper()
	tokens := tokenResolver{
		"admin":      {UserID: "u-admin", OrgID: org, Roles: []string{"admin"}, MFAAuthenticated: true},
		"adminnomfa": {UserID: "u-admin2", OrgID: org, Roles: []string{"admin"}},
		"coach":      {UserID: "u-coach", OrgID: org, Roles: []string{"coach"}},
		"parent":     {UserID: "u-parent", OrgID: org, HouseholdID: "hh-1", Roles: []string{"parent"}},
	}
	a := audit.NewService()
	h := NewComplianceHandlers(svc, a)
	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokens))
		p.Use(appmiddleware.RequireOrgScope)
		h.RegisterRoutes(p)
	})
	return &cov12gComplianceEnv{h: h, router: r, org: org, audit: a}
}

func (e *cov12gComplianceEnv) do(method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", e.org)
	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)
	return rr
}

// cov12gClosedCompliance returns a database-backed service whose pool has
// been closed, so every read and write fails like an unreachable database.
func cov12gClosedCompliance(t *testing.T) *compliance.Service {
	t.Helper()
	pool := testutil.IsolatedSchemaPool(t)
	svc := compliance.NewServiceWithPool(pool, 7, 14)
	pool.Close()
	return svc
}

func TestCov12gComplianceBadBodiesAreRejected(t *testing.T) {
	t.Parallel()
	e := cov12gNewCompliance(t, engOrg, compliance.NewService())

	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-export-requests", "admin", `{}`), http.StatusUnprocessableEntity, "Request validation failed.")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-export-requests", "admin", `{not json`), http.StatusBadRequest, "Invalid JSON body.")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-deletion-requests", "admin", `{"reason":"x"}`), http.StatusUnprocessableEntity, "Request validation failed.")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-deletion-requests", "admin", `[]`), http.StatusBadRequest, "Invalid JSON body.")

	// Role and household boundaries.
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-export-requests", "coach", `{"household_id":"hh-1"}`), http.StatusForbidden, "Insufficient role to request data export.")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-deletion-requests", "coach", `{"household_id":"hh-1"}`), http.StatusForbidden, "Insufficient role to request data deletion.")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-export-requests", "parent", `{"household_id":"hh-other"}`), http.StatusForbidden, "own household")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-deletion-requests", "parent", `{"household_id":"hh-other"}`), http.StatusForbidden, "own household")
}

func TestCov12gExportWithoutQueueIsMarkedFailed(t *testing.T) {
	t.Parallel()
	svc := compliance.NewService()
	e := cov12gNewCompliance(t, engOrg, svc)

	// No job queue is wired, so nothing could ever process the request.
	rr := e.do(http.MethodPost, "/compliance/data-export-requests", "parent", `{"household_id":"hh-1"}`)
	cov12gWant(t, rr, http.StatusServiceUnavailable, "export_queue_unavailable")
	list, err := svc.ListDataExportRequests(engOrg, "hh-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != "failed" {
		t.Fatalf("an unqueued export must be closed out as failed, got %+v", list)
	}
}

// cov12gClosingQueue fails every enqueue after closing the database, so the
// compensating "mark failed" write in the handler fails too.
type cov12gClosingQueue struct {
	workers.Queue
	pool *pgxpool.Pool
}

func (q cov12gClosingQueue) Enqueue(context.Context, *workers.Task, ...workers.Option) (*workers.TaskInfo, error) {
	q.pool.Close()
	return nil, errors.New("redis: connection refused")
}

func TestCov12gExportQueueAndDatabaseBothFailing(t *testing.T) {
	t.Parallel()
	pool := testutil.IsolatedSchemaPool(t)
	// A second handle onto the same schema survives the first pool closing.
	observer, err := pgxpool.NewWithConfig(context.Background(), pool.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Close)

	org := uuid.NewString()
	e := cov12gNewCompliance(t, org, compliance.NewServiceWithPool(pool, 7, 14))
	e.h.SetJobQueue(cov12gClosingQueue{pool: pool})

	hh := uuid.NewString()
	rr := e.do(http.MethodPost, "/compliance/data-export-requests", "admin", `{"household_id":"`+hh+`"}`)
	cov12gWant(t, rr, http.StatusServiceUnavailable, "export_queue_unavailable")

	// The compensating write could not run, so the request is still pending in
	// the database (an admin can see and retry it) and the creation was audited.
	var status string
	if err := observer.QueryRow(context.Background(), `SELECT status FROM data_export_requests WHERE household_id = $1`, hh).Scan(&status); err != nil {
		t.Fatalf("exported row missing: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}
}

func TestCov12gComplianceDatabaseFailuresReturn500(t *testing.T) {
	t.Parallel()
	e := cov12gNewCompliance(t, engOrg, cov12gClosedCompliance(t))

	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-export-requests", "admin", `{"household_id":"hh-1"}`), http.StatusInternalServerError, "Failed to create data export request.")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-export-requests", "admin", ""), http.StatusInternalServerError, "Failed to list data export requests.")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-export-requests/some-id", "admin", ""), http.StatusInternalServerError, "Failed to load data export request.")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-deletion-requests", "admin", `{"household_id":"hh-1","reason":"leaving"}`), http.StatusInternalServerError, "Failed to create data deletion request.")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-deletion-requests", "admin", ""), http.StatusInternalServerError, "Failed to list data deletion requests.")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-deletion-requests/some-id", "admin", ""), http.StatusInternalServerError, "Failed to load data deletion request.")

	// A storage-layer failure is not a "not found" and never leaks details.
	rr := e.do(http.MethodGet, "/compliance/data-deletion-requests/some-id", "admin", "")
	if strings.Contains(rr.Body.String(), "closed pool") {
		t.Fatalf("driver error leaked to the client: %s", rr.Body.String())
	}
}

func TestCov12gDeletionReviewRules(t *testing.T) {
	t.Parallel()
	svc := compliance.NewService()
	e := cov12gNewCompliance(t, engOrg, svc)

	create := func() string {
		t.Helper()
		rr := e.do(http.MethodPost, "/compliance/data-deletion-requests", "parent", `{"household_id":"hh-1","reason":"moving away"}`)
		cov12gWant(t, rr, http.StatusCreated, `"status":"pending"`)
		return cov12gBody(t, rr)["id"].(string)
	}
	id := create()
	path := "/compliance/data-deletion-requests/" + id

	// Only admins review; approving additionally needs an MFA session.
	cov12gWant(t, e.do(http.MethodPost, path+"/approve", "coach", `{}`), http.StatusForbidden, "Only admins can review deletion requests.")
	cov12gWant(t, e.do(http.MethodPost, path+"/reject", "parent", `{}`), http.StatusForbidden, "Only admins can review deletion requests.")
	cov12gWant(t, e.do(http.MethodPost, path+"/approve", "adminnomfa", `{}`), http.StatusForbidden, "mfa_required")

	// A malformed review body is rejected before the request is touched.
	cov12gWant(t, e.do(http.MethodPost, path+"/reject", "admin", `{bad`), http.StatusBadRequest, "Invalid JSON body.")
	cov12gWant(t, e.do(http.MethodPost, path+"/reject", "admin", `{"unknown_field":1}`), http.StatusBadRequest, "Invalid JSON body.")

	// Rejection needs notes and unknown ids are 404.
	cov12gWant(t, e.do(http.MethodPost, path+"/reject", "admin", `{}`), http.StatusUnprocessableEntity, "invalid_review")
	cov12gWant(t, e.do(http.MethodPost, "/compliance/data-deletion-requests/missing/reject", "admin", `{"review_notes":"n"}`), http.StatusNotFound, "Data deletion request not found.")

	// Approval with no job queue is committed but reported as not queued.
	cov12gWant(t, e.do(http.MethodPost, path+"/approve", "admin", `{"review_notes":"verified"}`), http.StatusServiceUnavailable, "deletion_queue_unavailable")
	got, err := svc.GetDataDeletionRequest(engOrg, id)
	if err != nil || got.Status != "approved" {
		t.Fatalf("approval must stay committed when queueing fails: %+v, %v", got, err)
	}
	// A reviewed request cannot be reviewed again.
	cov12gWant(t, e.do(http.MethodPost, path+"/reject", "admin", `{"review_notes":"changed my mind"}`), http.StatusConflict, "invalid_transition")

	// Listing is admin-only and supports the household filter.
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-deletion-requests", "coach", ""), http.StatusForbidden, "requires admin role")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-export-requests", "coach", ""), http.StatusForbidden, "requires admin role")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-deletion-requests?household_id=hh-none", "admin", ""), http.StatusOK, `"requests":[]`)
}

func TestCov12gExportDownloadURLRules(t *testing.T) {
	t.Parallel()
	svc := compliance.NewService()
	e := cov12gNewCompliance(t, engOrg, svc)

	ready := func(key string, expires time.Time) string {
		t.Helper()
		req, err := svc.CreateDataExportRequest(engOrg, "hh-1", "u-admin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.MarkDataExportProcessing(engOrg, req.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CompleteDataExport(engOrg, req.ID, key, expires); err != nil {
			t.Fatal(err)
		}
		return req.ID
	}
	future := time.Now().Add(24 * time.Hour)

	// Without object storage wired there is nothing to presign: the raw object
	// key must never be handed to the client.
	id := ready("compliance/"+engOrg+"/exports/a.json", future)
	rr := e.do(http.MethodGet, "/compliance/data-export-requests/"+id, "admin", "")
	cov12gWant(t, rr, http.StatusOK, `"status":"ready"`)
	if strings.Contains(rr.Body.String(), "a.json") {
		t.Fatalf("object key leaked without a store: %s", rr.Body.String())
	}

	// With storage, an export past its expiry is no longer downloadable.
	e.h.SetStorage(storage.NewStubStorage(""))
	expired := ready("compliance/"+engOrg+"/exports/old.json", time.Now().Add(-time.Hour))
	rr = e.do(http.MethodGet, "/compliance/data-export-requests/"+expired, "admin", "")
	cov12gWant(t, rr, http.StatusOK, `"status":"ready"`)
	if strings.Contains(rr.Body.String(), "old.json") || strings.Contains(rr.Body.String(), "X-Stub") {
		t.Fatalf("expired export must not expose a download link: %s", rr.Body.String())
	}

	// A live export gets a fresh presigned link, in single and list reads.
	live := ready("compliance/"+engOrg+"/exports/live.json", future)
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-export-requests/"+live, "admin", ""), http.StatusOK, "live.json")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-export-requests?household_id=hh-1", "admin", ""), http.StatusOK, "live.json")

	// A non-admin requester only sees their own export; others get 404.
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-export-requests/"+live, "parent", ""), http.StatusNotFound, "Data export request not found.")
	cov12gWant(t, e.do(http.MethodGet, "/compliance/data-export-requests/missing", "admin", ""), http.StatusNotFound, "Data export request not found.")
}
