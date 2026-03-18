package handlers

import (
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	appmiddleware "github.com/dareaquatics/aquify/server/internal/app/middleware"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func AuthzPing(w http.ResponseWriter, r *http.Request) {
	principal, ok := appmiddleware.PrincipalFromContext(r.Context())
	if !ok || principal == nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication context is missing.", nil)
		return
	}

	orgID, _ := appmiddleware.OrgIDFromContext(r.Context())

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":            "ok",
		"user_id":           principal.UserID,
		"roles":             principal.Roles,
		"org_id":            orgID,
		"principal_org_id":  principal.OrgID,
		"mfa_authenticated": principal.MFAAuthenticated,
		"message":           "Auth/RBAC/org-scope middleware is active.",
		"requestId":         chimiddleware.GetReqID(r.Context()),
	})
}
