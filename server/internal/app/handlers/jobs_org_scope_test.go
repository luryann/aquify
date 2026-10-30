package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/workers"
	"github.com/go-chi/chi/v5"
)

func TestJobsAdminIsScopedToOwnOrg(t *testing.T) {
	srv := workers.NewServer(workers.ServerConfig{
		Concurrency: 1,
		RetryPolicies: map[string]workers.RetryPolicy{
			workers.QueueDefault: {MaxRetries: 0, Backoff: []time.Duration{time.Millisecond}},
		},
	})
	srv.RegisterHandler("t", func(context.Context, *workers.Task) error { return errors.New("boom") })
	srv.Start()
	defer srv.Stop()

	ids := map[string]string{}
	for name, payload := range map[string]map[string]string{
		"mine": {"org_id": engOrg}, "other": {"org_id": "org-other"}, "none": {"x": "y"},
	} {
		task, _ := workers.NewTask("t", payload)
		info, err := srv.Enqueue(context.Background(), task)
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = info.ID
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if d, _ := srv.DLQTasks(); len(d) == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tasks never reached DLQ")
		}
		time.Sleep(10 * time.Millisecond)
	}

	r := chi.NewRouter()
	r.Group(func(p chi.Router) {
		p.Use(appmiddleware.Auth(tokenResolver{"admin": {UserID: "u", OrgID: engOrg, Roles: []string{"admin"}}}))
		p.Use(appmiddleware.RequireOrgScope)
		NewJobsHandlers(srv).RegisterRoutes(p)
	})
	do := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer admin")
		req.Header.Set("X-Org-Id", engOrg)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	for _, path := range []string{"/jobs/dlq", "/jobs"} {
		rr := do(http.MethodGet, path)
		expectStatus(t, rr, http.StatusOK, path)
		var body struct {
			Tasks []workers.TaskInfo `json:"tasks"`
			DLQ   []workers.TaskInfo `json:"dead_letter_tasks"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		got := append(body.Tasks, body.DLQ...)
		if len(got) != 1 || got[0].ID != ids["mine"] {
			t.Fatalf("%s: want only own-org task, got %+v", path, got)
		}
	}
	expectStatus(t, do(http.MethodPost, "/jobs/dlq/"+ids["other"]+"/retry"), http.StatusNotFound, "retry foreign")
	expectStatus(t, do(http.MethodDelete, "/jobs/dlq/"+ids["other"]), http.StatusNotFound, "delete foreign")
	expectStatus(t, do(http.MethodDelete, "/jobs/dlq/"+ids["none"]), http.StatusNotFound, "delete org-less")
	expectStatus(t, do(http.MethodDelete, "/jobs/dlq/"+ids["mine"]), http.StatusNoContent, "delete own")
	if d, _ := srv.DLQTasks(); len(d) != 2 {
		t.Fatalf("foreign tasks must remain, got %d", len(d))
	}
}
