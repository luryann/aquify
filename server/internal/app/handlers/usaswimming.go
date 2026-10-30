package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/members"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/dareaquatics/aquify/server/internal/app/usaswimming"
	"github.com/go-chi/chi/v5"
)

// USASwimmingHandlers exposes the org-level USA Swimming (SWIMS) integration:
// settings, connection test and times sync. All routes are admin-only.
// See package usaswimming for what is real vs stubbed.
type USASwimmingHandlers struct {
	svc     *usaswimming.Service
	perf    usaSyncRecorder
	members *members.Service
	audit   *audit.Service
}

// usaSyncRecorder is the slice of the performance service the sync uses.
type usaSyncRecorder interface {
	HasResultTime(orgID, swimmerID string, distance int, stroke, course string, finTimeMs int) bool
	UpsertResultWithRecords(orgID, actorID string, principal *appmiddleware.Principal, input performance.UpsertResultInput) (*performance.Result, []performance.RecordBreak, error)
}

func NewUSASwimmingHandlers(svc *usaswimming.Service, perf *performance.Service, m *members.Service, a *audit.Service) *USASwimmingHandlers {
	return &USASwimmingHandlers{svc: svc, perf: perf, members: m, audit: a}
}

func (h *USASwimmingHandlers) RegisterRoutes(router chi.Router) {
	router.Get("/integrations/usa-swimming", h.get)
	router.Put("/integrations/usa-swimming", h.update)
	router.Post("/integrations/usa-swimming/test", h.test)
	router.Post("/integrations/usa-swimming/sync", h.sync)
	router.Get("/integrations/usa-swimming/clubs", h.clubs)
	router.Get("/integrations/usa-swimming/roster-compare", h.rosterCompare)
	router.Get("/integrations/usa-swimming/registration-link", h.registrationLink)
	router.Post("/integrations/usa-swimming/verify-members", h.verifyMembers)
	router.Post("/integrations/usa-swimming/verify-time", h.verifyTime)
	router.Post("/integrations/usa-swimming/registrations", h.submitRegistration)
	router.Get("/integrations/usa-swimming/registrations/{requestID}/status", h.registrationStatus)
}

func (h *USASwimmingHandlers) adminScope(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return nil, "", false
	}
	if !hasAnyRole(principal, "admin") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admins may manage the USA Swimming integration.", nil)
		return nil, "", false
	}
	return principal, orgID, true
}

func (h *USASwimmingHandlers) get(w http.ResponseWriter, r *http.Request) {
	_, orgID, ok := h.adminScope(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Get(orgID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load integration settings.", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"integration": v})
}

type usaSettingsBody struct {
	BaseURL  *string `json:"base_url"`
	APIKey   *string `json:"api_key"`
	Enabled  *bool   `json:"enabled"`
	TokenURL *string `json:"token_url"`
	ClientID *string `json:"client_id"`
	ClubID   *string `json:"club_id"`
}

func (h *USASwimmingHandlers) update(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := h.adminScope(w, r)
	if !ok {
		return
	}
	var body usaSettingsBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}
	v, err := h.svc.Update(orgID, usaswimming.Updates{BaseURL: body.BaseURL, APIKey: body.APIKey, Enabled: body.Enabled,
		TokenURL: body.TokenURL, ClientID: body.ClientID, ClubID: body.ClubID}, principal.UserID)
	switch {
	case errors.Is(err, usaswimming.ErrInvalidSettings):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_integration_settings", strings.TrimPrefix(err.Error(), usaswimming.ErrInvalidSettings.Error()+"\n"), nil)
		return
	case errors.Is(err, usaswimming.ErrEncryptionUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "encryption_not_configured", "INTEGRATIONS_ENCRYPTION_KEY is not configured on the server; the API key cannot be stored.", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update integration settings.", err)
		return
	}
	// Audit records which fields changed, never the key itself.
	meta := map[string]string{"api_key_changed": fmt.Sprint(body.APIKey != nil)}
	if body.BaseURL != nil {
		meta["base_url"] = v.BaseURL
	}
	if body.Enabled != nil {
		meta["enabled"] = fmt.Sprint(v.Enabled)
	}
	recordAudit(h.audit, principal, orgID, "integrations.usa_swimming.update", "integration", "usa-swimming", nil, nil, meta)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"integration": v})
}

func (h *USASwimmingHandlers) writeClientErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usaswimming.ErrUnsupported):
		httpx.WriteError(w, r, http.StatusNotImplemented, "integration_unsupported", err.Error(), nil)
	case errors.Is(err, usaswimming.ErrUnauthorized):
		httpx.WriteError(w, r, http.StatusForbidden, "tier_not_authorized", "USA Swimming rejected this call; the credentials may not include the required API tier.", nil)
	case errors.Is(err, usaswimming.ErrDisabled), errors.Is(err, usaswimming.ErrNotConfigured):
		httpx.WriteError(w, r, http.StatusConflict, "integration_not_ready", err.Error(), nil)
	default:
		httpx.WriteError(w, r, http.StatusBadGateway, "integration_unavailable", err.Error(), nil)
	}
}

func (h *USASwimmingHandlers) test(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := h.adminScope(w, r)
	if !ok {
		return
	}
	client, err := h.svc.TestClient(orgID)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	testErr := client.TestConnection(ctx)
	msg := "Connection OK"
	if testErr != nil {
		msg = testErr.Error()
	}
	h.svc.RecordTest(orgID, testErr == nil, msg)
	recordAudit(h.audit, principal, orgID, "integrations.usa_swimming.test", "integration", "usa-swimming", nil, nil, map[string]string{"ok": fmt.Sprint(testErr == nil)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": testErr == nil, "message": msg})
}

type usaSyncBody struct {
	SwimmerIDs []string `json:"swimmer_ids"`
}

type usaSyncSwimmer struct {
	SwimmerID  string `json:"swimmer_id"`
	Name       string `json:"name"`
	Imported   int    `json:"imported"`
	Duplicates int    `json:"duplicates"`
	Skipped    int    `json:"skipped"`
	Status     string `json:"status"` // synced | not_found | error
	Error      string `json:"error,omitempty"`
}

const maxSyncSwimmers = 500

// sync pulls official times for club swimmers that have a USA Swimming ID and
// records each as an official result through the same path as results-file
// imports (UpsertResultWithRecords, so best times recompute). Dedupe: an
// identical finish time for the same swimmer/event/course is skipped, so
// re-running a sync (or syncing times already imported from a meet file) never
// double-records.
func (h *USASwimmingHandlers) sync(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := h.adminScope(w, r)
	if !ok {
		return
	}
	var body usaSyncBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}
	if !h.svc.SupportsTimes(orgID) {
		h.writeClientErr(w, r, usaswimming.ErrUnsupported)
		return
	}
	client, err := h.svc.Client(orgID)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	want := map[string]bool{}
	for _, id := range body.SwimmerIDs {
		want[id] = true
	}
	var targets []members.Swimmer
	for _, sw := range h.members.SnapshotSwimmers(orgID) {
		if strings.TrimSpace(sw.USASwimmingID) == "" || (len(want) > 0 && !want[sw.ID]) {
			continue
		}
		targets = append(targets, sw)
	}
	if len(targets) > maxSyncSwimmers {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "too_many_swimmers", fmt.Sprintf("Sync at most %d swimmers per request.", maxSyncSwimmers), nil)
		return
	}

	out := make([]usaSyncSwimmer, 0, len(targets))
	var total usaSyncSwimmer
	var breaks int
	for _, sw := range targets {
		res := usaSyncSwimmer{SwimmerID: sw.ID, Name: strings.TrimSpace(sw.FirstName + " " + sw.LastName), Status: "synced"}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		times, err := client.OfficialTimes(ctx, normalizeUSS(sw.USASwimmingID))
		cancel()
		switch {
		case errors.Is(err, usaswimming.ErrSwimmerNotFound):
			res.Status = "not_found"
		case err != nil:
			res.Status, res.Error = "error", err.Error()
		default:
			for _, t := range times {
				stroke, course := strings.ToLower(t.Stroke), strings.ToUpper(t.Course)
				if t.SwimDate.IsZero() || t.Distance <= 0 || t.TimeMs <= 0 || !validIndividualStroke(stroke) || !validCourse(course) {
					res.Skipped++
					continue
				}
				if h.perf.HasResultTime(orgID, sw.ID, t.Distance, stroke, course, t.TimeMs) {
					res.Duplicates++
					continue
				}
				ms := t.TimeMs
				_, rb, err := h.perf.UpsertResultWithRecords(orgID, principal.UserID, principal, performance.UpsertResultInput{
					SwimmerID: sw.ID, Distance: t.Distance, Stroke: stroke, Course: course,
					Gender: resultGender("", sw.Gender), FinTimeMs: &ms, IsOfficial: true,
				})
				if err != nil && !errors.Is(err, performance.ErrRecordDetection) {
					res.Status, res.Error = "error", "could not record a time"
					break
				}
				breaks += len(rb)
				res.Imported++
			}
		}
		total.Imported += res.Imported
		total.Duplicates += res.Duplicates
		total.Skipped += res.Skipped
		out = append(out, res)
	}
	summary := fmt.Sprintf("%d swimmers, %d imported, %d already recorded", len(out), total.Imported, total.Duplicates)
	h.svc.RecordSync(orgID, summary)
	recordAudit(h.audit, principal, orgID, "integrations.usa_swimming.sync", "integration", "usa-swimming", nil, nil, map[string]string{
		"swimmers": fmt.Sprint(len(out)), "imported": fmt.Sprint(total.Imported), "duplicates": fmt.Sprint(total.Duplicates), "skipped": fmt.Sprint(total.Skipped),
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swimmers": out, "swimmer_count": len(out), "imported": total.Imported, "duplicates": total.Duplicates,
		"skipped": total.Skipped, "new_team_records": breaks, "summary": summary,
	})
}

func validIndividualStroke(s string) bool {
	switch s {
	case "free", "back", "breast", "fly", "im":
		return true
	}
	return false
}

func validCourse(c string) bool { return c == "SCY" || c == "SCM" || c == "LCM" }

// Vendor-call timeouts: lists and lookups vs. bulk roster/verification calls.
const (
	vendorShortTimeout = 20 * time.Second
	vendorLongTimeout  = 30 * time.Second
)

// vendorCall is the resolved state every vendor-API route starts from.
type vendorCall struct {
	vc        usaswimming.VendorClient
	principal *appmiddleware.Principal
	orgID     string
	clubID    string
}

// vendor does the admin check and settings read shared by all vendor routes.
// needClub also requires the club ID to be configured.
func (h *USASwimmingHandlers) vendor(w http.ResponseWriter, r *http.Request, needClub bool) (vendorCall, bool) {
	principal, orgID, ok := h.adminScope(w, r)
	if !ok {
		return vendorCall{}, false
	}
	vc, clubID, err := h.svc.Vendor(orgID)
	if err != nil {
		h.writeClientErr(w, r, err)
		return vendorCall{}, false
	}
	if needClub && clubID == "" {
		httpx.WriteError(w, r, http.StatusConflict, "integration_not_ready", "Set the USA Swimming club ID before using this feature.", nil)
		return vendorCall{}, false
	}
	return vendorCall{vc: vc, principal: principal, orgID: orgID, clubID: clubID}, true
}

func (c vendorCall) audit(h *USASwimmingHandlers, action string, meta map[string]string) {
	recordAudit(h.audit, c.principal, c.orgID, "integrations.usa_swimming."+action, "integration", "usa-swimming", nil, nil, meta)
}

func (h *USASwimmingHandlers) clubs(w http.ResponseWriter, r *http.Request) {
	c, ok := h.vendor(w, r, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), vendorShortTimeout)
	defer cancel()
	clubs, err := c.vc.Clubs(ctx)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"clubs": clubs})
}

func (h *USASwimmingHandlers) registrationLink(w http.ResponseWriter, r *http.Request) {
	c, ok := h.vendor(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), vendorShortTimeout)
	defer cancel()
	link, err := c.vc.RegistrationLink(ctx, c.clubID)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"registration_link": link})
}

// rosterDiff is one row of the SWIMS-vs-Aquify comparison.
type rosterDiff struct {
	MemberID    string   `json:"member_id,omitempty"`
	SwimmerID   string   `json:"swimmer_id,omitempty"`
	Name        string   `json:"name"`
	Differences []string `json:"differences,omitempty"`
}

// compareRoster is read-only: it never changes Aquify data. Swimmers match on
// normalized USA Swimming ID; mismatches are name and date of birth.
func compareRoster(swims []usaswimming.RosterMember, local []members.Swimmer) (matched int, onlySwims, onlyLocal, changed []rosterDiff) {
	byID := map[string]members.Swimmer{}
	for _, sw := range local {
		if id := normalizeUSS(sw.USASwimmingID); id != "" {
			byID[id] = sw
		}
	}
	seen := map[string]bool{}
	for _, m := range swims {
		id := normalizeUSS(m.MemberID)
		name := strings.TrimSpace(m.FirstName + " " + m.LastName)
		sw, ok := byID[id]
		if !ok {
			onlySwims = append(onlySwims, rosterDiff{MemberID: m.MemberID, Name: name})
			continue
		}
		seen[id] = true
		matched++
		var diffs []string
		if !strings.EqualFold(strings.TrimSpace(sw.LastName), strings.TrimSpace(m.LastName)) {
			diffs = append(diffs, "last name differs")
		}
		if m.DateOfBirth != "" && sw.DateOfBirth != "" && m.DateOfBirth != sw.DateOfBirth {
			diffs = append(diffs, "date of birth differs")
		}
		if len(diffs) > 0 {
			changed = append(changed, rosterDiff{MemberID: m.MemberID, SwimmerID: sw.ID, Name: name, Differences: diffs})
		}
	}
	for id, sw := range byID {
		if !seen[id] {
			onlyLocal = append(onlyLocal, rosterDiff{MemberID: sw.USASwimmingID, SwimmerID: sw.ID, Name: strings.TrimSpace(sw.FirstName + " " + sw.LastName)})
		}
	}
	return
}

// rosterCompare pulls the club roster from SWIMS and reports who is missing or
// mismatched on either side. Roster contents are personal data: only counts go
// to the audit log.
func (h *USASwimmingHandlers) rosterCompare(w http.ResponseWriter, r *http.Request) {
	c, ok := h.vendor(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), vendorLongTimeout)
	defer cancel()
	roster, err := c.vc.Roster(ctx, c.clubID, nil)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	matched, onlySwims, onlyLocal, changed := compareRoster(roster, h.members.SnapshotSwimmers(c.orgID))
	c.audit(h, "roster_compare", map[string]string{
		"swims_members": fmt.Sprint(len(roster)), "matched": fmt.Sprint(matched),
		"only_in_swims": fmt.Sprint(len(onlySwims)), "only_in_aquify": fmt.Sprint(len(onlyLocal)), "mismatched": fmt.Sprint(len(changed)),
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"swims_count": len(roster), "matched": matched,
		"only_in_swims": onlySwims, "only_in_aquify": onlyLocal, "mismatched": changed,
	})
}

type usaVerifyMembersBody struct {
	SwimmerIDs []string `json:"swimmer_ids"`
}

// verifyMembers (Tier 3) checks membership for swimmers that have a USA Swimming ID.
func (h *USASwimmingHandlers) verifyMembers(w http.ResponseWriter, r *http.Request) {
	c, ok := h.vendor(w, r, true)
	if !ok {
		return
	}
	var body usaVerifyMembersBody
	if err := decodeOptionalJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}
	want := map[string]bool{}
	for _, id := range body.SwimmerIDs {
		want[id] = true
	}
	var ids []string
	nameByID := map[string]string{}
	for _, sw := range h.members.SnapshotSwimmers(c.orgID) {
		id := normalizeUSS(sw.USASwimmingID)
		if id == "" || (len(want) > 0 && !want[sw.ID]) {
			continue
		}
		ids = append(ids, id)
		nameByID[id] = strings.TrimSpace(sw.FirstName + " " + sw.LastName)
	}
	if len(ids) == 0 || len(ids) > maxSyncSwimmers {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "invalid_swimmer_count", fmt.Sprintf("Select between 1 and %d swimmers with a USA Swimming ID.", maxSyncSwimmers), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), vendorLongTimeout)
	defer cancel()
	checks, err := c.vc.VerifyMembers(ctx, c.clubID, ids)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	type row struct {
		usaswimming.MemberCheck
		Name string `json:"name"`
	}
	out, invalid := make([]row, 0, len(checks)), 0
	for _, ck := range checks {
		if !ck.Valid {
			invalid++
		}
		out = append(out, row{MemberCheck: ck, Name: nameByID[normalizeUSS(ck.MemberID)]})
	}
	c.audit(h, "verify_members", map[string]string{"checked": fmt.Sprint(len(checks)), "invalid": fmt.Sprint(invalid)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": out, "invalid": invalid})
}

type usaVerifyTimeBody struct {
	SwimmerID string `json:"swimmer_id"`
	Event     string `json:"event"` // e.g. "100 Free SCY"
	Time      string `json:"time"`  // e.g. "00:52.34"
}

var swimTimeRE = regexp.MustCompile(`^\d{1,2}:\d{2}\.\d{2}$`)

// verifyTime (Tier 3) confirms a claimed time against USA Swimming's records.
func (h *USASwimmingHandlers) verifyTime(w http.ResponseWriter, r *http.Request) {
	c, ok := h.vendor(w, r, false)
	if !ok {
		return
	}
	var body usaVerifyTimeBody
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}
	body.Event, body.Time = strings.TrimSpace(body.Event), strings.TrimSpace(body.Time)
	if body.Event == "" || !swimTimeRE.MatchString(body.Time) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_failed", "event is required and time must look like 00:52.34.", nil)
		return
	}
	var memberID string
	for _, sw := range h.members.SnapshotSwimmers(c.orgID) {
		if sw.ID == body.SwimmerID {
			memberID = normalizeUSS(sw.USASwimmingID)
			break
		}
	}
	if memberID == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_failed", "Swimmer not found or has no USA Swimming ID.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), vendorShortTimeout)
	defer cancel()
	valid, err := c.vc.VerifyTime(ctx, memberID, body.Event, body.Time)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"valid": valid})
}

// submitRegistration (Tier 2) validates locally, then submits one registration.
func (h *USASwimmingHandlers) submitRegistration(w http.ResponseWriter, r *http.Request) {
	c, ok := h.vendor(w, r, true)
	if !ok {
		return
	}
	var body usaswimming.Registration
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONDecodeError(w, r, err, "bad_request", "Invalid JSON body.")
		return
	}
	if problems := usaswimming.ValidateRegistration(body); len(problems) > 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "validation_failed", strings.Join(problems, "; "), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), vendorLongTimeout)
	defer cancel()
	res, err := c.vc.SubmitRegistration(ctx, c.clubID, body)
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	c.audit(h, "registration_submit", map[string]string{"request_id": res.RequestID, "status": res.Status, "ok": fmt.Sprint(res.RegistrationOK)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"result": res})
}

func (h *USASwimmingHandlers) registrationStatus(w http.ResponseWriter, r *http.Request) {
	c, ok := h.vendor(w, r, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), vendorShortTimeout)
	defer cancel()
	status, err := c.vc.RegistrationStatus(ctx, chi.URLParam(r, "requestID"))
	if err != nil {
		h.writeClientErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": status})
}
