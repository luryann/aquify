// Package usaswimming is the seam between Aquify and USA Swimming's SWIMS /
// Data Hub. It holds org-level integration settings (API key encrypted at
// rest and never returned), the Client interface, an HTTP implementation and
// an in-memory stub.
//
// WHAT IS REAL vs STUBBED
//
//   - Real: settings storage (encrypted key, redacted reads), the Client
//     interface, the HTTP transport (timeouts, size cap, no secrets in logs or
//     errors), and the sync pipeline that imports official times without
//     double-recording (see handlers/usaswimming.go).
//   - ASSUMED: the remote wire contract in http.go (paths, auth header, JSON
//     shape). USA Swimming's real SWIMS/Data Hub API spec and credentials were
//     not available, so the endpoints are configurable defaults to be adjusted
//     (ClientConfig.PingPath / TimesPath) once the real contract is known.
//   - Stub: StubClient serves canned data in development/test only; production
//     never uses it.
package usaswimming

import (
	"context"
	"errors"
	"time"
)

// Errors returned by Client implementations.
var (
	ErrNotConfigured   = errors.New("usa swimming integration is not configured")
	ErrSwimmerNotFound = errors.New("swimmer not found at USA Swimming")
	ErrUnauthorized    = errors.New("usa swimming rejected the credentials")
)

// OfficialTime is one official swim reported by USA Swimming.
type OfficialTime struct {
	Distance int       `json:"distance"`
	Stroke   string    `json:"stroke"` // free|back|breast|fly|im
	Course   string    `json:"course"` // SCY|SCM|LCM
	TimeMs   int       `json:"time_ms"`
	SwimDate time.Time `json:"swim_date"`
	MeetName string    `json:"meet_name,omitempty"`
}

// Client is what the rest of the app needs from USA Swimming.
type Client interface {
	// TestConnection verifies base URL and credentials.
	TestConnection(ctx context.Context) error
	// OfficialTimes returns a swimmer's official times by USA Swimming ID.
	// It returns ErrSwimmerNotFound for an unknown ID.
	OfficialTimes(ctx context.Context, usaSwimmingID string) ([]OfficialTime, error)
}
