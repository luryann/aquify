package handlers

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/luryann/aquify/meetfiles"
	"github.com/luryann/aquify/meetfiles/entries"
	"github.com/luryann/aquify/server/internal/app/httpx"
	"github.com/luryann/aquify/server/internal/app/meets"
	"github.com/luryann/aquify/server/internal/app/members"
	appmiddleware "github.com/luryann/aquify/server/internal/app/middleware"
	"github.com/luryann/aquify/server/internal/app/orgsettings"
	"github.com/luryann/aquify/server/internal/app/performance"
)

// BestTimeSourceLister reports which meet each swimmer's best times came
// from; *performance.Service implements it.
type BestTimeSourceLister interface {
	ListBestTimeSources(orgID string, principal *appmiddleware.Principal) ([]performance.BestTimeSource, error)
}

// WithBestTimeSources wires the seed-time source lookup used by the entry
// bundle's reports. Without it every seed time is reported as manually added.
func (h *ExportsHandlers) WithBestTimeSources(lister BestTimeSourceLister) *ExportsHandlers {
	h.bestTimes = lister
	return h
}

// entryBundleMeetStatuses: entries are exported once finalized (and may be
// downloaded again after the meet completes).
var entryBundleMeetStatuses = map[string]bool{"entry_finalized": true, "completed": true}

// exportEntryBundle handles POST /meets/{meetID}/entries/export. It returns a
// ZIP holding everything the meet host needs: <base>.sd3, <base>.hy3,
// <base>-EntryReport-ByEvent.pdf, <base>-EntryReport-BySwimmer.pdf and
// <base>-EntryLog.txt, where <base> is
// "<team code>-Entries-<meet name>-<DDMonYYYY>-<HHMM>" in the club's time zone.
func (h *ExportsHandlers) exportEntryBundle(w http.ResponseWriter, r *http.Request) {
	principal, orgID, ok := scopedRequestFromContext(w, r)
	if !ok {
		return
	}
	if !hasAnyRole(principal, "admin", "coach") {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "Only admin or coach can export entry files.", nil)
		return
	}
	if h.meetsService == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", "Meet export is not configured.", nil)
		return
	}
	meetID := strings.TrimSpace(chi.URLParam(r, "meetID"))

	meet, err := h.meetsService.GetMeet(principal, orgID, meetID)
	if err != nil {
		writeEntryBundleMeetError(w, r, err)
		return
	}
	if !entryBundleMeetStatuses[meet.Status] {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entries_not_finalized", "Finalize entries before exporting the entry files.", nil)
		return
	}
	events, err := h.meetsService.ListMeetEvents(principal, orgID, meetID)
	if err != nil {
		writeEntryBundleMeetError(w, r, err)
		return
	}
	meetEntries, err := h.meetsService.ListEntries(principal, orgID, meetID)
	if err != nil {
		writeEntryBundleMeetError(w, r, err)
		return
	}
	if err := h.refreshMembers(orgID); err != nil {
		writeEntryBundleMeetError(w, r, err)
		return
	}

	var settings *orgsettings.OrgSettings
	if h.orgSettings != nil {
		if settings, err = h.orgSettings.Get(orgID); err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "entry_export_failed", "Failed to load club settings.", err)
			return
		}
	}
	// The host's meet software attributes entries by the team record, so a
	// bundle without a team code cannot be imported.
	if settings == nil || strings.TrimSpace(settings.TeamCode) == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "team_code_required", "Set your club's team code in Settings before exporting entry files.", nil)
		return
	}
	start := parseEntryBundleDate(meet.StartDate)
	if start.IsZero() {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "meet_dates_required", "Set the meet's start date before exporting entry files.", nil)
		return
	}

	input := entries.Input{
		Club: entries.Club{
			Name:       strings.TrimSpace(settings.TeamName),
			Code:       strings.TrimSpace(settings.TeamCode),
			LSC:        strings.TrimSpace(settings.LSCCode),
			Address1:   settings.AddressLine1,
			Address2:   settings.AddressLine2,
			City:       settings.AddressCity,
			State:      settings.AddressState,
			PostalCode: settings.AddressPostalCode,
		},
		Meet: entries.Meet{
			Name:            meet.Name,
			Facility:        meet.Location,
			Start:           start,
			End:             parseEntryBundleDate(meet.EndDate),
			AgeUpDate:       parseEntryBundleDate(meet.AgeUpDate),
			Course:          meetfiles.Course(strings.ToUpper(strings.TrimSpace(meet.Course))),
			AthleteFeeCents: meet.SwimmerFeeCents,
		},
		GeneratedAt: time.Now().In(clubLocation(settings.Timezone)),
		GeneratedBy: entryBundleAuthor(principal),
	}

	seedSources := h.entrySeedSources(principal, orgID)
	exportedIDs, skippedRelays, skippedMissing, problems := appendBundleEntries(&input, meetEntries, eventsByID(events), h.swimmersByID(orgID), seedSources)
	if len(problems) > 0 {
		const shown = 5
		message := "Some entries cannot be written to an entry file: " + strings.Join(problems[:min(len(problems), shown)], "; ")
		if len(problems) > shown {
			message += fmt.Sprintf("; and %d more", len(problems)-shown)
		}
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entries_not_exportable", message+".", nil)
		return
	}
	if len(input.Entries) == 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "no_exportable_entries", "There are no approved individual entries to export.", nil)
		return
	}

	bundle, err := entries.Build(input)
	if err != nil {
		if errors.Is(err, entries.ErrInvalidInput) {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "entry_export_failed", err.Error(), nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "entry_export_failed", "Failed to build the entry files.", err)
		return
	}
	archive, err := bundle.Zip()
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "entry_export_failed", "Failed to package the entry files.", err)
		return
	}

	marked, _, err := h.meetsService.MarkEntriesExported(principal, orgID, meetID, exportedIDs)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "entry_export_state_failed", "Failed to record exported entries.", err)
		return
	}
	h.emitAudit(principal, "meet.entry_bundle_exported", "meet", meetID, map[string]string{
		"bundle":         bundle.BaseName,
		"athlete_count":  fmt.Sprintf("%d", bundle.Athletes),
		"entry_count":    fmt.Sprintf("%d", bundle.IndividualEntries),
		"newly_exported": fmt.Sprintf("%d", marked),
		"skipped_relays": fmt.Sprintf("%d", skippedRelays),
		"skipped_orphan": fmt.Sprintf("%d", skippedMissing),
	})

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", attachmentDisposition(bundle.BaseName+".zip"))
	w.Header().Set("X-Entry-Count", fmt.Sprintf("%d", bundle.IndividualEntries))
	w.Header().Set("X-Athlete-Count", fmt.Sprintf("%d", bundle.Athletes))
	w.Header().Set("X-Skipped-Relays", fmt.Sprintf("%d", skippedRelays))
	w.Header().Set("X-Skipped-Orphans", fmt.Sprintf("%d", skippedMissing))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive)
}

// seedSourceKey identifies one swimmer's event for the seed-source lookup.
type seedSourceKey struct {
	swimmerID string
	distance  int
	stroke    meetfiles.Stroke
	course    string
}

type seedSource struct {
	timeMs int
	meet   string
	date   time.Time
}

// entrySeedSources maps each swimmer's best time to the meet it was swum at.
// It is best-effort: any failure just means seed times are reported as
// manually added.
func (h *ExportsHandlers) entrySeedSources(principal *appmiddleware.Principal, orgID string) map[seedSourceKey]seedSource {
	out := map[seedSourceKey]seedSource{}
	if h.bestTimes == nil || h.meetsService == nil {
		return out
	}
	sources, err := h.bestTimes.ListBestTimeSources(orgID, principal)
	if err != nil {
		return out
	}
	meetsByID := map[string]meets.Meet{}
	for _, m := range h.meetsService.SnapshotMeets(orgID) {
		meetsByID[m.ID] = m
	}
	for _, src := range sources {
		m, ok := meetsByID[src.MeetID]
		if !ok {
			continue
		}
		key := seedSourceKey{src.SwimmerID, src.Distance, bundleStroke(src.Stroke), strings.ToUpper(strings.TrimSpace(src.Course))}
		out[key] = seedSource{timeMs: src.BestTimeMs, meet: m.Name, date: parseEntryBundleDate(m.StartDate)}
	}
	return out
}

// appendBundleEntries adds every approved/exported individual entry to input
// in a stable order (oldest first), returning their IDs, the number of relay
// entries skipped, the number skipped because their event or swimmer no longer
// exists (left unexported and reported to the coach), and a description of each
// entry that cannot be written.
func appendBundleEntries(input *entries.Input, meetEntries []meets.Entry, eventByID map[string]meets.MeetEvent, swimmerByID map[string]members.Swimmer, seedSources map[seedSourceKey]seedSource) (exportedIDs []string, skippedRelays, skippedMissing int, problems []string) {
	ordered := append([]meets.Entry(nil), meetEntries...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	reported := map[string]bool{}
	problem := func(key, message string) {
		if !reported[key] {
			reported[key] = true
			problems = append(problems, message)
		}
	}
	for _, e := range ordered {
		if !entryIsExportable(e) {
			continue
		}
		ev, ok := eventByID[e.MeetEventID]
		if !ok {
			skippedMissing++
			continue
		}
		if e.IsRelay || ev.IsRelay {
			skippedRelays++
			continue
		}
		sw, ok := swimmerByID[e.SwimmerID]
		if !ok {
			skippedMissing++
			continue
		}
		name := strings.TrimSpace(sw.FirstName + " " + sw.LastName)
		gender := bundleSwimmerGender(sw.Gender)
		if gender == "" {
			problem("g"+sw.ID, name+" needs a competition category of male or female")
			continue
		}
		dob := parseEntryBundleDate(sw.DateOfBirth)
		if dob.IsZero() {
			problem("d"+sw.ID, name+" has no date of birth")
			continue
		}
		stroke := bundleStroke(ev.Stroke)
		if stroke == meetfiles.StrokeUnknown || ev.Distance <= 0 {
			problem("e"+ev.ID, fmt.Sprintf("event %s has no distance or stroke", ev.EventNumber))
			continue
		}
		course := meetfiles.Course(strings.ToUpper(strings.TrimSpace(ev.Course)))
		if course == meetfiles.CourseUnknown {
			course = input.Meet.Course
		}
		entry := entries.Entry{
			Swimmer: entries.Swimmer{
				ID:            sw.ID,
				LastName:      sw.LastName,
				FirstName:     sw.FirstName,
				MiddleInitial: sw.MiddleInitial,
				PreferredName: sw.PreferredName,
				USSID:         sw.USASwimmingID,
				DOB:           dob,
				Gender:        gender,
			},
			Event: entries.Event{
				Number:   ev.EventNumber,
				Distance: ev.Distance,
				Stroke:   stroke,
				Gender:   bundleEventGender(ev.Gender),
				MinAge:   ev.MinAge,
				MaxAge:   ev.MaxAge,
				FeeCents: ev.EntryFeeCents,
			},
			SeedCourse: course,
		}
		if e.SeedTimeMs != nil && *e.SeedTimeMs > 0 {
			entry.SeedTimeMs = *e.SeedTimeMs
			if src, ok := seedSources[seedSourceKey{sw.ID, ev.Distance, stroke, string(course)}]; ok && src.timeMs == entry.SeedTimeMs {
				entry.SeedMeet, entry.SeedDate = src.meet, src.date
			}
		}
		input.Entries = append(input.Entries, entry)
		exportedIDs = append(exportedIDs, e.ID)
	}
	return exportedIDs, skippedRelays, skippedMissing, problems
}

func writeEntryBundleMeetError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, meets.ErrMeetNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "meet_not_found", "Meet was not found.", nil)
	case errors.Is(err, meets.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "You are not allowed to export this meet.", nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "entry_export_failed", "Failed to load meet data for export.", err)
	}
}

// parseEntryBundleDate reads a YYYY-MM-DD (or longer ISO) date; zero if blank
// or malformed.
func parseEntryBundleDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 10 {
		raw = raw[:10]
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// attachmentDisposition builds an RFC 6266 attachment header. Names with
// separators, quotes or non-ASCII characters come out quoted or as the
// filename*=UTF-8” form, so neither a ';' in a meet name nor an accent
// truncates or mangles the saved file.
func attachmentDisposition(name string) string {
	if v := mime.FormatMediaType("attachment", map[string]string{"filename": name}); v != "" {
		return v
	}
	return "attachment"
}

// clubLocation resolves the club's IANA time zone, defaulting to UTC.
func clubLocation(zone string) *time.Location {
	if zone = strings.TrimSpace(zone); zone != "" {
		if loc, err := time.LoadLocation(zone); err == nil {
			return loc
		}
	}
	return time.UTC
}

func entryBundleAuthor(principal *appmiddleware.Principal) string {
	if name := strings.TrimSpace(principal.Name); name != "" {
		return name
	}
	return strings.TrimSpace(principal.Email)
}

func bundleStroke(stroke string) meetfiles.Stroke {
	switch strings.ToLower(strings.TrimSpace(stroke)) {
	case "freestyle", "free":
		return meetfiles.StrokeFreestyle
	case "backstroke", "back":
		return meetfiles.StrokeBackstroke
	case "breaststroke", "breast":
		return meetfiles.StrokeBreaststroke
	case "butterfly", "fly":
		return meetfiles.StrokeButterfly
	case "individual_medley", "im":
		return meetfiles.StrokeIM
	}
	return meetfiles.StrokeUnknown
}

func bundleSwimmerGender(gender string) string {
	switch strings.ToLower(strings.TrimSpace(gender)) {
	case "female", "f":
		return "F"
	case "male", "m":
		return "M"
	}
	return ""
}

// bundleEventGender maps the meet vocabulary (M|F|X|O) to the entry files';
// open events are written as mixed.
func bundleEventGender(gender string) string {
	switch g := strings.ToUpper(strings.TrimSpace(gender)); g {
	case "M", "F":
		return g
	}
	return "X"
}
