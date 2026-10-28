package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, payload any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	if cause, ok := details.(error); ok {
		// Handlers pass the underlying error for 5xx responses: log it for
		// operators, but never serialise internal error text to clients.
		details = nil
		if status >= http.StatusInternalServerError {
			slog.ErrorContext(r.Context(), "request failed",
				"status", status,
				"code", code,
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", chimiddleware.GetReqID(r.Context()),
				"error", cause.Error(),
			)
		}
	}
	_ = WriteJSON(w, status, ErrorEnvelope{
		Error: APIError{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: chimiddleware.GetReqID(r.Context()),
		},
	})
}
