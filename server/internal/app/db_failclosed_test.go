package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Outside development a database failure after the startup preflight must end
// the process, never return a nil pool (which builds in-memory services that lose
// every write on restart). Development keeps the convenient in-memory fallback.
func TestOpenValidatedDBPoolFailsClosedOutsideDevelopment(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	oldOpen, oldExit := openDBPool, exitOnStartupFailure
	t.Cleanup(func() { openDBPool, exitOnStartupFailure = oldOpen, oldExit })

	var exited []string
	exitOnStartupFailure = func(_ *slog.Logger, msg string, _ error) { exited = append(exited, msg) }

	cases := []struct {
		name     string
		env      string
		url      string
		open     func(context.Context, string) (*pgxpool.Pool, error)
		wantExit bool
	}{
		{"production, connection fails", "production", "postgres://x", func(context.Context, string) (*pgxpool.Pool, error) { return nil, errors.New("boom") }, true},
		{"staging, connection fails", "staging", "postgres://x", func(context.Context, string) (*pgxpool.Pool, error) { return nil, errors.New("boom") }, true},
		{"production, nil pool", "production", "postgres://x", func(context.Context, string) (*pgxpool.Pool, error) { return nil, nil }, true},
		// Empty URL: rejected earlier by ValidateStartupDependencies, not here.
		{"production, no DATABASE_URL", "production", "", nil, false},
		{"development, connection fails", "development", "postgres://x", func(context.Context, string) (*pgxpool.Pool, error) { return nil, errors.New("boom") }, false},
		{"test, no DATABASE_URL", "test", "", nil, false},
		{"hand-built config with empty APP_ENV behaves as test", "", "", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exited = nil
			if c.open != nil {
				openDBPool = c.open
			}
			pool := openValidatedDBPool(context.Background(), Config{AppEnv: c.env, DatabaseURL: c.url}, logger)
			if pool != nil {
				t.Fatalf("expected a nil pool, got %v", pool)
			}
			if got := len(exited) > 0; got != c.wantExit {
				t.Fatalf("exit called = %v (%v), want %v", got, exited, c.wantExit)
			}
		})
	}
}
