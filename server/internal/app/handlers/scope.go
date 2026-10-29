package handlers

import (
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
)

// requirePrincipal returns the authenticated principal or writes a 401.
func requirePrincipal(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, bool) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return nil, false
	}
	return principal, true
}

func scopedRequestFromContext(w http.ResponseWriter, r *http.Request) (*appmiddleware.Principal, string, bool) {
	principal, ok := requirePrincipal(w, r)
	if !ok {
		return nil, "", false
	}

	orgID, ok := appmiddleware.OrgIDFromContext(r.Context())
	if !ok || strings.TrimSpace(orgID) == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "missing_org_scope", "Missing X-Org-Id header.", nil)
		return nil, "", false
	}

	return principal, orgID, true
}

// hasAnyRole reports whether the principal holds any of the roles (case-insensitive).
func hasAnyRole(principal *appmiddleware.Principal, roles ...string) bool {
	if principal == nil {
		return false
	}
	for _, pr := range principal.Roles {
		for _, allowed := range roles {
			if strings.EqualFold(pr, allowed) {
				return true
			}
		}
	}
	return false
}

func validateBodyOrgScope(w http.ResponseWriter, r *http.Request, headerOrgID, bodyOrgID string) bool {
	if strings.TrimSpace(bodyOrgID) != strings.TrimSpace(headerOrgID) {
		httpx.WriteError(w, r, http.StatusForbidden, "scope_violation", "Body org_id must match X-Org-Id scope.", map[string]any{
			"header_org_id": headerOrgID,
			"body_org_id":   bodyOrgID,
		})
		return false
	}
	return true
}
