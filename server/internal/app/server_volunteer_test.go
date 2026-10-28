package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func issueVolunteerToken(t *testing.T, router http.Handler, userID, orgID, role, householdID string) string {
	t.Helper()
	payload := map[string]any{"user_id": userID, "org_id": orgID, "roles": []string{role}}
	if householdID != "" {
		payload["household_id"] = householdID
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/issue", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("issue token: %d %s", rr.Code, rr.Body.String())
	}
	return issuedAccessTokenFromResponse(t, rr)
}

func volunteerReq(t *testing.T, router http.Handler, method, path, token, orgID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-Id", orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func decodeVolunteer[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	return out
}

func expectStatus(t *testing.T, rr *httptest.ResponseRecorder, want int, label string) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("%s: want %d, got %d: %s", label, want, rr.Code, rr.Body.String())
	}
}

// TestVolunteerEndToEnd walks the full flow over HTTP: season + requirement,
// jobs, parent signup with household scoping, check-in, ledger, and a buyout
// billed through the billing API, proving allow and deny paths per role.
func TestVolunteerEndToEnd(t *testing.T) {
	router := NewRouter(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	orgID := "org-volunteer-test"
	adminTok := issueVolunteerToken(t, router, "vol-admin", orgID, "admin", "")
	coachTok := issueVolunteerToken(t, router, "vol-coach", orgID, "coach", "")

	// Households.
	hhIDs := make([]string, 2)
	for i, name := range []string{"Alpha Family", "Bravo Family"} {
		rr := volunteerReq(t, router, http.MethodPost, "/api/v1/members/households", adminTok, orgID, map[string]any{
			"org_id": orgID, "household_name": name, "primary_email": fmt.Sprintf("hh%d@example.com", i),
		})
		expectStatus(t, rr, http.StatusCreated, "create household")
		hhIDs[i] = decodeVolunteer[struct {
			Household struct{ ID string } `json:"household"`
		}](t, rr).Household.ID
	}
	parentA := issueVolunteerToken(t, router, "vol-parent-a", orgID, "parent", hhIDs[0])
	parentB := issueVolunteerToken(t, router, "vol-parent-b", orgID, "parent", hhIDs[1])
	swimmerA := issueVolunteerToken(t, router, "vol-swimmer-a", orgID, "swimmer", hhIDs[0])

	// Season + requirement (admin only).
	now := time.Now().UTC()
	rr := volunteerReq(t, router, http.MethodPost, "/api/v1/seasons", adminTok, orgID, map[string]any{
		"name": "Test Season", "status": "active",
		"start_date": now.AddDate(0, -1, 0).Format("2006-01-02"), "end_date": now.AddDate(0, 6, 0).Format("2006-01-02"),
	})
	expectStatus(t, rr, http.StatusCreated, "create season")
	seasonID := decodeVolunteer[struct {
		Season struct{ ID string } `json:"season"`
	}](t, rr).Season.ID

	reqBody := map[string]any{"season_id": seasonID, "required_hours": 6, "buyout_cents_per_hour": 2000}
	expectStatus(t, volunteerReq(t, router, http.MethodPut, "/api/v1/volunteer/requirements", coachTok, orgID, reqBody), http.StatusForbidden, "coach set requirement")
	expectStatus(t, volunteerReq(t, router, http.MethodPut, "/api/v1/volunteer/requirements", adminTok, orgID, reqBody), http.StatusOK, "admin set requirement")

	// Settings: admin writes, coach reads, parent denied.
	expectStatus(t, volunteerReq(t, router, http.MethodPut, "/api/v1/volunteer/settings", coachTok, orgID, map[string]any{"cancellation_cutoff_hours": 24}), http.StatusForbidden, "coach settings")
	expectStatus(t, volunteerReq(t, router, http.MethodPut, "/api/v1/volunteer/settings", adminTok, orgID, map[string]any{"cancellation_cutoff_hours": 24, "timezone": "America/Los_Angeles"}), http.StatusOK, "admin settings")
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/settings", coachTok, orgID, nil), http.StatusOK, "coach read settings")
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/settings", parentA, orgID, nil), http.StatusForbidden, "parent read settings")

	// Jobs: coach creates, parent cannot.
	start := now.Add(72 * time.Hour).Truncate(time.Minute)
	jobBody := map[string]any{
		"event_name": "Fall Fundraiser", "role": "concessions", "title": "Snack bar",
		"starts_at": start.Format(time.RFC3339), "ends_at": start.Add(4 * time.Hour).Format(time.RFC3339), "capacity": 2,
	}
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/jobs", parentA, orgID, jobBody), http.StatusForbidden, "parent create job")
	rr = volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/jobs", coachTok, orgID, jobBody)
	expectStatus(t, rr, http.StatusCreated, "coach create job")
	jobID := decodeVolunteer[struct {
		Job struct{ ID string } `json:"job"`
	}](t, rr).Job.ID
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/jobs", coachTok, orgID, map[string]any{
		"event_name": "x", "role": "timer", "title": "t", "starts_at": "not-a-time", "ends_at": "nope", "capacity": 1,
	}), http.StatusUnprocessableEntity, "bad timestamps")
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/jobs", coachTok, orgID, map[string]any{
		"event_name": "x", "role": "timer", "title": "t", "starts_at": start.Format(time.RFC3339), "ends_at": start.Add(time.Hour).Format(time.RFC3339), "capacity": 1, "bogus": true,
	}), http.StatusBadRequest, "unknown field")

	// Signups: swimmer denied, parent signs up own household, full job conflicts.
	path := "/api/v1/volunteer/jobs/" + jobID + "/signups"
	expectStatus(t, volunteerReq(t, router, http.MethodPost, path, swimmerA, orgID, map[string]any{"volunteer_name": "Kid"}), http.StatusForbidden, "swimmer signup")
	expectStatus(t, volunteerReq(t, router, http.MethodPost, path, parentA, orgID, map[string]any{"volunteer_name": "Pat", "household_id": hhIDs[1]}), http.StatusForbidden, "parent foreign household")
	rr = volunteerReq(t, router, http.MethodPost, path, parentA, orgID, map[string]any{"volunteer_name": "Pat Alpha"})
	expectStatus(t, rr, http.StatusCreated, "parent signup")
	signupA := decodeVolunteer[struct {
		Signup struct {
			ID        string `json:"id"`
			CanCancel bool   `json:"can_cancel"`
		} `json:"signup"`
	}](t, rr).Signup
	expectStatus(t, volunteerReq(t, router, http.MethodPost, path, parentA, orgID, map[string]any{"volunteer_name": "pat alpha"}), http.StatusConflict, "duplicate person")
	rr = volunteerReq(t, router, http.MethodPost, path, parentB, orgID, map[string]any{"volunteer_name": "Bo Bravo"})
	expectStatus(t, rr, http.StatusCreated, "parent B signup")
	signupB := decodeVolunteer[struct {
		Signup struct{ ID string } `json:"signup"`
	}](t, rr).Signup
	rr = volunteerReq(t, router, http.MethodPost, path, parentB, orgID, map[string]any{"volunteer_name": "Second Bravo"})
	expectStatus(t, rr, http.StatusConflict, "job full")
	if !strings.Contains(rr.Body.String(), "job_full") {
		t.Fatalf("expected job_full code: %s", rr.Body.String())
	}

	// Listing: parent sees only own household; job roster hidden from parent.
	rr = volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/signups", parentA, orgID, nil)
	expectStatus(t, rr, http.StatusOK, "parent list signups")
	if got := decodeVolunteer[struct{ Total int }](t, rr).Total; got != 1 {
		t.Fatalf("parent should see 1 signup, got %d", got)
	}
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/signups?household_id="+hhIDs[1], parentA, orgID, nil), http.StatusForbidden, "parent foreign filter")
	rr = volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/jobs?upcoming=true", swimmerA, orgID, nil)
	expectStatus(t, rr, http.StatusOK, "swimmer list jobs")
	jobsResp := decodeVolunteer[struct {
		Jobs []struct {
			FilledCount int              `json:"filled_count"`
			Signups     []map[string]any `json:"signups"`
		} `json:"jobs"`
		Summary struct {
			FillRate float64 `json:"fill_rate"`
		} `json:"summary"`
	}](t, rr)
	if len(jobsResp.Jobs) != 1 || jobsResp.Jobs[0].FilledCount != 2 || jobsResp.Summary.FillRate != 1 || jobsResp.Jobs[0].Signups != nil {
		t.Fatalf("unexpected job list: %s", rr.Body.String())
	}

	// Cancel: other parent cannot touch A's signup; A cancels (outside 24h cutoff).
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/signups/"+signupA.ID+"/cancel", parentB, orgID, nil), http.StatusNotFound, "foreign cancel")
	if !signupA.CanCancel {
		t.Fatal("expected can_cancel for shift 72h out with 24h cutoff")
	}
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/signups/"+signupA.ID+"/cancel", parentA, orgID, map[string]any{"reason": "conflict"}), http.StatusOK, "parent cancel")

	// Check-in: coach marks B worked with 5 hours; parent denied.
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/signups/"+signupB.ID+"/check-in", parentB, orgID, map[string]any{"status": "worked"}), http.StatusForbidden, "parent check-in")
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/signups/"+signupB.ID+"/check-in", coachTok, orgID, map[string]any{"status": "worked", "hours": 5}), http.StatusOK, "coach check-in")

	// Manual credit for A (coach allowed).
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/credits", coachTok, orgID, map[string]any{
		"season_id": seasonID, "household_id": hhIDs[0], "hours": 2, "reason": "Board meeting minutes",
	}), http.StatusCreated, "coach credit")

	// Ledger: staff season view; parent only own household view.
	rr = volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/ledger?season_id="+seasonID, coachTok, orgID, nil)
	expectStatus(t, rr, http.StatusOK, "coach ledger")
	type ledgerRow struct {
		HouseholdID    string  `json:"household_id"`
		EarnedHours    float64 `json:"earned_hours"`
		RemainingHours float64 `json:"remaining_hours"`
		BuyoutDueCents int     `json:"buyout_due_cents"`
		Status         string  `json:"status"`
	}
	ledger := decodeVolunteer[struct{ Ledger []ledgerRow }](t, rr).Ledger
	byHH := map[string]ledgerRow{}
	for _, row := range ledger {
		byHH[row.HouseholdID] = row
	}
	if a := byHH[hhIDs[0]]; a.EarnedHours != 2 || a.RemainingHours != 4 || a.BuyoutDueCents != 8000 || a.Status != "short" {
		t.Fatalf("household A ledger: %+v", a)
	}
	if b := byHH[hhIDs[1]]; b.EarnedHours != 5 || b.RemainingHours != 1 {
		t.Fatalf("household B ledger: %+v", b)
	}
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/ledger?season_id="+seasonID, parentA, orgID, nil), http.StatusForbidden, "parent season ledger")
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/households/"+hhIDs[1]+"/ledger", parentA, orgID, nil), http.StatusForbidden, "parent foreign ledger")
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/households/"+hhIDs[0]+"/ledger", swimmerA, orgID, nil), http.StatusOK, "swimmer own ledger")

	// Buyout: coach denied; admin bills via the billing service; repeat conflicts.
	buyoutBody := map[string]any{"season_id": seasonID, "household_id": hhIDs[0], "issue": true}
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/buyouts", coachTok, orgID, buyoutBody), http.StatusForbidden, "coach buyout")
	rr = volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/buyouts", adminTok, orgID, buyoutBody)
	expectStatus(t, rr, http.StatusCreated, "admin buyout")
	inv := decodeVolunteer[struct {
		Invoice struct {
			InvoiceID     string `json:"invoice_id"`
			InvoiceStatus string `json:"invoice_status"`
		} `json:"invoice"`
	}](t, rr).Invoice
	if inv.InvoiceStatus != "issued" {
		t.Fatalf("expected issued invoice: %+v", inv)
	}
	rr = volunteerReq(t, router, http.MethodGet, "/api/v1/invoices/"+inv.InvoiceID, parentA, orgID, nil)
	expectStatus(t, rr, http.StatusOK, "parent reads buyout invoice")
	if total := decodeVolunteer[struct {
		Invoice struct {
			TotalCents int `json:"total_cents"`
		} `json:"invoice"`
	}](t, rr).Invoice.TotalCents; total != 8000 {
		t.Fatalf("invoice total: want 8000, got %d", total)
	}
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/buyouts", adminTok, orgID, buyoutBody), http.StatusConflict, "repeat buyout")

	// Delete: job with worked signup is protected.
	expectStatus(t, volunteerReq(t, router, http.MethodDelete, "/api/v1/volunteer/jobs/"+jobID, adminTok, orgID, nil), http.StatusConflict, "delete worked job")
	expectStatus(t, volunteerReq(t, router, http.MethodDelete, "/api/v1/volunteer/jobs/"+jobID, parentA, orgID, nil), http.StatusForbidden, "parent delete job")

	// Audit trail captured privileged writes.
	rr = volunteerReq(t, router, http.MethodGet, "/api/v1/audit-events?resource_type=volunteer_buyout", adminTok, orgID, nil)
	expectStatus(t, rr, http.StatusOK, "audit list")
	if !strings.Contains(rr.Body.String(), "volunteer.buyout.bill") {
		t.Fatalf("expected buyout audit event: %s", rr.Body.String())
	}
}

// TestVolunteerBulkTemplateAndMeetScope covers template bulk-create against a
// real meet and the meet_id filter used by the meet detail Volunteers tab.
func TestVolunteerBulkTemplateAndMeetScope(t *testing.T) {
	router := NewRouter(Config{Auth: AuthConfig{EnableDevStubTokens: true}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	orgID := "org-volunteer-bulk"
	coachTok := issueVolunteerToken(t, router, "vb-coach", orgID, "coach", "")
	parentTok := issueVolunteerToken(t, router, "vb-parent", orgID, "parent", "hh-x")

	rr := volunteerReq(t, router, http.MethodPost, "/api/v1/meets", coachTok, orgID, map[string]any{
		"name": "Home Invite", "meet_type": "invitational", "course": "SCY",
		"start_date": time.Now().AddDate(0, 0, 10).Format("2006-01-02"), "end_date": time.Now().AddDate(0, 0, 11).Format("2006-01-02"),
	})
	expectStatus(t, rr, http.StatusCreated, "create meet")
	meetID := decodeVolunteer[struct {
		Meet struct{ ID string } `json:"meet"`
	}](t, rr).Meet.ID

	start := time.Now().UTC().AddDate(0, 0, 10).Truncate(time.Hour)
	bulk := map[string]any{
		"meet_id": meetID, "template_id": "standard_meet",
		"shifts": []map[string]any{
			{"starts_at": start.Format(time.RFC3339), "ends_at": start.Add(4 * time.Hour).Format(time.RFC3339), "label": "Session 1"},
		},
	}
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/jobs/bulk", parentTok, orgID, bulk), http.StatusForbidden, "parent bulk")
	rr = volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/jobs/bulk", coachTok, orgID, bulk)
	expectStatus(t, rr, http.StatusCreated, "coach bulk")
	if n := decodeVolunteer[struct{ Count int }](t, rr).Count; n != 7 {
		t.Fatalf("want 7 template jobs, got %d", n)
	}
	bulk["meet_id"] = "does-not-exist"
	expectStatus(t, volunteerReq(t, router, http.MethodPost, "/api/v1/volunteer/jobs/bulk", coachTok, orgID, bulk), http.StatusNotFound, "bulk unknown meet")

	rr = volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/jobs?meet_id="+meetID, parentTok, orgID, nil)
	expectStatus(t, rr, http.StatusOK, "list by meet")
	resp := decodeVolunteer[struct {
		Total int
		Jobs  []struct {
			MeetName string `json:"meet_name"`
		} `json:"jobs"`
	}](t, rr)
	if resp.Total != 7 || resp.Jobs[0].MeetName != "Home Invite" {
		t.Fatalf("unexpected meet job list: %s", rr.Body.String())
	}
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/templates", parentTok, orgID, nil), http.StatusForbidden, "parent templates")
	expectStatus(t, volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/templates", coachTok, orgID, nil), http.StatusOK, "coach templates")
	// Cross-org isolation: another org's coach sees nothing.
	otherTok := issueVolunteerToken(t, router, "vb-other", "org-other", "coach", "")
	rr = volunteerReq(t, router, http.MethodGet, "/api/v1/volunteer/jobs?meet_id="+meetID, otherTok, "org-other", nil)
	expectStatus(t, rr, http.StatusOK, "other org list")
	if decodeVolunteer[struct{ Total int }](t, rr).Total != 0 {
		t.Fatal("jobs leaked across orgs")
	}
}
