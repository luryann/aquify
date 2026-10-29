package handlers

import (
	"log/slog"

	"github.com/dareaquatics/aquify/server/internal/app/audit"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// recordAudit appends an audit event on behalf of the acting principal. It is
// a no-op when auditing is not wired (nil service) or there is no principal.
// It is the single place every handler appends audit events. The privileged
// action has already been committed when this runs, so a failed append does not
// fail the request, but it is never silent: the failure is logged at error level
// with the action and resource (so it can be alerted on and reconciled) and also
// returned for callers that want to react.
func recordAudit(
	svc *audit.Service,
	principal *appmiddleware.Principal,
	orgID, action, resourceType, resourceID string,
	before, after any,
	meta map[string]string,
) error {
	if svc == nil || principal == nil {
		return nil
	}
	return appendAudit(svc, audit.AppendRequest{
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

// appendAudit appends a fully-formed audit request (for callers whose actor is
// not an authenticated principal, e.g. self-registration) and logs any failure.
func appendAudit(svc *audit.Service, req audit.AppendRequest) error {
	if svc == nil {
		return nil
	}
	if _, err := svc.Append(req); err != nil {
		slog.Error("audit append failed; privileged action is unaudited",
			"action", req.Action, "resource_type", req.ResourceType, "resource_id", req.ResourceID,
			"org_id", req.OrgID, "actor_user_id", req.ActorUserID, "error", err)
		return err
	}
	return nil
}
