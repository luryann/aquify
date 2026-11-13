package handlers

import (
	"github.com/luryann/aquify/server/internal/testutil"
	"net/http"
	"strings"
	"testing"

	"github.com/luryann/aquify/server/internal/app/audit"
	"github.com/luryann/aquify/server/internal/app/comms"
)

// When the announcement cannot be persisted the draft endpoint must fail with
// an opaque 500, leave no audit trail for a draft that never existed, and not
// leak the storage error.
func TestCov10gDraftRecordAnnouncementPersistenceFailure(t *testing.T) {
	t.Parallel()
	f := cov6xNewPerf(t, false)
	aud := audit.NewService()
	h := NewPerformanceHandlersWithAudit(f.svc, aud)
	h.SetCommsService(comms.NewServiceWithPool(testutil.ClosedPool(t)))
	router := cov6xRouter(h.registerExtensionRoutes)

	rr := cov6xDo(router, http.MethodPost, "/performance/records/"+f.recordEntryID+"/announcement", "admin", "")
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "announcement_failed") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "persist announcement") {
		t.Fatalf("storage error leaked: %s", rr.Body.String())
	}
	if got := cov6xAuditActions(t, aud)["performance.team_record.announcement_drafted"]; len(got) != 0 {
		t.Fatalf("no audit event expected for a failed draft, got %+v", got)
	}
}
