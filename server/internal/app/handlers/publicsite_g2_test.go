package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/comms"
	"github.com/dareaquatics/aquify/server/internal/app/orgsettings"
	"github.com/go-chi/chi/v5"
)

type slowStaff struct {
	calls   atomic.Int32
	release chan struct{}
}

func (s *slowStaff) ListStaff(ctx context.Context, orgID string) ([]StaffMember, error) {
	s.calls.Add(1)
	<-s.release
	return []StaffMember{{ID: "c1", FirstName: "Cara"}}, nil
}

func TestCachedStaffListerCollapsesConcurrentMisses(t *testing.T) {
	inner := &slowStaff{release: make(chan struct{})}
	cache := NewCachedStaffLister(inner, time.Minute, nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.ListStaff(context.Background(), "org-1"); err != nil {
				t.Error(err)
			}
		}()
	}
	// Give every goroutine time to reach the cache before the lookup finishes.
	time.Sleep(100 * time.Millisecond)
	close(inner.release)
	wg.Wait()
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("inner calls = %d, want 1", got)
	}
}

func TestPublicSiteAnnouncementCapKeepsNewest(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var list []comms.Announcement
	// Source order is oldest first; 60 items so the cap (50) must drop the oldest 10.
	for i := 0; i < 60; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		list = append(list, comms.Announcement{ID: fmt.Sprintf("a%02d", i), OrgID: "org-1", Title: "t", Status: "published", Audience: "all", Public: true, PublishedAt: &ts})
	}
	h := NewPublicSiteHandlers(PublicSiteSources{
		Now: func() time.Time { return base.Add(100 * time.Hour) },
		Settings: func(slug string) (*orgsettings.OrgSettings, error) {
			return &orgsettings.OrgSettings{OrgID: "org-1", TeamName: "D", PublicSlug: slug, PublicSiteEnabled: true}, nil
		},
		Announcements: func(string) []comms.Announcement { return list },
	}, nil)
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public/orgs/dare/site", nil))
	var body struct {
		Announcements []struct {
			ID string `json:"id"`
		} `json:"announcements"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Announcements) != 50 || body.Announcements[0].ID != "a59" || body.Announcements[49].ID != "a10" {
		t.Fatalf("want newest 50 first (a59..a10), got %d first=%v", len(body.Announcements), body.Announcements[:1])
	}
}
