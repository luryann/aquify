package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/luryann/aquify/server/internal/app/httpx"
)

// isInfrastructureError reports whether err stems from the database or
// network layer rather than from invalid caller input.
func isInfrastructureError(err error) bool {
	var pgErr *pgconn.PgError
	var connErr *pgconn.ConnectError
	var netErr net.Error
	return errors.As(err, &pgErr) || errors.As(err, &connErr) || errors.As(err, &netErr) ||
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// writeServiceBadRequest answers a service validation failure with 400 and the
// validation message. Unique violations become a fixed 409 and any other
// infrastructure failure becomes a logged 500 whose cause is never sent to the client.
func writeServiceBadRequest(w http.ResponseWriter, r *http.Request, code string, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		httpx.WriteError(w, r, http.StatusConflict, "conflict", "A record with those details already exists.", nil)
		return
	}
	if isInfrastructureError(err) {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Internal error.", err)
		return
	}
	httpx.WriteError(w, r, http.StatusBadRequest, code, err.Error(), nil)
}
