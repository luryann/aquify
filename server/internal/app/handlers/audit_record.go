package handlers

import (
	"github.com/dareaquatics/aquify/server/internal/app/audit"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// recordAudit appends an audit event on behalf of the acting principal. It is
// a no-op when auditing is not wired (nil service) or there is no principal,
// and it never fails the request: audit write errors are intentionally dropped.
func recordAudit(
	svc *audit.Service,
	principal *appmiddleware.Principal,
	orgID, action, resourceType, resourceID string,
	before, after any,
	meta map[string]string,
) {
	if svc == nil || principal == nil {
		return
	}
	_, _ = svc.Append(audit.AppendRequest{
		OrgID:        orgID,
		ActorUserID:  principal.UserID,
		ActorRoles:   principal.Roles,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       before,
		After:        after,
		Metadata:     meta,
	})
}
