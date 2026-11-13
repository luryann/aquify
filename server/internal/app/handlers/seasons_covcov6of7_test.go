package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/seasons"
	"github.com/luryann/aquify/server/internal/testutil"
)

func TestCov6xSeasonsCRUDAndValidation(t *testing.T) {
	t.Parallel()
	a := audit.NewService()
	h := NewSeasonsHandlers(seasons.NewService(), a)
	router := cov6xRouter(h.RegisterRoutes)

	// Role gates.
	rr := cov6xDo(router, http.MethodGet, "/seasons", "parent", "")
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	rr = cov6xDo(router, http.MethodGet, "/seasons/x", "parent", "")
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	rr = cov6xDo(router, http.MethodPost, "/seasons", "coach", `{"name":"2026","status":"active"}`)
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	rr = cov6xDo(router, http.MethodPut, "/seasons/x", "coach", `{}`)
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")
	rr = cov6xDo(router, http.MethodDelete, "/seasons/x", "coach", "")
	cov6xWant(t, rr, http.StatusForbidden, "forbidden")

	// Create validation: body-level (missing fields) and service-level.
	rr = cov6xDo(router, http.MethodPost, "/seasons", "admin", `{"name":"x"}`)
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing status should be rejected, got %d %s", rr.Code, rr.Body.String())
	}
	rr = cov6xDo(router, http.MethodPost, "/seasons", "admin", `{"name":"`+strings.Repeat("n", 121)+`","status":"active"}`)
	cov6xWant(t, rr, http.StatusBadRequest, "season name is required")
	rr = cov6xDo(router, http.MethodPost, "/seasons", "admin", `{"name":"S","status":"active","start_date":"2027-01-01","end_date":"2026-01-01"}`)
	cov6xWant(t, rr, http.StatusBadRequest, "start_date on or before end_date")
	rr = cov6xDo(router, http.MethodPost, "/seasons", "admin", `{"name":"S","status":"bogus"}`)
	cov6xWant(t, rr, http.StatusBadRequest, "Invalid season status.")

	rr = cov6xDo(router, http.MethodPost, "/seasons", "admin", `{"name":"2026-27","status":"upcoming","start_date":"2026-09-01","end_date":"2027-08-31","goals":"qualify"}`)
	cov6xWant(t, rr, http.StatusCreated, `"name":"2026-27"`)
	id := cov6xDecode(t, rr)["season"].(map[string]any)["id"].(string)

	rr = cov6xDo(router, http.MethodGet, "/seasons", "coach", "")
	cov6xWant(t, rr, http.StatusOK, id)
	rr = cov6xDo(router, http.MethodGet, "/seasons/"+id, "coach", "")
	cov6xWant(t, rr, http.StatusOK, `"goals":"qualify"`)
	rr = cov6xDo(router, http.MethodGet, "/seasons/missing", "coach", "")
	cov6xWant(t, rr, http.StatusNotFound, "not_found")

	// Update: malformed body, then each service-level validation error.
	rr = cov6xDo(router, http.MethodPut, "/seasons/"+id, "admin", `{"name":`)
	cov6xWant(t, rr, http.StatusBadRequest, "")
	rr = cov6xDo(router, http.MethodPut, "/seasons/missing", "admin", `{"goals":"x"}`)
	cov6xWant(t, rr, http.StatusNotFound, "not_found")
	rr = cov6xDo(router, http.MethodPut, "/seasons/"+id, "admin", `{"name":""}`)
	cov6xWant(t, rr, http.StatusBadRequest, "season name is required")
	rr = cov6xDo(router, http.MethodPut, "/seasons/"+id, "admin", `{"start_date":"2030-01-01"}`)
	cov6xWant(t, rr, http.StatusBadRequest, "start_date on or before end_date")
	rr = cov6xDo(router, http.MethodPut, "/seasons/"+id, "admin", `{"status":"bogus"}`)
	cov6xWant(t, rr, http.StatusBadRequest, "Invalid season status.")
	rr = cov6xDo(router, http.MethodPut, "/seasons/"+id, "admin", `{"status":"active","goals":"win"}`)
	cov6xWant(t, rr, http.StatusOK, `"status":"active"`)

	rr = cov6xDo(router, http.MethodDelete, "/seasons/missing", "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "not_found")
	rr = cov6xDo(router, http.MethodDelete, "/seasons/"+id, "admin", "")
	cov6xWant(t, rr, http.StatusOK, `"deleted":true`)
	rr = cov6xDo(router, http.MethodGet, "/seasons/"+id, "admin", "")
	cov6xWant(t, rr, http.StatusNotFound, "not_found")

	actions := cov6xAuditActions(t, a)
	for _, want := range []string{"season.create", "season.update", "season.delete"} {
		if len(actions[want]) != 1 {
			t.Errorf("expected exactly one %s audit event, got %d", want, len(actions[want]))
		}
	}
}

func TestCov6xSeasonsStorageFailuresAre500WithoutLeakingDetail(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	svc := seasons.NewServiceWithPool(pool)
	tn := cov6xNewTenant(NewSeasonsHandlers(svc, nil).RegisterRoutes)

	rr := tn.do(http.MethodPost, "/seasons", "admin", `{"name":"2026","status":"active"}`)
	cov6xWant(t, rr, http.StatusCreated, "")
	id := cov6xDecode(t, rr)["season"].(map[string]any)["id"].(string)

	cov6xBreakTable(t, pool, "seasons")

	for _, tc := range []struct{ method, path, body, msg string }{
		{http.MethodGet, "/seasons", "", "Failed to list seasons."},
		{http.MethodGet, "/seasons/" + id, "", "Failed to get season."},
		{http.MethodPost, "/seasons", `{"name":"again","status":"active"}`, "Failed to create season."},
		{http.MethodPut, "/seasons/" + id, `{"goals":"x"}`, "Failed to update season."},
		{http.MethodDelete, "/seasons/" + id, "", "Failed to delete season."},
	} {
		rr := tn.do(tc.method, tc.path, "admin", tc.body)
		cov6xWant(t, rr, http.StatusInternalServerError, tc.msg)
		if strings.Contains(rr.Body.String(), "seasons") && strings.Contains(rr.Body.String(), "relation") {
			t.Errorf("%s %s leaked database detail: %s", tc.method, tc.path, rr.Body.String())
		}
	}
}
