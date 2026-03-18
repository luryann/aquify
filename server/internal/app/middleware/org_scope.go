package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

type orgContextKey struct{}

func OrgScopeStub(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := strings.TrimSpace(r.Header.Get("X-Org-Id"))
		if orgID == "" {
			httpx.WriteError(w, r, http.StatusBadRequest, "missing_org_scope", "Missing X-Org-Id header.", nil)
			return
		}

		ctx := context.WithValue(r.Context(), orgContextKey{}, orgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func OrgIDFromContext(ctx context.Context) (string, bool) {
	orgID, ok := ctx.Value(orgContextKey{}).(string)
	return orgID, ok
}
