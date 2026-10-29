package handlers

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/events"
	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/app/meets"
	"github.com/dareaquatics/aquify/server/internal/app/orgsettings"
	"github.com/dareaquatics/aquify/server/internal/app/performance"
	"github.com/go-chi/chi/v5"
)

// PublicSiteStaff is a coach/admin shown on the public team site: name and
// role label only.
type PublicSiteStaff struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// PublicSiteSources supplies the data behind the public team website. Every
// source is optional; a nil source yields an empty section.
type PublicSiteSources struct {
	Settings      func(slug string) (*orgsettings.OrgSettings, error)
	Announcements func(orgID string) []comms.Announcement
	Meets         func(orgID string) []meets.Meet
	Events        func(orgID string, from time.Time) []*events.Event
	Records       func(orgID string) []performance.TeamRecord
	Staff         func(ctx context.Context, orgID string) []PublicSiteStaff
	Now           func() time.Time
}

// PublicSiteHandlers serves the unauthenticated, read-only team website feed.
type PublicSiteHandlers struct {
	src     PublicSiteSources
	limiter func(http.Handler) http.Handler
}

// NewPublicSiteHandlers builds the handlers. limiter is applied to the route.
func NewPublicSiteHandlers(src PublicSiteSources, limiter func(http.Handler) http.Handler) *PublicSiteHandlers {
	if src.Now == nil {
		src.Now = time.Now
	}
	return &PublicSiteHandlers{src: src, limiter: limiter}
}

// RegisterPublicRoutes mounts the unauthenticated site route (no auth, no org header).
func (h *PublicSiteHandlers) RegisterPublicRoutes(router chi.Router) {
	router.Group(func(r chi.Router) {
		if h.limiter != nil {
			r.Use(h.limiter)
		}
		r.Get("/public/orgs/{slug}/site", h.site)
	})
}

type publicClub struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	About        string `json:"about,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
	ContactPhone string `json:"contact_phone,omitempty"`
	JoinURL      string `json:"join_url"`
}

type publicAnnouncement struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

type publicMeet struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	Location  string `json:"location,omitempty"`
	HostOrg   string `json:"host_org,omitempty"`
	Course    string `json:"course"`
}

type publicEvent struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Category    string     `json:"category"`
	Description string     `json:"description,omitempty"`
	Location    string     `json:"location,omitempty"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	AllDay      bool       `json:"all_day"`
}

type publicRecord struct {
	Label       string `json:"label"`
	Course      string `json:"course"`
	Gender      string `json:"gender"`
	AgeGroup    string `json:"age_group"`
	Distance    int    `json:"distance"`
	Stroke      string `json:"stroke"`
	TimeMs      int    `json:"time_ms"`
	SwimmerName string `json:"swimmer_name"`
	SwimDate    string `json:"swim_date"`
	MeetName    string `json:"meet_name,omitempty"`
}

const (
	publicMaxAnnouncements = 50
	publicMaxMeets         = 30
	publicMaxEvents        = 50
)

func (h *PublicSiteHandlers) site(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "slug")))
	var settings *orgsettings.OrgSettings
	if h.src.Settings != nil && slug != "" {
		var err error
		settings, err = h.src.Settings(slug)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Failed to load site.", err)
			return
		}
	}
	if settings == nil || !settings.PublicSiteEnabled || settings.PublicSlug != slug {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Team site not found.", nil)
		return
	}
	orgID := settings.OrgID
	now := h.src.Now().UTC()
	today := now.Format("2006-01-02")

	name := strings.TrimSpace(settings.TeamName)
	if name == "" {
		name = slug
	}
	resp := map[string]any{
		"club": publicClub{
			Name: name, Slug: slug, About: settings.PublicAbout,
			ContactEmail: settings.PublicContactEmail, ContactPhone: settings.PublicContactPhone,
			JoinURL: "/register",
		},
		"announcements": []publicAnnouncement{},
		"meets":         []publicMeet{},
		"events":        []publicEvent{},
		"records":       []publicRecord{},
		"coaches":       []PublicSiteStaff{},
	}

	if h.src.Announcements != nil {
		out := []publicAnnouncement{}
		for _, a := range h.src.Announcements(orgID) {
			if !a.IsPubliclyVisible() || len(out) >= publicMaxAnnouncements {
				continue
			}
			out = append(out, publicAnnouncement{ID: a.ID, Title: a.Title, Body: a.Body, PublishedAt: a.PublishedAt})
		}
		resp["announcements"] = out
	}

	if h.src.Meets != nil {
		list := []meets.Meet{}
		for _, m := range h.src.Meets(orgID) {
			if !m.IsPubliclyListed(orgID, today) {
				continue
			}
			list = append(list, m)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].StartDate < list[j].StartDate })
		out := []publicMeet{}
		for _, m := range list {
			if len(out) >= publicMaxMeets {
				break
			}
			out = append(out, publicMeet{ID: m.ID, Name: m.Name, StartDate: m.StartDate, EndDate: m.EndDate, Location: m.Location, HostOrg: m.HostOrg, Course: m.Course})
		}
		resp["meets"] = out
	}

	if h.src.Events != nil {
		out := []publicEvent{}
		for _, e := range h.src.Events(orgID, now) {
			if !e.IsPubliclyVisible(orgID) || len(out) >= publicMaxEvents {
				continue
			}
			out = append(out, publicEvent{ID: e.ID, Title: e.Title, Category: e.Category, Description: e.Description,
				Location: e.Location, StartsAt: e.StartsAt, EndsAt: e.EndsAt, AllDay: e.AllDay})
		}
		resp["events"] = out
	}

	if h.src.Records != nil {
		out := []publicRecord{}
		for _, tr := range h.src.Records(orgID) {
			if tr.Record == nil {
				continue
			}
			rec := tr.Record
			out = append(out, publicRecord{Label: tr.Label, Course: tr.Course, Gender: tr.Gender, AgeGroup: tr.AgeGroup,
				Distance: tr.Distance, Stroke: tr.Stroke, TimeMs: rec.TimeMs, SwimmerName: rec.SwimmerName,
				SwimDate: rec.SwimDate, MeetName: rec.MeetName})
		}
		resp["records"] = out
	}

	if settings.PublicShowCoaches && h.src.Staff != nil {
		resp["coaches"] = h.src.Staff(r.Context(), orgID)
	}

	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// PublicRecordsPrincipal is the least-privileged reader used to fetch records
// for the public site; family-level redaction applies.
func PublicRecordsPrincipal(orgID string) *appmiddleware.Principal {
	return &appmiddleware.Principal{UserID: "public-site", OrgID: orgID, Roles: []string{"parent"}}
}
