package app

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
	"github.com/dareaquatics/aquify/server/internal/storage"
	"github.com/go-chi/chi/v5"
)

// devStoragePath is where the stub object store presigns URLs in local
// development. Without it, presigned uploads/downloads point at a MinIO that
// usually isn't running, so document uploads, message attachments, and report
// downloads all fail locally. Only mounted when storage fell back to the stub
// in an explicit development env (non-dev envs require real storage).
const devStoragePath = "/api/v1/dev-storage"

// maxDevStorageObjectBytes mirrors the largest upload the API accepts.
const maxDevStorageObjectBytes = 25 << 20

func devStorageHandler(store *storage.StubStorage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := url.PathUnescape(chi.URLParam(r, "*"))
		if err != nil || strings.TrimSpace(key) == "" {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_key", "Invalid object key.", nil)
			return
		}
		q := r.URL.Query()
		expires, err := strconv.ParseInt(q.Get("X-Stub-Expires"), 10, 64)
		if err != nil || time.Now().Unix() > expires {
			httpx.WriteError(w, r, http.StatusForbidden, "url_expired", "This storage URL has expired.", nil)
			return
		}
		if q.Get("X-Stub-Method") != r.Method {
			httpx.WriteError(w, r, http.StatusForbidden, "method_not_signed", "This storage URL does not allow that method.", nil)
			return
		}

		switch r.Method {
		case http.MethodPut:
			data, err := io.ReadAll(io.LimitReader(r.Body, maxDevStorageObjectBytes+1))
			if err != nil || len(data) > maxDevStorageObjectBytes {
				httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "object_too_large", "Object is too large.", nil)
				return
			}
			if err := store.PutObject(r.Context(), key, r.Header.Get("Content-Type"), data); err != nil {
				httpx.WriteError(w, r, http.StatusInternalServerError, "put_failed", "Failed to store object.", nil)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			data, contentType, err := store.GetObject(r.Context(), key)
			if err != nil {
				httpx.WriteError(w, r, http.StatusNotFound, "object_not_found", "Object not found.", nil)
				return
			}
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
